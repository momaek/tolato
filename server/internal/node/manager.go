package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/model"
)

// ============================================================================
// AgentConn — owns the underlying WebSocket connection for a single agent and
// multiplexes messages between many concurrent senders / receivers.
//
// Only ONE goroutine (started by run()) ever calls conn.ReadMessage(). All
// writes go through WriteJSON() which holds writeMu. Callers interact via:
//
//   - Request(msgType, payload, timeout)  → one-shot request/response
//   - OpenStream(openType, payload)       → long-lived stream (e.g. PTY)
//   - SetSystemHandlers(onHB, onReRegister)
//     registers callbacks for unsolicited agent messages
// ============================================================================

// AgentFrame is a message decoded off the agent's socket. Payload is kept as
// raw JSON so each handler can unmarshal it into its own typed struct via
// Decode() — no generic `map[string]any` round-trip, no double-marshal.
type AgentFrame struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Decode unmarshals the frame's payload into `out`. Safe to call with empty
// payloads (no-op).
func (f *AgentFrame) Decode(out any) error {
	if len(f.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(f.Payload, out)
}

// SystemHandlers collects callbacks for unsolicited agent messages. The raw
// payload bytes are handed through so each handler unmarshals into its own
// typed struct.
type SystemHandlers struct {
	OnHeartbeat  func(payload json.RawMessage)
	OnReRegister func(payload json.RawMessage)
}

// AgentConn represents a connected agent with a message router.
type AgentConn struct {
	NodeID  string
	Conn    *websocket.Conn
	Metrics *model.NodeMetrics // cached metrics from last heartbeat

	writeMu sync.Mutex

	mu       sync.Mutex
	pending  map[string]chan *AgentFrame // one-shot request id → reply
	streams  map[string]chan *AgentFrame // long-lived stream id → frames
	handlers SystemHandlers

	done      chan struct{}
	closeOnce sync.Once
}

// WriteJSON sends a JSON message to the agent, thread-safe.
func (ac *AgentConn) WriteJSON(v any) error {
	ac.writeMu.Lock()
	defer ac.writeMu.Unlock()
	return ac.Conn.WriteJSON(v)
}

// Done returns a channel that is closed when the connection terminates.
func (ac *AgentConn) Done() <-chan struct{} {
	return ac.done
}

// SetSystemHandlers registers callbacks for heartbeat / re-register messages.
// Safe to call once after RegisterConn, before run() starts consuming.
func (ac *AgentConn) SetSystemHandlers(h SystemHandlers) {
	ac.mu.Lock()
	ac.handlers = h
	ac.mu.Unlock()
}

// Request performs a one-shot request/response over the agent WS.
// A new request ID is generated; the returned message is the first agent-sent
// message with a matching ID.
func (ac *AgentConn) Request(ctx context.Context, msgType string, payload any, timeout time.Duration) (*AgentFrame, error) {
	id := uuid.New().String()

	ch := make(chan *AgentFrame, 1)
	ac.mu.Lock()
	ac.pending[id] = ch
	ac.mu.Unlock()

	defer func() {
		ac.mu.Lock()
		delete(ac.pending, id)
		ac.mu.Unlock()
	}()

	msg := map[string]any{
		"type":    msgType,
		"id":      id,
		"payload": payload,
	}
	if err := ac.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("send %s: %w", msgType, err)
	}

	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	select {
	case reply := <-ch:
		return reply, nil
	case <-time.After(timeout):
		return nil, errors.New("request timed out")
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ac.done:
		return nil, errors.New("agent disconnected")
	}
}

// Stream represents a long-lived subscription to agent messages keyed by ID.
type Stream struct {
	ID   string
	Ch   <-chan *AgentFrame
	conn *AgentConn
}

// Send forwards a message to the agent with this stream's ID.
func (s *Stream) Send(msgType string, payload any) error {
	return s.conn.WriteJSON(map[string]any{
		"type":    msgType,
		"id":      s.ID,
		"payload": payload,
	})
}

// Close unregisters the stream; the underlying channel is closed.
func (s *Stream) Close() {
	s.conn.mu.Lock()
	if ch, ok := s.conn.streams[s.ID]; ok {
		delete(s.conn.streams, s.ID)
		close(ch)
	}
	s.conn.mu.Unlock()
}

// OpenStream allocates a new stream and sends the `openType` message with its
// ID to the agent. Agent replies carrying the same ID are delivered on the
// returned channel. The caller is responsible for calling Close() on the stream.
func (ac *AgentConn) OpenStream(openType string, payload any) (*Stream, error) {
	id := uuid.New().String()
	ch := make(chan *AgentFrame, 64)

	ac.mu.Lock()
	ac.streams[id] = ch
	ac.mu.Unlock()

	msg := map[string]any{
		"type":    openType,
		"id":      id,
		"payload": payload,
	}
	if err := ac.WriteJSON(msg); err != nil {
		ac.mu.Lock()
		delete(ac.streams, id)
		ac.mu.Unlock()
		close(ch)
		return nil, fmt.Errorf("send %s: %w", openType, err)
	}

	return &Stream{ID: id, Ch: ch, conn: ac}, nil
}

// run is the single owner of conn.ReadMessage(). It demultiplexes incoming
// messages onto pending requests, streams, or system handlers.
func (ac *AgentConn) run() {
	defer ac.closeOnce.Do(func() {
		close(ac.done)
		// Flush any waiters / subscribers.
		ac.mu.Lock()
		for id, ch := range ac.streams {
			close(ch)
			delete(ac.streams, id)
		}
		// Pending one-shots will observe ac.done and bail.
		ac.mu.Unlock()
	})

	for {
		_, raw, err := ac.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[agent_router] node=%s read error: %v", ac.NodeID, err)
			}
			return
		}

		var msg AgentFrame
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("[agent_router] node=%s parse error: %v", ac.NodeID, err)
			continue
		}

		ac.dispatch(&msg)
	}
}

func (ac *AgentConn) dispatch(msg *AgentFrame) {
	// ID-keyed messages: prefer stream match, then pending.
	if msg.ID != "" {
		ac.mu.Lock()
		if ch, ok := ac.streams[msg.ID]; ok {
			ac.mu.Unlock()
			select {
			case ch <- msg:
			default:
				log.Printf("[agent_router] node=%s stream=%s chan full, dropping frame", ac.NodeID, msg.ID)
			}
			return
		}
		if ch, ok := ac.pending[msg.ID]; ok {
			delete(ac.pending, msg.ID)
			ac.mu.Unlock()
			ch <- msg
			return
		}
		ac.mu.Unlock()
		// Fallthrough: untagged-by-handler message with an unknown ID — dispatch by type.
	}

	// Unsolicited messages: dispatch by type.
	switch msg.Type {
	case model.AgentTypeHeartbeat:
		if ac.handlers.OnHeartbeat != nil {
			ac.handlers.OnHeartbeat(msg.Payload)
		}
	case model.AgentTypeRegister:
		if ac.handlers.OnReRegister != nil {
			ac.handlers.OnReRegister(msg.Payload)
		}
	default:
		log.Printf("[agent_router] node=%s dropped message type=%s id=%q", ac.NodeID, msg.Type, msg.ID)
	}
}

// ============================================================================
// NodeManager — tracks AgentConn per nodeID.
// ============================================================================

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

// RegisterConn registers a new agent connection and starts its read loop.
// Any existing connection for the same node is closed first.
func (m *NodeManager) RegisterConn(nodeID string, conn *websocket.Conn) *AgentConn {
	m.mu.Lock()
	defer m.mu.Unlock()

	ac := &AgentConn{
		NodeID:  nodeID,
		Conn:    conn,
		pending: make(map[string]chan *AgentFrame),
		streams: make(map[string]chan *AgentFrame),
		done:    make(chan struct{}),
	}

	if old, ok := m.conns[nodeID]; ok {
		_ = old.Conn.Close()
	}
	m.conns[nodeID] = ac

	go ac.run()
	return ac
}

// RemoveConn removes an agent connection (called after run() returns).
func (m *NodeManager) RemoveConn(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ac, ok := m.conns[nodeID]; ok {
		// Ensure run() has terminated / resources released.
		_ = ac.Conn.Close()
		delete(m.conns, nodeID)
	}
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

// ExecuteCommand sends an execute_command to an agent and waits for the result.
// It delegates to the AgentConn's request router so it plays nicely with other
// concurrent traffic (PTY streams, file ops, heartbeats).
func (m *NodeManager) ExecuteCommand(ctx context.Context, nodeID, command string, timeout int) (*model.AgentCommandResultPayload, error) {
	ac, ok := m.GetConn(nodeID)
	if !ok {
		return nil, errors.New("node is not online")
	}

	timeoutDuration := time.Duration(timeout) * time.Second
	if timeoutDuration <= 0 {
		timeoutDuration = 60 * time.Second
	}

	reply, err := ac.Request(ctx, model.AgentTypeCommand, model.AgentCommandPayload{
		Action:  "execute_command",
		Command: command,
		Timeout: timeout,
	}, timeoutDuration+10*time.Second) // router timeout > command timeout
	if err != nil {
		return nil, err
	}

	if reply.Type != model.AgentTypeCommandResult {
		return nil, fmt.Errorf("unexpected reply type: %s", reply.Type)
	}

	var result model.AgentCommandResultPayload
	if err := reply.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	return &result, nil
}
