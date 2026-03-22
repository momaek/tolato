package devloop

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/momaek/tolato/internal/server/app/policy"
	"github.com/momaek/tolato/internal/server/app/runtime"
	"github.com/momaek/tolato/internal/server/domain"
)

func TestProviderLowRiskFlow(t *testing.T) {
	t.Parallel()

	provider := New()

	first, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{
		SessionID: "sess-1",
		Conversation: []runtime.ConversationItem{{
			Role:    string(domain.MessageRoleUser),
			Content: "diagnose tokyo nginx",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn(first) error = %v", err)
	}
	if first.ToolCall == nil || first.ToolCall.Name != "resolve_target_nodes" {
		t.Fatalf("first = %#v, want resolve_target_nodes", first)
	}

	second, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{
		ProviderState: first.ProviderState,
		Conversation: []runtime.ConversationItem{{
			ToolName: "resolve_target_nodes",
			ToolInput: mustJSON(t, policy.ResolveTargetNodesOutput{
				TargetContext: domain.ActiveTargetContext{
					Status:       domain.TargetStatusPendingConfirmation,
					Scope:        domain.TargetScopeSingle,
					NodeIDs:      []string{"jp-tokyo-01"},
					DisplayLabel: "jp-tokyo-01",
				},
			}),
		}},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn(second) error = %v", err)
	}
	if second.ToolCall == nil || second.ToolCall.Name != "request_target_confirmation" {
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
	if third.ToolCall == nil || third.ToolCall.Name != "propose_plan" {
		t.Fatalf("third = %#v, want propose_plan", third)
	}
}

func TestProviderSummarizesTerminalExecution(t *testing.T) {
	t.Parallel()

	provider := New()
	result, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{
		ProviderState: mustJSON(t, state{Stage: "summarized"}),
		Conversation: []runtime.ConversationItem{{
			ToolName: "summarize_execution",
			ToolInput: mustJSON(t, policy.SummarizeExecutionOutput{
				Summary: "execution completed successfully on jp-tokyo-01 (1/1 succeeded)",
			}),
		}},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.AssistantText == nil || *result.AssistantText == "" || !result.Done {
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
