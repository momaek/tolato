package ws

import (
	"sync"
	"time"
)

type MemoryAgentRegistry struct {
	hub clientLookup

	mu         sync.RWMutex
	bindings   map[string]string
	heartbeats map[string]time.Time
}

func NewMemoryAgentRegistry(hub clientLookup) *MemoryAgentRegistry {
	return &MemoryAgentRegistry{
		hub:        hub,
		bindings:   make(map[string]string),
		heartbeats: make(map[string]time.Time),
	}
}

func (r *MemoryAgentRegistry) BindNode(nodeID string, clientID string) {
	var replaced Client

	r.mu.Lock()
	if previousClientID, ok := r.bindings[nodeID]; ok && previousClientID != clientID && r.hub != nil {
		replaced, _ = r.hub.Client(previousClientID)
	}
	r.bindings[nodeID] = clientID
	r.mu.Unlock()

	if replaced != nil {
		replaced.Close(1000, "replaced")
	}
}

func (r *MemoryAgentRegistry) UnbindNode(nodeID string, clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if boundClientID, ok := r.bindings[nodeID]; !ok || boundClientID != clientID {
		return
	}
	delete(r.bindings, nodeID)
	delete(r.heartbeats, nodeID)
}

func (r *MemoryAgentRegistry) Heartbeat(nodeID string, clientID string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	boundClientID, ok := r.bindings[nodeID]
	if !ok {
		return ErrNodeNotBound
	}
	if boundClientID != clientID {
		return ErrClientNotFound
	}
	r.heartbeats[nodeID] = at.UTC()
	return nil
}

func (r *MemoryAgentRegistry) PublishDispatch(nodeID string, msg []byte) error {
	r.mu.RLock()
	clientID, ok := r.bindings[nodeID]
	r.mu.RUnlock()
	if !ok {
		return ErrNodeNotBound
	}

	client, ok := r.hub.Client(clientID)
	if !ok {
		return ErrClientNotFound
	}
	if !client.Send(msg) {
		client.Close(1001, "send queue full")
	}
	return nil
}

func (r *MemoryAgentRegistry) LastHeartbeat(nodeID string) (time.Time, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	at, ok := r.heartbeats[nodeID]
	return at, ok
}
