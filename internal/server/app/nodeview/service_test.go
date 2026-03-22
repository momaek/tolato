package nodeview

import (
	"context"
	"errors"
	"testing"

	"github.com/momaek/tolato/internal/server/app/policy"
	"github.com/momaek/tolato/internal/server/domain"
)

func TestServiceListNodesFiltersResults(t *testing.T) {
	svc := NewService(fakeNodeSource{nodes: []policy.NodeSummary{
		{
			ID:       "jp-tokyo-01",
			Hostname: "jp-tokyo-01",
			Region:   "Tokyo",
			OS:       "Ubuntu 24.04",
			Version:  "1.28.3",
			Tags:     []string{"edge", "prod"},
			Status:   "busy",
			Busy:     true,
			LastSeen: "2026-03-22T12:00:00Z",
			Metrics:  policy.Metrics{CPU: 32, Memory: 48, Disk: 61},
		},
		{
			ID:       "us-sfo-01",
			Hostname: "us-sfo-01",
			Region:   "San Francisco",
			OS:       "Debian 12",
			Version:  "1.28.2",
			Tags:     []string{"api"},
			Status:   "online",
			Busy:     false,
			LastSeen: "2026-03-22T11:59:00Z",
			Metrics:  policy.Metrics{CPU: 20, Memory: 41, Disk: 57},
		},
	}}, Repositories{})

	busy := true
	items, err := svc.ListNodes(context.Background(), ListFilter{
		Query:  "tokyo",
		Status: "busy",
		Busy:   &busy,
		Tag:    "edge",
	})
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListNodes() len = %d, want 1", len(items))
	}
	if items[0].ID != "jp-tokyo-01" {
		t.Fatalf("ListNodes() first id = %q, want jp-tokyo-01", items[0].ID)
	}
	if items[0].LastSeenAt != "2026-03-22T12:00:00Z" {
		t.Fatalf("ListNodes() lastSeenAt = %q, want preserved source timestamp", items[0].LastSeenAt)
	}
}

func TestServiceGetNodeReturnsDetail(t *testing.T) {
	svc := NewService(fakeNodeSource{nodes: []policy.NodeSummary{{
		ID:       "jp-tokyo-01",
		Hostname: "jp-tokyo-01",
		Region:   "Tokyo",
		OS:       "Ubuntu 24.04",
		Version:  "1.28.3",
		Tags:     []string{"edge"},
		Status:   "busy",
		Busy:     true,
		LastSeen: "2026-03-22T12:00:00Z",
		Metrics:  policy.Metrics{CPU: 32, Memory: 48, Disk: 61},
	}}}, Repositories{})

	item, err := svc.GetNode(context.Background(), "jp-tokyo-01")
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if item.Hostname != "jp-tokyo-01" || item.Provider == "unknown" {
		t.Fatalf("GetNode() = %#v, want mapped detail metadata", item)
	}
	if len(item.RiskSignal) == 0 || len(item.RecentTask) != 0 {
		t.Fatalf("GetNode() should derive risk signals and default recent tasks to empty: %#v", item)
	}
}

func TestServiceGetNodeReturnsNotFound(t *testing.T) {
	svc := NewService(fakeNodeSource{}, Repositories{})

	_, err := svc.GetNode(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetNode() error = %v, want ErrNotFound", err)
	}
}

type fakeNodeSource struct {
	nodes []policy.NodeSummary
	err   error
}

func (f fakeNodeSource) ListNodes(ctx context.Context) ([]policy.NodeSummary, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]policy.NodeSummary(nil), f.nodes...), nil
}
