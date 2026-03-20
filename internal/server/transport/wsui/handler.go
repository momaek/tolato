package wsui

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/internal/shared/types"
	"go.uber.org/zap"
)

type Handler struct {
	logger   *zap.Logger
	upgrader websocket.Upgrader

	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

func NewHandler(logger *zap.Logger) *Handler {
	return &Handler{
		logger: logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		clients: make(map[*websocket.Conn]struct{}),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("upgrade ws/ui failed", zap.Error(err))
		return
	}
	defer conn.Close()

	h.register(conn)
	defer h.unregister(conn)

	h.logger.Info("ws/ui client connected", zap.String("remote_addr", r.RemoteAddr))
	h.writeJSON(conn, map[string]any{
		"type":      "connection.synced",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *Handler) BroadcastNodeUpdated(node types.Node) {
	h.broadcast(map[string]any{
		"type": "node.updated",
		"node": map[string]any{
			"id":       node.ID,
			"hostname": node.Hostname,
			"region":   node.Region,
			"os":       node.OS,
			"version":  node.Version,
			"tags":     node.Tags,
			"status":   node.Status,
			"busy":     node.Busy,
			"lastSeen": node.LastSeenAt.UTC().Format(time.RFC3339),
			"metrics": map[string]float64{
				"cpu":    node.Metrics.CPU,
				"memory": node.Metrics.Memory,
				"disk":   node.Metrics.Disk,
			},
		},
	})
}

func (h *Handler) BroadcastTaskStatus(taskID, status string, at time.Time) {
	h.broadcast(map[string]any{
		"type":      "task.status",
		"taskId":    taskID,
		"status":    status,
		"timestamp": at.UTC().Format(time.RFC3339),
	})
}

func (h *Handler) BroadcastTaskLog(taskID, executionID, nodeID, stream, chunk string, at time.Time) {
	h.broadcast(map[string]any{
		"type":        "task.log",
		"taskId":      taskID,
		"executionId": executionID,
		"nodeId":      nodeID,
		"stream":      stream,
		"chunk":       chunk,
		"timestamp":   at.UTC().Format(time.RFC3339),
	})
}

func (h *Handler) register(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = struct{}{}
}

func (h *Handler) unregister(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, conn)
}

func (h *Handler) broadcast(payload any) {
	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for conn := range h.clients {
		clients = append(clients, conn)
	}
	h.mu.RUnlock()

	for _, conn := range clients {
		if err := h.writeJSON(conn, payload); err != nil {
			h.logger.Debug("ws/ui broadcast failed", zap.Error(err))
			_ = conn.Close()
			h.unregister(conn)
		}
	}
}

func (h *Handler) writeJSON(conn *websocket.Conn, payload any) error {
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return conn.WriteJSON(payload)
}
