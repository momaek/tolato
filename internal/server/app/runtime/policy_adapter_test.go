package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/momaek/tolato/internal/server/app/policy"
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
