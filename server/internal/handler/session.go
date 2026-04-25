package handler

import (
	"sync"

	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/model"
)

// ChatSession wraps a frontend WebSocket connection with a write mutex.
// gorilla/websocket forbids concurrent writes; the writer goroutine and the
// SessionManager.Replace path can both want to write the same connection,
// so all writes go through WriteJSON to serialize them.
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

// SessionManager tracks the single active frontend WebSocket session. A new
// connection kicks the previous one with a "session_replaced" notification.
type SessionManager struct {
	mu      sync.Mutex
	current *ChatSession
}

// NewSessionManager creates an empty SessionManager.
func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

// Replace swaps in a new session, kicking the previous one. The kicked session
// receives a session_replaced notification (best-effort) and is then closed.
// The notification + close happen under the old session's write mutex, so this
// cannot race with the old writer goroutine.
func (sm *SessionManager) Replace(newSession *ChatSession) {
	sm.mu.Lock()
	old := sm.current
	sm.current = newSession
	sm.mu.Unlock()

	if old != nil {
		// WriteJSON acquires old.writeMu, serializing with the old write loop.
		_ = old.WriteJSON(model.WSMessage{
			Type: "session_replaced",
			Payload: map[string]string{
				"reason": "Another tab has connected",
			},
		})
		_ = old.Close()
	}
}

// Remove clears the registry entry if it still points at the given session.
// Idempotent and safe to defer at connection teardown.
func (sm *SessionManager) Remove(session *ChatSession) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.current == session {
		sm.current = nil
	}
}
