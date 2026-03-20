package execution

import (
	"context"
	"errors"
	"time"

	"github.com/momaek/tolato/internal/agent/executor/runner"
	validatorpkg "github.com/momaek/tolato/internal/agent/executor/validator"
	"github.com/momaek/tolato/internal/agent/infra/agentstate"
	"github.com/momaek/tolato/internal/agent/infra/cancellation"
	"github.com/momaek/tolato/internal/agent/transport/wsclient"
	"github.com/momaek/tolato/internal/shared/protocol"
	"go.uber.org/zap"
)

type Loop struct {
	Logger    *zap.Logger
	NodeID    string
	Queue     <-chan runner.Job
	Cancel    <-chan runner.CancelRequest
	Runner    runner.Runner
	Validator validatorpkg.Validator
	WSClient  wsclient.Client
	Busy      *agentstate.BusyTracker
	Cancels   *cancellation.Store
}

func (l Loop) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case cancelReq := <-l.Cancel:
			l.markCancelled(cancelReq)
		case job := <-l.Queue:
			if reason, ok := l.consumeQueuedCancellation(job.ExecutionID); ok {
				l.sendResult(ctx, job, runner.Result{
					Status:     "cancelled",
					ExitCode:   130,
					StdoutTail: "",
					StderrTail: reason,
					Duration:   0,
				})
				continue
			}

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
			l.executeJob(ctx, job)
		}
	}
}

func (l Loop) executeJob(ctx context.Context, job runner.Job) {
	defer func() {
		if l.Busy != nil {
			l.Busy.Done()
		}
	}()

	runCtx, cancel := context.WithCancel(ctx)
	if job.TimeoutSec > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(job.TimeoutSec)*time.Second)
	}
	defer cancel()

	logEnv, err := protocol.NewEnvelope(protocol.TypeTaskLog, job.TaskID, l.NodeID, time.Now().UnixNano(), protocol.TaskLogPayload{
		ExecutionID: job.ExecutionID,
		Stream:      "stdout",
		Chunk:       "runner accepted task and started execution",
	})
	if err == nil {
		_ = l.WSClient.Send(ctx, logEnv)
	}

	type outcome struct {
		result runner.Result
		err    error
	}

	resultCh := make(chan outcome, 1)
	go func() {
		result, runErr := l.Runner.Run(runCtx, job)
		resultCh <- outcome{result: result, err: runErr}
	}()

	for {
		select {
		case <-ctx.Done():
			cancel()
			return
		case cancelReq := <-l.Cancel:
			l.markCancelled(cancelReq)
			if cancelReq.ExecutionID == job.ExecutionID {
				cancel()
			}
		case out := <-resultCh:
			if out.err != nil && !errors.Is(out.err, context.Canceled) && !errors.Is(out.err, context.DeadlineExceeded) {
				l.Logger.Warn("job execution finished with error", zap.Error(out.err), zap.String("execution_id", job.ExecutionID))
			}

			if reason, ok := l.consumeQueuedCancellation(job.ExecutionID); ok && out.result.Status != "timeout" {
				out.result.Status = "cancelled"
				out.result.ExitCode = 130
				if out.result.StderrTail == "" {
					out.result.StderrTail = reason
				}
			}

			l.sendResult(ctx, job, out.result)
			return
		}
	}
}

func (l Loop) sendResult(ctx context.Context, job runner.Job, result runner.Result) {
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
}

func (l Loop) markCancelled(cancelReq runner.CancelRequest) {
	if l.Cancels != nil {
		l.Cancels.Mark(cancelReq.ExecutionID, cancelReq.Reason)
	}
}

func (l Loop) consumeQueuedCancellation(executionID string) (string, bool) {
	if l.Cancels == nil {
		return "", false
	}
	return l.Cancels.Consume(executionID)
}
