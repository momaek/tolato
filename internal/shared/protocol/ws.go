package protocol

import (
	"encoding/json"
	"time"

	"github.com/momaek/tolato/internal/shared/types"
)

const (
	TypeHello        = "hello"
	TypeHeartbeat    = "heartbeat"
	TypeTaskDispatch = "task.dispatch"
	TypeTaskAck      = "task.ack"
	TypeTaskLog      = "task.log"
	TypeTaskResult   = "task.result"
	TypeTaskCancel   = "task.cancel"
	TypeError        = "error"
)

type Envelope struct {
	Type      string          `json:"type"`
	TaskID    string          `json:"task_id,omitempty"`
	NodeID    string          `json:"node_id"`
	Seq       int64           `json:"seq"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type HelloPayload struct {
	SessionID    string   `json:"session_id"`
	AgentVersion string   `json:"agent_version"`
	Capabilities []string `json:"capabilities"`
}

type HeartbeatPayload struct {
	Hostname string `json:"hostname"`
	Load     string `json:"load"`
	Memory   string `json:"memory"`
	Disk     string `json:"disk"`
	Busy     bool   `json:"busy"`
}

type TaskDispatchPayload struct {
	ExecutionID string           `json:"execution_id"`
	Steps       []types.PlanStep `json:"steps"`
	TimeoutSec  int              `json:"timeout_sec"`
}

type TaskAckPayload struct {
	ExecutionID string `json:"execution_id"`
	Accepted    bool   `json:"accepted"`
	Reason      string `json:"reason,omitempty"`
}

type TaskLogPayload struct {
	ExecutionID string `json:"execution_id"`
	Stream      string `json:"stream"`
	Chunk       string `json:"chunk"`
}

type TaskResultPayload struct {
	ExecutionID string `json:"execution_id"`
	Status      string `json:"status"`
	ExitCode    int    `json:"exit_code"`
	StdoutTail  string `json:"stdout_tail"`
	StderrTail  string `json:"stderr_tail"`
	DurationMS  int64  `json:"duration_ms"`
}

type TaskCancelPayload struct {
	ExecutionID string `json:"execution_id"`
	Reason      string `json:"reason"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewEnvelope(msgType, taskID, nodeID string, seq int64, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}

	return Envelope{
		Type:      msgType,
		TaskID:    taskID,
		NodeID:    nodeID,
		Seq:       seq,
		Timestamp: time.Now().UTC(),
		Payload:   raw,
	}, nil
}
