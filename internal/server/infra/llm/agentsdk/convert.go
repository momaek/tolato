package agentsdk

import (
	"encoding/json"
	"strings"

	"github.com/momaek/tolato/internal/server/agentapi"
)

// conversationToPrompt converts the agentapi conversation items into a
// text prompt that agent-sdk-go can use. This is necessary because
// agent-sdk-go's Agent.Run() accepts a single string input rather than
// a structured conversation.
//
// For the initial message this is trivial (just the user text).
// For resumed runs this function is not called — instead we feed the
// tool result directly through the channel.
func conversationToPrompt(items []agentapi.Item) string {
	// Find the last user message — that's the input to the agent.
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if strings.TrimSpace(item.Role) == "user" || strings.TrimSpace(item.Type) == "" && strings.TrimSpace(item.Role) == "user" {
			return agentapi.MessageText(item)
		}
	}
	// Fallback: concatenate all user messages.
	var b strings.Builder
	for _, item := range items {
		if strings.TrimSpace(item.Role) == "user" {
			text := agentapi.MessageText(item)
			if text != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(text)
			}
		}
	}
	return b.String()
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
