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

	unreadByClient map[string]map[string]int
}

func NewMemorySessionRegistry(hub clientLookup) *MemorySessionRegistry {
	return &MemorySessionRegistry{
		hub:             hub,
		activeByClient:  make(map[string]string),
		activeBySession: make(map[string]map[string]struct{}),
		watchByClient:   make(map[string]map[string]struct{}),
		watchBySession:  make(map[string]map[string]struct{}),
		unreadByClient:  make(map[string]map[string]int),
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
	r.clearUnreadLocked(clientID, sessionID)
}

func (r *MemorySessionRegistry) SetWatchSessions(clientID string, sessionIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if oldWatch, ok := r.watchByClient[clientID]; ok {
		for sessionID := range oldWatch {
			removeFromSet(r.watchBySession, sessionID, clientID)
			if _, keep := uniqueContains(sessionIDs, sessionID); !keep {
				r.deleteUnreadLocked(clientID, sessionID)
			}
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

func (r *MemorySessionRegistry) PublishToClient(clientID string, msg []byte) {
	client, ok := r.hub.Client(clientID)
	if !ok {
		r.forgetClient(clientID)
		return
	}
	if !client.Send(msg) {
		client.Close(1001, "send queue full")
	}
}

func (r *MemorySessionRegistry) SummaryRecipients(sessionID string) []string {
	return r.clientsForSession(sessionID, false)
}

func (r *MemorySessionRegistry) IncrementUnread(sessionID string) []SessionUnreadState {
	r.mu.Lock()
	defer r.mu.Unlock()

	clientIDs, ok := r.watchBySession[sessionID]
	if !ok {
		return nil
	}

	updates := make([]SessionUnreadState, 0, len(clientIDs))
	for clientID := range clientIDs {
		if r.activeByClient[clientID] == sessionID {
			continue
		}
		counts, exists := r.unreadByClient[clientID]
		if !exists {
			counts = make(map[string]int)
			r.unreadByClient[clientID] = counts
		}
		counts[sessionID]++
		updates = append(updates, SessionUnreadState{
			ClientID:  clientID,
			SessionID: sessionID,
			Unread:    counts[sessionID],
		})
	}
	return updates
}

func (r *MemorySessionRegistry) ClearUnread(clientID string, sessionID string) (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.clearUnreadLocked(clientID, sessionID)
}

func (r *MemorySessionRegistry) UnreadCount(clientID string, sessionID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if counts, ok := r.unreadByClient[clientID]; ok {
		return counts[sessionID]
	}
	return 0
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

	delete(r.unreadByClient, clientID)
}

func (r *MemorySessionRegistry) clearUnreadLocked(clientID string, sessionID string) (int, bool) {
	counts, ok := r.unreadByClient[clientID]
	if !ok {
		return 0, false
	}
	if _, exists := counts[sessionID]; !exists {
		return 0, false
	}
	delete(counts, sessionID)
	if len(counts) == 0 {
		delete(r.unreadByClient, clientID)
	}
	return 0, true
}

func (r *MemorySessionRegistry) deleteUnreadLocked(clientID string, sessionID string) {
	counts, ok := r.unreadByClient[clientID]
	if !ok {
		return
	}
	delete(counts, sessionID)
	if len(counts) == 0 {
		delete(r.unreadByClient, clientID)
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

func uniqueContains(sessionIDs []string, sessionID string) (string, bool) {
	for _, candidate := range sessionIDs {
		if candidate == sessionID {
			return sessionID, true
		}
	}
	return "", false
}
