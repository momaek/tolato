package wsui

import (
	"net/http"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type Handler struct {
	logger   *zap.Logger
	upgrader websocket.Upgrader
}

func NewHandler(logger *zap.Logger) *Handler {
	return &Handler{
		logger: logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("upgrade ws/ui failed", zap.Error(err))
		return
	}
	defer conn.Close()

	h.logger.Info("ws/ui client connected", zap.String("remote_addr", r.RemoteAddr))
	_ = conn.WriteJSON(map[string]any{
		"type":    "welcome",
		"message": "ws/ui placeholder connected",
	})
}
