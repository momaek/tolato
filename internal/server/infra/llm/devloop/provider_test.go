package devloop

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/policy"
	"github.com/momaek/tolato/internal/server/app/runtime"
	"github.com/momaek/tolato/internal/server/domain"
)

func TestProviderLowRiskFlow(t *testing.T) {
	t.Parallel()

	provider := New()

	first, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{
		SessionID:    "sess-1",
		Conversation: []agentapi.Item{agentapi.UserMessage("diagnose tokyo nginx")},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn(first) error = %v", err)
	}
	if len(first.Items) != 1 || first.Items[0].Type != "function_call" || first.Items[0].Name != "resolve_target_nodes" {
		t.Fatalf("first = %#v, want resolve_target_nodes", first)
	}

	second, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{
		ProviderState: first.ProviderState,
		Conversation: []agentapi.Item{
			agentapi.FunctionCallOutput("call_resolve_target_nodes", string(mustJSON(t, policy.ResolveTargetNodesOutput{
				TargetContext: domain.ActiveTargetContext{
					Status:       domain.TargetStatusPendingConfirmation,
					Scope:        domain.TargetScopeSingle,
					NodeIDs:      []string{"jp-tokyo-01"},
					DisplayLabel: "jp-tokyo-01",
				},
			}))),
		},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn(second) error = %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Type != "function_call" || second.Items[0].Name != "request_target_confirmation" {
		t.Fatalf("second = %#v, want request_target_confirmation", second)
	}

	third, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{
		ProviderState: second.ProviderState,
		ActiveTargetContext: domain.ActiveTargetContext{
			Status:       domain.TargetStatusConfirmed,
			Scope:        domain.TargetScopeSingle,
			NodeIDs:      []string{"jp-tokyo-01"},
			DisplayLabel: "jp-tokyo-01",
		},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn(third) error = %v", err)
	}
	if len(third.Items) != 1 || third.Items[0].Type != "function_call" || third.Items[0].Name != "propose_plan" {
		t.Fatalf("third = %#v, want propose_plan", third)
	}
}

func TestProviderSummarizesTerminalExecution(t *testing.T) {
	t.Parallel()

	provider := New()
	result, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{
		ProviderState: mustJSON(t, state{Stage: "summarized"}),
		Conversation: []agentapi.Item{
			agentapi.FunctionCallOutput("call_summarize_execution", string(mustJSON(t, policy.SummarizeExecutionOutput{
				Summary: "execution completed successfully on jp-tokyo-01 (1/1 succeeded)",
			}))),
		},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if len(result.Items) != 1 || agentapi.MessageText(result.Items[0]) == "" || !result.Done {
		t.Fatalf("result = %#v, want final assistant summary", result)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}
