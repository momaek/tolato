package node

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/nodemetrics"
)

func TestMetricsCacheLifecycle(t *testing.T) {
	ac := &AgentConn{NodeID: "trusted"}
	reg := model.AgentRegisterPayload{NodeExporterEnabled: true, MetricsIntervalSeconds: 30}
	ac.ConfigureExporter(reg)
	payload, _ := json.Marshal(nodemetrics.Snapshot{Version: 1, CollectedAt: 123, Data: "# TYPE node_load1 gauge\nnode_load1 1\n"})
	if ac.ExporterState().Fresh(time.Now(), 0) {
		t.Fatal("fresh before first sample")
	}
	ac.acceptNodeMetrics(payload)
	state := ac.ExporterState()
	if !state.Success || !state.Fresh(state.ReceivedAt.Add(89*time.Second), 0) {
		t.Fatal("valid cache not fresh")
	}
	if state.Fresh(state.ReceivedAt.Add(91*time.Second), 0) {
		t.Fatal("automatic TTL ignored")
	}
	if state.Fresh(state.ReceivedAt.Add(11*time.Second), 10*time.Second) {
		t.Fatal("configured TTL ignored")
	}
	ac.acceptNodeMetrics(json.RawMessage(`{"version":1,"error":"collection timeout"}`))
	if ac.ExporterState().Fresh(time.Now(), 0) || ac.ExporterState().Families != nil {
		t.Fatal("failed collection still exported")
	}
	if ac.ExporterState().CollectedAt != 123 {
		t.Fatal("last success time lost")
	}
	ac.ConfigureExporter(model.AgentRegisterPayload{})
	ac.acceptNodeMetrics(payload)
	if ac.ExporterState().Enabled || ac.ExporterState().Success {
		t.Fatal("disabled exporter accepted snapshot")
	}
}

func TestReplacingConnectionDoesNotRemoveNewCache(t *testing.T) {
	nm := NewNodeManager()
	url, ch := serveOne(t, nm, "node")
	first := dial(t, url)
	_ = first
	old := <-ch
	second := dial(t, url)
	_ = second
	replacement := <-ch
	nm.RemoveConn("node", old)
	if got, ok := nm.GetConn("node"); !ok || got != replacement {
		t.Fatal("old handler removed replacement connection")
	}
}
