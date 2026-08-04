package model

import "time"

// ============================================================================
// REST API Request/Response Types
// ============================================================================

// --- Common ---

type PaginationQuery struct {
	Page     int `form:"page" json:"page"`           // 1-based, default 1
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
	User      UserItem  `json:"user"`
}

// PUT /api/auth/password — self-service change, proves the current password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// --- OIDC / single sign-on ---
//
// Stored as one JSON blob under the setting key "oidc.config" and edited from
// the admin UI, so enabling SSO needs no config-file change or restart.
//
// ClientSecret is masked on GET (e.g. "abcd****wxyz"). On PUT, a value still
// containing the "****" sentinel means "unchanged" and the stored secret is
// kept — same pattern as the LLM and web-fetch API keys.
type OIDCSettings struct {
	Enabled bool `json:"enabled"`
	// Issuer is the IdP base URL; its /.well-known/openid-configuration is
	// fetched for endpoint discovery.
	Issuer       string `json:"issuer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"` // masked on GET
	// Scopes requested at authorization. "openid" is always added.
	Scopes []string `json:"scopes,omitempty"`
	// AdminEmails lists verified email addresses provisioned as admins on first
	// sign-in. Everyone else becomes a member.
	AdminEmails []string `json:"admin_emails,omitempty"`
	// AllowSignup gates whether an unrecognized subject may create an account.
	// Off means only users who already exist locally can sign in via SSO.
	AllowSignup bool `json:"allow_signup"`
}

// GET /api/settings/oidc — adds the read-only callback URL for the admin to
// register with the IdP.
type OIDCSettingsResponse struct {
	OIDCSettings
	RedirectURL string `json:"redirect_url"`
}

// POST /api/settings/oidc/verify
type VerifyOIDCResponse struct {
	Success               bool   `json:"success"`
	Issuer                string `json:"issuer,omitempty"`
	AuthorizationEndpoint string `json:"authorization_endpoint,omitempty"`
	Error                 string `json:"error,omitempty"`
}

// GET /api/auth/oidc/status — unauthenticated; drives the login page's SSO
// button. Deliberately exposes nothing but whether SSO is on.
type OIDCStatusResponse struct {
	Enabled bool `json:"enabled"`
}

// --- Users (admin-managed) ---

type UserItem struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Email       string     `json:"email,omitempty"`
	Role        string     `json:"role"`        // admin, member
	Status      string     `json:"status"`      // active, disabled
	AuthSource  string     `json:"auth_source"` // local, oidc
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// POST /api/users
type CreateUserRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Role        string `json:"role"` // defaults to member
}

// PUT /api/users/:id — every field optional; omitted fields are left alone.
type UpdateUserRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Email       *string `json:"email,omitempty"`
	Role        *string `json:"role,omitempty"`
	Status      *string `json:"status,omitempty"`
	Password    *string `json:"password,omitempty"` // admin reset; no current-password check
}

// --- Groups and grants ---

type UserGroupItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	MemberIDs   []string  `json:"member_ids"`
	CreatedAt   time.Time `json:"created_at"`
}

type NodeGroupItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	MemberIDs   []string  `json:"member_ids"`
	CreatedAt   time.Time `json:"created_at"`
}

// POST/PUT for both group kinds. MemberIDs replaces the membership wholesale
// when present; omit it to leave membership untouched.
type GroupRequest struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	MemberIDs   *[]string `json:"member_ids,omitempty"`
}

// GrantItem carries resolved subject/object names so the UI can render a grant
// without cross-referencing three more endpoints.
type GrantItem struct {
	ID          string    `json:"id"`
	SubjectType string    `json:"subject_type"`
	SubjectID   string    `json:"subject_id"`
	SubjectName string    `json:"subject_name"`
	ObjectType  string    `json:"object_type"`
	ObjectID    string    `json:"object_id,omitempty"`
	ObjectName  string    `json:"object_name"`
	Level       string    `json:"level"`
	CreatedAt   time.Time `json:"created_at"`
}

// POST /api/grants — granting the same subject on the same object again edits
// the level rather than creating a duplicate.
type CreateGrantRequest struct {
	SubjectType string `json:"subject_type" binding:"required"` // user, user_group
	SubjectID   string `json:"subject_id" binding:"required"`
	ObjectType  string `json:"object_type" binding:"required"` // node, node_group, all
	ObjectID    string `json:"object_id"`                      // empty when object_type is "all"
	Level       string `json:"level" binding:"required"`       // viewer, operator, manager
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
	ID     string          `json:"id"`
	Tool   string          `json:"tool"` // list_nodes, get_node_info, execute_command
	Args   map[string]any  `json:"args"`
	Result *ToolResultItem `json:"result,omitempty"`
}

type ToolResultItem struct {
	ExitCode   *int    `json:"exit_code,omitempty"`
	Stdout     *string `json:"stdout,omitempty"`
	Stderr     *string `json:"stderr,omitempty"`
	DurationMS *int64  `json:"duration_ms,omitempty"`
	Data       any     `json:"data,omitempty"` // for non-command tool results

	// Preview metadata, set only on the history GET path (buildMessageItems).
	// When Truncated is true, Stdout/Stderr hold a head preview and the full
	// output can be fetched lazily via GET .../tool-calls/:toolCallId/output.
	// StdoutLines/StderrLines report the full line counts so the UI can show
	// "view all N lines". Live (WebSocket) results never set these — they carry
	// the full output inline.
	Truncated   bool `json:"truncated,omitempty"`
	StdoutLines *int `json:"stdout_lines,omitempty"`
	StderrLines *int `json:"stderr_lines,omitempty"`
}

// ToolCallOutputResponse is the full, untruncated output for a single tool call.
// GET /api/conversations/:id/tool-calls/:toolCallId/output
type ToolCallOutputResponse struct {
	Stdout *string `json:"stdout,omitempty"`
	Stderr *string `json:"stderr,omitempty"`
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
	// NodeGroupID enrols every node registered with this token into that group,
	// so the grants already on the group apply the moment the agent connects.
	NodeGroupID *string `json:"node_group_id,omitempty"`
}

type CreateNodeResponse struct {
	Token       string `json:"token"`        // reusable registration token (valid for multiple agents)
	InstallCmd  string `json:"install_cmd"`  // full install command for copy-paste
	TokenExpiry string `json:"token_expiry"` // e.g., "24h"
}

// GET /api/nodes
type NodeListItem struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Alias         *string    `json:"alias,omitempty"`
	IP            string     `json:"ip"`
	CountryCode   string     `json:"country_code,omitempty"`
	City          string     `json:"city,omitempty"`
	ASN           string     `json:"asn,omitempty"`
	Status        string     `json:"status"` // online, offline
	OS            string     `json:"os"`
	CPUCores      int        `json:"cpu_cores,omitempty"`       // hardware spec
	MemoryTotalMB int        `json:"memory_total_mb,omitempty"` // hardware spec
	DiskTotalGB   int        `json:"disk_total_gb,omitempty"`   // hardware spec
	CPU           *float64   `json:"cpu,omitempty"`             // current CPU usage %
	Memory        *float64   `json:"memory,omitempty"`          // current memory usage %
	Disk          *float64   `json:"disk,omitempty"`            // current disk usage %
	Extra         JSONMap    `json:"extra,omitempty"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	Groups        []string   `json:"groups,omitempty"`   // node group names
	MyLevel       string     `json:"my_level,omitempty"` // caller's effective level
}

// GET /api/nodes/:id
type NodeDetail struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Alias         *string    `json:"alias,omitempty"`
	IP            string     `json:"ip"`
	CountryCode   string     `json:"country_code,omitempty"`
	City          string     `json:"city,omitempty"`
	ASN           string     `json:"asn,omitempty"`
	OS            string     `json:"os"`
	Kernel        string     `json:"kernel"`
	AgentVersion  string     `json:"agent_version"`
	CPUCores      int        `json:"cpu_cores"`
	MemoryTotalMB int        `json:"memory_total_mb"`
	DiskTotalGB   int        `json:"disk_total_gb"`
	Status        string     `json:"status"`
	Extra         JSONMap    `json:"extra,omitempty"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	Groups        []string   `json:"groups,omitempty"`   // node group names
	MyLevel       string     `json:"my_level,omitempty"` // caller's effective level

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
	// Extra is partial-merged into the existing extra map: keys you supply
	// overwrite, keys you omit are kept, and explicit null values delete that key.
	Extra map[string]any `json:"extra,omitempty"`
}

// --- Settings ---

// GET/PUT /api/settings/llm
type LLMSettings struct {
	APIBaseURL          string  `json:"api_base_url"`
	APIKey              string  `json:"api_key"`
	DefaultModel        string  `json:"default_model"`
	MaxRounds           int     `json:"max_rounds"`
	Temperature         float64 `json:"temperature"`
	InterleavedThinking bool    `json:"interleaved_thinking"`
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
	CommandTimeout    int `json:"command_timeout"`    // seconds, default 60
	OutputMaxLines    int `json:"output_max_lines"`   // default 10000
}

// GET/PUT /api/settings/chat
type ChatSettings struct {
	ContextRounds       int     `json:"context_rounds"`
	OutputTruncateLines int     `json:"output_truncate_lines"`
	CustomSystemPrompt  *string `json:"custom_system_prompt,omitempty"`
}

// GET/PUT /api/settings/webfetch
//
// Mode: "jina" routes the web_fetch tool through https://r.jina.ai (Reader API).
// Local mode is reserved for a future direct-fetch implementation; for now the
// only supported value is "jina" and other values cause the tool to error.
//
// JinaAPIKey is masked on GET (e.g. "jina_****abcd"). On PUT, a value still
// containing the mask sentinel "****" is treated as "unchanged" and skipped,
// so saving the form without re-typing the key keeps the stored value intact.
type WebFetchSettings struct {
	Mode       string `json:"mode"`         // "jina" | "local" (local not yet implemented)
	JinaAPIKey string `json:"jina_api_key"` // masked on GET
	TimeoutSec int    `json:"timeout_sec"`
	MaxKB      int    `json:"max_kb"`
}

// --- Notifications (GET/PUT /api/settings/notify) ---
//
// Node offline/online notifications. The whole struct is persisted as a single
// JSON setting under the key "notify.config" (rather than one key per field)
// because token_sources/channels are variable-length lists.
//
// Sensitive values inside TokenSource.Params / Channel.Params (anything whose
// key contains "secret"/"token"/"password"/"key") are masked on GET and a
// submitted masked value is treated as "unchanged" on PUT — same pattern as the
// LLM/WebFetch API keys.
type NotifySettings struct {
	// OfflineThresholdSeconds: a node with no heartbeat for this long is marked
	// offline by the background monitor. <=0 disables the monitor (disconnect
	// detection still works). Default 90.
	OfflineThresholdSeconds int `json:"offline_threshold_seconds"`
	// RecoverNotify toggles offline→online recovery notifications. Default true.
	RecoverNotify bool `json:"recover_notify"`
	// StartupGraceSeconds suppresses offline notifications for this long after
	// server start, so a restart (where all nodes briefly look stale) doesn't
	// fire a storm. Default 90.
	StartupGraceSeconds int                 `json:"startup_grace_seconds"`
	TokenSources        []NotifyTokenSource `json:"token_sources"`
	Channels            []NotifyChannel     `json:"channels"`
}

// NotifyTokenSource is an independently configured "exchange credentials for a
// bearer token" step (e.g. WeCom gettoken, Feishu tenant_access_token).
// Channels reference it by Name; the resolved token is cached and shared.
type NotifyTokenSource struct {
	Name         string             `json:"name"`
	Request      NotifyHTTPRequest  `json:"request"`
	Params       map[string]string  `json:"params,omitempty"` // template vars; sensitive values masked on GET
	Extract      NotifyTokenExtract `json:"extract"`
	InvalidateOn *NotifyJSONMatch   `json:"invalidate_on,omitempty"` // response match → token stale → refresh+retry
}

type NotifyHTTPRequest struct {
	Method  string            `json:"method,omitempty"` // default POST
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

type NotifyTokenExtract struct {
	TokenPath          string `json:"token_path"`             // dot-path into response JSON, e.g. "access_token"
	ExpiresPath        string `json:"expires_path,omitempty"` // dot-path to TTL seconds, e.g. "expires_in"
	TTLFallbackSeconds int    `json:"ttl_fallback_seconds,omitempty"`
}

// NotifyJSONMatch matches a value at JSONPath (dot-separated) against any of
// Values. Used for both success detection (success_check) and token-invalid
// detection (invalidate_on).
type NotifyJSONMatch struct {
	JSONPath string `json:"json_path"`
	Values   []any  `json:"values,omitempty"`
}

// NotifyChannel is one notification target. All channels share a single webhook
// engine; Preset fills in default url/body/success_check and the user only
// supplies Params. Preset "custom" uses Webhook verbatim. TokenSource is empty
// for single-step channels (Telegram/Discord/Slack) and set for two-step ones.
type NotifyChannel struct {
	Name        string             `json:"name"`
	Enabled     bool               `json:"enabled"`
	Preset      string             `json:"preset"` // telegram|discord|slack|wecom|feishu|custom
	TokenSource string             `json:"token_source,omitempty"`
	Params      map[string]string  `json:"params,omitempty"`  // template vars; sensitive values masked on GET
	Webhook     *NotifyWebhookSpec `json:"webhook,omitempty"` // overrides preset defaults; required for "custom"
}

type NotifyWebhookSpec struct {
	Method       string            `json:"method,omitempty"` // default POST
	URL          string            `json:"url,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyTemplate string            `json:"body_template,omitempty"`
	SuccessCheck *NotifyJSONMatch  `json:"success_check,omitempty"` // nil → success = HTTP 2xx
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
	Actor      string    `json:"actor,omitempty"` // username that ran it
	Command    string    `json:"command"`
	ExitCode   *int      `json:"exit_code,omitempty"`
	Stdout     *string   `json:"stdout,omitempty"`
	Stderr     *string   `json:"stderr,omitempty"`
	DurationMS *int64    `json:"duration_ms,omitempty"`
	Confirmed  bool      `json:"confirmed"`
	Source     string    `json:"source"` // webui, terminal, api, cli
	CreatedAt  time.Time `json:"created_at"`
}
