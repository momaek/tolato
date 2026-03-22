package ws

import "sync"

type clientLookup interface {
	Client(clientID string) (Client, bool)
}

type MemorySessionRegistry struct {
	hub clientLookup

	mu sync.RWMutex

	activeByClient  map[string]string
	activeBySession map[string]map[string]struct{}

	watchByClient  map[string]map[string]struct{}
	watchBySession map[string]map[string]struct{}
}

func NewMemorySessionRegistry(hub clientLookup) *MemorySessionRegistry {
	return &MemorySessionRegistry{
		hub:             hub,
		activeByClient:  make(map[string]string),
		activeBySession: make(map[string]map[string]struct{}),
		watchByClient:   make(map[string]map[string]struct{}),
		watchBySession:  make(map[string]map[string]struct{}),
	}
}

func (r *MemorySessionRegistry) SetActive(clientID string, sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if oldSessionID, ok := r.activeByClient[clientID]; ok && oldSessionID != sessionID {
		removeFromSet(r.activeBySession, oldSessionID, clientID)
	}
	if sessionID == "" {
		delete(r.activeByClient, clientID)
		return
	}

	r.activeByClient[clientID] = sessionID
	addToSet(r.activeBySession, sessionID, clientID)
}

func (r *MemorySessionRegistry) SetWatchSessions(clientID string, sessionIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if oldWatch, ok := r.watchByClient[clientID]; ok {
		for sessionID := range oldWatch {
			removeFromSet(r.watchBySession, sessionID, clientID)
		}
	}

	unique := uniqueSessionIDs(sessionIDs)
	if len(unique) == 0 {
		delete(r.watchByClient, clientID)
		return
	}

	watchSet := make(map[string]struct{}, len(unique))
	for _, sessionID := range unique {
		watchSet[sessionID] = struct{}{}
		addToSet(r.watchBySession, sessionID, clientID)
	}
	r.watchByClient[clientID] = watchSet
}

func (r *MemorySessionRegistry) PublishToSession(sessionID string, msg []byte) {
	r.publish(sessionID, msg, true)
}

func (r *MemorySessionRegistry) PublishSummary(sessionID string, msg []byte) {
	r.publish(sessionID, msg, false)
}

func (r *MemorySessionRegistry) ForgetClient(clientID string) {
	r.forgetClient(clientID)
}

func (r *MemorySessionRegistry) publish(sessionID string, msg []byte, timelineOnly bool) {
	clientIDs := r.clientsForSession(sessionID, timelineOnly)
	for _, clientID := range clientIDs {
		client, ok := r.hub.Client(clientID)
		if !ok {
			r.forgetClient(clientID)
			continue
		}
		if !client.Send(msg) {
			client.Close(1001, "send queue full")
		}
	}
}

func (r *MemorySessionRegistry) clientsForSession(sessionID string, timelineOnly bool) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]struct{})
	out := make([]string, 0)

	if clientIDs, ok := r.activeBySession[sessionID]; ok {
		for clientID := range clientIDs {
			if _, exists := seen[clientID]; exists {
				continue
			}
			seen[clientID] = struct{}{}
			out = append(out, clientID)
		}
	}
	if timelineOnly {
		return out
	}
	if clientIDs, ok := r.watchBySession[sessionID]; ok {
		for clientID := range clientIDs {
			if _, exists := seen[clientID]; exists {
				continue
			}
			seen[clientID] = struct{}{}
			out = append(out, clientID)
		}
	}
	return out
}

func (r *MemorySessionRegistry) forgetClient(clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sessionID, ok := r.activeByClient[clientID]; ok {
		removeFromSet(r.activeBySession, sessionID, clientID)
		delete(r.activeByClient, clientID)
	}

	if watchSet, ok := r.watchByClient[clientID]; ok {
		for sessionID := range watchSet {
			removeFromSet(r.watchBySession, sessionID, clientID)
		}
		delete(r.watchByClient, clientID)
	}
}

func addToSet(m map[string]map[string]struct{}, key string, value string) {
	set, ok := m[key]
	if !ok {
		set = make(map[string]struct{})
		m[key] = set
	}
	set[value] = struct{}{}
}

func removeFromSet(m map[string]map[string]struct{}, key string, value string) {
	set, ok := m[key]
	if !ok {
		return
	}
	delete(set, value)
	if len(set) == 0 {
		delete(m, key)
	}
}

func uniqueSessionIDs(sessionIDs []string) []string {
	if len(sessionIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(sessionIDs))
	out := make([]string, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if sessionID == "" {
			continue
		}
		if _, ok := seen[sessionID]; ok {
			continue
		}
		seen[sessionID] = struct{}{}
		out = append(out, sessionID)
	}
	return out
}
