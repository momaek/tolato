// ============================================================================
// REST API Request/Response Types — Frontend (Vue 3 + TypeScript)
// ============================================================================

// --- Common ---

export interface PaginationQuery {
  page?: number        // 1-based, default 1
  page_size?: number   // default 20, max 100
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

export interface ErrorResponse {
  error: string
  message: string
}

// --- Auth ---

export interface LoginRequest {
  username: string
  password: string
}

export interface LoginResponse {
  token: string
  expires_at: string // ISO 8601
}

// ============================================================================
// Conversations
// ============================================================================

export interface CreateConversationRequest {
  title?: string
  model?: string
  default_node_id?: string
}

export interface ConversationSummary {
  id: string
  title: string
  model: string
  created_at: string
  updated_at: string
}

export interface ConversationDetail {
  id: string
  title: string
  model: string
  default_node_id?: string
  messages: MessageItem[]
  created_at: string
  updated_at: string
}

export interface MessageItem {
  id: string
  role: 'user' | 'assistant' | 'tool'
  content?: string
  reasoning?: string
  tool_calls?: ToolCallItem[]
  tool_call_id?: string
  created_at: string
  /**
   * Streaming-only: chronological interleaving of content chunks and tool calls.
   * When present, the renderer uses this in preference to `content` + `tool_calls`
   * (which lose ordering when a turn spans multiple LLM rounds).
   */
  segments?: MessageSegment[]
}

export type MessageSegment =
  | { type: 'content'; text: string }
  | { type: 'tool_call'; toolCall: ToolCallItem }

export interface ToolCallItem {
  id: string
  tool: 'list_nodes' | 'get_node_info' | 'edit_node_info' | 'execute_command'
  args: Record<string, unknown>
  result?: ToolResultItem
}

export interface ToolResultItem {
  exit_code?: number
  stdout?: string
  stderr?: string
  duration_ms?: number
  data?: unknown // for non-command tool results
}

export interface UpdateConversationRequest {
  title?: string
  model?: string
  default_node_id?: string
}

// ============================================================================
// Nodes
// ============================================================================

export interface CreateNodeRequest {
  alias?: string
}

export interface CreateNodeResponse {
  id: string
  token: string
  install_cmd: string
  token_expiry: string
}

export interface NodeListItem {
  id: string
  name: string
  alias?: string
  ip: string
  country_code?: string  // ISO 3166-1 alpha-2 (e.g. "JP")
  city?: string          // English city name
  asn?: string           // autonomous system org name
  status: 'online' | 'offline'
  os: string
  cpu_cores?: number
  memory_total_mb?: number
  disk_total_gb?: number
  cpu?: number
  memory?: number
  disk?: number
  extra?: NodeExtra
  last_heartbeat?: string
}

// NodeExtra is a free-form bag of metadata. Conventional keys are surfaced as
// optional typed fields, but any key is allowed (the AI assistant may add more).
export interface NodeExtra {
  provider?: string
  plan?: string
  expires_at?: string         // ISO date (YYYY-MM-DD or RFC3339)
  monthly_cost?: number
  currency?: string
  billing_cycle?: string      // e.g. "monthly", "yearly"
  renewal_url?: string
  account_email?: string
  notes?: string
  [key: string]: unknown
}

export interface NodeDetail {
  id: string
  name: string
  alias?: string
  ip: string
  country_code?: string
  city?: string
  asn?: string
  os: string
  kernel: string
  agent_version: string
  cpu_cores: number
  memory_total_mb: number
  disk_total_gb: number
  status: 'online' | 'offline'
  extra?: NodeExtra
  last_heartbeat?: string
  created_at: string
  metrics?: NodeMetrics
}

export interface NodeMetrics {
  cpu: number
  memory: number
  disk: number
  uptime: number
  load_avg: [number, number, number]
}

export interface UpdateNodeRequest {
  alias?: string
  extra?: Record<string, unknown>  // partial-merged on the server
}

export interface NodeCommandItem {
  id: number
  command: string
  exit_code?: number
  duration_ms?: number
  created_at: string
}

// ============================================================================
// Settings
// ============================================================================

export interface LLMSettings {
  api_base_url: string
  api_key: string          // masked on GET: "sk-****abcd"
  default_model: string
  max_rounds: number
  temperature: number
}

export interface VerifyLLMResponse {
  success: boolean
  models?: string[]
  error?: string
}

export interface SecuritySettings {
  confirm_enabled: boolean
  sensitive_keywords: string[]
  command_blacklist: string[]
}

export interface AgentSettings {
  heartbeat_interval: number
  command_timeout: number
  output_max_lines: number
}

export interface ChatSettings {
  context_rounds: number
  output_truncate_lines: number
  custom_system_prompt?: string
}

export interface WebFetchSettings {
  mode: 'jina' | 'local'
  jina_api_key: string         // masked on GET: "jina_****abcd"
  timeout_sec: number
  max_kb: number
}

export interface VerifyWebFetchResponse {
  success: boolean
  error?: string
  sample?: string
}

// ============================================================================
// Audit Logs
// ============================================================================

export interface AuditLogQuery extends PaginationQuery {
  node_id?: string
  keyword?: string
  from?: string  // RFC3339
  to?: string    // RFC3339
}

export interface AuditLogItem {
  id: number
  node_id: string
  node_name: string
  command: string
  exit_code?: number
  stdout?: string
  stderr?: string
  duration_ms?: number
  confirmed: boolean
  source: 'webui' | 'api' | 'mcp'
  created_at: string
}

// ============================================================================
// External API (v1)
// ============================================================================

export interface ExecuteCommandRequest {
  command: string
  timeout?: number
  confirm?: boolean
  stream?: boolean
}

export interface ExecuteCommandResponse {
  id: string
  node_id: string
  command: string
  exit_code: number
  stdout: string
  stderr: string
  duration_ms: number
}

export interface SensitiveOperationError {
  error: 'sensitive_operation'
  message: string
  matched_rule: string
}

// ============================================================================
// API Key Management
// ============================================================================

export type APIKeyPermission = 'readonly' | 'standard' | 'admin'

export interface CreateAPIKeyRequest {
  name: string
  permission: APIKeyPermission
}

export interface CreateAPIKeyResponse {
  id: string
  name: string
  key: string            // full key, only shown once
  key_prefix: string
  permission: APIKeyPermission
  created_at: string
}

export interface APIKeyListItem {
  id: string
  name: string
  key_prefix: string
  permission: APIKeyPermission
  status: 'active' | 'revoked'
  last_used_at?: string
  created_at: string
}

// ============================================================================
// NodeProbe
// ============================================================================

export interface ProbeNodeItem {
  id: string
  name: string
  role?: 'entry' | 'relay' | 'landing'
  status: 'online' | 'offline'
  canvas_x?: number
  canvas_y?: number
  last_seen?: string
}

export interface UpdateProbeNodeRequest {
  role?: 'entry' | 'relay' | 'landing'
  canvas_x?: number
  canvas_y?: number
}

export interface CreateProbeLinkRequest {
  source_id: string
  target_id: string
}

export type ProbeLinkStatus = 'normal' | 'warning' | 'alert' | 'no_data'

export interface ProbeLinkItem {
  id: string
  source_id: string
  source_name: string
  target_id: string
  target_name: string
  status: ProbeLinkStatus
  latest?: ProbeMetricSnapshot
}

export interface ProbeMetricSnapshot {
  latency_avg?: number
  packet_loss?: number
  tcp_connect_time?: number
  bandwidth_mbps?: number
  timestamp: string
}

export interface ProbeMetricQuery {
  from?: string  // RFC3339
  to?: string    // RFC3339
}

export interface ProbeMetricItem {
  timestamp: string
  latency_min?: number
  latency_avg?: number
  latency_max?: number
  packet_loss?: number
  tcp_connect_time?: number
  bandwidth_mbps?: number
}

export interface ProbeAlertQuery extends PaginationQuery {
  link_id?: string
  type?: ProbeAlertType
  status?: 'all' | 'unresolved' | 'resolved'
}

export type ProbeAlertType = 'latency' | 'packet_loss' | 'tcp' | 'bandwidth' | 'offline'

export interface ProbeAlertItem {
  id: number
  link_id: string
  link_name: string
  type: ProbeAlertType
  message: string
  status: 'unresolved' | 'resolved'
  duration?: string
  triggered_at: string
  resolved_at?: string
}
