package schema

// ============================================================================
// WebSocket Message Types
// ============================================================================

// --- Server ↔ Frontend (Chat WebSocket: /ws/chat) ---

// WSMessage is the envelope for all WebSocket messages.
type WSMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

// Server → Frontend event types
const (
	WSTypeReasoning      = "reasoning"       // AI thinking/reasoning delta
	WSTypeContent        = "content"         // AI text content delta
	WSTypeToolCall       = "tool_call"       // tool execution started
	WSTypeToolResult     = "tool_result"     // tool execution completed
	WSTypeConfirmRequest = "confirm_request" // sensitive operation confirmation request
	WSTypeDone           = "done"            // agent loop completed
	WSTypeError          = "error"           // error occurred
)

// Frontend → Server event types
const (
	WSTypeUserMessage     = "user_message"     // user sends a chat message
	WSTypeConfirmResponse = "confirm_response" // user confirms/rejects sensitive operation
)

// --- Server → Frontend payloads ---

type WSReasoningEvent struct {
	Delta string `json:"delta"` // incremental reasoning text
}

type WSContentEvent struct {
	Delta string `json:"delta"` // incremental content text
}

type WSToolCallEvent struct {
	ID   string         `json:"id"`   // tool call ID
	Tool string         `json:"tool"` // tool name: list_nodes, get_node_info, execute_command
	Args map[string]any `json:"args"` // tool arguments
}

type WSToolResultEvent struct {
	ID     string `json:"id"`     // matches tool call ID
	Result any    `json:"result"` // tool execution result (ToolResultItem or similar)
}

type WSConfirmRequestEvent struct {
	ID   string         `json:"id"`   // confirmation ID
	Tool string         `json:"tool"` // tool that triggered confirmation
	Args map[string]any `json:"args"` // tool arguments for user review
}

type WSDoneEvent struct {
	// empty, signals completion
}

type WSErrorEvent struct {
	Message string `json:"message"`
}

// --- Frontend → Server payloads ---

type WSUserMessageEvent struct {
	Content       string  `json:"content"`                  // user message text
	Model         *string `json:"model,omitempty"`          // override model for this message
	DefaultNodeID *string `json:"default_node_id,omitempty"` // override default node
}

type WSConfirmResponseEvent struct {
	ID       string `json:"id"`       // matches confirm request ID
	Approved bool   `json:"approved"` // true = proceed, false = reject
}

// ============================================================================
// Server ↔ Node Agent (Agent WebSocket: /ws/agent)
// ============================================================================

// Agent → Server event types
const (
	AgentTypeRegister      = "register"
	AgentTypeHeartbeat     = "heartbeat"
	AgentTypeCommandResult = "command_result"
	AgentTypeCommandStream = "command_stream"
)

// Server → Agent event types
const (
	AgentTypeCommand     = "command"
	AgentTypeProbeConfig = "probe_config"
)

// --- Agent → Server payloads ---

type AgentRegisterPayload struct {
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Kernel       string `json:"kernel"`
	IP           string `json:"ip"`
	AgentVersion string `json:"agent_version"`
	CPUCores     int    `json:"cpu_cores"`
	MemoryTotalMB int   `json:"memory_total_mb"`
	DiskTotalGB  int    `json:"disk_total_gb"`
}

type AgentHeartbeatPayload struct {
	CPU     float64   `json:"cpu"`      // CPU usage %
	Memory  float64   `json:"memory"`   // memory usage %
	Disk    float64   `json:"disk"`     // disk usage %
	Uptime  int64     `json:"uptime"`   // seconds
	LoadAvg []float64 `json:"load_avg"` // [1min, 5min, 15min]
}

type AgentCommandResultPayload struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"duration_ms"`
}

type AgentCommandStreamPayload struct {
	Stream string `json:"stream"` // "stdout" or "stderr"
	Data   string `json:"data"`   // incremental output data
}

// --- Server → Agent payloads ---

type AgentCommandPayload struct {
	Action  string `json:"action"`  // "execute_command"
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // seconds
}

type AgentProbeConfigPayload struct {
	Enabled   bool               `json:"enabled"`
	ReportURL string             `json:"report_url"`
	Targets   []ProbeTargetConfig `json:"targets"`
}

type ProbeTargetConfig struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Host         string  `json:"host"`
	PingCount    int     `json:"ping_count"`     // number of ICMP packets
	TCPPort      int     `json:"tcp_port"`       // port for TCP connect test
	BandwidthURL *string `json:"bandwidth_url,omitempty"` // HTTP URL for bandwidth test
}
