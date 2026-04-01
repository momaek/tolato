package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/policy"
	"github.com/momaek/tolato/internal/server/domain"
)

var ErrEmptyModelOutput = errors.New("empty model output")

type Service interface {
	HandleUserMessage(ctx context.Context, sessionID string, text string, clientMessageID string) error
}

type Repositories struct {
	Sessions    domain.SessionRepository
	Messages    domain.ThreadMessageRepository
	Timelines   domain.TimelineRepository
	ToolCalls   domain.ToolCallRepository
	ToolResults domain.ToolResultRepository
	Tasks       domain.TaskRepository
	Executions  domain.ExecutionRepository
	Audits      domain.AuditRepository
}

type LLMClient interface {
	RunTurn(ctx context.Context, input ModelTurnInput, tools []agentapi.ToolSpec) (ModelTurnOutput, error)
}

type ToolRegistry interface {
	Definitions() []agentapi.ToolSpec
	Call(ctx context.Context, call agentapi.Item) (policy.ToolResult, error)
}

type Runtime struct {
	repos  Repositories
	llm    LLMClient
	tools  ToolRegistry
	clock  domain.Clock
	ids    domain.IDGenerator
	locks  domain.LockManager
	events EventPublisher
	logger domain.Logger
}

type EventPublisher interface {
	SessionStateUpdated(ctx context.Context, session domain.Session) error
	TimelineRowAppended(ctx context.Context, session domain.Session, row domain.TimelineRow) error
	LLMSSEEvent(ctx context.Context, sessionID string, responseID string, sequenceNumber int, upstreamEventType string, rawEvent json.RawMessage) error
	LLMResponseCompleted(ctx context.Context, sessionID string, responseID string, rawResponse json.RawMessage) error
}

type Option func(*Runtime)

func WithEventPublisher(events EventPublisher) Option {
	return func(r *Runtime) { r.events = events }
}

func WithLockManager(locks domain.LockManager) Option {
	return func(r *Runtime) { r.locks = locks }
}

func WithLogger(logger domain.Logger) Option {
	return func(r *Runtime) { r.logger = logger }
}

func NewService(repos Repositories, llm LLMClient, tools ToolRegistry, clock domain.Clock, ids domain.IDGenerator, options ...Option) Service {
	runtime := &Runtime{
		repos: repos,
		llm:   llm,
		tools: tools,
		clock: clock,
		ids:   ids,
	}
	for _, option := range options {
		if option != nil {
			option(runtime)
		}
	}
	return runtime
}

type ModelTurnInput struct {
	SessionID     string
	Conversation  []agentapi.Item
	ProviderState json.RawMessage
}

type ModelTurnOutput struct {
	ResponseID    string
	Items         []agentapi.Item
	Done          bool
	ProviderState []byte
	Streamed      bool
}

func (r *Runtime) HandleUserMessage(ctx context.Context, sessionID string, text string, clientMessageID string) error {
	if clientMessageID == "" {
		return domain.ErrInvalidArgument
	}
	return r.withSessionLock(ctx, sessionID, func(ctx context.Context) error {
		if err := r.validateReady(); err != nil {
			return err
		}

		duplicated, err := r.hasProcessedClientMessage(ctx, sessionID, clientMessageID)
		if err != nil {
			return err
		}
		if duplicated {
			r.logInfo(ctx, "runtime duplicate client message ignored",
				"session_id", sessionID,
				"client_message_id", clientMessageID,
			)
			return nil
		}

		session, err := r.repos.Sessions.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		if session.Status == domain.SessionStatusRunning {
			return domain.ErrSessionBusy
		}

		now := r.clock.Now()
		userMessage := domain.ThreadMessage{
			ID:              r.ids.NewID("msg"),
			SessionID:       sessionID,
			ClientMessageID: strPtr(clientMessageID),
			Role:            domain.MessageRoleUser,
			Kind:            domain.ThreadMessageKindUserMessage,
			Content:         text,
			CreatedAt:       now,
		}
		if err := r.repos.Messages.Append(ctx, userMessage); err != nil {
			return err
		}
		userRow := domain.TimelineRow{
			ID:        r.ids.NewID("row"),
			SessionID: sessionID,
			Kind:      domain.TimelineRowKindUserMessage,
			CreatedAt: now,
			Text:      text,
		}
		if err := r.repos.Timelines.Append(ctx, userRow); err != nil {
			return err
		}

		session.Status = domain.SessionStatusRunning
		if err := r.bumpSession(ctx, &session); err != nil {
			return err
		}
		if err := r.publishTimelineRow(ctx, session, userRow); err != nil {
			return err
		}
		if err := r.publishSessionState(ctx, session); err != nil {
			return err
		}
		r.logInfo(ctx, "runtime accepted user message",
			"session_id", sessionID,
			"client_message_id", clientMessageID,
			"text_preview", previewText(text, 240),
		)

		conversation, providerState, err := r.loadConversationState(ctx, session)
		if err != nil {
			return err
		}
		conversation = append(conversation, agentapi.UserMessage(text))
		if err := r.persistConversationState(ctx, &session, conversation, providerState); err != nil {
			return err
		}

		// Inject session ID into context so tools can access it.
		toolCtx := policy.ContextWithSessionID(ctx, sessionID)
		return r.continueLoop(toolCtx, &session, conversation, providerState)
	})
}

// continueLoop is the standard agentic loop. It never interrupts.
// Tool calls are executed (may block for async execution), results are fed
// back to the LLM, and the loop continues until the LLM responds with text.
func (r *Runtime) continueLoop(ctx context.Context, session *domain.Session, conversation []agentapi.Item, providerState json.RawMessage) error {
	const maxToolRounds = 15
	for round := 0; ; round++ {
		if round >= maxToolRounds {
			r.logError(ctx, "runtime max tool rounds reached, stopping loop",
				"session_id", session.ID, "rounds", round)
			session.Status = domain.SessionStatusFailed
			_ = r.bumpSession(ctx, session)
			_ = r.publishSessionState(ctx, *session)
			return fmt.Errorf("max tool rounds (%d) exceeded", maxToolRounds)
		}
		r.logInfo(ctx, "runtime llm turn started",
			"session_id", session.ID,
			"conversation_items", len(conversation),
		)
		output, err := r.llm.RunTurn(ctx, ModelTurnInput{
			SessionID:     session.ID,
			Conversation:  conversation,
			ProviderState: providerState,
		}, r.tools.Definitions())
		if err != nil {
			r.logError(ctx, "runtime llm turn failed",
				"session_id", session.ID,
				"error", err,
			)
			session.Status = domain.SessionStatusFailed
			_ = r.bumpSession(ctx, session)
			_ = r.publishSessionState(ctx, *session)
			return err
		}

		providerState = cloneRaw(output.ProviderState)
		conversation = append(conversation, agentapi.CloneItems(output.Items)...)
		if err := r.persistConversationState(ctx, session, conversation, providerState); err != nil {
			return err
		}

		// ── Tool call path ──
		if call, ok := firstFunctionCall(output.Items); ok {
			r.logInfo(ctx, "runtime tool call",
				"session_id", session.ID,
				"tool_name", call.Name,
				"tool_args_preview", previewText(call.Arguments, 320),
			)
			callRecord, callRow, err := r.appendToolCall(ctx, session.ID, call)
			if err != nil {
				return err
			}
			_ = r.publishTimelineRow(ctx, *session, callRow)

			// Execute tool — may block for async execution (up to 300s).
			result, toolErr := r.tools.Call(ctx, call)
			if toolErr != nil {
				r.logError(ctx, "runtime tool call failed",
					"session_id", session.ID,
					"tool_name", call.Name,
					"error", toolErr,
				)
				errResultRow, _ := r.appendToolResult(ctx, session.ID, callRecord.ID, call.CallID,
					call.Name, domain.ToolResultStatusFailed, toolErr.Error(), nil)
				_ = r.publishTimelineRow(ctx, *session, errResultRow)

				// Feed error back to LLM so it can handle gracefully.
				errorOutput := agentapi.FunctionCallOutput(call.CallID,
					fmt.Sprintf(`{"error": %q}`, toolErr.Error()))
				conversation = append(conversation, errorOutput)
				if err := r.persistConversationState(ctx, session, conversation, providerState); err != nil {
					return err
				}
				continue
			}

			resultRow, err := r.appendToolResult(ctx, session.ID, callRecord.ID, call.CallID,
				call.Name, domain.ToolResultStatusSucceeded, result.MetaText, result.ToolMessage)
			if err != nil {
				return err
			}
			_ = r.publishTimelineRow(ctx, *session, resultRow)

			r.logInfo(ctx, "runtime tool call completed",
				"session_id", session.ID,
				"tool_name", call.Name,
				"meta_preview", previewText(result.MetaText, 240),
			)

			if result.OutputItem.CallID != "" {
				conversation = append(conversation, result.OutputItem)
				if err := r.persistConversationState(ctx, session, conversation, providerState); err != nil {
					return err
				}
			}
			continue // ← Always continue, never return
		}

		// ── Text response path — turn is done ──
		text := outputMessageText(output.Items)
		if text != "" {
			r.logInfo(ctx, "runtime llm returned assistant text",
				"session_id", session.ID,
				"done", output.Done,
				"text_preview", previewText(text, 240),
			)
			if !output.Streamed {
				_ = r.publishAssistantStream(ctx, session.ID, text)
			}
			row, err := r.appendAssistant(ctx, session.ID, text)
			if err != nil {
				return err
			}
			_ = r.publishTimelineRow(ctx, *session, row)
		}

		if len(output.Items) == 0 {
			// agent-sdk-go sometimes skips its internal final-synthesis step and
			// returns empty output even though the model responded. When the
			// conversation already contains tool results, retry once so a fresh
			// runner sees the full history and generates the summary directly.
			hasPriorToolResult := false
			for _, item := range conversation {
				if strings.TrimSpace(item.Type) == "function_call_output" {
					hasPriorToolResult = true
					break
				}
			}
			if hasPriorToolResult && round < maxToolRounds-1 {
				r.logInfo(ctx, "runtime agent skipped synthesis, retrying for final text",
					"session_id", session.ID, "round", round)
				continue
			}
			r.logError(ctx, "runtime llm returned empty output", "session_id", session.ID)
			session.Status = domain.SessionStatusFailed
			_ = r.bumpSession(ctx, session)
			_ = r.publishSessionState(ctx, *session)
			return ErrEmptyModelOutput
		}

		session.Status = domain.SessionStatusIdle
		_ = r.bumpSession(ctx, session)
		_ = r.publishSessionState(ctx, *session)
		return nil
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (r *Runtime) publishAssistantStream(ctx context.Context, sessionID string, text string) error {
	if r.events == nil || text == "" {
		return nil
	}
	responseID := r.ids.NewID("resp")
	sequenceNumber := 1
	for _, chunk := range chunkText(text, 18) {
		rawEvent := mustMarshalJSON(map[string]any{"delta": chunk})
		if err := r.events.LLMSSEEvent(ctx, sessionID, responseID, sequenceNumber, "response.output_text.delta", rawEvent); err != nil {
			return err
		}
		sequenceNumber++
	}
	return r.events.LLMResponseCompleted(ctx, sessionID, responseID, mustMarshalJSON(map[string]any{
		"id":          responseID,
		"output_text": text,
	}))
}

func chunkText(text string, size int) []string {
	if size <= 0 || text == "" {
		return nil
	}
	runes := []rune(text)
	chunks := make([]string, 0, (len(runes)+size-1)/size)
	for start := 0; start < len(runes); start += size {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func (r *Runtime) appendAssistant(ctx context.Context, sessionID string, text string) (domain.TimelineRow, error) {
	now := r.clock.Now()
	if err := r.repos.Messages.Append(ctx, domain.ThreadMessage{
		ID:        r.ids.NewID("msg"),
		SessionID: sessionID,
		Role:      domain.MessageRoleAssistant,
		Kind:      domain.ThreadMessageKindAssistantText,
		Content:   text,
		CreatedAt: now,
	}); err != nil {
		return domain.TimelineRow{}, err
	}
	row := domain.TimelineRow{
		ID:        r.ids.NewID("row"),
		SessionID: sessionID,
		Kind:      domain.TimelineRowKindAssistantText,
		CreatedAt: now,
		Text:      text,
	}
	return row, r.repos.Timelines.Append(ctx, row)
}

func (r *Runtime) appendToolCall(ctx context.Context, sessionID string, item agentapi.Item) (domain.ToolCall, domain.TimelineRow, error) {
	now := r.clock.Now()
	argsPreview := item.Arguments
	arguments := agentapi.ArgumentsJSON(item)
	call := domain.ToolCall{
		ID:          r.ids.NewID("toolcall"),
		SessionID:   sessionID,
		ToolName:    item.Name,
		CallID:      optionalStringPtr(item.CallID),
		Arguments:   cloneRaw(arguments),
		ArgsPreview: &argsPreview,
		Source:      domain.ToolCallSourceAgentLoop,
		CreatedAt:   now,
	}
	if err := r.repos.ToolCalls.Append(ctx, call); err != nil {
		return domain.ToolCall{}, domain.TimelineRow{}, err
	}
	row := domain.TimelineRow{
		ID:          r.ids.NewID("row"),
		SessionID:   sessionID,
		Kind:        domain.TimelineRowKindToolCallMeta,
		CreatedAt:   now,
		ToolName:    item.Name,
		ArgsPreview: &argsPreview,
		Source:      domain.TimelineRowSourceAgentLoop,
	}
	if err := r.repos.Timelines.Append(ctx, row); err != nil {
		return domain.ToolCall{}, domain.TimelineRow{}, err
	}
	return call, row, nil
}

func (r *Runtime) appendToolResult(ctx context.Context, sessionID, toolCallID string, callID string, toolName string, status domain.ToolResultStatus, text string, payload json.RawMessage) (domain.TimelineRow, error) {
	now := r.clock.Now()
	if err := r.repos.ToolResults.Append(ctx, domain.ToolResult{
		ID:         r.ids.NewID("toolresult"),
		SessionID:  sessionID,
		ToolCallID: &toolCallID,
		CallID:     strPtr(callID),
		ToolName:   toolName,
		Status:     status,
		Text:       text,
		Source:     domain.TimelineRowSourceAgentLoop,
		Payload:    cloneRaw(payload),
		CreatedAt:  now,
	}); err != nil {
		return domain.TimelineRow{}, err
	}
	row := domain.TimelineRow{
		ID:         r.ids.NewID("row"),
		SessionID:  sessionID,
		Kind:       domain.TimelineRowKindToolResultMeta,
		CreatedAt:  now,
		Text:       text,
		ToolName:   toolName,
		ToolStatus: status,
		Source:     domain.TimelineRowSourceAgentLoop,
	}
	return row, r.repos.Timelines.Append(ctx, row)
}

func (r *Runtime) bumpSession(ctx context.Context, session *domain.Session) error {
	session.Revision++
	session.UpdatedAt = r.clock.Now()
	return r.repos.Sessions.Update(ctx, *session)
}

func (r *Runtime) validateReady() error {
	if r.repos.Sessions == nil || r.repos.Messages == nil || r.repos.Timelines == nil || r.repos.ToolCalls == nil || r.repos.ToolResults == nil {
		return errors.New("runtime repositories are incomplete")
	}
	if r.llm == nil {
		return errors.New("llm client is not configured")
	}
	if r.tools == nil {
		return errors.New("tool registry is not configured")
	}
	return nil
}

func (r *Runtime) withSessionLock(ctx context.Context, sessionID string, fn func(context.Context) error) error {
	if r.locks == nil {
		return fn(ctx)
	}
	unlock, err := r.locks.LockSession(ctx, sessionID)
	if err != nil {
		return err
	}
	defer unlock()
	return fn(ctx)
}

func (r *Runtime) hasProcessedClientMessage(ctx context.Context, sessionID, clientMessageID string) (bool, error) {
	messages, err := r.repos.Messages.ListBySession(ctx, sessionID, domain.CursorPage{})
	if err != nil {
		return false, err
	}
	for _, message := range messages {
		if message.ClientMessageID != nil && *message.ClientMessageID == clientMessageID {
			return true, nil
		}
	}
	return false, nil
}

func (r *Runtime) publishSessionState(ctx context.Context, session domain.Session) error {
	if r.events == nil {
		return nil
	}
	return r.events.SessionStateUpdated(ctx, session)
}

func (r *Runtime) publishTimelineRow(ctx context.Context, session domain.Session, row domain.TimelineRow) error {
	if r.events == nil {
		return nil
	}
	return r.events.TimelineRowAppended(ctx, session, row)
}

func (r *Runtime) logInfo(ctx context.Context, msg string, args ...any) {
	if r.logger != nil {
		r.logger.InfoContext(ctx, msg, args...)
	}
}

func (r *Runtime) logError(ctx context.Context, msg string, args ...any) {
	if r.logger != nil {
		r.logger.ErrorContext(ctx, msg, args...)
	}
}

func previewText(text string, max int) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", "\\n")
	text = strings.ReplaceAll(text, "\r", "\\r")
	if max <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max]) + "..."
}

func strPtr(v string) *string { return &v }

func optionalStringPtr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func cloneRaw(in []byte) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func mustMarshalJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
