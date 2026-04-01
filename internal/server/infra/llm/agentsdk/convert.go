package agentsdk

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"

	"github.com/momaek/tolato/internal/server/agentapi"
)

// buildConversationMemory converts the agentapi conversation items into an
// agent-sdk-go ConversationBuffer. This preserves full multi-turn context
// including tool calls and their results, so the LLM can reason about
// prior exchanges.
//
// The returned prompt is the last user message (the current turn input).
func buildConversationMemory(items []agentapi.Item) (interfaces.Memory, string) {
	mem := memory.NewConversationBuffer(memory.WithMaxSize(200))
	ctx := context.Background()

	var lastUserMessage string

	for _, item := range items {
		role := strings.TrimSpace(item.Role)
		typ := strings.TrimSpace(item.Type)

		switch {
		case role == "user":
			text := agentapi.MessageText(item)
			if text != "" {
				_ = mem.AddMessage(ctx, interfaces.Message{
					Role:    interfaces.MessageRoleUser,
					Content: text,
				})
				lastUserMessage = text
			}

		case role == "assistant" || typ == "message" && role == "assistant":
			text := agentapi.MessageText(item)
			if text != "" {
				_ = mem.AddMessage(ctx, interfaces.Message{
					Role:    interfaces.MessageRoleAssistant,
					Content: text,
				})
			}

		case typ == "function_call":
			// Record tool calls as assistant messages with tool call metadata.
			_ = mem.AddMessage(ctx, interfaces.Message{
				Role:    interfaces.MessageRoleAssistant,
				Content: "",
				ToolCalls: []interfaces.ToolCall{{
					ID:        item.CallID,
					Name:      item.Name,
					Arguments: item.Arguments,
				}},
			})

		case typ == "function_call_output":
			// Record tool results as tool messages.
			_ = mem.AddMessage(ctx, interfaces.Message{
				Role:       interfaces.MessageRoleTool,
				Content:    item.Output,
				ToolCallID: item.CallID,
			})
		}
	}

	return mem, lastUserMessage
}

// extractLastFunctionOutput finds the last function_call_output item in
// the conversation. This is used by resumeRunner to feed the tool result
// back through the channel.
func extractLastFunctionOutput(items []agentapi.Item) string {
	for i := len(items) - 1; i >= 0; i-- {
		if strings.TrimSpace(items[i].Type) == "function_call_output" {
			return items[i].Output
		}
	}
	return ""
}

// toolCallToItem converts an InterceptedCall into an agentapi.Item with
// type "function_call" so the Runtime's existing tool handling works.
func toolCallToItem(call InterceptedCall, callID string) agentapi.Item {
	return agentapi.Item{
		Type:      "function_call",
		Name:      call.Name,
		Arguments: call.Arguments,
		CallID:    callID,
	}
}

// messageItem creates an assistant message agentapi.Item from text.
func messageItem(text string) agentapi.Item {
	raw, _ := json.Marshal([]agentapi.ContentPart{{
		Type: "output_text",
		Text: text,
	}})
	return agentapi.Item{
		Type:    "message",
		Role:    "assistant",
		Content: raw,
	}
}
