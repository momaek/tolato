package devloop

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/policy"
	"github.com/momaek/tolato/internal/server/app/runtime"
)

func TestProviderListNodesFlow(t *testing.T) {
	t.Parallel()

	provider := New()

	// Step 1: user message about nodes → list_nodes
	first, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{
		SessionID:    "sess-1",
		Conversation: []agentapi.Item{agentapi.UserMessage("show me all nodes")},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn(first) error = %v", err)
	}
	if len(first.Items) != 1 || first.Items[0].Name != "list_nodes" {
		t.Fatalf("first = %#v, want list_nodes", first)
	}

	// Step 2: list_nodes result → text summary
	second, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{
		SessionID:     "sess-1",
		ProviderState: first.ProviderState,
		Conversation: []agentapi.Item{
			agentapi.FunctionCallOutput("call_list_nodes", string(mustJSON(t, policy.ListNodesOutput{
				Nodes: []policy.NodeSummary{
					{ID: "node-1", Hostname: "jp-tokyo-01"},
					{ID: "node-2", Hostname: "us-east-02"},
				},
			}))),
		},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn(second) error = %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Type != "message" || !second.Done {
		t.Fatalf("second = %#v, want final assistant text summary", second)
	}
	text := agentapi.MessageText(second.Items[0])
	if text == "" {
		t.Fatal("summary text is empty")
	}
}

func TestProviderRunOnNodeFlow(t *testing.T) {
	t.Parallel()

	provider := New()

	// Step 1: user message → run_on_node
	first, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{
		SessionID:    "sess-2",
		Conversation: []agentapi.Item{agentapi.UserMessage("check tokyo disk")},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn(first) error = %v", err)
	}
	if len(first.Items) != 1 || first.Items[0].Name != "run_on_node" {
		t.Fatalf("first = %#v, want run_on_node", first)
	}

	// Step 2: run_on_node result → text summary
	second, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{
		SessionID:     "sess-2",
		ProviderState: first.ProviderState,
		Conversation: []agentapi.Item{
			agentapi.FunctionCallOutput("call_run_on_node", string(mustJSON(t, policy.RunOnNodeOutput{
				Status: "completed",
				Results: []policy.NodeExecResult{
					{NodeID: "node-1", Hostname: "jp-tokyo-01", Output: "Disk usage: 40%", ExitCode: 0, Status: "success"},
				},
			}))),
		},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn(second) error = %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Type != "message" || !second.Done {
		t.Fatalf("second = %#v, want final assistant text summary", second)
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
