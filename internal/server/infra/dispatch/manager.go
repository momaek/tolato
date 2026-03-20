package dispatch

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/internal/shared/protocol"
	"github.com/momaek/tolato/internal/shared/types"
	"go.uber.org/zap"
)

type agentClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *agentClient) writeJSON(ctx context.Context, payload any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(deadline)
	} else {
		_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}

	return c.conn.WriteJSON(payload)
}

type Manager struct {
	logger *zap.Logger

	mu      sync.RWMutex
	clients map[string]*agentClient
}

func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		logger:  logger,
		clients: make(map[string]*agentClient),
	}
}

func (m *Manager) Register(nodeID string, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[nodeID] = &agentClient{conn: conn}
}

func (m *Manager) Unregister(nodeID string, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, ok := m.clients[nodeID]
	if !ok || client.conn != conn {
		return
	}
	delete(m.clients, nodeID)
}

func (m *Manager) HasNode(nodeID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.clients[nodeID]
	return ok
}

func (m *Manager) DispatchTask(ctx context.Context, nodeID, taskID, executionID string, steps []types.PlanStep, timeoutSec int) error {
	env, err := protocol.NewEnvelope(protocol.TypeTaskDispatch, taskID, nodeID, time.Now().UnixNano(), protocol.TaskDispatchPayload{
		ExecutionID: executionID,
		Steps:       steps,
		TimeoutSec:  timeoutSec,
	})
	if err != nil {
		return err
	}

	return m.send(ctx, nodeID, env)
}

func (m *Manager) CancelTask(ctx context.Context, nodeID, taskID, executionID, reason string) error {
	env, err := protocol.NewEnvelope(protocol.TypeTaskCancel, taskID, nodeID, time.Now().UnixNano(), protocol.TaskCancelPayload{
		ExecutionID: executionID,
		Reason:      reason,
	})
	if err != nil {
		return err
	}

	return m.send(ctx, nodeID, env)
}

func (m *Manager) send(ctx context.Context, nodeID string, env protocol.Envelope) error {
	m.mu.RLock()
	client, ok := m.clients[nodeID]
	m.mu.RUnlock()
	if !ok {
		return errors.New("node is not connected")
	}

	if err := client.writeJSON(ctx, env); err != nil {
		m.logger.Warn("dispatch to agent failed", zap.String("node_id", nodeID), zap.Error(err))
		return err
	}

	return nil
}
