package agent

import (
	"context"
	"encoding/json"
	"log"

	"github.com/google/uuid"
	"github.com/momaek/tolato/server/internal/llm"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// LoopRunner is the core AI agent loop for a single conversation.
type LoopRunner struct {
	conversationID string
	llmClient      *llm.Client
	toolExecutor   *ToolExecutor
	promptBuilder  *PromptBuilder
	eventCh        chan<- any    // output events to WS handler
	confirmCh      chan bool     // receives confirm/reject from user
	maxRounds      int
	contextRounds  int

	// getNodeInfos is called to get current node list for prompt building
	getNodeInfos func() []NodeInfo
	// getCustomPrompt returns custom system prompt from settings
	getCustomPrompt func() string
}

// LoopRunnerConfig holds configuration for creating a LoopRunner.
type LoopRunnerConfig struct {
	ConversationID  string
	LLMClient       *llm.Client
	ToolExecutor    *ToolExecutor
	PromptBuilder   *PromptBuilder
	EventCh         chan<- any
	MaxRounds       int
	ContextRounds   int
	GetNodeInfos    func() []NodeInfo
	GetCustomPrompt func() string
}

// NewLoopRunner creates a new LoopRunner.
func NewLoopRunner(cfg LoopRunnerConfig) *LoopRunner {
	return &LoopRunner{
		conversationID:  cfg.ConversationID,
		llmClient:       cfg.LLMClient,
		toolExecutor:    cfg.ToolExecutor,
		promptBuilder:   cfg.PromptBuilder,
		eventCh:         cfg.EventCh,
		confirmCh:       make(chan bool, 1),
		maxRounds:       cfg.MaxRounds,
		contextRounds:   cfg.ContextRounds,
		getNodeInfos:    cfg.GetNodeInfos,
		getCustomPrompt: cfg.GetCustomPrompt,
	}
}

// ReceiveConfirm sends a confirm/reject signal to the running loop.
func (lr *LoopRunner) ReceiveConfirm(approved bool) {
	select {
	case lr.confirmCh <- approved:
	default:
	}
}

// Run executes the agent loop for a user message.
func (lr *LoopRunner) Run(ctx context.Context, input UserMessageInput) {
	convID := lr.conversationID
	log.Printf("[loop] start conv=%s content_len=%d model=%s", convID, len(input.Content), lr.llmClient.Model())

	// Build system prompt
	nodes := lr.getNodeInfos()
	customPrompt := lr.getCustomPrompt()
	defaultNodeID := ""
	if input.DefaultNodeID != nil {
		defaultNodeID = *input.DefaultNodeID
	}
	systemPrompt := lr.promptBuilder.Build(nodes, customPrompt, defaultNodeID)

	// Load history messages
	history, err := lr.loadHistory()
	if err != nil {
		lr.emitError(ctx, "Failed to load conversation history: "+err.Error())
		return
	}

	// Build message list
	messages := make([]llm.ChatMessage, 0, len(history)+2)
	messages = append(messages, llm.ChatMessage{Role: "system", Content: systemPrompt})
	messages = append(messages, history...)
	messages = append(messages, llm.ChatMessage{Role: "user", Content: input.Content})

	// Track messages to persist at the end (assistant + tool turns).
	// User message is persisted up-front so it survives any mid-stream cancel
	// (network drop, LLM error, ctx cancellation) — without this, refresh after
	// disconnect erases the user's last input from history.
	var newMessages []model.Message
	seq, _ := store.GetMaxSeq(convID)

	seq++
	userMsg := model.Message{
		ID:             uuid.New().String(),
		ConversationID: convID,
		Role:           "user",
		Content:        &input.Content,
		Seq:            seq,
	}
	if err := store.BatchCreateMessages([]model.Message{userMsg}); err != nil {
		log.Printf("[loop] failed to persist user message for conv %s: %v", convID, err)
		lr.emitError(ctx, "Failed to save message: "+err.Error())
		return
	}

	// Main loop
	for round := 0; round < lr.maxRounds; round++ {
		if ctx.Err() != nil {
			return
		}

		// Call LLM with streaming. The callback runs synchronously inside the
		// SDK's stream.Next() loop, so a blocked send here would stall the
		// whole stream — we use ctx-aware sends so a dead writer / cancelled
		// connection unblocks the LLM read promptly instead of deadlocking.
		var reasoning, content string
		result, err := lr.llmClient.ChatStream(ctx, messages, func(delta llm.StreamDelta) {
			switch delta.Type {
			case "reasoning":
				reasoning += delta.Reasoning
				lr.send(ctx, ReasoningEvent{ConversationID: convID, Delta: delta.Reasoning})
			case "content":
				content += delta.Content
				lr.send(ctx, ContentEvent{ConversationID: convID, Delta: delta.Content})
			}
		})

		if err != nil {
			log.Printf("[loop] conv=%s round=%d LLM error: %v", convID, round, err)
			lr.emitError(ctx, "LLM error: "+err.Error())
			return
		}

		log.Printf("[loop] conv=%s round=%d llm_result content_len=%d reasoning_len=%d tool_calls=%d",
			convID, round, len(result.Content), len(result.Reasoning), len(result.ToolCalls))

		// No tool calls → final response, done
		if len(result.ToolCalls) == 0 {
			seq++
			assistantMsg := model.Message{
				ID:             uuid.New().String(),
				ConversationID: convID,
				Role:           "assistant",
				Content:        &result.Content,
				Seq:            seq,
			}
			if result.Reasoning != "" {
				assistantMsg.Reasoning = &result.Reasoning
			}
			newMessages = append(newMessages, assistantMsg)
			break
		}

		// Has tool calls
		// Save assistant message with tool calls
		toolCallsJSON := marshalToolCalls(result.ToolCalls)
		seq++
		assistantMsg := model.Message{
			ID:             uuid.New().String(),
			ConversationID: convID,
			Role:           "assistant",
			Content:        &result.Content,
			ToolCalls:      &toolCallsJSON,
			Seq:            seq,
		}
		if result.Reasoning != "" {
			assistantMsg.Reasoning = &result.Reasoning
		}
		newMessages = append(newMessages, assistantMsg)

		// Add assistant message to LLM context
		messages = append(messages, llm.ChatMessage{
			Role:      "assistant",
			Content:   result.Content,
			Reasoning: result.Reasoning,
			ToolCalls: result.ToolCalls,
		})

		// Check blacklist
		if blocked, found := lr.toolExecutor.IsBlacklisted(result.ToolCalls); found {
			lr.emitError(ctx, "Command is blacklisted: "+getCommandFromToolCall(blocked))
			break
		}

		// Check sensitive operations
		if sensitive := lr.toolExecutor.NeedConfirmation(result.ToolCalls); sensitive != nil {
			if !lr.send(ctx, ConfirmRequestEvent{
				ConversationID: convID,
				ID:             sensitive.ID,
				Tool:           sensitive.Name,
				Args:           sensitive.Args,
			}) {
				return
			}

			// Wait for user confirmation
			select {
			case approved := <-lr.confirmCh:
				if !approved {
					// User rejected — add rejection as tool result
					for _, tc := range result.ToolCalls {
						if !lr.send(ctx, ToolCallEvent{
							ConversationID: convID,
							ID:             tc.ID,
							Tool:           tc.Name,
							Args:           tc.Args,
						}) {
							return
						}
						rejectedResult := &model.ToolResultItem{
							Data: map[string]any{"error": "Operation rejected by user"},
						}
						if !lr.send(ctx, ToolResultEvent{
							ConversationID: convID,
							ID:             tc.ID,
							Result:         rejectedResult,
						}) {
							return
						}
						messages = append(messages, llm.ChatMessage{
							Role:       "tool",
							Content:    ResultToJSON(rejectedResult),
							ToolCallID: tc.ID,
						})
						seq++
						toolCallID := tc.ID
						rejectedJSON := ResultToJSON(rejectedResult)
						newMessages = append(newMessages, model.Message{
							ID:             uuid.New().String(),
							ConversationID: convID,
							Role:           "tool",
							Content:        &rejectedJSON,
							ToolCallID:     &toolCallID,
							Seq:            seq,
						})
					}
					continue // let LLM respond to rejection
				}
			case <-ctx.Done():
				return
			}
		}

		// Execute tool calls
		for _, tc := range result.ToolCalls {
			if !lr.send(ctx, ToolCallEvent{
				ConversationID: convID,
				ID:             tc.ID,
				Tool:           tc.Name,
				Args:           tc.Args,
			}) {
				return
			}
		}

		results := lr.toolExecutor.ExecuteToolCalls(ctx, result.ToolCalls)

		// Emit results and build messages
		for _, tc := range result.ToolCalls {
			toolResult := results[tc.ID]
			if !lr.send(ctx, ToolResultEvent{
				ConversationID: convID,
				ID:             tc.ID,
				Result:         toolResult,
			}) {
				return
			}

			resultJSON := ResultToJSON(toolResult)
			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				Content:    resultJSON,
				ToolCallID: tc.ID,
			})

			seq++
			toolCallID := tc.ID
			newMessages = append(newMessages, model.Message{
				ID:             uuid.New().String(),
				ConversationID: convID,
				Role:           "tool",
				Content:        &resultJSON,
				ToolCallID:     &toolCallID,
				Seq:            seq,
			})
		}
	}

	// Persist all new messages
	if len(newMessages) > 0 {
		if err := store.BatchCreateMessages(newMessages); err != nil {
			log.Printf("[loop] failed to persist messages for conv %s: %v", convID, err)
		}
	}

	// Emit done
	log.Printf("[loop] done conv=%s new_messages=%d", convID, len(newMessages))
	lr.send(ctx, DoneEvent{ConversationID: convID})
}

// send delivers an event to the writer goroutine, returning false if ctx
// was cancelled before the writer accepted the event. A false return means
// the connection is gone — callers must abort the current iteration.
func (lr *LoopRunner) send(ctx context.Context, evt any) bool {
	select {
	case lr.eventCh <- evt:
		return true
	case <-ctx.Done():
		return false
	}
}

func (lr *LoopRunner) loadHistory() ([]llm.ChatMessage, error) {
	dbMsgs, err := store.ListMessagesByConversation(lr.conversationID)
	if err != nil {
		return nil, err
	}

	// Trim to contextRounds (keep last N*2 messages approximately)
	maxMsgs := lr.contextRounds * 2
	if len(dbMsgs) > maxMsgs {
		dbMsgs = dbMsgs[len(dbMsgs)-maxMsgs:]
	}

	messages := make([]llm.ChatMessage, 0, len(dbMsgs))
	for _, m := range dbMsgs {
		msg := llm.ChatMessage{
			Role: m.Role,
		}
		if m.Content != nil {
			msg.Content = *m.Content
		}
		if m.Reasoning != nil {
			msg.Reasoning = *m.Reasoning
		}
		if m.ToolCallID != nil {
			msg.ToolCallID = *m.ToolCallID
		}
		if m.ToolCalls != nil {
			var toolCalls []llm.ToolCall
			json.Unmarshal([]byte(*m.ToolCalls), &toolCalls)
			msg.ToolCalls = toolCalls
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (lr *LoopRunner) emitError(ctx context.Context, message string) {
	lr.send(ctx, ErrorEvent{
		ConversationID: lr.conversationID,
		Message:        message,
	})
}

func marshalToolCalls(calls []llm.ToolCall) string {
	data, err := json.Marshal(calls)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func getCommandFromToolCall(tc *llm.ToolCall) string {
	if cmd, ok := tc.Args["command"].(string); ok {
		return cmd
	}
	return ""
}
