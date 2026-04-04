// Package model defines the database models and API contracts for tolato.
package model

import "time"

// ============================================================================
// Database Models (GORM)
// ============================================================================

// --- Core: Conversations ---

type Conversation struct {
	ID        string    `json:"id" gorm:"primaryKey;type:text"`
	Title     string    `json:"title" gorm:"type:text;not null;default:'新对话'"`
	Model     string    `json:"model" gorm:"type:text"` // LLM model used
	DefaultNodeID *string `json:"default_node_id,omitempty" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`

	Messages []Message `json:"messages,omitempty" gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE"`
}

type Message struct {
	ID             string  `json:"id" gorm:"primaryKey;type:text"`
	ConversationID string  `json:"conversation_id" gorm:"type:text;not null;index"`
	Role           string  `json:"role" gorm:"type:text;not null"` // user, assistant
	Content        *string `json:"content,omitempty" gorm:"type:text"`
	Reasoning      *string `json:"reasoning,omitempty" gorm:"type:text"`          // AI thinking/reasoning content
	ToolCalls      *string `json:"tool_calls,omitempty" gorm:"type:text"`         // JSON array of tool calls
	ToolCallID     *string `json:"tool_call_id,omitempty" gorm:"type:text"`       // for tool result messages
	Seq            int     `json:"seq" gorm:"not null"`                           // ordering within conversation
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// --- Core: Nodes ---

type Node struct {
	ID            string    `json:"id" gorm:"primaryKey;type:text"`
	Name          string    `json:"name" gorm:"type:text;not null"`          // hostname reported by agent
	Alias         *string   `json:"alias,omitempty" gorm:"type:text"`        // user-defined alias
	IP            string    `json:"ip" gorm:"type:text"`
	OS            string    `json:"os" gorm:"type:text"`
	Kernel        string    `json:"kernel" gorm:"type:text"`
	AgentVersion  string    `json:"agent_version" gorm:"type:text"`
	CPUCores      int       `json:"cpu_cores" gorm:"default:0"`
	MemoryTotalMB int       `json:"memory_total_mb" gorm:"default:0"`
	DiskTotalGB   int       `json:"disk_total_gb" gorm:"default:0"`
	Status        string    `json:"status" gorm:"type:text;not null;default:'offline'"` // online, offline
	AgentSecret   string    `json:"-" gorm:"type:text"`                                 // never expose to frontend
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"autoUpdateTime"`

	// NodeProbe extension fields
	CanvasX *float64 `json:"canvas_x,omitempty" gorm:"type:real"`
	CanvasY *float64 `json:"canvas_y,omitempty" gorm:"type:real"`
	Role    *string  `json:"role,omitempty" gorm:"type:text"` // entry, relay, landing
}

// --- Core: Registration Tokens ---

// RegistrationToken is a reusable token for agent registration.
// One token can register multiple nodes before it expires.
type RegistrationToken struct {
	ID          string    `json:"id" gorm:"primaryKey;type:text"`           // token value itself
	AliasPrefix *string   `json:"alias_prefix,omitempty" gorm:"type:text"` // optional alias prefix for nodes registered with this token
	ExpiresAt   time.Time `json:"expires_at" gorm:"not null"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// --- Core: Audit Logs ---

type AuditLog struct {
	ID          uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	NodeID      string    `json:"node_id" gorm:"type:text;not null;index"`
	NodeName    string    `json:"node_name" gorm:"type:text"`
	Command     string    `json:"command" gorm:"type:text;not null"`
	ExitCode    *int      `json:"exit_code,omitempty"`
	Stdout      *string   `json:"stdout,omitempty" gorm:"type:text"`
	Stderr      *string   `json:"stderr,omitempty" gorm:"type:text"`
	DurationMS  *int64    `json:"duration_ms,omitempty"`
	Confirmed   bool      `json:"confirmed" gorm:"default:false"`             // required 2FA confirmation
	Source      string    `json:"source" gorm:"type:text;not null;default:'webui'"` // webui, api, mcp
	APIKeyID    *string   `json:"api_key_id,omitempty" gorm:"type:text"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime;index"`
}

// --- Core: Settings (key-value) ---

type Setting struct {
	Key       string    `json:"key" gorm:"primaryKey;type:text"`
	Value     string    `json:"value" gorm:"type:text;not null"` // JSON-encoded value
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// --- Core: External API Keys ---

type APIKey struct {
	ID          string    `json:"id" gorm:"primaryKey;type:text"`
	Name        string    `json:"name" gorm:"type:text;not null"`
	KeyHash     string    `json:"-" gorm:"type:text;not null;uniqueIndex"` // hashed key, never expose
	KeyPrefix   string    `json:"key_prefix" gorm:"type:text"`            // first 8 chars for display
	Permission  string    `json:"permission" gorm:"type:text;not null"`   // readonly, standard, admin
	Status      string    `json:"status" gorm:"type:text;not null;default:'active'"` // active, revoked
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// --- NodeProbe: Links ---

type ProbeLink struct {
	ID       string `json:"id" gorm:"primaryKey;type:text"` // format: {source_id}->{target_id}
	SourceID string `json:"source_id" gorm:"type:text;not null;index"`
	TargetID string `json:"target_id" gorm:"type:text;not null;index"`

	Source *Node `json:"source,omitempty" gorm:"foreignKey:SourceID;references:ID"`
	Target *Node `json:"target,omitempty" gorm:"foreignKey:TargetID;references:ID"`
}

// --- NodeProbe: Metrics ---

type ProbeMetric struct {
	ID             uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	LinkID         string    `json:"link_id" gorm:"type:text;not null;index:idx_probe_metrics_link_ts,priority:1"`
	Timestamp      time.Time `json:"timestamp" gorm:"not null;index:idx_probe_metrics_link_ts,priority:2,sort:desc"`
	LatencyMin     *float64  `json:"latency_min,omitempty" gorm:"type:real"`
	LatencyAvg     *float64  `json:"latency_avg,omitempty" gorm:"type:real"`
	LatencyMax     *float64  `json:"latency_max,omitempty" gorm:"type:real"`
	PacketLoss     *float64  `json:"packet_loss,omitempty" gorm:"type:real"`
	TCPConnectTime *float64  `json:"tcp_connect_time,omitempty" gorm:"type:real"`
	BandwidthMbps  *float64  `json:"bandwidth_mbps,omitempty" gorm:"type:real"`
}

// --- NodeProbe: Alerts ---

type ProbeAlert struct {
	ID         uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	LinkID     string     `json:"link_id" gorm:"type:text;not null;index:idx_probe_alerts_link_ts,priority:1"`
	Type       string     `json:"type" gorm:"type:text;not null"` // latency, packet_loss, tcp, bandwidth, offline
	Message    string     `json:"message" gorm:"type:text"`
	TriggeredAt time.Time `json:"triggered_at" gorm:"not null;index:idx_probe_alerts_link_ts,priority:2,sort:desc"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
