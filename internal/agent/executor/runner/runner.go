package runner

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/shared/types"
)

type Job struct {
	TaskID      string
	NodeID      string
	ExecutionID string
	Steps       []types.PlanStep
	TimeoutSec  int
}

type Result struct {
	Status     string
	ExitCode   int
	StdoutTail string
	StderrTail string
	Duration   time.Duration
}

type Runner interface {
	Run(ctx context.Context, job Job) (Result, error)
}

type NoopRunner struct{}

func NewNoopRunner() NoopRunner {
	return NoopRunner{}
}

func (NoopRunner) Run(ctx context.Context, job Job) (Result, error) {
	start := time.Now()
	select {
	case <-ctx.Done():
		return Result{
			Status:     "cancelled",
			ExitCode:   1,
			StdoutTail: "",
			StderrTail: ctx.Err().Error(),
			Duration:   time.Since(start),
		}, ctx.Err()
	case <-time.After(50 * time.Millisecond):
		return Result{
			Status:     "success",
			ExitCode:   0,
			StdoutTail: "noop runner executed placeholder task",
			StderrTail: "",
			Duration:   time.Since(start),
		}, nil
	}
}
