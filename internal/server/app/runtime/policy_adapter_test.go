package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/momaek/tolato/internal/server/app/policy"
	"github.com/momaek/tolato/internal/server/domain"
)

func TestPolicyToolRegistryAdapter(t *testing.T) {
	reg := policy.NewRegistry(fakeNodeSource{})
	adapter := NewPolicyToolRegistry(reg)

	defs := adapter.Definitions()
	if len(defs) != 7 {
		t.Fatalf("len(defs) = %d, want 7", len(defs))
	}

	result, err := adapter.Call(context.Background(), ToolCallInput{
		Name: "list_nodes",
		Args: mustPolicyRaw(t, policy.ListNodesInput{}),
	})
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if result.MetaText == "" {
		t.Fatal("MetaText is empty")
	}
}

func TestPolicyToolRegistryAdapterAugmentsExecOnNodesArgs(t *testing.T) {
	reg := &stubPolicyRegistry{
		result: policy.ToolResult{MetaText: "queued execution for 1 node(s)"},
	}
	adapter := NewPolicyToolRegistry(reg)

	_, err := adapter.Call(context.Background(), ToolCallInput{
		SessionID: "sess-1",
		ActiveTargetContext: domain.ActiveTargetContext{
			Status:       domain.TargetStatusConfirmed,
			Scope:        domain.TargetScopeSingle,
			NodeIDs:      []string{"node-1"},
			DisplayLabel: "jp-tokyo-01",
			Source:       domain.TargetSourceUserExplicit,
		},
		Name: "exec_on_nodes",
		Args: json.RawMessage(`{"task_text":"ls -la ~","node_ids":["legacy-node"]}`),
	})
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	if reg.name != "exec_on_nodes" {
		t.Fatalf("registry name = %q, want exec_on_nodes", reg.name)
	}

	var req policy.ExecOnNodesInput
	if err := json.Unmarshal(reg.args, &req); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if req.SessionID != "sess-1" {
		t.Fatalf("SessionID = %q, want sess-1", req.SessionID)
	}
	if req.InputText != "ls -la ~" {
		t.Fatalf("InputText = %q, want shell snippet", req.InputText)
	}
	if req.Command != "bash" {
		t.Fatalf("Command = %q, want bash", req.Command)
	}
	if len(req.CommandArgs) != 2 || req.CommandArgs[0] != "-lc" || req.CommandArgs[1] != "ls -la ~" {
		t.Fatalf("CommandArgs = %#v, want bash -lc snippet", req.CommandArgs)
	}
	if len(req.TargetContext.NodeIDs) != 1 || req.TargetContext.NodeIDs[0] != "node-1" {
		t.Fatalf("TargetContext = %#v, want active target context preserved", req.TargetContext)
	}
}

type fakeNodeSource struct{}

func (fakeNodeSource) ListNodes(ctx context.Context) ([]policy.NodeSummary, error) {
	_ = ctx
	return []policy.NodeSummary{{
		ID:       "node-1",
		Hostname: "jp-tokyo-01",
		Region:   "asia",
		Status:   "online",
	}}, nil
}

func mustPolicyRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}

type stubPolicyRegistry struct {
	name   string
	args   json.RawMessage
	result policy.ToolResult
	err    error
}

func (s *stubPolicyRegistry) Definitions() []policy.ToolDefinition {
	return nil
}

func (s *stubPolicyRegistry) Call(ctx context.Context, name string, input json.RawMessage) (policy.ToolResult, error) {
	_ = ctx
	s.name = name
	s.args = append(json.RawMessage(nil), input...)
	return s.result, s.err
}
