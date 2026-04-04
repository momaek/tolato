// ============================================================================
// WebSocket Message Types — Frontend (Vue 3 + TypeScript)
// ============================================================================

// --- Envelope ---

export interface WSMessage<T = unknown> {
  type: string
  payload?: T
}

// ============================================================================
// Server ↔ Frontend (Chat WebSocket: /ws/chat)
// ============================================================================

// --- Server → Frontend event types ---

export const WS_TYPE = {
  // Server → Frontend
  REASONING: 'reasoning',
  CONTENT: 'content',
  TOOL_CALL: 'tool_call',
  TOOL_RESULT: 'tool_result',
  CONFIRM_REQUEST: 'confirm_request',
  DONE: 'done',
  ERROR: 'error',

  // Frontend → Server
  USER_MESSAGE: 'user_message',
  CONFIRM_RESPONSE: 'confirm_response',
} as const

// --- Server → Frontend payloads ---

export interface WSReasoningEvent {
  delta: string
}

export interface WSContentEvent {
  delta: string
}

export interface WSToolCallEvent {
  id: string
  tool: 'list_nodes' | 'get_node_info' | 'execute_command'
  args: Record<string, unknown>
}

export interface WSToolResultEvent {
  id: string
  result: {
    exit_code?: number
    stdout?: string
    stderr?: string
    duration_ms?: number
    data?: unknown
  }
}

export interface WSConfirmRequestEvent {
  id: string
  tool: string
  args: Record<string, unknown>
}

// WSTypeEone has no payload
export type WSDoneEvent = Record<string, never>

export interface WSErrorEvent {
  message: string
}

// --- Frontend → Server payloads ---

export interface WSUserMessageEvent {
  content: string
  model?: string
  default_node_id?: string
}

export interface WSConfirmResponseEvent {
  id: string
  approved: boolean
}

// --- Union type for all server events ---

export type ServerWSEvent =
  | WSMessage<WSReasoningEvent> & { type: typeof WS_TYPE.REASONING }
  | WSMessage<WSContentEvent> & { type: typeof WS_TYPE.CONTENT }
  | WSMessage<WSToolCallEvent> & { type: typeof WS_TYPE.TOOL_CALL }
  | WSMessage<WSToolResultEvent> & { type: typeof WS_TYPE.TOOL_RESULT }
  | WSMessage<WSConfirmRequestEvent> & { type: typeof WS_TYPE.CONFIRM_REQUEST }
  | WSMessage<WSDoneEvent> & { type: typeof WS_TYPE.DONE }
  | WSMessage<WSErrorEvent> & { type: typeof WS_TYPE.ERROR }

// --- Union type for all client events ---

export type ClientWSEvent =
  | WSMessage<WSUserMessageEvent> & { type: typeof WS_TYPE.USER_MESSAGE }
  | WSMessage<WSConfirmResponseEvent> & { type: typeof WS_TYPE.CONFIRM_RESPONSE }

// ============================================================================
// Server ↔ Node Agent (Agent WebSocket: /ws/agent)
// These types are for reference only — the agent is a Go binary, not frontend.
// Included here for protocol documentation completeness.
// ============================================================================

export const AGENT_TYPE = {
  // Agent → Server
  REGISTER: 'register',
  HEARTBEAT: 'heartbeat',
  COMMAND_RESULT: 'command_result',
  COMMAND_STREAM: 'command_stream',

  // Server → Agent
  COMMAND: 'command',
  PROBE_CONFIG: 'probe_config',
} as const

export interface AgentRegisterPayload {
  hostname: string
  os: string
  kernel: string
  ip: string
  agent_version: string
  cpu_cores: number
  memory_total_mb: number
  disk_total_gb: number
}

export interface AgentHeartbeatPayload {
  cpu: number
  memory: number
  disk: number
  uptime: number
  load_avg: [number, number, number]
}

export interface AgentCommandResultPayload {
  exit_code: number
  stdout: string
  stderr: string
  duration_ms: number
}

export interface AgentCommandStreamPayload {
  stream: 'stdout' | 'stderr'
  data: string
}

export interface AgentCommandPayload {
  action: 'execute_command'
  command: string
  timeout: number
}

export interface AgentProbeConfigPayload {
  enabled: boolean
  report_url: string
  targets: ProbeTargetConfig[]
}

export interface ProbeTargetConfig {
  id: string
  name: string
  host: string
  ping_count: number
  tcp_port: number
  bandwidth_url?: string
}
