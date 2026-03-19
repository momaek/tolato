package dispatch

import (
	"context"
	"encoding/json"

	"github.com/momaek/tolato/internal/agent/executor/runner"
	"github.com/momaek/tolato/internal/agent/transport/wsclient"
	"github.com/momaek/tolato/internal/shared/protocol"
	"go.uber.org/zap"
)

type Loop struct {
	Logger   *zap.Logger
	Incoming <-chan protocol.Envelope
	Queue    chan<- runner.Job
	WSClient wsclient.Client
}

func (l Loop) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case env := <-l.Incoming:
			if env.Type != protocol.TypeTaskDispatch {
				continue
			}

			var payload protocol.TaskDispatchPayload
			if err := json.Unmarshal(env.Payload, &payload); err != nil {
				l.Logger.Warn("invalid task.dispatch payload", zap.Error(err))
				continue
			}

			job := runner.Job{
				TaskID:      env.TaskID,
				NodeID:      env.NodeID,
				ExecutionID: payload.ExecutionID,
				Steps:       payload.Steps,
				TimeoutSec:  payload.TimeoutSec,
			}

			ackEnv, err := protocol.NewEnvelope(protocol.TypeTaskAck, env.TaskID, env.NodeID, env.Seq+1, protocol.TaskAckPayload{
				ExecutionID: payload.ExecutionID,
				Accepted:    true,
			})
			if err == nil {
				_ = l.WSClient.Send(ctx, ackEnv)
			}

			select {
			case l.Queue <- job:
			default:
				l.Logger.Warn("dispatch queue is full", zap.String("execution_id", job.ExecutionID))
			}
		}
	}
}
