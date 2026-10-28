package node

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/nodemetrics"
	dto "github.com/prometheus/client_model/go"
)

// ExporterState is copied under the connection's metrics lock. Families are
// immutable after validation. A new connection starts with an empty cache.
type ExporterState struct {
	Enabled     bool
	Interval    time.Duration
	ReceivedAt  time.Time
	CollectedAt float64
	Success     bool
	Families    []*dto.MetricFamily
}
type exporterCache struct {
	mu    sync.RWMutex
	state ExporterState
}

func (ac *AgentConn) ConfigureExporter(reg model.AgentRegisterPayload) {
	interval := time.Duration(reg.MetricsIntervalSeconds * float64(time.Second))
	enabled := reg.NodeExporterEnabled && interval >= 5*time.Second && interval <= time.Hour
	ac.exporter.mu.Lock()
	defer ac.exporter.mu.Unlock()
	ac.exporter.state = ExporterState{Enabled: enabled, Interval: interval}
}

func (ac *AgentConn) ExporterState() ExporterState {
	ac.exporter.mu.RLock()
	defer ac.exporter.mu.RUnlock()
	return ac.exporter.state
}

func (ac *AgentConn) acceptNodeMetrics(raw json.RawMessage) {
	if !ac.ExporterState().Enabled {
		return
	}
	// JSON escaping can expand the text by up to six times; the text itself has
	// a separate limit in Parse. Do not allocate a decoded oversized snapshot.
	if len(raw) > 6*nodemetrics.MaxBytes+2048 {
		ac.rejectNodeMetrics("oversized metrics envelope")
		return
	}
	var snapshot nodemetrics.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		ac.rejectNodeMetrics(err.Error())
		return
	}
	families, err := nodemetrics.Parse(snapshot, ac.NodeID)
	if err != nil {
		ac.rejectNodeMetrics(err.Error())
		return
	}
	ac.exporter.mu.Lock()
	defer ac.exporter.mu.Unlock()
	ac.exporter.state.ReceivedAt = time.Now()
	ac.exporter.state.CollectedAt = snapshot.CollectedAt
	ac.exporter.state.Success = true
	ac.exporter.state.Families = families
}
func (ac *AgentConn) rejectNodeMetrics(reason string) {
	log.Printf("[node_exporter] node=%s rejected snapshot: %.1024s", ac.NodeID, reason)
	ac.exporter.mu.Lock()
	defer ac.exporter.mu.Unlock()
	ac.exporter.state.Success = false
	ac.exporter.state.Families = nil
}

func (s ExporterState) Fresh(now time.Time, ttl time.Duration) bool {
	if ttl <= 0 {
		ttl = 3 * s.Interval
	}
	return s.Enabled && s.Success && !s.ReceivedAt.IsZero() && now.Sub(s.ReceivedAt) <= ttl
}
