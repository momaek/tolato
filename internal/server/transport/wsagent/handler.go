package wsagent

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/internal/server/app/usecase"
	"github.com/momaek/tolato/internal/shared/protocol"
	"go.uber.org/zap"
)

type Handler struct {
	logger            *zap.Logger
	authenticateAgent usecase.AuthenticateAgent
	heartbeatNode     usecase.HeartbeatNode
	upgrader          websocket.Upgrader
}

func NewHandler(logger *zap.Logger, authenticateAgent usecase.AuthenticateAgent, heartbeatNode usecase.HeartbeatNode) *Handler {
	return &Handler{
		logger:            logger,
		authenticateAgent: authenticateAgent,
		heartbeatNode:     heartbeatNode,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node_id")
	secret := r.URL.Query().Get("secret")
	if _, err := h.authenticateAgent.Execute(r.Context(), nodeID, secret); err != nil {
		http.Error(w, "unauthorized agent", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("upgrade ws/agent failed", zap.Error(err))
		return
	}
	defer conn.Close()

	for {
		var env protocol.Envelope
		if err := conn.ReadJSON(&env); err != nil {
			h.logger.Info("ws/agent connection closed", zap.Error(err))
			return
		}

		input := usecase.HeartbeatInput{
			NodeID:     nodeID,
			RemoteAddr: r.RemoteAddr,
		}

		if env.Type == protocol.TypeHello {
			var payload protocol.HelloPayload
			if err := json.Unmarshal(env.Payload, &payload); err == nil {
				input.SessionID = payload.SessionID
				input.AgentVersion = payload.AgentVersion
				input.Capabilities = payload.Capabilities
				h.logger.Debug("agent hello payload", zap.Any("payload", payload))
			}
		}

		if env.Type == protocol.TypeHeartbeat {
			input.SessionID = r.URL.Query().Get("session_id")
		}

		if env.Type == protocol.TypeHeartbeat || env.Type == protocol.TypeHello {
			if err := h.heartbeatNode.Execute(r.Context(), input); err != nil {
				h.logger.Warn("heartbeat use case failed", zap.Error(err))
			}
		}

		h.logger.Info("ws/agent message received", zap.String("type", env.Type), zap.String("node_id", nodeID))
	}
}
