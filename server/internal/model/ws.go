package model

// ============================================================================
// WebSocket Message Types
// ============================================================================

// --- Server <-> Frontend (Chat WebSocket: /ws/chat) ---

// WSMessage is the envelope for all WebSocket messages.
type WSMessage struct {
	Type           string `json:"type"`
	ID             string `json:"id,omitempty"` // request/stream id (agent protocol)
	ConversationID string `json:"conversation_id,omitempty"`
	Payload        any    `json:"payload,omitempty"`
}

// Server -> Frontend event types
const (
	WSTypeReasoning      = "reasoning"       // AI thinking/reasoning delta
	WSTypeContent        = "content"         // AI text content delta
	WSTypeToolCall       = "tool_call"       // tool execution started
	WSTypeToolResult     = "tool_result"     // tool execution completed
	WSTypeConfirmRequest = "confirm_request" // sensitive operation confirmation request
	WSTypeDone           = "done"            // agent loop completed
	WSTypeError          = "error"           // error occurred
)

// Frontend -> Server event types
const (
	WSTypeAuth            = "auth"             // first message: JWT authentication
	WSTypeUserMessage     = "user_message"     // user sends a chat message
	WSTypeConfirmResponse = "confirm_response" // user confirms/rejects sensitive operation
	WSTypeStop            = "stop"             // user aborts the in-flight agent loop for a conversation
)

// --- Server -> Frontend payloads ---

type WSReasoningEvent struct {
	Delta string `json:"delta"` // incremental reasoning text
}

type WSContentEvent struct {
	Delta string `json:"delta"` // incremental content text
}

type WSToolCallEvent struct {
	ID   string         `json:"id"`   // tool call ID
	Tool string         `json:"tool"` // tool name
	Args map[string]any `json:"args"` // tool arguments
}

type WSToolResultEvent struct {
	ID     string `json:"id"`     // matches tool call ID
	Result any    `json:"result"` // tool execution result
}

type WSConfirmRequestEvent struct {
	ID   string         `json:"id"`   // confirmation ID
	Tool string         `json:"tool"` // tool that triggered confirmation
	Args map[string]any `json:"args"` // tool arguments for user review
}

type WSDoneEvent struct{}

type WSErrorEvent struct {
	Message string `json:"message"`
}

// --- Frontend -> Server payloads ---

type WSUserMessageEvent struct {
	Content       string  `json:"content"`
	Model         *string `json:"model,omitempty"`
	DefaultNodeID *string `json:"default_node_id,omitempty"`
}

type WSConfirmResponseEvent struct {
	ID       string `json:"id"`
	Approved bool   `json:"approved"`
}

// ============================================================================
// Server <-> Node Agent (Agent WebSocket: /ws/agent)
// ============================================================================

// Agent -> Server event types
const (
	AgentTypeRegister      = "register"
	AgentTypeHeartbeat     = "heartbeat"
	AgentTypeCommandResult = "command_result"
	AgentTypeCommandStream = "command_stream"
	AgentTypePTYOutput     = "pty_output"
	AgentTypePTYExit       = "pty_exit"
	AgentTypeFileResult    = "file_result"
)

// Server -> Agent event types
const (
	AgentTypeCommand     = "command"
	AgentTypePTYOpen     = "pty_open"
	AgentTypePTYInput    = "pty_input"
	AgentTypePTYResize   = "pty_resize"
	AgentTypePTYClose    = "pty_close"
	AgentTypeFileOp      = "file_op"
)

// --- Agent -> Server payloads ---

type AgentRegisterPayload struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Kernel        string `json:"kernel"`
	IP            string `json:"ip"`
	AgentVersion  string `json:"agent_version"`
	CPUCores      int    `json:"cpu_cores"`
	MemoryTotalMB int    `json:"memory_total_mb"`
	DiskTotalGB   int    `json:"disk_total_gb"`
}

type AgentHeartbeatPayload struct {
	CPU     float64   `json:"cpu"`
	Memory  float64   `json:"memory"`
	Disk    float64   `json:"disk"`
	Uptime  int64     `json:"uptime"`
	LoadAvg []float64 `json:"load_avg"`
}

type AgentCommandResultPayload struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"duration_ms"`
}

type AgentCommandStreamPayload struct {
	Stream string `json:"stream"` // "stdout" or "stderr"
	Data   string `json:"data"`
}

// --- Server -> Agent payloads ---

type AgentCommandPayload struct {
	Action  string `json:"action"`  // "execute_command"
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // seconds
}

// AgentAuthResponse is sent to the agent after successful token validation.
type AgentAuthResponse struct {
	NodeID string `json:"node_id"`
	Secret string `json:"secret"`
}

// ============================================================================
// PTY / File Op payloads (tunnelled over the existing /ws/agent connection)
// ============================================================================

// Server -> Agent

type AgentPTYOpenPayload struct {
	Cols  uint16 `json:"cols"`
	Rows  uint16 `json:"rows"`
	Shell string `json:"shell,omitempty"` // optional override; agent picks default
	Cwd   string `json:"cwd,omitempty"`
}

type AgentPTYInputPayload struct {
	Data string `json:"data"` // base64 encoded bytes
}

type AgentPTYResizePayload struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// AgentPTYClosePayload is sent to tell the agent to kill the PTY.
type AgentPTYClosePayload struct{}

// AgentFileOpPayload is a single file operation request.
// Op: list | stat | read | write | mkdir | delete
type AgentFileOpPayload struct {
	Op     string `json:"op"`
	Path   string `json:"path"`
	Data   string `json:"data,omitempty"`   // base64 for write
	Mode   uint32 `json:"mode,omitempty"`   // for mkdir (default 0755)
	Offset int64  `json:"offset,omitempty"` // for read
	Length int64  `json:"length,omitempty"` // for read (0 = read to EOF; capped at server limit)
}

// Agent -> Server

type AgentPTYOutputPayload struct {
	Data string `json:"data"` // base64 encoded bytes
}

type AgentPTYExitPayload struct {
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

// FileEntry describes a single directory entry (returned by list).
type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    uint32 `json:"mode"`
	ModTime int64  `json:"mod_time"` // unix seconds
	IsDir   bool   `json:"is_dir"`
}

// AgentFileResultPayload carries the response for every Op kind.
// Only one of Entries / Data / Stat will typically be populated.
type AgentFileResultPayload struct {
	OK      bool        `json:"ok"`
	Error   string      `json:"error,omitempty"`
	Entries []FileEntry `json:"entries,omitempty"` // for list
	Data    string      `json:"data,omitempty"`    // base64 for read
	Stat    *FileEntry  `json:"stat,omitempty"`    // for stat
	EOF     bool        `json:"eof,omitempty"`     // for read (true → no more bytes after Offset+len(Data))
}

// ============================================================================
// Frontend /ws/terminal message types (browser ↔ server)
// ============================================================================

const (
	// Client -> Server
	WSTermTypeAuth    = "auth"
	WSTermTypeOpen    = "open"
	WSTermTypeInput   = "input"
	WSTermTypeResize  = "resize"
	WSTermTypeClose   = "close"
	WSTermTypeFileOp  = "file_op"

	// Server -> Client
	WSTermTypeAuthOK     = "auth_ok"
	WSTermTypeReady      = "ready"
	WSTermTypeOutput     = "output"
	WSTermTypeExit       = "exit"
	WSTermTypeTermError  = "error"
	WSTermTypeFileResult = "file_result"
)

// Client -> Server payloads

type WSTermOpenPayload struct {
	NodeID string `json:"node_id"`
	Cols   uint16 `json:"cols"`
	Rows   uint16 `json:"rows"`
	Shell  string `json:"shell,omitempty"`
	Cwd    string `json:"cwd,omitempty"`
}

type WSTermInputPayload struct {
	Data string `json:"data"` // base64
}

type WSTermResizePayload struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// WSTermFileOpPayload carries a browser-initiated file op request.
// ReqID is an opaque client id used to match the response.
type WSTermFileOpPayload struct {
	ReqID  string `json:"req_id"`
	NodeID string `json:"node_id,omitempty"` // optional, falls back to current session node
	Op     string `json:"op"`
	Path   string `json:"path"`
	Data   string `json:"data,omitempty"`
	Mode   uint32 `json:"mode,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Length int64  `json:"length,omitempty"`
}

// Server -> Client payloads

type WSTermReadyPayload struct {
	SessionID string `json:"session_id"`
}

type WSTermOutputPayload struct {
	Data string `json:"data"` // base64
}

type WSTermExitPayload struct {
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

type WSTermErrorPayload struct {
	Message string `json:"message"`
}

type WSTermFileResultPayload struct {
	ReqID string                  `json:"req_id"`
	Result AgentFileResultPayload `json:"result"`
}
