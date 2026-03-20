package execution

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/agent/executor/runner"
	validatorpkg "github.com/momaek/tolato/internal/agent/executor/validator"
	"github.com/momaek/tolato/internal/agent/infra/agentstate"
	"github.com/momaek/tolato/internal/agent/transport/wsclient"
	"github.com/momaek/tolato/internal/shared/protocol"
	"go.uber.org/zap"
)

type Loop struct {
	Logger    *zap.Logger
	NodeID    string
	Queue     <-chan runner.Job
	Runner    runner.Runner
	Validator validatorpkg.Validator
	WSClient  wsclient.Client
	Busy      *agentstate.BusyTracker
}

func (l Loop) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case job := <-l.Queue:
			if l.Busy != nil {
				l.Busy.Start()
			}
			if err := l.Validator.Validate(job); err != nil {
				if l.Busy != nil {
					l.Busy.Done()
				}
				l.Logger.Warn("job validation failed", zap.Error(err), zap.String("execution_id", job.ExecutionID))
				continue
			}
			func() {
				defer func() {
					if l.Busy != nil {
						l.Busy.Done()
					}
				}()

				runCtx := ctx
				if job.TimeoutSec > 0 {
					var cancel context.CancelFunc
					runCtx, cancel = context.WithTimeout(ctx, time.Duration(job.TimeoutSec)*time.Second)
					defer cancel()
				}

				logEnv, err := protocol.NewEnvelope(protocol.TypeTaskLog, job.TaskID, l.NodeID, time.Now().UnixNano(), protocol.TaskLogPayload{
					ExecutionID: job.ExecutionID,
					Stream:      "stdout",
					Chunk:       "noop runner is executing placeholder task",
				})
				if err == nil {
					_ = l.WSClient.Send(ctx, logEnv)
				}

				result, err := l.Runner.Run(runCtx, job)
				if err != nil {
					l.Logger.Warn("job execution finished with error", zap.Error(err))
				}

				resultEnv, envErr := protocol.NewEnvelope(protocol.TypeTaskResult, job.TaskID, l.NodeID, time.Now().UnixNano(), protocol.TaskResultPayload{
					ExecutionID: job.ExecutionID,
					Status:      result.Status,
					ExitCode:    result.ExitCode,
					StdoutTail:  result.StdoutTail,
					StderrTail:  result.StderrTail,
					DurationMS:  result.Duration.Milliseconds(),
				})
				if envErr == nil {
					_ = l.WSClient.Send(ctx, resultEnv)
				}
			}()
		}
	}
}
