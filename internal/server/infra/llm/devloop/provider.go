package devloop

import (
	"context"
	"encoding/json"
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

		return toolOutput("resolve_target_nodes", policy.ResolveTargetNodesInput{
			Query: userText,
		}, state{
			Stage:     "resolved",
			InputText: userText,
		})

	case "resolved":
		resolved, ok := lastToolPayload[policy.ResolveTargetNodesOutput](input.Conversation, "resolve_target_nodes")
		if !ok {
			return assistantOutput("Unable to resolve targets from the latest turn.", true), nil
		}

		return toolOutput("request_target_confirmation", policy.RequestTargetConfirmationInput{
			TargetContext: resolved.TargetContext,
		}, state{
			Stage:     "waiting_target",
			InputText: st.InputText,
		})

	case "waiting_target":
		if input.ActiveTargetContext.Status != domain.TargetStatusConfirmed {
			return assistantOutput("Waiting for target confirmation.", false), nil
		}

		return toolOutput("propose_plan", policy.ProposePlanInput{
			InputText:        st.InputText,
			TargetContext:    input.ActiveTargetContext,
			RiskLevel:        domain.RiskLevelLow,
			RequiresApproval: boolPtr(false),
		}, state{
			Stage:     "planned",
			InputText: st.InputText,
		})

	case "planned":
		plan, ok := lastToolPayload[policy.ProposedPlan](input.Conversation, "propose_plan")
		if !ok {
			return assistantOutput("Unable to build a plan for the current turn.", true), nil
		}

		risk := plan.RiskLevel
		if risk == "" {
			risk = domain.RiskLevelLow
		}
		if risk != domain.RiskLevelLow {
			return assistantOutput("Development console currently auto-runs only low-risk read-only tasks.", true), nil
		}

		return toolOutput("exec_on_nodes", policy.ExecOnNodesInput{
			SessionID:     input.SessionID,
			InputText:     st.InputText,
			TargetContext: input.ActiveTargetContext,
			RiskLevel:     risk,
		}, state{
			Stage:     "dispatched",
			InputText: st.InputText,
			RiskLevel: risk,
		})

	case "dispatched":
		if input.CurrentTask == nil {
			return assistantOutput("Waiting for execution state.", false), nil
		}

		return toolOutput("summarize_execution", policy.SummarizeExecutionInput{
			TaskID:      input.CurrentTask.TaskID,
			Status:      input.CurrentTask.Status,
			Aggregate:   input.CurrentTask.Aggregate,
			TargetLabel: targetLabel(input.ActiveTargetContext),
		}, state{
			Stage: "summarized",
		})

	case "summarized":
		summary, ok := lastToolPayload[policy.SummarizeExecutionOutput](input.Conversation, "summarize_execution")
		if !ok {
			return assistantOutput("Execution completed.", true), nil
		}
		return assistantOutput(summary.Summary, true), nil

	default:
		return assistantOutput("Unsupported development loop state.", true), nil
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

func targetLabel(ctx domain.ActiveTargetContext) string {
	if strings.TrimSpace(ctx.DisplayLabel) != "" {
		return ctx.DisplayLabel
	}
	switch len(ctx.NodeIDs) {
	case 0:
		return "selected targets"
	case 1:
		return ctx.NodeIDs[0]
	default:
		return "selected targets"
	}
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
		if item.Type != "function_call_output" || len(item.Content) != 0 {
			// no-op
		}
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

func boolPtr(v bool) *bool {
	return &v
}

func strPtr(v string) *string {
	return &v
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
