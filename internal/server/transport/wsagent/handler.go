package wsagent

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/internal/server/app/usecase"
	"github.com/momaek/tolato/internal/server/infra/dispatch"
	"github.com/momaek/tolato/internal/server/infra/presence"
	"github.com/momaek/tolato/internal/server/transport/wsui"
	"github.com/momaek/tolato/internal/shared/protocol"
	"github.com/momaek/tolato/internal/shared/types"
	"go.uber.org/zap"
)

type Handler struct {
	logger            *zap.Logger
	authenticateAgent usecase.AuthenticateAgent
	heartbeatNode     usecase.HeartbeatNode
	getNode           usecase.GetNode
	recordTaskLog     usecase.RecordTaskLog
	recordTaskResult  usecase.RecordTaskResult
	dispatcher        *dispatch.Manager
	presence          *presence.Store
	uiws              *wsui.Handler
	upgrader          websocket.Upgrader
}

func NewHandler(
	logger *zap.Logger,
	authenticateAgent usecase.AuthenticateAgent,
	heartbeatNode usecase.HeartbeatNode,
	getNode usecase.GetNode,
	recordTaskLog usecase.RecordTaskLog,
	recordTaskResult usecase.RecordTaskResult,
	dispatcher *dispatch.Manager,
	presenceStore *presence.Store,
	uiwsHandler *wsui.Handler,
) *Handler {
	return &Handler{
		logger:            logger,
		authenticateAgent: authenticateAgent,
		heartbeatNode:     heartbeatNode,
		getNode:           getNode,
		recordTaskLog:     recordTaskLog,
		recordTaskResult:  recordTaskResult,
		dispatcher:        dispatcher,
		presence:          presenceStore,
		uiws:              uiwsHandler,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node_id")
	secret := r.URL.Query().Get("secret")
	authenticatedNode, err := h.authenticateAgent.Execute(r.Context(), nodeID, secret)
	if err != nil {
		http.Error(w, "unauthorized agent", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("upgrade ws/agent failed", zap.Error(err))
		return
	}
	defer conn.Close()

	currentNode := *authenticatedNode
	if h.dispatcher != nil {
		h.dispatcher.Register(currentNode.ID, conn)
		defer h.dispatcher.Unregister(currentNode.ID, conn)
	}

	for {
		var env protocol.Envelope
		if err := conn.ReadJSON(&env); err != nil {
			h.logger.Info("ws/agent connection closed", zap.Error(err))
			return
		}

		switch env.Type {
		case protocol.TypeHello, protocol.TypeHeartbeat:
			currentNode = h.handlePresence(r, env, currentNode)
		case protocol.TypeTaskAck:
			h.logger.Info("ws/agent task acknowledged", zap.String("task_id", env.TaskID), zap.String("node_id", nodeID))
		case protocol.TypeTaskLog:
			h.handleTaskLog(r, env, currentNode.ID)
		case protocol.TypeTaskResult:
			h.handleTaskResult(r, env, currentNode.ID)
		}

		h.logger.Info("ws/agent message received", zap.String("type", env.Type), zap.String("node_id", nodeID))
	}
}

func (h *Handler) handlePresence(r *http.Request, env protocol.Envelope, currentNode types.Node) types.Node {
	input := usecase.HeartbeatInput{
		NodeID:     currentNode.ID,
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

	if err := h.heartbeatNode.Execute(r.Context(), input); err != nil {
		h.logger.Warn("heartbeat use case failed", zap.Error(err))
	}

	if env.Type == protocol.TypeHeartbeat {
		var payload protocol.HeartbeatPayload
		if err := json.Unmarshal(env.Payload, &payload); err == nil {
			snapshot := presence.Snapshot{
				Busy: payload.Busy,
				Metrics: types.NodeMetrics{
					CPU:    parseMetric(payload.Load),
					Memory: parseMetric(payload.Memory),
					Disk:   parseMetric(payload.Disk),
				},
				LastSeenAt: env.Timestamp.UTC(),
			}
			if h.presence != nil {
				h.presence.Upsert(currentNode.ID, snapshot)
			}

			currentNode.Busy = snapshot.Busy
			currentNode.Metrics = snapshot.Metrics
			currentNode.LastSeenAt = snapshot.LastSeenAt
			currentNode.Status = "online"
			h.broadcastNodeUpdate(currentNode)
		}
	}

	if nodeView, err := h.getNode.Execute(r.Context(), currentNode.ID); err == nil && nodeView != nil {
		currentNode = *nodeView
	}

	return currentNode
}

func (h *Handler) handleTaskLog(r *http.Request, env protocol.Envelope, nodeID string) {
	var payload protocol.TaskLogPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		h.logger.Warn("invalid task.log payload", zap.Error(err))
		return
	}

	resp, err := h.recordTaskLog.Execute(r.Context(), usecase.TaskLogInput{
		TaskID:      env.TaskID,
		ExecutionID: payload.ExecutionID,
		NodeID:      nodeID,
		Stream:      payload.Stream,
		Chunk:       payload.Chunk,
		Timestamp:   env.Timestamp.UTC(),
	})
	if err != nil {
		h.logger.Warn("record task log failed", zap.Error(err), zap.String("task_id", env.TaskID))
		return
	}

	if h.uiws != nil {
		h.uiws.BroadcastTaskStatus(resp.Task.ID, resp.Task.FinalStatus, env.Timestamp.UTC())
		h.uiws.BroadcastTaskLog(resp.Task.ID, payload.ExecutionID, nodeID, payload.Stream, payload.Chunk, env.Timestamp.UTC())
	}
}

func (h *Handler) handleTaskResult(r *http.Request, env protocol.Envelope, nodeID string) {
	var payload protocol.TaskResultPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		h.logger.Warn("invalid task.result payload", zap.Error(err))
		return
	}

	resp, err := h.recordTaskResult.Execute(r.Context(), usecase.TaskResultInput{
		TaskID:      env.TaskID,
		ExecutionID: payload.ExecutionID,
		NodeID:      nodeID,
		Status:      payload.Status,
		ExitCode:    payload.ExitCode,
		StdoutTail:  payload.StdoutTail,
		StderrTail:  payload.StderrTail,
		Timestamp:   env.Timestamp.UTC(),
	})
	if err != nil {
		h.logger.Warn("record task result failed", zap.Error(err), zap.String("task_id", env.TaskID))
		return
	}

	if h.uiws != nil {
		h.uiws.BroadcastTaskStatus(resp.Task.ID, resp.Task.FinalStatus, env.Timestamp.UTC())
		h.uiws.BroadcastTaskResult(resp.Task.ID, types.TaskExecution{
			ID:           payload.ExecutionID,
			TaskID:       resp.Task.ID,
			NodeID:       nodeID,
			Status:       payload.Status,
			StartedAt:    env.Timestamp.UTC(),
			FinishedAt:   env.Timestamp.UTC(),
			ExitCode:     payload.ExitCode,
			StdoutTail:   payload.StdoutTail,
			StderrTail:   payload.StderrTail,
			StatusReason: payload.Status,
		}, env.Timestamp.UTC())
	}
}

func (h *Handler) broadcastNodeUpdate(node types.Node) {
	if h.uiws == nil {
		return
	}
	h.uiws.BroadcastNodeUpdated(node)
}

func parseMetric(raw string) float64 {
	value := strings.TrimSpace(strings.TrimSuffix(raw, "%"))
	if value == "" {
		return 0
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}
