package model

import "time"

// ============================================================================
// REST API Request/Response Types
// ============================================================================

// --- Common ---

type PaginationQuery struct {
	Page     int `form:"page" json:"page"`         // 1-based, default 1
	PageSize int `form:"page_size" json:"page_size"` // default 20, max 100
}

type PaginatedResponse struct {
	Items      any `json:"items"`
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

type ConversationDetail struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Model         string        `json:"model"`
	DefaultNodeID *string       `json:"default_node_id,omitempty"`
	Messages      []MessageItem `json:"messages"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

type MessageItem struct {
	ID         string         `json:"id"`
	Role       string         `json:"role"` // user, assistant, tool
	Content    *string        `json:"content,omitempty"`
	Reasoning  *string        `json:"reasoning,omitempty"`
	ToolCalls  []ToolCallItem `json:"tool_calls,omitempty"`
	ToolCallID *string        `json:"tool_call_id,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type ToolCallItem struct {
	ID     string         `json:"id"`
	Tool   string         `json:"tool"`     // list_nodes, get_node_info, execute_command
	Args   map[string]any `json:"args"`
	Result *ToolResultItem `json:"result,omitempty"`
}

type ToolResultItem struct {
	ExitCode   *int    `json:"exit_code,omitempty"`
	Stdout     *string `json:"stdout,omitempty"`
	Stderr     *string `json:"stderr,omitempty"`
	DurationMS *int64  `json:"duration_ms,omitempty"`
	Data       any     `json:"data,omitempty"` // for non-command tool results
}

// PUT /api/conversations/:id
type UpdateConversationRequest struct {
	Title         *string `json:"title,omitempty"`
	Model         *string `json:"model,omitempty"`
	DefaultNodeID *string `json:"default_node_id,omitempty"`
}

// --- Nodes ---

// POST /api/nodes — generates a reusable registration token
type CreateNodeRequest struct {
	Alias *string `json:"alias,omitempty"` // optional alias prefix for nodes registered with this token
}

type CreateNodeResponse struct {
	Token       string `json:"token"`        // reusable registration token (valid for multiple agents)
	InstallCmd  string `json:"install_cmd"`   // full install command for copy-paste
	TokenExpiry string `json:"token_expiry"` // e.g., "24h"
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

// --- Settings ---

// GET/PUT /api/settings/llm
type LLMSettings struct {
	APIBaseURL   string  `json:"api_base_url"`
	APIKey       string  `json:"api_key"`
	DefaultModel string  `json:"default_model"`
	MaxRounds    int     `json:"max_rounds"`
	Temperature  float64 `json:"temperature"`
}

// GET/PUT /api/settings/security
type SecuritySettings struct {
	ConfirmEnabled    bool     `json:"confirm_enabled"`
	SensitiveKeywords []string `json:"sensitive_keywords"`
	CommandBlacklist  []string `json:"command_blacklist"`
}

// GET/PUT /api/settings/agent
type AgentSettings struct {
	HeartbeatInterval int `json:"heartbeat_interval"` // seconds, default 30
	CommandTimeout    int `json:"command_timeout"`     // seconds, default 60
	OutputMaxLines    int `json:"output_max_lines"`    // default 10000
}

// GET/PUT /api/settings/chat
type ChatSettings struct {
	ContextRounds       int     `json:"context_rounds"`
	OutputTruncateLines int     `json:"output_truncate_lines"`
	CustomSystemPrompt  *string `json:"custom_system_prompt,omitempty"`
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
