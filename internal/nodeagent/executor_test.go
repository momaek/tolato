package nodeagent

import (
	"context"
	"errors"
	"testing"
	"time"

	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
)

func TestLocalExecutorSelectsControlledCommandSet(t *testing.T) {
	t.Parallel()

	var specs []CommandSpec
	executor := &LocalExecutor{
		NodeID:   "node-1",
		Hostname: "node-1",
		Timeout:  time.Second,
		RunCommand: func(ctx context.Context, spec CommandSpec, emit func(stream domain.ExecutionStream, text string) error) error {
			_ = ctx
			specs = append(specs, spec)
			return emit(domain.ExecutionStreamStdout, spec.Name+"\n")
		},
	}

	result := executor.Execute(context.Background(), appexecution.DispatchCommand{
		Action: "run_command",
		Args:   []byte(`{"command":"df","args":["-h"]}`),
	}, captureEmitter{})
	if result.Status != domain.ExecutionStatusSuccess {
		t.Fatalf("Execute() status = %q, want success", result.Status)
	}
	if len(specs) != 1 || specs[0].Name != "df" || len(specs[0].Args) != 1 || specs[0].Args[0] != "-h" {
		t.Fatalf("command specs = %#v, want single dispatched command", specs)
	}
}

func TestLocalExecutorRejectsUnsupportedAction(t *testing.T) {
	t.Parallel()

	executor := &LocalExecutor{}
	result := executor.Execute(context.Background(), appexecution.DispatchCommand{
		Action: "unsupported",
	}, captureEmitter{})
	if result.Status != domain.ExecutionStatusFailed || result.StatusReason == nil {
		t.Fatalf("Execute() = %#v", result)
	}
}

func TestLocalExecutorMapsTimeouts(t *testing.T) {
	t.Parallel()

	executor := &LocalExecutor{
		Timeout: time.Second,
		RunCommand: func(ctx context.Context, spec CommandSpec, emit func(stream domain.ExecutionStream, text string) error) error {
			_ = spec
			_ = emit
			return context.DeadlineExceeded
		},
	}

	result := executor.Execute(context.Background(), appexecution.DispatchCommand{
		Action: "run_command",
		Args:   []byte(`{"command":"uptime"}`),
	}, captureEmitter{})
	if result.Status != domain.ExecutionStatusTimeout {
		t.Fatalf("Execute() status = %q, want timeout", result.Status)
	}
}

func TestLocalExecutorMapsExitCodes(t *testing.T) {
	t.Parallel()

	executor := &LocalExecutor{
		RunCommand: func(ctx context.Context, spec CommandSpec, emit func(stream domain.ExecutionStream, text string) error) error {
			_ = ctx
			_ = spec
			_ = emit
			return &CommandRunError{ExitCode: 42, Err: errors.New("boom")}
		},
	}

	result := executor.Execute(context.Background(), appexecution.DispatchCommand{
		Action: "run_command",
		Args:   []byte(`{"command":"uptime"}`),
	}, captureEmitter{})
	if result.Status != domain.ExecutionStatusFailed || result.ExitCode == nil || *result.ExitCode != 42 {
		t.Fatalf("Execute() = %#v", result)
	}
}

type captureEmitter struct{}

func (captureEmitter) Emit(stream domain.ExecutionStream, text string) error {
	_ = stream
	_ = text
	return nil
}
