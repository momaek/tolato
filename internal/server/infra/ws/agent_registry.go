package ws

import (
	"sync"
	"time"
)

type MemoryAgentRegistry struct {
	hub clientLookup

	mu         sync.RWMutex
	bindings   map[string]string
	reverse    map[string]string
	heartbeats map[string]time.Time
	metadata   map[string]AgentNodeMetadata
	runtime    map[string]AgentNodeRuntime
}

func NewMemoryAgentRegistry(hub clientLookup) *MemoryAgentRegistry {
	return &MemoryAgentRegistry{
		hub:        hub,
		bindings:   make(map[string]string),
		reverse:    make(map[string]string),
		heartbeats: make(map[string]time.Time),
		metadata:   make(map[string]AgentNodeMetadata),
		runtime:    make(map[string]AgentNodeRuntime),
	}
}

func (r *MemoryAgentRegistry) BindNode(nodeID string, clientID string, meta AgentNodeMetadata) {
	var replaced Client

	r.mu.Lock()
	if previousNodeID, ok := r.reverse[clientID]; ok && previousNodeID != nodeID {
		delete(r.bindings, previousNodeID)
	}
	if previousClientID, ok := r.bindings[nodeID]; ok && previousClientID != clientID && r.hub != nil {
		replaced, _ = r.hub.Client(previousClientID)
		delete(r.reverse, previousClientID)
	}
	r.bindings[nodeID] = clientID
	r.reverse[clientID] = nodeID
	r.metadata[nodeID] = cloneAgentNodeMetadata(meta)
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
	delete(r.reverse, clientID)
}

func (r *MemoryAgentRegistry) ForgetClient(clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	nodeID, ok := r.reverse[clientID]
	if !ok {
		return
	}
	if boundClientID, ok := r.bindings[nodeID]; ok && boundClientID == clientID {
		delete(r.bindings, nodeID)
	}
	delete(r.reverse, clientID)
}

func (r *MemoryAgentRegistry) Heartbeat(nodeID string, clientID string, state AgentNodeRuntime, at time.Time) error {
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
	r.runtime[nodeID] = state
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

func (r *MemoryAgentRegistry) Snapshots() []AgentPresenceSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodeIDs := make(map[string]struct{}, len(r.bindings)+len(r.heartbeats))
	for nodeID := range r.bindings {
		nodeIDs[nodeID] = struct{}{}
	}
	for nodeID := range r.heartbeats {
		nodeIDs[nodeID] = struct{}{}
	}

	out := make([]AgentPresenceSnapshot, 0, len(nodeIDs))
	for nodeID := range nodeIDs {
		snapshot := AgentPresenceSnapshot{
			NodeID: nodeID,
		}
		if clientID, ok := r.bindings[nodeID]; ok {
			snapshot.ClientID = clientID
			snapshot.Bound = true
		}
		if at, ok := r.heartbeats[nodeID]; ok {
			hb := at
			snapshot.LastHeartbeat = &hb
		}
		if meta, ok := r.metadata[nodeID]; ok {
			snapshot.Hostname = meta.Hostname
			snapshot.Region = meta.Region
			snapshot.OS = meta.OS
			snapshot.Version = meta.Version
			snapshot.IPAddress = meta.IPAddress
			snapshot.Tags = append([]string(nil), meta.Tags...)
		}
		if state, ok := r.runtime[nodeID]; ok {
			snapshot.Busy = state.Busy
			snapshot.Metrics = state.Metrics
		}
		out = append(out, snapshot)
	}
	return out
}

func cloneAgentNodeMetadata(meta AgentNodeMetadata) AgentNodeMetadata {
	return AgentNodeMetadata{
		Hostname:  meta.Hostname,
		Region:    meta.Region,
		OS:        meta.OS,
		Version:   meta.Version,
		IPAddress: meta.IPAddress,
		Tags:      append([]string(nil), meta.Tags...),
	}
}
