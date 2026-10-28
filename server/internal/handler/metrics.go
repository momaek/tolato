package handler

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/authz"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/nodemetrics"
	"github.com/momaek/tolato/server/internal/store"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// ExternalMetrics uses exactly the same live authorization scope as /nodes.
// No permissions or rendered responses are cached between requests.
func ExternalMetrics(deps *Deps) gin.HandlerFunc {
	return externalMetrics(deps, func(c *gin.Context) ([]model.Node, error) {
		ids, all, err := authz.VisibleNodeIDs(subjectOf(c))
		if err != nil {
			return nil, err
		}
		if !all && len(ids) == 0 {
			return nil, nil
		}
		return store.ListMetricsNodesScoped(ids, all)
	})
}

func externalMetrics(deps *Deps, visibleNodes func(*gin.Context) ([]model.Node, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodes, err := visibleNodes(c)
		if err != nil {
			internalError(c, "failed to resolve metrics visibility")
			return
		}
		ttl := time.Duration(0)
		if deps.Config != nil {
			ttl = deps.Config.Metrics.FreshnessTTL
		}
		families := gatherNodeMetrics(deps.NodeManager, nodes, time.Now(), ttl)
		c.Header("Cache-Control", "no-store")
		promhttp.HandlerFor(prometheus.GathererFunc(func() ([]*dto.MetricFamily, error) { return families, nil }), promhttp.HandlerOpts{}).ServeHTTP(c.Writer, c.Request)
	}
}

func gatherNodeMetrics(manager *node.NodeManager, nodes []model.Node, now time.Time, ttl time.Duration) []*dto.MetricFamily {
	output := map[string]*dto.MetricFamily{}
	schema := nodemetrics.NewSchema()
	add := func(f *dto.MetricFamily) {
		if existing := output[f.GetName()]; existing != nil {
			existing.Metric = append(existing.Metric, f.Metric...)
		} else {
			output[f.GetName()] = &dto.MetricFamily{Name: f.Name, Help: f.Help, Type: f.Type, Metric: append([]*dto.Metric(nil), f.Metric...)}
		}
	}
	for _, n := range nodes {
		state := node.ExporterState{}
		connected := false
		if ac, ok := manager.GetConn(n.ID); ok {
			state = ac.ExporterState()
			select {
			case <-ac.Done():
			default:
				connected = true
			}
		}
		fresh := connected && state.Fresh(now, ttl)
		exported := fresh && schema.Add(state.Families)
		if exported {
			for _, f := range state.Families {
				add(f)
			}
		}
		labels := prometheus.Labels{"node_id": n.ID}
		add(metricGauge("tolato_node_connected", "Whether the node WebSocket is connected.", labels, boolFloat(connected)))
		add(metricGauge("tolato_node_metrics_enabled", "Whether the connected agent has enabled node_exporter.", labels, boolFloat(state.Enabled)))
		add(metricGauge("tolato_node_metrics_fresh", "Whether a valid node metrics snapshot is within its freshness TTL.", labels, boolFloat(fresh)))
		add(metricGauge("tolato_node_metrics_collection_success", "Whether the latest metrics snapshot passed collection and validation.", labels, boolFloat(state.Success)))
		add(metricGauge("tolato_node_metrics_export_success", "Whether this node's host metrics are included in this response; schema conflicts suppress the entire node snapshot.", labels, boolFloat(exported)))
		add(metricGauge("tolato_node_metrics_last_success_timestamp_seconds", "Agent collection time of the last successfully validated snapshot, or zero.", labels, state.CollectedAt))
		received := float64(0)
		if !state.ReceivedAt.IsZero() {
			received = float64(state.ReceivedAt.UnixMilli()) / 1000
		}
		add(metricGauge("tolato_node_metrics_last_received_timestamp_seconds", "Server receipt time of the last successfully validated snapshot, or zero.", labels, received))
		alias := ""
		if n.Alias != nil {
			alias = *n.Alias
		}
		add(metricGauge("tolato_node_info", "Node metadata; use node_id to join with host metrics.", prometheus.Labels{"node_id": n.ID, "hostname": n.Name, "alias": alias, "ip": n.IP}, 1))
	}
	return nodemetrics.Sorted(output)
}

func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
func metricGauge(name, help string, labels prometheus.Labels, value float64) *dto.MetricFamily {
	desc := prometheus.NewDesc(name, help, nil, labels)
	metric, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, value)
	if err != nil {
		panic(fmt.Sprintf("invalid internal metric %s: %v", name, err))
	}
	m := &dto.Metric{}
	_ = metric.Write(m)
	typ := dto.MetricType_GAUGE
	return &dto.MetricFamily{Name: &name, Help: &help, Type: &typ, Metric: []*dto.Metric{m}}
}
