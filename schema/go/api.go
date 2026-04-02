package schema

import "time"

// ============================================================================
// REST API Request/Response Types
// ============================================================================

// --- Common ---

type PaginationQuery struct {
	Page     int `form:"page" json:"page"`         // 1-based, default 1
	PageSize int `form:"page_size" json:"page_size"` // default 20, max 100
}

type PaginatedResponse[T any] struct {
	Items      []T `json:"items"`
	Total      int `json:"total"`
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalPages int `json:"total_pages"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// --- Auth ---

// POST /api/auth/login
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// --- Conversations ---

// POST /api/conversations
type CreateConversationRequest struct {
	Title         string  `json:"title"`
	Model         string  `json:"model"`
	DefaultNodeID *string `json:"default_node_id,omitempty"`
}

type ConversationSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GET /api/conversations/:id
type ConversationDetail struct {
	ID            string           `json:"id"`
	Title         string           `json:"title"`
	Model         string           `json:"model"`
	DefaultNodeID *string          `json:"default_node_id,omitempty"`
	Messages      []MessageItem    `json:"messages"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

type MessageItem struct {
	ID         string          `json:"id"`
	Role       string          `json:"role"` // user, assistant, tool
	Content    *string         `json:"content,omitempty"`
	Reasoning  *string         `json:"reasoning,omitempty"`
	ToolCalls  []ToolCallItem  `json:"tool_calls,omitempty"`
	ToolCallID *string         `json:"tool_call_id,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

type ToolCallItem struct {
	ID       string            `json:"id"`
	Tool     string            `json:"tool"`     // list_nodes, get_node_info, execute_command
	Args     map[string]any    `json:"args"`
	Result   *ToolResultItem   `json:"result,omitempty"`
}

type ToolResultItem struct {
	ExitCode   *int    `json:"exit_code,omitempty"`
	Stdout     *string `json:"stdout,omitempty"`
	Stderr     *string `json:"stderr,omitempty"`
	DurationMS *int64  `json:"duration_ms,omitempty"`
	Data       any     `json:"data,omitempty"` // for non-command tool results (e.g., list_nodes)
}

// PUT /api/conversations/:id
type UpdateConversationRequest struct {
	Title         *string `json:"title,omitempty"`
	Model         *string `json:"model,omitempty"`
	DefaultNodeID *string `json:"default_node_id,omitempty"`
}

// --- Nodes ---

// POST /api/nodes
type CreateNodeRequest struct {
	Alias *string `json:"alias,omitempty"`
}

type CreateNodeResponse struct {
	ID           string `json:"id"`
	Token        string `json:"token"`                   // one-time registration token
	InstallCmd   string `json:"install_cmd"`             // full install command for copy-paste
	TokenExpiry  string `json:"token_expiry"`            // e.g., "24h"
}

// GET /api/nodes
type NodeListItem struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Alias         *string    `json:"alias,omitempty"`
	IP            string     `json:"ip"`
	Status        string     `json:"status"` // online, offline
	OS            string     `json:"os"`
	CPU           *float64   `json:"cpu,omitempty"`    // current CPU usage %
	Memory        *float64   `json:"memory,omitempty"` // current memory usage %
	Disk          *float64   `json:"disk,omitempty"`   // current disk usage %
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
}

// GET /api/nodes/:id
type NodeDetail struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Alias         *string    `json:"alias,omitempty"`
	IP            string     `json:"ip"`
	OS            string     `json:"os"`
	Kernel        string     `json:"kernel"`
	AgentVersion  string     `json:"agent_version"`
	CPUCores      int        `json:"cpu_cores"`
	MemoryTotalMB int        `json:"memory_total_mb"`
	DiskTotalGB   int        `json:"disk_total_gb"`
	Status        string     `json:"status"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`

	// Real-time metrics from heartbeat cache
	Metrics *NodeMetrics `json:"metrics,omitempty"`
}

type NodeMetrics struct {
	CPU     float64   `json:"cpu"`      // CPU usage %
	Memory  float64   `json:"memory"`   // memory usage %
	Disk    float64   `json:"disk"`     // disk usage %
	Uptime  int64     `json:"uptime"`   // seconds
	LoadAvg []float64 `json:"load_avg"` // [1min, 5min, 15min]
}

// PUT /api/nodes/:id
type UpdateNodeRequest struct {
	Alias *string `json:"alias,omitempty"`
}

// GET /api/nodes/:id/commands
type NodeCommandItem struct {
	ID         uint      `json:"id"`
	Command    string    `json:"command"`
	ExitCode   *int      `json:"exit_code,omitempty"`
	DurationMS *int64    `json:"duration_ms,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// --- Settings ---

// GET/PUT /api/settings/llm
type LLMSettings struct {
	APIBaseURL   string  `json:"api_base_url"`   // e.g., https://api.openai.com
	APIKey       string  `json:"api_key"`         // masked on GET: sk-****abcd; full value on PUT
	DefaultModel string  `json:"default_model"`
	MaxRounds    int     `json:"max_rounds"`      // tool call loop limit, default 20
	Temperature  float64 `json:"temperature"`     // 0.0 - 2.0
}

// POST /api/settings/llm/verify
type VerifyLLMResponse struct {
	Success bool     `json:"success"`
	Models  []string `json:"models,omitempty"` // available models from /v1/models
	Error   *string  `json:"error,omitempty"`
}

// GET/PUT /api/settings/security
type SecuritySettings struct {
	ConfirmEnabled    bool     `json:"confirm_enabled"`     // global 2FA toggle
	SensitiveKeywords []string `json:"sensitive_keywords"`  // trigger words for confirmation
	CommandBlacklist  []string `json:"command_blacklist"`   // directly rejected commands
}

// GET/PUT /api/settings/agent
type AgentSettings struct {
	HeartbeatInterval int `json:"heartbeat_interval"` // seconds, default 30
	CommandTimeout    int `json:"command_timeout"`     // seconds, default 60
	OutputMaxLines    int `json:"output_max_lines"`    // default 10000
}

// GET/PUT /api/settings/chat
type ChatSettings struct {
	ContextRounds      int     `json:"context_rounds"`       // history retention, default 20
	OutputTruncateLines int    `json:"output_truncate_lines"` // truncate output, default 100
	CustomSystemPrompt *string `json:"custom_system_prompt,omitempty"`
}

// --- Audit Logs ---

// GET /api/audit-logs?node_id=&keyword=&from=&to=&page=&page_size=
type AuditLogQuery struct {
	PaginationQuery
	NodeID  *string `form:"node_id"`
	Keyword *string `form:"keyword"`
	From    *string `form:"from"` // RFC3339
	To      *string `form:"to"`   // RFC3339
}

type AuditLogItem struct {
	ID         uint      `json:"id"`
	NodeID     string    `json:"node_id"`
	NodeName   string    `json:"node_name"`
	Command    string    `json:"command"`
	ExitCode   *int      `json:"exit_code,omitempty"`
	Stdout     *string   `json:"stdout,omitempty"`
	Stderr     *string   `json:"stderr,omitempty"`
	DurationMS *int64    `json:"duration_ms,omitempty"`
	Confirmed  bool      `json:"confirmed"`
	Source     string    `json:"source"` // webui, api, mcp
	CreatedAt  time.Time `json:"created_at"`
}

// --- External API (v1) ---

// POST /api/v1/nodes/:id/execute
type ExecuteCommandRequest struct {
	Command string `json:"command" binding:"required"`
	Timeout int    `json:"timeout"` // seconds, default 60
	Confirm bool   `json:"confirm"` // explicit confirmation for sensitive ops
	Stream  bool   `json:"stream"`  // use SSE for streaming output
}

type ExecuteCommandResponse struct {
	ID         string  `json:"id"`
	NodeID     string  `json:"node_id"`
	Command    string  `json:"command"`
	ExitCode   int     `json:"exit_code"`
	Stdout     string  `json:"stdout"`
	Stderr     string  `json:"stderr"`
	DurationMS int64   `json:"duration_ms"`
}

// 403 for sensitive operations without confirm
type SensitiveOperationError struct {
	Error       string `json:"error"`        // "sensitive_operation"
	Message     string `json:"message"`
	MatchedRule string `json:"matched_rule"`
}

// --- API Key Management ---

// POST /api/settings/api-keys
type CreateAPIKeyRequest struct {
	Name       string `json:"name" binding:"required"`
	Permission string `json:"permission" binding:"required"` // readonly, standard, admin
}

type CreateAPIKeyResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Key        string    `json:"key"`        // full key, only shown once
	KeyPrefix  string    `json:"key_prefix"`
	Permission string    `json:"permission"`
	CreatedAt  time.Time `json:"created_at"`
}

// GET /api/settings/api-keys
type APIKeyListItem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	Permission string     `json:"permission"`
	Status     string     `json:"status"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// --- NodeProbe API ---

// POST /api/v1/probe/report (from agent)
type ProbeReportRequest struct {
	NodeID    string              `json:"node_id" binding:"required"`
	Timestamp time.Time           `json:"timestamp" binding:"required"`
	Metrics   []ProbeMetricReport `json:"metrics" binding:"required"`
}

type ProbeMetricReport struct {
	TargetID       string   `json:"target_id" binding:"required"`
	LatencyMin     *float64 `json:"latency_min,omitempty"`
	LatencyAvg     *float64 `json:"latency_avg,omitempty"`
	LatencyMax     *float64 `json:"latency_max,omitempty"`
	PacketLoss     *float64 `json:"packet_loss,omitempty"`
	TCPConnectTime *float64 `json:"tcp_connect_time,omitempty"`
	BandwidthMbps  *float64 `json:"bandwidth_mbps,omitempty"`
}

// GET /api/v1/probe/nodes
type ProbeNodeItem struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Role     *string  `json:"role,omitempty"` // entry, relay, landing
	Status   string   `json:"status"`         // online, offline
	CanvasX  *float64 `json:"canvas_x,omitempty"`
	CanvasY  *float64 `json:"canvas_y,omitempty"`
	LastSeen *time.Time `json:"last_seen,omitempty"`
}

// PUT /api/v1/probe/nodes/:id
type UpdateProbeNodeRequest struct {
	Role    *string  `json:"role,omitempty"`
	CanvasX *float64 `json:"canvas_x,omitempty"`
	CanvasY *float64 `json:"canvas_y,omitempty"`
}

// POST /api/v1/probe/links
type CreateProbeLinkRequest struct {
	SourceID string `json:"source_id" binding:"required"`
	TargetID string `json:"target_id" binding:"required"`
}

// DELETE /api/v1/probe/links/:id

// GET /api/v1/probe/links
type ProbeLinkItem struct {
	ID         string          `json:"id"`
	SourceID   string          `json:"source_id"`
	SourceName string          `json:"source_name"`
	TargetID   string          `json:"target_id"`
	TargetName string          `json:"target_name"`
	Status     string          `json:"status"` // normal, warning, alert, no_data
	Latest     *ProbeMetricSnapshot `json:"latest,omitempty"`
}

type ProbeMetricSnapshot struct {
	LatencyAvg     *float64  `json:"latency_avg,omitempty"`
	PacketLoss     *float64  `json:"packet_loss,omitempty"`
	TCPConnectTime *float64  `json:"tcp_connect_time,omitempty"`
	BandwidthMbps  *float64  `json:"bandwidth_mbps,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
}

// GET /api/v1/probe/links/:id/metrics?from=&to=
type ProbeMetricQuery struct {
	From *string `form:"from"` // RFC3339
	To   *string `form:"to"`   // RFC3339
}

type ProbeMetricItem struct {
	Timestamp      time.Time `json:"timestamp"`
	LatencyMin     *float64  `json:"latency_min,omitempty"`
	LatencyAvg     *float64  `json:"latency_avg,omitempty"`
	LatencyMax     *float64  `json:"latency_max,omitempty"`
	PacketLoss     *float64  `json:"packet_loss,omitempty"`
	TCPConnectTime *float64  `json:"tcp_connect_time,omitempty"`
	BandwidthMbps  *float64  `json:"bandwidth_mbps,omitempty"`
}

// GET /api/v1/probe/alerts?link_id=&type=&status=&page=&page_size=
type ProbeAlertQuery struct {
	PaginationQuery
	LinkID *string `form:"link_id"`
	Type   *string `form:"type"`   // latency, packet_loss, tcp, bandwidth, offline
	Status *string `form:"status"` // all, unresolved, resolved
}

type ProbeAlertItem struct {
	ID          uint       `json:"id"`
	LinkID      string     `json:"link_id"`
	LinkName    string     `json:"link_name"` // "source → target" display
	Type        string     `json:"type"`
	Message     string     `json:"message"`
	Status      string     `json:"status"` // unresolved, resolved
	Duration    *string    `json:"duration,omitempty"` // human-readable duration
	TriggeredAt time.Time  `json:"triggered_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}
