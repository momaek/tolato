package wsagent

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
	"github.com/momaek/tolato/internal/server/transport/ginws"
)

type ExecutionService interface {
	RecordChunk(ctx context.Context, input appexecution.RecordChunkInput) error
	FinishExecution(ctx context.Context, input appexecution.FinishExecutionInput) error
}

type Dispatcher struct {
	Agents     infraws.AgentRegistry
	Executions ExecutionService
	Now        func() time.Time
}

func (d Dispatcher) Dispatch(ctx context.Context, raw []byte) ([]byte, error) {
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		return json.Marshal(Error{
			Type:      TypeAgentError,
			Code:      "bad_request",
			Message:   err.Error(),
			Timestamp: d.now().UTC().Format(time.RFC3339),
		})
	}

	clientID, ok := ClientIDFromContext(ctx)
	if !ok {
		return nil, errors.New("ws/agent client id is missing from context")
	}

	switch msg.Type {
	case TypeAgentRegister:
		if d.Agents == nil {
			return nil, errors.New("agent registry is not configured")
		}
		payload, err := decodePayload[RegisterPayload](msg)
		if err != nil {
			return nil, err
		}
		if payload.NodeID == "" {
			return nil, errors.New("nodeId is required")
		}
		meta := payload.Metadata
		if meta.IPAddress == "" {
			meta.IPAddress = clientIPFromContext(ctx)
		}
		d.Agents.BindNode(payload.NodeID, clientID, meta)
		return json.Marshal(Ack{
			Type:      TypeAgentAck,
			NodeID:    payload.NodeID,
			Timestamp: d.now().UTC().Format(time.RFC3339),
		})

	case TypeAgentHeartbeat:
		if d.Agents == nil {
			return nil, errors.New("agent registry is not configured")
		}
		payload, err := decodePayload[HeartbeatPayload](msg)
		if err != nil {
			return nil, err
		}
		nodeID := payload.NodeID
		if nodeID == "" {
			nodeID = msg.NodeID
		}
		if nodeID == "" {
			return nil, errors.New("nodeId is required")
		}
		if err := d.Agents.Heartbeat(nodeID, clientID, payload.Runtime, d.now()); err != nil {
			return nil, err
		}
		return json.Marshal(Ack{
			Type:      TypeAgentAck,
			NodeID:    nodeID,
			Timestamp: d.now().UTC().Format(time.RFC3339),
		})

	case TypeExecutionChunk:
		if d.Executions == nil {
			return nil, errors.New("execution service is not configured")
		}
		payload, err := decodePayload[ChunkPayload](msg)
		if err != nil {
			return nil, err
		}
		if err := d.Executions.RecordChunk(ctx, appexecution.RecordChunkInput{
			SessionID:   payload.SessionID,
			TaskID:      payload.TaskID,
			ExecutionID: payload.ExecutionID,
			NodeID:      payload.NodeID,
			Chunk:       payload.Chunk,
		}); err != nil {
			return nil, err
		}
		return json.Marshal(Ack{
			Type:      TypeAgentAck,
			NodeID:    payload.NodeID,
			TaskID:    payload.TaskID,
			Timestamp: d.now().UTC().Format(time.RFC3339),
		})

	case TypeExecutionFinish:
		if d.Executions == nil {
			return nil, errors.New("execution service is not configured")
		}
		payload, err := decodePayload[FinishedPayload](msg)
		if err != nil {
			return nil, err
		}
		if err := d.Executions.FinishExecution(ctx, appexecution.FinishExecutionInput{
			SessionID:    payload.SessionID,
			TaskID:       payload.TaskID,
			ExecutionID:  payload.ExecutionID,
			NodeID:       payload.NodeID,
			Status:       payload.Status,
			ExitCode:     payload.ExitCode,
			StatusReason: payload.StatusReason,
		}); err != nil {
			return nil, err
		}
		return json.Marshal(Ack{
			Type:      TypeAgentAck,
			NodeID:    payload.NodeID,
			TaskID:    payload.TaskID,
			Timestamp: d.now().UTC().Format(time.RFC3339),
		})

	default:
		return json.Marshal(Error{
			Type:      TypeAgentError,
			Code:      "unknown_type",
			Message:   "unsupported ws/agent message type",
			Timestamp: d.now().UTC().Format(time.RFC3339),
		})
	}
}

func (d Dispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func clientIPFromContext(ctx context.Context) string {
	req, ok := ginws.HTTPRequestFromContext(ctx)
	if !ok || req == nil {
		return ""
	}
	// Check common reverse-proxy headers first.
	for _, header := range []string{"X-Forwarded-For", "X-Real-Ip"} {
		if value := req.Header.Get(header); value != "" {
			// X-Forwarded-For may be comma-separated; take the first.
			if ip, _, ok := strings.Cut(value, ","); ok {
				return strings.TrimSpace(ip)
			}
			return strings.TrimSpace(value)
		}
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}
