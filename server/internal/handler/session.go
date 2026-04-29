package handler

import (
	"sync"

	"github.com/gorilla/websocket"
)

// ChatSession wraps a frontend WebSocket connection with a write mutex.
// gorilla/websocket forbids concurrent writes; routing all writes through
// WriteJSON serializes the writer goroutine against any other path that may
// share the same connection.
type ChatSession struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
	closed  bool
}

// NewChatSession wraps an authenticated WebSocket connection.
func NewChatSession(conn *websocket.Conn) *ChatSession {
	return &ChatSession{conn: conn}
}

// Conn returns the underlying connection. Callers may use it for ReadMessage,
// which gorilla/websocket allows to run concurrently with writes; do NOT call
// WriteJSON on it directly — go through (*ChatSession).WriteJSON.
func (s *ChatSession) Conn() *websocket.Conn {
	return s.conn
}

// WriteJSON serializes writes against any other goroutine using the same
// session. Returns the underlying error or nil on success; if the session was
// already closed, the call is a no-op.
func (s *ChatSession) WriteJSON(v any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.closed {
		return nil
	}
	return s.conn.WriteJSON(v)
}

// Close marks the session closed and shuts the underlying connection. Subsequent
// WriteJSON calls become no-ops. Safe to call concurrently with WriteJSON
// (close is taken under the same mutex).
func (s *ChatSession) Close() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.conn.Close()
}

