package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/model"
)

// AgentConn represents a connected agent.
type AgentConn struct {
	NodeID  string
	Conn    *websocket.Conn
	Metrics *model.NodeMetrics // cached metrics from last heartbeat
	mu      sync.Mutex
}

// WriteJSON sends a JSON message to the agent, thread-safe.
func (ac *AgentConn) WriteJSON(v any) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.Conn.WriteJSON(v)
}

// NodeManager manages active agent WebSocket connections.
type NodeManager struct {
	mu    sync.RWMutex
	conns map[string]*AgentConn // nodeID -> conn
}

// NewNodeManager creates a new NodeManager.
func NewNodeManager() *NodeManager {
	return &NodeManager{
		conns: make(map[string]*AgentConn),
	}
}

// RegisterConn registers a new agent connection.
func (m *NodeManager) RegisterConn(nodeID string, conn *websocket.Conn) *AgentConn {
	m.mu.Lock()
	defer m.mu.Unlock()

	ac := &AgentConn{
		NodeID: nodeID,
		Conn:   conn,
	}
	// If there's an existing connection, close it
	if old, ok := m.conns[nodeID]; ok {
		old.Conn.Close()
	}
	m.conns[nodeID] = ac
	return ac
}

// RemoveConn removes an agent connection.
func (m *NodeManager) RemoveConn(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.conns, nodeID)
}

// GetConn returns the agent connection for a node.
func (m *NodeManager) GetConn(nodeID string) (*AgentConn, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ac, ok := m.conns[nodeID]
	return ac, ok
}

// ListOnlineNodes returns IDs of all connected nodes.
func (m *NodeManager) ListOnlineNodes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.conns))
	for id := range m.conns {
		ids = append(ids, id)
	}
	return ids
}

// GetMetrics returns cached metrics for a node.
func (m *NodeManager) GetMetrics(nodeID string) *model.NodeMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ac, ok := m.conns[nodeID]; ok {
		return ac.Metrics
	}
	return nil
}

// UpdateMetrics updates the cached metrics for a node.
func (m *NodeManager) UpdateMetrics(nodeID string, metrics *model.NodeMetrics) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ac, ok := m.conns[nodeID]; ok {
		ac.Metrics = metrics
	}
}

// ExecuteCommand sends a command to an agent and waits for the result.
func (m *NodeManager) ExecuteCommand(ctx context.Context, nodeID, command string, timeout int) (*model.AgentCommandResultPayload, error) {
	ac, ok := m.GetConn(nodeID)
	if !ok {
		return nil, errors.New("node is not online")
	}

	// Send command to agent
	msg := model.WSMessage{
		Type: model.AgentTypeCommand,
		Payload: model.AgentCommandPayload{
			Action:  "execute_command",
			Command: command,
			Timeout: timeout,
		},
	}

	if err := ac.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	// Wait for result with timeout
	timeoutDuration := time.Duration(timeout) * time.Second
	if timeoutDuration == 0 {
		timeoutDuration = 60 * time.Second
	}

	resultCh := make(chan *model.AgentCommandResultPayload, 1)
	errCh := make(chan error, 1)

	go func() {
		for {
			_, raw, err := ac.Conn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}

			var wsMsg model.WSMessage
			if err := json.Unmarshal(raw, &wsMsg); err != nil {
				continue
			}

			if wsMsg.Type == model.AgentTypeCommandResult {
				payloadBytes, err := json.Marshal(wsMsg.Payload)
				if err != nil {
					errCh <- err
					return
				}
				var result model.AgentCommandResultPayload
				if err := json.Unmarshal(payloadBytes, &result); err != nil {
					errCh <- err
					return
				}
				resultCh <- &result
				return
			}
		}
	}()

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(timeoutDuration):
		return nil, errors.New("command execution timed out")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
