package nodes

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/server/app/policy"
)

type StaticSource struct {
	nodes []policy.NodeSummary
}

func NewStaticSource(now time.Time) *StaticSource {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return &StaticSource{nodes: sampleNodes(now.UTC())}
}

func (s *StaticSource) ListNodes(ctx context.Context) ([]policy.NodeSummary, error) {
	_ = ctx
	return append([]policy.NodeSummary(nil), s.nodes...), nil
}

func sampleNodes(now time.Time) []policy.NodeSummary {
	format := now.Format(time.RFC3339)
	return []policy.NodeSummary{
		{
			ID:       "jp-tokyo-01",
			Hostname: "jp-tokyo-01",
			Region:   "Tokyo",
			OS:       "Ubuntu 24.04",
			Version:  "1.28.3",
			Tags:     []string{"edge", "prod", "nginx"},
			Status:   "busy",
			Busy:     true,
			LastSeen: format,
			Metrics: policy.Metrics{
				CPU:    0.42,
				Memory: 0.58,
				Disk:   0.71,
			},
		},
		{
			ID:       "jp-tokyo-02",
			Hostname: "jp-tokyo-02",
			Region:   "Tokyo",
			OS:       "Ubuntu 24.04",
			Version:  "1.28.3",
			Tags:     []string{"edge", "prod", "nginx"},
			Status:   "online",
			Busy:     false,
			LastSeen: format,
			Metrics: policy.Metrics{
				CPU:    0.31,
				Memory: 0.44,
				Disk:   0.63,
			},
		},
		{
			ID:       "us-sfo-01",
			Hostname: "us-sfo-01",
			Region:   "San Francisco",
			OS:       "Debian 12",
			Version:  "1.28.2",
			Tags:     []string{"api", "docker", "prod"},
			Status:   "online",
			Busy:     false,
			LastSeen: format,
			Metrics: policy.Metrics{
				CPU:    0.22,
				Memory: 0.39,
				Disk:   0.54,
			},
		},
		{
			ID:       "eu-fra-01",
			Hostname: "eu-fra-01",
			Region:   "Frankfurt",
			OS:       "Ubuntu 22.04",
			Version:  "1.27.9",
			Tags:     []string{"batch", "readonly"},
			Status:   "stale",
			Busy:     false,
			LastSeen: format,
			Metrics: policy.Metrics{
				CPU:    0.15,
				Memory: 0.33,
				Disk:   0.49,
			},
		},
		{
			ID:       "sg-edge-01",
			Hostname: "sg-edge-01",
			Region:   "Singapore",
			OS:       "Ubuntu 24.04",
			Version:  "1.28.3",
			Tags:     []string{"edge", "cdn"},
			Status:   "offline",
			Busy:     false,
			LastSeen: format,
			Metrics: policy.Metrics{
				CPU:    0,
				Memory: 0,
				Disk:   0,
			},
		},
	}
}
