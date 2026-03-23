package nodes

import (
	"context"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/app/policy"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

func TestObservedSourceOverlaysHeartbeatStatus(t *testing.T) {
	hub := infraws.NewMemoryHub()
	registry := infraws.NewMemoryAgentRegistry(hub)
	registry.BindNode("jp-tokyo-01", "agent-1", infraws.AgentNodeMetadata{
		Hostname: "jp-agent-01",
		Region:   "Tokyo",
		OS:       "linux",
		Version:  "2.0.0",
		Tags:     []string{"prod"},
	})
	if err := registry.Heartbeat("jp-tokyo-01", "agent-1", infraws.AgentNodeRuntime{
		Busy: true,
		Metrics: infraws.AgentNodeMetrics{
			CPU:    0.5,
			Memory: 0.4,
			Disk:   0.3,
		},
	}, time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}

	source := NewObservedSource(staticNodeSource{nodes: []policy.NodeSummary{{
		ID:       "jp-tokyo-01",
		Hostname: "jp-tokyo-01",
		Status:   "offline",
	}}}, registry)
	source.Now = func() time.Time { return time.Date(2026, 3, 22, 12, 0, 10, 0, time.UTC) }

	nodes, err := source.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("ListNodes() len = %d, want 1", len(nodes))
	}
	if nodes[0].Status != "online" {
		t.Fatalf("ListNodes() status = %q, want online", nodes[0].Status)
	}
	if nodes[0].LastSeen != "2026-03-22T12:00:00Z" {
		t.Fatalf("ListNodes() lastSeen = %q", nodes[0].LastSeen)
	}
	if nodes[0].Hostname != "jp-agent-01" || !nodes[0].Busy || nodes[0].Metrics.CPU != 0.5 {
		t.Fatalf("ListNodes() overlay = %#v", nodes[0])
	}
}

func TestObservedSourceBuildsSyntheticNodeFromPresence(t *testing.T) {
	hub := infraws.NewMemoryHub()
	registry := infraws.NewMemoryAgentRegistry(hub)
	registry.BindNode("custom-node-1", "agent-2", infraws.AgentNodeMetadata{
		Hostname: "custom-agent",
		Region:   "Singapore",
	})
	if err := registry.Heartbeat("custom-node-1", "agent-2", infraws.AgentNodeRuntime{
		Metrics: infraws.AgentNodeMetrics{Disk: 0.8},
	}, time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	registry.ForgetClient("agent-2")

	source := NewObservedSource(nil, registry)
	source.Now = func() time.Time { return time.Date(2026, 3, 22, 12, 0, 20, 0, time.UTC) }

	nodes, err := source.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("ListNodes() len = %d, want 1", len(nodes))
	}
	if nodes[0].ID != "custom-node-1" || nodes[0].Status != "stale" || nodes[0].Hostname != "custom-agent" || nodes[0].Metrics.Disk != 0.8 {
		t.Fatalf("synthetic node = %#v", nodes[0])
	}
}

type staticNodeSource struct {
	nodes []policy.NodeSummary
}

func (s staticNodeSource) ListNodes(ctx context.Context) ([]policy.NodeSummary, error) {
	_ = ctx
	return append([]policy.NodeSummary(nil), s.nodes...), nil
}
