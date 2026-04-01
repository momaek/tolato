package model

import "time"

// NodeRole represents the role of a node in the network topology.
type NodeRole string

const (
	NodeRoleEntry   NodeRole = "entry"
	NodeRoleRelay   NodeRole = "relay"
	NodeRoleLanding NodeRole = "landing"
)

// Node represents a monitored network node.
type Node struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Role     NodeRole  `json:"role"`
	LastSeen time.Time `json:"last_seen"`
}

// Link represents a directional connection between two nodes.
type Link struct {
	ID       string `json:"id"` // format: "sourceID->targetID"
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
}

// LinkID builds a link ID from source and target.
func LinkID(sourceID, targetID string) string {
	return sourceID + "->" + targetID
}

// MetricReport is the JSON body POSTed by the agent.
type MetricReport struct {
	NodeID    string         `json:"node_id"`
	Timestamp time.Time      `json:"timestamp"`
	Metrics   []TargetMetric `json:"metrics"`
}

// TargetMetric holds probe results for a single target.
type TargetMetric struct {
	TargetID       string   `json:"target_id"`
	LatencyMin     float64  `json:"latency_min"`
	LatencyAvg     float64  `json:"latency_avg"`
	LatencyMax     float64  `json:"latency_max"`
	PacketLoss     float64  `json:"packet_loss"`
	TCPConnectTime float64  `json:"tcp_connect_time"`
	BandwidthMbps  *float64 `json:"bandwidth_mbps"`
}

// MetricRow is a database record for a single metric sample.
type MetricRow struct {
	ID             int64     `json:"id"`
	LinkID         string    `json:"link_id"`
	Timestamp      time.Time `json:"timestamp"`
	LatencyMin     float64   `json:"latency_min"`
	LatencyAvg     float64   `json:"latency_avg"`
	LatencyMax     float64   `json:"latency_max"`
	PacketLoss     float64   `json:"packet_loss"`
	TCPConnectTime float64   `json:"tcp_connect_time"`
	BandwidthMbps  *float64  `json:"bandwidth_mbps"`
}

// AlertType classifies the kind of alert.
type AlertType string

const (
	AlertTypeLatency    AlertType = "latency"
	AlertTypePacketLoss AlertType = "packet_loss"
	AlertTypeTCP        AlertType = "tcp"
	AlertTypeBandwidth  AlertType = "bandwidth"
	AlertTypeOffline    AlertType = "offline"
)

// Alert represents a triggered (or recovered) alert.
type Alert struct {
	ID          int64      `json:"id"`
	LinkID      string     `json:"link_id"`
	Type        AlertType  `json:"type"`
	Message     string     `json:"message"`
	TriggeredAt time.Time  `json:"triggered_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

// LinkStatus is an enriched link with latest metric values and status.
type LinkStatus struct {
	Link
	SourceName     string   `json:"source_name"`
	TargetName     string   `json:"target_name"`
	LatencyAvg     *float64 `json:"latency_avg,omitempty"`
	PacketLoss     *float64 `json:"packet_loss,omitempty"`
	TCPConnectTime *float64 `json:"tcp_connect_time,omitempty"`
	BandwidthMbps  *float64 `json:"bandwidth_mbps,omitempty"`
	Status         string   `json:"status"` // "ok", "warn", "alert", "unknown"
	LastUpdated    *time.Time `json:"last_updated,omitempty"`
}

// ReportResponse is the server response to a metric report.
type ReportResponse struct {
	Status   string `json:"status"`
	Received int    `json:"received"`
}

// AlertFilter holds optional filters for querying alerts.
type AlertFilter struct {
	LinkID *string    `json:"link_id,omitempty"`
	Type   *AlertType `json:"type,omitempty"`
	Status *string    `json:"status,omitempty"` // "open" or "resolved"
	Limit  int        `json:"limit,omitempty"`
}
