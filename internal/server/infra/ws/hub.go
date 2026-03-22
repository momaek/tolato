package ws

import "sync"

type MemoryHub struct {
	mu      sync.RWMutex
	clients map[string]Client
}

func NewMemoryHub() *MemoryHub {
	return &MemoryHub{
		clients: make(map[string]Client),
	}
}

func (h *MemoryHub) Register(client Client) {
	if client == nil {
		return
	}

	var old Client
	h.mu.Lock()
	old = h.clients[client.ID()]
	h.clients[client.ID()] = client
	h.mu.Unlock()

	if old != nil && old != client {
		old.Close(1000, "replaced")
	}
}

func (h *MemoryHub) Unregister(clientID string) {
	h.mu.Lock()
	old := h.clients[clientID]
	delete(h.clients, clientID)
	h.mu.Unlock()

	if old != nil {
		old.Close(1000, "unregistered")
	}
}

func (h *MemoryHub) Client(clientID string) (Client, bool) {
	h.mu.RLock()
	client, ok := h.clients[clientID]
	h.mu.RUnlock()
	return client, ok
}

func (h *MemoryHub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

