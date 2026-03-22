package nodes

import (
	"context"
	"testing"
	"time"
)

func TestStaticSourceListNodesReturnsSeededNodes(t *testing.T) {
	t.Parallel()

	source := NewStaticSource(time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC))
	nodes, err := source.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if len(nodes) != 5 {
		t.Fatalf("ListNodes() len = %d, want 5", len(nodes))
	}
	if nodes[0].Hostname != "jp-tokyo-01" || nodes[0].LastSeen == "" {
		t.Fatalf("first node = %#v, want seeded development node", nodes[0])
	}
}
