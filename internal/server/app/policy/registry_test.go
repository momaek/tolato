package policy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/agentapi"
	appexecution "github.com/momaek/tolato/internal/server/app/execution"
)

const testTimeLayout = "2006-01-02T15:04:05Z07:00"

func TestRegistryDefinitions(t *testing.T) {
	registry := NewRegistry(fakeNodeSource{nodes: sampleNodes()})
	defs := registry.Definitions()

	want := []string{"list_nodes", "run_on_node"}
	if len(defs) != len(want) {
		t.Fatalf("len(defs) = %d, want %d", len(defs), len(want))
	}
	for i, def := range defs {
		if def.Function.Name != want[i] {
			t.Fatalf("defs[%d].Function.Name = %q, want %q", i, def.Function.Name, want[i])
		}
	}
}

func TestListNodesTool(t *testing.T) {
	registry := NewRegistry(fakeNodeSource{nodes: sampleNodes()})

	raw := mustJSON(t, ListNodesInput{Region: "asia", Tag: "prod", Busy: boolPtr(true)})
	result, err := registry.Call(context.Background(), functionCall("list_nodes", raw))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if result.MetaText != "listed 1 nodes" {
		t.Fatalf("MetaText = %q", result.MetaText)
	}
	var output ListNodesOutput
	if err := json.Unmarshal(result.ToolMessage, &output); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(output.Nodes) != 1 || output.Nodes[0].Hostname != "jp-tokyo-01" {
		t.Fatalf("output = %#v", output)
	}
}

func TestRunOnNodeNoMatch(t *testing.T) {
	registry := NewRegistry(fakeNodeSource{nodes: sampleNodes()}, WithExecutionStarter(&fakeExecutionStarter{}))

	raw := mustJSON(t, RunOnNodeInput{Target: "nonexistent", Command: "system_status"})
	ctx := ContextWithSessionID(context.Background(), "sess-1")
	result, err := registry.Call(ctx, functionCall("run_on_node", raw))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	var output RunOnNodeOutput
	if err := json.Unmarshal(result.ToolMessage, &output); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if output.Status != "no_match" {
		t.Fatalf("status = %q, want no_match", output.Status)
	}
	if len(output.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2 (all nodes)", len(output.Candidates))
	}
}

func TestRunOnNodeAmbiguous(t *testing.T) {
	nodes := sampleNodes()
	// Add a second node with overlapping region
	nodes = append(nodes, NodeSummary{
		ID:       "node-3",
		Hostname: "jp-osaka-01",
		Region:   "asia",
		Status:   "online",
		Tags:     []string{"prod"},
		LastSeen: time.Now().Format(testTimeLayout),
	})
	registry := NewRegistry(fakeNodeSource{nodes: nodes}, WithExecutionStarter(&fakeExecutionStarter{}))

	raw := mustJSON(t, RunOnNodeInput{Target: "asia", Command: "system_status"})
	ctx := ContextWithSessionID(context.Background(), "sess-1")
	result, err := registry.Call(ctx, functionCall("run_on_node", raw))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	var output RunOnNodeOutput
	if err := json.Unmarshal(result.ToolMessage, &output); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if output.Status != "ambiguous" {
		t.Fatalf("status = %q, want ambiguous", output.Status)
	}
}

func TestInferRiskForbidden(t *testing.T) {
	if risk := inferRisk("rm -rf /var/lib/app"); risk != "forbidden" {
		t.Fatalf("inferRisk() = %q, want forbidden", risk)
	}
}

type fakeExecutionStarter struct{}

func (f *fakeExecutionStarter) StartDispatch(_ context.Context, _ appexecution.StartDispatchInput) (appexecution.StartDispatchResult, error) {
	return appexecution.StartDispatchResult{}, nil
}

func (f *fakeExecutionStarter) StartUpgrade(_ context.Context, _ appexecution.StartUpgradeInput) (appexecution.StartDispatchResult, error) {
	return appexecution.StartDispatchResult{}, nil
}

func (f *fakeExecutionStarter) CancelTask(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

func (f *fakeExecutionStarter) RecordChunk(_ context.Context, _ appexecution.RecordChunkInput) error {
	return nil
}

func (f *fakeExecutionStarter) FinishExecution(_ context.Context, _ appexecution.FinishExecutionInput) error {
	return nil
}

func (f *fakeExecutionStarter) SendShellInput(_ context.Context, _ appexecution.ShellInputInput) error {
	return nil
}

func (f *fakeExecutionStarter) ResizeShell(_ context.Context, _ appexecution.ShellResizeInput) error {
	return nil
}

type fakeNodeSource struct {
	nodes []NodeSummary
}

func functionCall(name string, raw json.RawMessage) agentapi.Item {
	return agentapi.Item{
		Type:      "function_call",
		Name:      name,
		Arguments: string(raw),
		CallID:    "call_" + name,
	}
}

func (f fakeNodeSource) ListNodes(ctx context.Context) ([]NodeSummary, error) {
	return append([]NodeSummary(nil), f.nodes...), nil
}

func sampleNodes() []NodeSummary {
	now := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	return []NodeSummary{
		{
			ID:       "node-1",
			Hostname: "jp-tokyo-01",
			Region:   "asia",
			OS:       "Debian 11",
			Version:  "1.0.0",
			Tags:     []string{"prod", "web"},
			Status:   "online",
			Busy:     true,
			LastSeen: now.Format(testTimeLayout),
			Metrics:  Metrics{CPU: 0.6, Memory: 0.7, Disk: 0.4},
		},
		{
			ID:       "node-2",
			Hostname: "us-east-02",
			Region:   "us",
			OS:       "Ubuntu 22.04",
			Version:  "1.0.0",
			Tags:     []string{"staging"},
			Status:   "online",
			Busy:     false,
			LastSeen: now.Format(testTimeLayout),
		},
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}

func boolPtr(v bool) *bool { return &v }
