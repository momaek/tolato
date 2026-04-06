package handler

import (
	"sync"

	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/model"
)

// SessionManager manages the single active frontend WebSocket connection.
// Only one frontend tab can be connected at a time; a new connection
// kicks the old one with a "session_replaced" message.
type SessionManager struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

// Replace registers a new WebSocket connection, closing the old one if it exists.
// The old connection receives a session_replaced notification before being closed.
func (sm *SessionManager) Replace(newConn *websocket.Conn) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.conn != nil {
		// Notify old connection before closing
		sm.conn.WriteJSON(model.WSMessage{
			Type: "session_replaced",
			Payload: map[string]string{
				"reason": "Another tab has connected",
			},
		})
		sm.conn.Close()
	}
	sm.conn = newConn
}

// Remove clears the current connection if it matches the given conn.
func (sm *SessionManager) Remove(conn *websocket.Conn) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.conn == conn {
		sm.conn = nil
	}
}

// Current returns the current active connection, or nil.
func (sm *SessionManager) Current() *websocket.Conn {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.conn
}
