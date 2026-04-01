package devloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/policy"
	"github.com/momaek/tolato/internal/server/app/runtime"
	"github.com/momaek/tolato/internal/server/domain"
)

type Provider struct{}

type state struct {
	Stage     string           `json:"stage"`
	InputText string           `json:"inputText,omitempty"`
	RiskLevel domain.RiskLevel `json:"riskLevel,omitempty"`
}

func New() *Provider {
	return &Provider{}
}

// RunTurn implements a simplified dev loop using the 2-tool model:
//
//	list_nodes for listing, run_on_node for execution (with built-in resolution and confirmation).
func (p *Provider) RunTurn(ctx context.Context, input runtime.ModelTurnInput, tools []agentapi.ToolSpec) (runtime.ModelTurnOutput, error) {
	_ = ctx
	_ = tools

	st := decodeState(input.ProviderState)

	switch st.Stage {
	case "":
		userText := lastUserText(input.Conversation)
		if strings.TrimSpace(userText) == "" {
			return assistantOutput("No user input available for this turn.", true), nil
		}

		// Simple heuristic: if the user is asking about nodes, list them.
		// Otherwise, try to run a command.
		lower := strings.ToLower(userText)
		if strings.Contains(lower, "节点") || strings.Contains(lower, "node") ||
			strings.Contains(lower, "多少") || strings.Contains(lower, "list") ||
			strings.Contains(lower, "show") {
			return toolOutput("list_nodes", policy.ListNodesInput{
				Query: userText,
			}, state{
				Stage:     "listed",
				InputText: userText,
			})
		}

		// Default: try run_on_node with the user's text as target.
		return toolOutput("run_on_node", policy.RunOnNodeInput{
			Target:  userText,
			Command: "system_status",
		}, state{
			Stage:     "executed",
			InputText: userText,
		})

	case "listed":
		// After listing nodes, produce a text summary.
		listed, ok := lastToolPayload[policy.ListNodesOutput](input.Conversation, "list_nodes")
		if !ok {
			return assistantOutput("Unable to parse node listing.", true), nil
		}
		summary := fmt.Sprintf("Found %d node(s).", len(listed.Nodes))
		return assistantOutput(summary, true), nil

	case "executed":
		// After run_on_node, produce a text summary from the result.
		result, ok := lastToolPayload[policy.RunOnNodeOutput](input.Conversation, "run_on_node")
		if !ok {
			return assistantOutput("Unable to parse execution result.", true), nil
		}
		return assistantOutput(formatRunResult(result), true), nil

	default:
		return assistantOutput("Unsupported development loop state.", true), nil
	}
}

func formatRunResult(result policy.RunOnNodeOutput) string {
	switch result.Status {
	case "completed":
		if len(result.Results) == 0 {
			if result.Message != "" {
				return result.Message
			}
			return "Execution completed."
		}
		var b strings.Builder
		for _, r := range result.Results {
			b.WriteString(fmt.Sprintf("[%s] %s (exit %d): %s\n", r.Status, r.Hostname, r.ExitCode, r.Output))
		}
		return b.String()
	case "needs_confirmation":
		return result.Message
	case "ambiguous":
		return result.Message
	case "no_match":
		return result.Message
	case "error":
		return "Error: " + result.Message
	default:
		return result.Message
	}
}

func toolOutput(name string, payload any, next state) (runtime.ModelTurnOutput, error) {
	return runtime.ModelTurnOutput{
		Items:         []agentapi.Item{agentapi.FunctionCall(name, payload)},
		ProviderState: mustState(next),
	}, nil
}

func assistantOutput(text string, done bool) runtime.ModelTurnOutput {
	return runtime.ModelTurnOutput{
		Items: []agentapi.Item{{
			Type:    "message",
			Role:    "assistant",
			Content: mustContent(text),
		}},
		Done: done,
	}
}

func lastUserText(conversation []agentapi.Item) string {
	for i := len(conversation) - 1; i >= 0; i-- {
		item := conversation[i]
		if item.Role == string(domain.MessageRoleUser) {
			if text := agentapi.MessageText(item); text != "" {
				return text
			}
		}
	}
	return ""
}

func decodeState(raw json.RawMessage) state {
	if len(raw) == 0 {
		return state{}
	}
	var out state
	if err := json.Unmarshal(raw, &out); err != nil {
		return state{}
	}
	return out
}

func mustState(st state) []byte {
	raw, err := json.Marshal(st)
	if err != nil {
		panic(err)
	}
	return raw
}

func lastToolPayload[T any](conversation []agentapi.Item, toolName string) (T, bool) {
	var out T
	for i := len(conversation) - 1; i >= 0; i-- {
		item := conversation[i]
		if item.Type != "function_call_output" || strings.TrimSpace(item.CallID) == "" {
			continue
		}
		callName := strings.TrimPrefix(item.CallID, "call_")
		if callName != toolName || strings.TrimSpace(item.Output) == "" {
			continue
		}
		if err := json.Unmarshal([]byte(item.Output), &out); err != nil {
			return out, false
		}
		return out, true
	}
	return out, false
}

func mustContent(text string) json.RawMessage {
	raw, err := json.Marshal([]agentapi.ContentPart{{
		Type: "output_text",
		Text: text,
	}})
	if err != nil {
		panic(err)
	}
	return raw
}
