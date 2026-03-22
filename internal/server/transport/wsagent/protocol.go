package wsagent

import (
	"encoding/json"

	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
)

const (
	TypeAgentRegister   = "agent.register"
	TypeAgentHeartbeat  = "agent.heartbeat"
	TypeExecutionChunk  = "execution.chunk"
	TypeExecutionFinish = "execution.finished"
	TypeTaskDispatch    = "task.dispatch"
	TypeAgentAck        = "agent.ack"
	TypeAgentError      = "agent.error"
)

type Message struct {
	Type    string          `json:"type"`
	NodeID  string          `json:"nodeId,omitempty"`
	TaskID  string          `json:"taskId,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type RegisterPayload struct {
	NodeID string `json:"nodeId"`
}

type HeartbeatPayload struct {
	NodeID string `json:"nodeId"`
}

type ChunkPayload struct {
	SessionID   string                `json:"sessionId"`
	TaskID      string                `json:"taskId"`
	ExecutionID string                `json:"executionId"`
	NodeID      string                `json:"nodeId"`
	Chunk       domain.ExecutionChunk `json:"chunk"`
}

type FinishedPayload struct {
	SessionID    string                 `json:"sessionId"`
	TaskID       string                 `json:"taskId"`
	ExecutionID  string                 `json:"executionId"`
	NodeID       string                 `json:"nodeId"`
	Status       domain.ExecutionStatus `json:"status"`
	ExitCode     *int                   `json:"exitCode,omitempty"`
	StatusReason *string                `json:"statusReason,omitempty"`
}

type DispatchCommand = appexecution.DispatchCommand

type Ack struct {
	Type      string `json:"type"`
	NodeID    string `json:"nodeId,omitempty"`
	TaskID    string `json:"taskId,omitempty"`
	Timestamp string `json:"timestamp"`
}

type Error struct {
	Type      string `json:"type"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

func decodePayload[T any](msg Message) (T, error) {
	var out T
	if len(msg.Payload) == 0 {
		return out, nil
	}
	err := json.Unmarshal(msg.Payload, &out)
	return out, err
}
