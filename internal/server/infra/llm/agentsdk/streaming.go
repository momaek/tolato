package agentsdk

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/momaek/tolato/internal/server/app/runtime"
)

// forwardStreamEvents reads agent-sdk-go stream events from the channel and
// publishes them to the UI via EventPublisher. Event type names are kept
// compatible with the existing OpenAI Responses API event names so the
// frontend needs no changes.
func forwardStreamEvents(
	ctx context.Context,
	sessionID string,
	responseID string,
	ch <-chan interfaces.AgentStreamEvent,
	events runtime.EventPublisher,
) {
	if events == nil {
		// Drain the channel to prevent goroutine leaks.
		for range ch {
		}
		return
	}

	seq := 0
	for event := range ch {
		select {
		case <-ctx.Done():
			return
		default:
		}

		seq++
		switch event.Type {
		case interfaces.AgentEventThinking:
			raw := marshalDelta(event.ThinkingStep)
			_ = events.LLMSSEEvent(ctx, sessionID, responseID, seq,
				"response.reasoning_text.delta", raw)

		case interfaces.AgentEventContent:
			raw := marshalDelta(event.Content)
			_ = events.LLMSSEEvent(ctx, sessionID, responseID, seq,
				"response.output_text.delta", raw)

		case interfaces.AgentEventToolCall:
			if event.ToolCall != nil {
				// Send output_item.added so frontend picks up tool name via readPendingToolName.
				itemRaw := marshalToolCallItem(event.ToolCall)
				_ = events.LLMSSEEvent(ctx, sessionID, responseID, seq,
					"response.output_item.added", itemRaw)

				// Send function_call_arguments.done so frontend picks up arguments via readPendingToolArguments.
				seq++
				doneRaw := marshalToolCallDone(event.ToolCall)
				_ = events.LLMSSEEvent(ctx, sessionID, responseID, seq,
					"response.function_call_arguments.done", doneRaw)
			}

		case interfaces.AgentEventComplete:
			_ = events.LLMResponseCompleted(ctx, sessionID, responseID,
				marshalComplete(responseID, event.Content))

		case interfaces.AgentEventError:
			// Errors are handled by the doneChan path.
		}
	}
}

func marshalDelta(text string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"delta": text})
	return raw
}

// marshalToolCallItem matches the format expected by frontend's readPendingToolName:
// it checks rawEvent.name, then rawEvent.item.name.
func marshalToolCallItem(tc *interfaces.ToolCallEvent) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"item": map[string]any{
			"type":    "function_call",
			"name":    tc.Name,
			"call_id": tc.ID,
		},
	})
	return raw
}

// marshalToolCallDone matches the format expected by frontend's readPendingToolArguments:
// it checks rawEvent.arguments, then rawEvent.item.arguments.
func marshalToolCallDone(tc *interfaces.ToolCallEvent) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"name":      tc.Name,
		"arguments": tc.Arguments,
		"call_id":   tc.ID,
	})
	return raw
}

// forwardStreamEventsWithDynamicID is like forwardStreamEvents but reads the
// response ID from the runner atomically, so a single goroutine handles
// streaming across multiple turns without leaking.
func forwardStreamEventsWithDynamicID(
	ctx context.Context,
	sessionID string,
	runner *activeRunner,
	events runtime.EventPublisher,
) {
	if events == nil {
		for range runner.streamChan {
		}
		return
	}

	seq := 0
	eventCount := 0
	for event := range runner.streamChan {
		// Stop forwarding if context is cancelled (session closed).
		select {
		case <-ctx.Done():
			slog.Info("agentsdk stream: forwarder cancelled",
				"session_id", sessionID, "total_events", eventCount)
			return
		default:
		}

		responseID := runner.getResponseID()
		seq++
		eventCount++
		switch event.Type {
		case interfaces.AgentEventThinking:
			raw := marshalDelta(event.ThinkingStep)
			_ = events.LLMSSEEvent(ctx, sessionID, responseID, seq,
				"response.reasoning_text.delta", raw)
		case interfaces.AgentEventContent:
			raw := marshalDelta(event.Content)
			_ = events.LLMSSEEvent(ctx, sessionID, responseID, seq,
				"response.output_text.delta", raw)
		case interfaces.AgentEventToolCall:
			if event.ToolCall != nil {
				slog.Info("agentsdk stream: tool_call event",
					"session_id", sessionID, "tool", event.ToolCall.Name)
				itemRaw := marshalToolCallItem(event.ToolCall)
				_ = events.LLMSSEEvent(ctx, sessionID, responseID, seq,
					"response.output_item.added", itemRaw)
				seq++
				doneRaw := marshalToolCallDone(event.ToolCall)
				_ = events.LLMSSEEvent(ctx, sessionID, responseID, seq,
					"response.function_call_arguments.done", doneRaw)
			}
		case interfaces.AgentEventComplete:
			slog.Info("agentsdk stream: complete event",
				"session_id", sessionID, "response_id", responseID,
				"total_events", eventCount,
				"content_len", len(event.Content))
			_ = events.LLMResponseCompleted(ctx, sessionID, responseID,
				marshalComplete(responseID, event.Content))
		case interfaces.AgentEventError:
			slog.Warn("agentsdk stream: error event",
				"session_id", sessionID, "error", event.Error)
		}
	}
	slog.Info("agentsdk stream: forwarder finished",
		"session_id", sessionID, "total_events", eventCount)
}

func marshalComplete(responseID, content string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"id":          responseID,
		"output_text": content,
	})
	return raw
}
