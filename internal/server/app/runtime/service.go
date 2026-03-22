package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

var ErrEmptyModelOutput = errors.New("empty model output")

type Service interface {
	HandleUserMessage(ctx context.Context, sessionID string, text string, clientMessageID string) error
	ResumeAfterTargetConfirmation(ctx context.Context, sessionID string, action ConfirmTargetAction) error
	ClearTargetContext(ctx context.Context, sessionID string, idempotencyKey string) error
	ResumeAfterApproval(ctx context.Context, sessionID string, action ApprovalAction) error
	HandleExecutionFinished(ctx context.Context, sessionID string, taskID string) error
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
	RunTurn(ctx context.Context, input ModelTurnInput, tools []ToolDefinition) (ModelTurnOutput, error)
}

type ToolRegistry interface {
	Definitions() []ToolDefinition
	Call(ctx context.Context, input ToolCallInput) (ToolResult, error)
}

type Runtime struct {
	repos  Repositories
	llm    LLMClient
	tools  ToolRegistry
	clock  domain.Clock
	ids    domain.IDGenerator
	locks  domain.LockManager
	events EventPublisher
}

type EventPublisher interface {
	SessionStateUpdated(ctx context.Context, session domain.Session) error
	TimelineRowAppended(ctx context.Context, session domain.Session, row domain.TimelineRow) error
	ThreadTargetPending(ctx context.Context, session domain.Session) error
	ThreadTargetConfirmed(ctx context.Context, session domain.Session) error
	ThreadTargetCleared(ctx context.Context, session domain.Session) error
	LLMSSEEvent(ctx context.Context, sessionID string, responseID string, sequenceNumber int, upstreamEventType string, rawEvent json.RawMessage) error
	LLMResponseCompleted(ctx context.Context, sessionID string, responseID string, rawResponse json.RawMessage) error
}

type Option func(*Runtime)

func WithEventPublisher(events EventPublisher) Option {
	return func(r *Runtime) {
		r.events = events
	}
}

func WithLockManager(locks domain.LockManager) Option {
	return func(r *Runtime) {
		r.locks = locks
	}
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
	SessionID           string
	Conversation        []ConversationItem
	ActiveTargetContext domain.ActiveTargetContext
	PendingAction       *domain.PendingAction
	ProviderState       json.RawMessage
	CurrentTask         *ExecutionContext
}

type ExecutionContext struct {
	TaskID    string
	Status    domain.TaskStatus
	Aggregate domain.ExecutionAggregate
}

type ConversationItem struct {
	Role      string
	Kind      string
	Content   string
	ToolName  string
	ToolInput json.RawMessage
}

type ModelTurnOutput struct {
	AssistantText *string
	ToolCall      *ToolInvocation
	Done          bool
	ProviderState []byte
}

type ToolInvocation struct {
	Name string
	Args json.RawMessage
}

type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ToolCallInput struct {
	SessionID           string
	ActiveTargetContext domain.ActiveTargetContext
	Name                string
	Args                json.RawMessage
}

type ToolResult struct {
	MetaText              string
	ToolMessage           json.RawMessage
	WaitForUser           bool
	PendingActionType     domain.PendingActionType
	PendingActionPayload  json.RawMessage
	AsyncExecutionStarted bool
	AppendPlanRow         bool
	AppendApprovalRow     bool
	AppendExecutionRow    bool
	AppendSummaryRow      bool
	TaskID                string
	ExecutionGroupID      string
}

type ConfirmTargetAction struct {
	NodeIDs        []string
	Scope          string
	IdempotencyKey string
}

type ApprovalAction struct {
	TaskID         string
	Approved       bool
	Reason         *string
	IdempotencyKey string
}

const userActionActorID = "ui_user"

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
			return nil
		}

		session, err := r.repos.Sessions.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		if !canAcceptMessage(session.Status) {
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
		session.PendingAction = nil
		if err := r.bumpSession(ctx, &session); err != nil {
			return err
		}
		if err := r.publishTimelineRow(ctx, session, userRow); err != nil {
			return err
		}
		if err := r.publishSessionState(ctx, session); err != nil {
			return err
		}

		conversation, err := r.rebuildConversation(ctx, sessionID)
		if err != nil {
			return err
		}
		return r.continueLoop(ctx, &session, conversation)
	})
}

func (r *Runtime) ResumeAfterTargetConfirmation(ctx context.Context, sessionID string, action ConfirmTargetAction) error {
	return r.withSessionLock(ctx, sessionID, func(ctx context.Context) error {
		if err := r.validateReady(); err != nil {
			return err
		}
		if r.repos.Audits == nil {
			return errors.New("audit repository is not configured")
		}
		if action.IdempotencyKey == "" {
			return domain.ErrInvalidArgument
		}

		duplicated, err := r.hasProcessedAction(ctx, sessionID, "target_confirmation", action.IdempotencyKey)
		if err != nil {
			return err
		}
		if duplicated {
			return nil
		}

		session, err := r.repos.Sessions.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		if session.Status != domain.SessionStatusPausedWaitTargetConfirmation || session.PendingAction == nil || session.PendingAction.Type != domain.PendingActionTypeTargetConfirmation {
			return fmt.Errorf("session %s is not waiting for target confirmation", sessionID)
		}

		targetCtx, err := decodeTargetContext(session.PendingAction.Payload)
		if err != nil {
			return err
		}
		targetCtx = confirmTargetContext(targetCtx, action, r.clock.Now())
		session.ActiveTargetContext = targetCtx
		session.PendingAction = nil
		session.Status = domain.SessionStatusRunning

		payload := mustMarshalJSON(map[string]any{
			"idempotencyKey": action.IdempotencyKey,
			"action":         "confirm",
			"targetContext":  targetCtx,
		})
		if err := r.appendAudit(ctx, session, "target_confirmation.confirmed", payload); err != nil {
			return err
		}
		resultRow, err := r.appendToolResultWithSource(ctx, session.ID, session.CurrentTaskID, nil, "target_confirmation", domain.ToolResultStatusSucceeded, targetConfirmationText(targetCtx), domain.TimelineRowSourceUserAction, payload)
		if err != nil {
			return err
		}
		if err := r.bumpSession(ctx, &session); err != nil {
			return err
		}
		if err := r.publishTimelineRow(ctx, session, resultRow); err != nil {
			return err
		}
		if err := r.publishTargetConfirmed(ctx, session); err != nil {
			return err
		}
		if err := r.publishSessionState(ctx, session); err != nil {
			return err
		}

		conversation, err := r.rebuildConversation(ctx, sessionID)
		if err != nil {
			return err
		}
		return r.continueLoop(ctx, &session, conversation)
	})
}

func (r *Runtime) ClearTargetContext(ctx context.Context, sessionID string, idempotencyKey string) error {
	return r.withSessionLock(ctx, sessionID, func(ctx context.Context) error {
		if err := r.validateReady(); err != nil {
			return err
		}
		if r.repos.Audits == nil {
			return errors.New("audit repository is not configured")
		}
		if idempotencyKey == "" {
			return domain.ErrInvalidArgument
		}

		duplicated, err := r.hasProcessedAction(ctx, sessionID, "target_clear", idempotencyKey)
		if err != nil {
			return err
		}
		if duplicated {
			return nil
		}

		session, err := r.repos.Sessions.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		if !canClearTargetContext(session) {
			return domain.ErrSessionBusy
		}
		if session.PendingAction == nil && session.ActiveTargetContext.Status == domain.TargetStatusUnset {
			return nil
		}

		session.ActiveTargetContext = clearedTargetContext()
		session.PendingAction = nil
		if session.Status == domain.SessionStatusPausedWaitTargetConfirmation {
			session.Status = domain.SessionStatusIdle
		}

		payload := mustMarshalJSON(map[string]any{
			"idempotencyKey": idempotencyKey,
			"action":         "clear",
			"targetContext":  session.ActiveTargetContext,
		})
		if err := r.appendAudit(ctx, session, "target_context.cleared", payload); err != nil {
			return err
		}
		resultRow, err := r.appendToolResultWithSource(ctx, session.ID, session.CurrentTaskID, nil, "target_clear", domain.ToolResultStatusSucceeded, "target context cleared", domain.TimelineRowSourceUserAction, payload)
		if err != nil {
			return err
		}
		if err := r.bumpSession(ctx, &session); err != nil {
			return err
		}
		if err := r.publishTimelineRow(ctx, session, resultRow); err != nil {
			return err
		}
		if err := r.publishTargetCleared(ctx, session); err != nil {
			return err
		}
		return r.publishSessionState(ctx, session)
	})
}

func (r *Runtime) ResumeAfterApproval(ctx context.Context, sessionID string, action ApprovalAction) error {
	return r.withSessionLock(ctx, sessionID, func(ctx context.Context) error {
		if err := r.validateReady(); err != nil {
			return err
		}
		if r.repos.Tasks == nil || r.repos.Audits == nil {
			return errors.New("runtime repositories are incomplete")
		}
		if action.IdempotencyKey == "" {
			return domain.ErrInvalidArgument
		}

		duplicated, err := r.hasProcessedAction(ctx, sessionID, "approval", action.IdempotencyKey)
		if err != nil {
			return err
		}
		if duplicated {
			return nil
		}

		session, err := r.repos.Sessions.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		if session.Status != domain.SessionStatusPausedWaitApproval || session.PendingAction == nil || session.PendingAction.Type != domain.PendingActionTypeApproval {
			return fmt.Errorf("session %s is not waiting for approval", sessionID)
		}

		taskID := action.TaskID
		if taskID == "" {
			taskID = pendingTaskID(session.PendingAction.Payload)
		}
		if taskID == "" && session.CurrentTaskID != nil {
			taskID = *session.CurrentTaskID
		}
		if taskID == "" {
			return domain.ErrInvalidArgument
		}

		task, err := r.repos.Tasks.Get(ctx, taskID)
		if err != nil {
			return err
		}
		payload := mustMarshalJSON(map[string]any{
			"idempotencyKey": action.IdempotencyKey,
			"action":         approvalActionName(action.Approved),
			"taskId":         taskID,
			"reason":         action.Reason,
		})

		if action.Approved {
			task.ApprovalStatus = domain.ApprovalStatusApproved
			task.Status = domain.TaskStatusApproved
			task.UpdatedAt = r.clock.Now()
			if err := r.repos.Tasks.Update(ctx, task); err != nil {
				return err
			}
			if err := r.appendAudit(ctx, session, "approval.approved", payload); err != nil {
				return err
			}
			resultRow, err := r.appendToolResultWithSource(ctx, session.ID, &taskID, nil, "approval", domain.ToolResultStatusSucceeded, "approval recorded", domain.TimelineRowSourceUserAction, payload)
			if err != nil {
				return err
			}
			session.PendingAction = nil
			session.Status = domain.SessionStatusRunning
			if err := r.bumpSession(ctx, &session); err != nil {
				return err
			}
			if err := r.publishTimelineRow(ctx, session, resultRow); err != nil {
				return err
			}
			if err := r.publishSessionState(ctx, session); err != nil {
				return err
			}
			conversation, err := r.rebuildConversation(ctx, sessionID)
			if err != nil {
				return err
			}
			return r.continueLoop(ctx, &session, conversation)
		}

		task.ApprovalStatus = domain.ApprovalStatusRejected
		task.Status = domain.TaskStatusCancelled
		task.UpdatedAt = r.clock.Now()
		if err := r.repos.Tasks.Update(ctx, task); err != nil {
			return err
		}
		if err := r.appendAudit(ctx, session, "approval.rejected", payload); err != nil {
			return err
		}
		resultRow, err := r.appendToolResultWithSource(ctx, session.ID, &taskID, nil, "approval", domain.ToolResultStatusSucceeded, approvalRejectedText(action.Reason), domain.TimelineRowSourceUserAction, payload)
		if err != nil {
			return err
		}
		session.PendingAction = nil
		session.Status = domain.SessionStatusCompleted
		if err := r.bumpSession(ctx, &session); err != nil {
			return err
		}
		if err := r.publishTimelineRow(ctx, session, resultRow); err != nil {
			return err
		}
		return r.publishSessionState(ctx, session)
	})
}

func (r *Runtime) HandleExecutionFinished(ctx context.Context, sessionID string, taskID string) error {
	return r.withSessionLock(ctx, sessionID, func(ctx context.Context) error {
		if err := r.validateReady(); err != nil {
			return err
		}
		if r.repos.Tasks == nil || r.repos.Executions == nil {
			return errors.New("runtime repositories are incomplete")
		}

		session, err := r.repos.Sessions.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		if session.Status != domain.SessionStatusWaitingAsyncExecution {
			return fmt.Errorf("session %s is not waiting for async execution", sessionID)
		}
		if session.CurrentTaskID == nil || *session.CurrentTaskID != taskID {
			return fmt.Errorf("session %s is not bound to task %s", sessionID, taskID)
		}

		task, err := r.repos.Tasks.Get(ctx, taskID)
		if err != nil {
			return err
		}
		aggregate, err := r.repos.Executions.AggregateByTask(ctx, taskID)
		if err != nil {
			return err
		}
		if !allExecutionsTerminal(aggregate) {
			return fmt.Errorf("task %s still has running executions", taskID)
		}

		session.Status = domain.SessionStatusRunning
		if err := r.bumpSession(ctx, &session); err != nil {
			return err
		}
		if err := r.publishSessionState(ctx, session); err != nil {
			return err
		}

		conversation, err := r.rebuildConversation(ctx, sessionID)
		if err != nil {
			return err
		}
		session.LastAgentState = mustMarshalJSON(ExecutionContext{
			TaskID:    task.ID,
			Status:    task.Status,
			Aggregate: aggregate,
		})
		return r.continueLoop(ctx, &session, conversation)
	})
}

func (r *Runtime) continueLoop(ctx context.Context, session *domain.Session, conversation []ConversationItem) error {
	for {
		output, err := r.llm.RunTurn(ctx, ModelTurnInput{
			SessionID:           session.ID,
			Conversation:        conversation,
			ActiveTargetContext: session.ActiveTargetContext,
			PendingAction:       session.PendingAction,
			ProviderState:       session.ProviderStateBlob,
			CurrentTask:         r.currentExecutionContext(ctx, session),
		}, r.tools.Definitions())
		if err != nil {
			session.Status = domain.SessionStatusFailed
			if saveErr := r.bumpSession(ctx, session); saveErr != nil {
				return errors.Join(err, saveErr)
			}
			if publishErr := r.publishSessionState(ctx, *session); publishErr != nil {
				return errors.Join(err, publishErr)
			}
			return err
		}

		session.ProviderStateBlob = cloneRaw(output.ProviderState)
		switch {
		case output.AssistantText != nil:
			if err := r.publishAssistantStream(ctx, session.ID, *output.AssistantText); err != nil {
				return err
			}
			row, err := r.appendAssistant(ctx, session.ID, *output.AssistantText)
			if err != nil {
				return err
			}
			if output.Done {
				session.Status = domain.SessionStatusCompleted
				if err := r.bumpSession(ctx, session); err != nil {
					return err
				}
				if err := r.publishTimelineRow(ctx, *session, row); err != nil {
					return err
				}
				return r.publishSessionState(ctx, *session)
			}
			session.Status = domain.SessionStatusIdle
			if err := r.bumpSession(ctx, session); err != nil {
				return err
			}
			if err := r.publishTimelineRow(ctx, *session, row); err != nil {
				return err
			}
			if err := r.publishSessionState(ctx, *session); err != nil {
				return err
			}
			conversation = append(conversation, ConversationItem{
				Role:    "assistant",
				Kind:    string(domain.ThreadMessageKindAssistantText),
				Content: *output.AssistantText,
			})
			return nil

		case output.ToolCall != nil:
			call, callRow, err := r.appendToolCall(ctx, session.ID, output.ToolCall)
			if err != nil {
				return err
			}
			if err := r.publishTimelineRow(ctx, *session, callRow); err != nil {
				return err
			}

			result, err := r.tools.Call(ctx, ToolCallInput{
				SessionID:           session.ID,
				ActiveTargetContext: session.ActiveTargetContext,
				Name:                output.ToolCall.Name,
				Args:                output.ToolCall.Args,
			})
			if err != nil {
				resultRow, appendErr := r.appendToolResult(ctx, session.ID, call.ID, output.ToolCall.Name, domain.ToolResultStatusFailed, err.Error(), nil)
				if appendErr != nil {
					return errors.Join(err, appendErr)
				}
				if publishErr := r.publishTimelineRow(ctx, *session, resultRow); publishErr != nil {
					return errors.Join(err, publishErr)
				}
				session.Status = domain.SessionStatusFailed
				if saveErr := r.bumpSession(ctx, session); saveErr != nil {
					return errors.Join(err, saveErr)
				}
				if publishErr := r.publishSessionState(ctx, *session); publishErr != nil {
					return errors.Join(err, publishErr)
				}
				return err
			}

			resultRow, err := r.appendToolResult(ctx, session.ID, call.ID, output.ToolCall.Name, domain.ToolResultStatusSucceeded, result.MetaText, result.ToolMessage)
			if err != nil {
				return err
			}
			if err := r.publishTimelineRow(ctx, *session, resultRow); err != nil {
				return err
			}
			if err := r.consumeToolResult(ctx, session, result); err != nil {
				return err
			}

			if result.WaitForUser || result.AsyncExecutionStarted {
				return nil
			}

			conversation = append(conversation, ConversationItem{
				Role:      "tool",
				Kind:      string(domain.TimelineRowKindToolResultMeta),
				Content:   result.MetaText,
				ToolName:  output.ToolCall.Name,
				ToolInput: cloneRaw(result.ToolMessage),
			})
			continue

		default:
			session.Status = domain.SessionStatusFailed
			if err := r.bumpSession(ctx, session); err != nil {
				return errors.Join(ErrEmptyModelOutput, err)
			}
			if err := r.publishSessionState(ctx, *session); err != nil {
				return errors.Join(ErrEmptyModelOutput, err)
			}
			return ErrEmptyModelOutput
		}
	}
}

func (r *Runtime) publishAssistantStream(ctx context.Context, sessionID string, text string) error {
	if r.events == nil || text == "" {
		return nil
	}

	responseID := r.ids.NewID("resp")
	sequenceNumber := 1
	for _, chunk := range chunkText(text, 18) {
		rawEvent := mustMarshalJSON(map[string]any{
			"delta": chunk,
		})
		if err := r.events.LLMSSEEvent(ctx, sessionID, responseID, sequenceNumber, "response.output_text.delta", rawEvent); err != nil {
			return err
		}
		sequenceNumber += 1
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

func (r *Runtime) rebuildConversation(ctx context.Context, sessionID string) ([]ConversationItem, error) {
	messages, err := r.repos.Messages.ListBySession(ctx, sessionID, domain.CursorPage{})
	if err != nil {
		return nil, err
	}
	results, err := r.repos.ToolResults.ListBySession(ctx, sessionID, domain.CursorPage{})
	if err != nil {
		return nil, err
	}

	items := make([]ConversationItem, 0, len(messages)+len(results))
	for _, msg := range messages {
		items = append(items, ConversationItem{
			Role:    string(msg.Role),
			Kind:    string(msg.Kind),
			Content: msg.Content,
		})
	}
	for _, result := range results {
		items = append(items, ConversationItem{
			Role:      "tool",
			Kind:      string(domain.TimelineRowKindToolResultMeta),
			Content:   result.Text,
			ToolName:  result.ToolName,
			ToolInput: cloneRaw(result.Payload),
		})
	}
	return items, nil
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

func (r *Runtime) appendToolCall(ctx context.Context, sessionID string, invocation *ToolInvocation) (domain.ToolCall, domain.TimelineRow, error) {
	now := r.clock.Now()
	argsPreview := string(invocation.Args)
	call := domain.ToolCall{
		ID:          r.ids.NewID("toolcall"),
		SessionID:   sessionID,
		ToolName:    invocation.Name,
		Arguments:   cloneRaw(invocation.Args),
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
		ToolName:    invocation.Name,
		ArgsPreview: &argsPreview,
		Source:      domain.TimelineRowSourceAgentLoop,
	}
	if err := r.repos.Timelines.Append(ctx, row); err != nil {
		return domain.ToolCall{}, domain.TimelineRow{}, err
	}
	return call, row, nil
}

func (r *Runtime) appendToolResult(ctx context.Context, sessionID, toolCallID, toolName string, status domain.ToolResultStatus, text string, payload json.RawMessage) (domain.TimelineRow, error) {
	return r.appendToolResultWithSource(ctx, sessionID, nil, &toolCallID, toolName, status, text, domain.TimelineRowSourceAgentLoop, payload)
}

func (r *Runtime) appendToolResultWithSource(ctx context.Context, sessionID string, taskID *string, toolCallID *string, toolName string, status domain.ToolResultStatus, text string, source domain.TimelineRowSource, payload json.RawMessage) (domain.TimelineRow, error) {
	now := r.clock.Now()
	if err := r.repos.ToolResults.Append(ctx, domain.ToolResult{
		ID:         r.ids.NewID("toolresult"),
		SessionID:  sessionID,
		TaskID:     taskID,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Status:     status,
		Text:       text,
		Source:     source,
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
		Source:     source,
	}
	return row, r.repos.Timelines.Append(ctx, row)
}

func (r *Runtime) consumeToolResult(ctx context.Context, session *domain.Session, result ToolResult) error {
	now := r.clock.Now()

	if result.AppendPlanRow && result.TaskID != "" {
		taskID := result.TaskID
		row := domain.TimelineRow{
			ID:        r.ids.NewID("row"),
			SessionID: session.ID,
			Kind:      domain.TimelineRowKindPlan,
			CreatedAt: now,
			TaskID:    &taskID,
		}
		if err := r.repos.Timelines.Append(ctx, row); err != nil {
			return err
		}
		if err := r.publishTimelineRow(ctx, *session, row); err != nil {
			return err
		}
	}
	if result.TaskID != "" {
		taskID := result.TaskID
		session.CurrentTaskID = &taskID
	}
	if result.ExecutionGroupID != "" {
		groupID := result.ExecutionGroupID
		session.CurrentExecutionGroupID = &groupID
	}
	if result.AppendApprovalRow && result.TaskID != "" {
		taskID := result.TaskID
		row := domain.TimelineRow{
			ID:        r.ids.NewID("row"),
			SessionID: session.ID,
			Kind:      domain.TimelineRowKindApproval,
			CreatedAt: now,
			TaskID:    &taskID,
		}
		if err := r.repos.Timelines.Append(ctx, row); err != nil {
			return err
		}
		if err := r.publishTimelineRow(ctx, *session, row); err != nil {
			return err
		}
	}
	if result.AppendExecutionRow && result.TaskID != "" {
		taskID := result.TaskID
		row := domain.TimelineRow{
			ID:        r.ids.NewID("row"),
			SessionID: session.ID,
			Kind:      domain.TimelineRowKindExecution,
			CreatedAt: now,
			TaskID:    &taskID,
		}
		if err := r.repos.Timelines.Append(ctx, row); err != nil {
			return err
		}
		if err := r.publishTimelineRow(ctx, *session, row); err != nil {
			return err
		}
	}
	if result.AppendSummaryRow && result.TaskID != "" {
		taskID := result.TaskID
		row := domain.TimelineRow{
			ID:        r.ids.NewID("row"),
			SessionID: session.ID,
			Kind:      domain.TimelineRowKindSummary,
			CreatedAt: now,
			TaskID:    &taskID,
			Text:      result.MetaText,
		}
		if err := r.repos.Timelines.Append(ctx, row); err != nil {
			return err
		}
		if r.repos.Tasks != nil {
			task, err := r.repos.Tasks.Get(ctx, taskID)
			if err != nil {
				return err
			}
			task.Summary = strPtr(result.MetaText)
			task.UpdatedAt = now
			if err := r.repos.Tasks.Update(ctx, task); err != nil {
				return err
			}
		}
		if err := r.publishTimelineRow(ctx, *session, row); err != nil {
			return err
		}
	}

	if result.WaitForUser {
		session.PendingAction = &domain.PendingAction{
			Type:    result.PendingActionType,
			Payload: cloneRaw(result.PendingActionPayload),
		}
		switch result.PendingActionType {
		case domain.PendingActionTypeTargetConfirmation:
			session.Status = domain.SessionStatusPausedWaitTargetConfirmation
			targetCtx, err := decodeTargetContext(result.PendingActionPayload)
			if err != nil {
				return err
			}
			session.ActiveTargetContext = targetCtx
			row := domain.TimelineRow{
				ID:            r.ids.NewID("row"),
				SessionID:     session.ID,
				Kind:          domain.TimelineRowKindTargetConfirmation,
				CreatedAt:     now,
				TargetContext: &targetCtx,
			}
			if err := r.repos.Timelines.Append(ctx, row); err != nil {
				return err
			}
			if err := r.bumpSession(ctx, session); err != nil {
				return err
			}
			if err := r.publishTimelineRow(ctx, *session, row); err != nil {
				return err
			}
			if err := r.publishTargetPending(ctx, *session); err != nil {
				return err
			}
			return r.publishSessionState(ctx, *session)
		case domain.PendingActionTypeApproval:
			session.Status = domain.SessionStatusPausedWaitApproval
		default:
			return fmt.Errorf("unsupported pending action type %q", result.PendingActionType)
		}
		if err := r.bumpSession(ctx, session); err != nil {
			return err
		}
		return r.publishSessionState(ctx, *session)
	}
	if result.AsyncExecutionStarted {
		session.PendingAction = nil
		session.Status = domain.SessionStatusWaitingAsyncExecution
		if err := r.bumpSession(ctx, session); err != nil {
			return err
		}
		return r.publishSessionState(ctx, *session)
	}

	session.Status = domain.SessionStatusRunning
	if err := r.bumpSession(ctx, session); err != nil {
		return err
	}
	return r.publishSessionState(ctx, *session)
}

func (r *Runtime) bumpSession(ctx context.Context, session *domain.Session) error {
	session.Revision++
	session.UpdatedAt = r.clock.Now()
	return r.repos.Sessions.Update(ctx, *session)
}

func canAcceptMessage(status domain.SessionStatus) bool {
	switch status {
	case domain.SessionStatusRunning,
		domain.SessionStatusPausedWaitTargetConfirmation,
		domain.SessionStatusPausedWaitApproval,
		domain.SessionStatusWaitingAsyncExecution:
		return false
	default:
		return true
	}
}

func allExecutionsTerminal(aggregate domain.ExecutionAggregate) bool {
	return aggregate.Total > 0 && aggregate.Queued == 0 && aggregate.Dispatched == 0 && aggregate.Running == 0
}

func (r *Runtime) currentExecutionContext(ctx context.Context, session *domain.Session) *ExecutionContext {
	if session == nil || session.CurrentTaskID == nil || r.repos.Tasks == nil || r.repos.Executions == nil {
		return nil
	}
	task, err := r.repos.Tasks.Get(ctx, *session.CurrentTaskID)
	if err != nil {
		return nil
	}
	aggregate, err := r.repos.Executions.AggregateByTask(ctx, task.ID)
	if err != nil {
		return nil
	}
	return &ExecutionContext{
		TaskID:    task.ID,
		Status:    task.Status,
		Aggregate: aggregate,
	}
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

func decodeTargetContext(raw json.RawMessage) (domain.ActiveTargetContext, error) {
	var payload struct {
		TargetContext domain.ActiveTargetContext `json:"targetContext"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return domain.ActiveTargetContext{}, err
	}
	return payload.TargetContext, nil
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

func (r *Runtime) hasProcessedAction(ctx context.Context, sessionID, toolName, idempotencyKey string) (bool, error) {
	results, err := r.repos.ToolResults.ListBySession(ctx, sessionID, domain.CursorPage{})
	if err != nil {
		return false, err
	}
	for _, result := range results {
		if result.ToolName != toolName || result.Source != domain.TimelineRowSourceUserAction {
			continue
		}
		var payload struct {
			IdempotencyKey string `json:"idempotencyKey"`
		}
		if err := json.Unmarshal(result.Payload, &payload); err != nil {
			continue
		}
		if payload.IdempotencyKey == idempotencyKey {
			return true, nil
		}
	}
	return false, nil
}

func (r *Runtime) appendAudit(ctx context.Context, session domain.Session, eventType string, payload json.RawMessage) error {
	return r.repos.Audits.Append(ctx, domain.AuditRecord{
		ID:        r.ids.NewID("audit"),
		SessionID: session.ID,
		TaskID:    session.CurrentTaskID,
		ActorID:   userActionActorID,
		EventType: eventType,
		Payload:   cloneRaw(payload),
		CreatedAt: r.clock.Now(),
	})
}

func confirmTargetContext(current domain.ActiveTargetContext, action ConfirmTargetAction, now time.Time) domain.ActiveTargetContext {
	if len(action.NodeIDs) > 0 {
		current.NodeIDs = append([]string(nil), action.NodeIDs...)
	}
	if action.Scope != "" {
		current.Scope = domain.TargetScope(action.Scope)
	}
	current.Status = domain.TargetStatusConfirmed
	confirmedAt := now.UTC()
	current.ConfirmedAt = &confirmedAt
	if current.DisplayLabel == "" || len(action.NodeIDs) > 0 {
		current.DisplayLabel = targetDisplayLabel(current.Scope, current.NodeIDs)
	}
	return current
}

func clearedTargetContext() domain.ActiveTargetContext {
	return domain.ActiveTargetContext{
		Status: domain.TargetStatusUnset,
	}
}

func canClearTargetContext(session domain.Session) bool {
	switch session.Status {
	case domain.SessionStatusPausedWaitTargetConfirmation, domain.SessionStatusIdle, domain.SessionStatusCompleted, domain.SessionStatusFailed:
		return true
	default:
		return false
	}
}

func targetDisplayLabel(scope domain.TargetScope, nodeIDs []string) string {
	switch {
	case scope == domain.TargetScopeAllOnline:
		return "All online nodes"
	case len(nodeIDs) == 1:
		return nodeIDs[0]
	case len(nodeIDs) > 1:
		return fmt.Sprintf("%d targets", len(nodeIDs))
	default:
		return "Confirmed target"
	}
}

func targetConfirmationText(ctx domain.ActiveTargetContext) string {
	switch len(ctx.NodeIDs) {
	case 0:
		return "target confirmed"
	case 1:
		return "1 target confirmed"
	default:
		return fmt.Sprintf("%d targets confirmed", len(ctx.NodeIDs))
	}
}

func pendingTaskID(raw json.RawMessage) string {
	var payload struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return payload.TaskID
}

func approvalActionName(approved bool) string {
	if approved {
		return "approve"
	}
	return "reject"
}

func approvalRejectedText(reason *string) string {
	if reason == nil || *reason == "" {
		return "approval rejected"
	}
	return fmt.Sprintf("approval rejected: %s", *reason)
}

func strPtr(v string) *string { return &v }

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

func (r *Runtime) publishTargetPending(ctx context.Context, session domain.Session) error {
	if r.events == nil {
		return nil
	}
	return r.events.ThreadTargetPending(ctx, session)
}

func (r *Runtime) publishTargetConfirmed(ctx context.Context, session domain.Session) error {
	if r.events == nil {
		return nil
	}
	return r.events.ThreadTargetConfirmed(ctx, session)
}

func (r *Runtime) publishTargetCleared(ctx context.Context, session domain.Session) error {
	if r.events == nil {
		return nil
	}
	return r.events.ThreadTargetCleared(ctx, session)
}
