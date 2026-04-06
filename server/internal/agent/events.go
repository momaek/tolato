package agent

// LoopEvent is the interface for all events emitted by a LoopRunner.
// Each event carries the conversation ID it belongs to.
type LoopEvent interface {
	GetConversationID() string
}

// ---------------------------------------------------------------------------
// Loop → WS Handler events (output events)
// ---------------------------------------------------------------------------

// ReasoningEvent streams a chunk of the model's chain-of-thought reasoning.
type ReasoningEvent struct {
	ConversationID string
	Delta          string
}

func (e ReasoningEvent) GetConversationID() string { return e.ConversationID }

// ContentEvent streams a chunk of the model's visible response text.
type ContentEvent struct {
	ConversationID string
	Delta          string
}

func (e ContentEvent) GetConversationID() string { return e.ConversationID }

// ToolCallEvent indicates the model is invoking a tool.
type ToolCallEvent struct {
	ConversationID string
	ID             string
	Tool           string
	Args           map[string]any
}

func (e ToolCallEvent) GetConversationID() string { return e.ConversationID }

// ToolResultEvent carries the result of a completed tool execution.
type ToolResultEvent struct {
	ConversationID string
	ID             string
	Result         any
}

func (e ToolResultEvent) GetConversationID() string { return e.ConversationID }

// ConfirmRequestEvent asks the user to approve a tool call before execution.
type ConfirmRequestEvent struct {
	ConversationID string
	ID             string
	Tool           string
	Args           map[string]any
}

func (e ConfirmRequestEvent) GetConversationID() string { return e.ConversationID }

// DoneEvent signals that the loop has finished processing the current turn.
type DoneEvent struct {
	ConversationID string
}

func (e DoneEvent) GetConversationID() string { return e.ConversationID }

// ErrorEvent reports an error that occurred during loop execution.
type ErrorEvent struct {
	ConversationID string
	Message        string
}

func (e ErrorEvent) GetConversationID() string { return e.ConversationID }

// SessionReplacedEvent is a connection-level event (not conversation-level)
// sent when the user's session is superseded by a new connection.
// It does not implement LoopEvent since it has no conversation context.
type SessionReplacedEvent struct {
	Reason string
}

// ---------------------------------------------------------------------------
// WS Handler → Loop inputs
// ---------------------------------------------------------------------------

// UserMessageInput carries a new message from the user into the loop.
type UserMessageInput struct {
	ConversationID string
	Content        string
	Model          *string
	DefaultNodeID  *string
}

// ConfirmInput carries the user's approval or rejection of a pending tool call.
type ConfirmInput struct {
	ConversationID string
	ID             string
	Approved       bool
}
