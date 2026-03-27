package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
)

type ChunkEmitter interface {
	Emit(stream domain.ExecutionStream, text string) error
}

type ExecutionResult struct {
	Status       domain.ExecutionStatus
	ExitCode     *int
	StatusReason *string
}

type Executor interface {
	Execute(ctx context.Context, cmd appexecution.DispatchCommand, emitter ChunkEmitter) ExecutionResult
}

type CommandSpec struct {
	Name string
	Args []string
}

type CommandRunError struct {
	ExitCode int
	Err      error
}

func (e *CommandRunError) Error() string {
	if e == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *CommandRunError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type CommandRunner func(ctx context.Context, spec CommandSpec, emit func(stream domain.ExecutionStream, text string) error) error

type LocalExecutor struct {
	NodeID     string
	Hostname   string
	Timeout    time.Duration
	RunCommand CommandRunner
}

func (e *LocalExecutor) Execute(ctx context.Context, cmd appexecution.DispatchCommand, emitter ChunkEmitter) ExecutionResult {
	if cmd.Action != "run_command" {
		return failureResult(64, fmt.Sprintf("unsupported action %q", cmd.Action))
	}

	var args appexecution.RunCommandArgs
	if len(cmd.Args) > 0 {
		if err := json.Unmarshal(cmd.Args, &args); err != nil {
			return failureResult(64, "invalid dispatch args")
		}
	}
	if strings.TrimSpace(args.Command) == "" {
		return failureResult(64, "dispatch command is required")
	}

	runCtx := ctx
	cancel := func() {}
	if e.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, e.Timeout)
	}
	defer cancel()

	commandLine := strings.TrimSpace(strings.Join(append([]string{args.Command}, args.Args...), " "))
	if err := emitter.Emit(domain.ExecutionStreamStdout, fmt.Sprintf("[nodeagent] node=%s command=%s\n", e.nodeLabel(), commandLine)); err != nil {
		return failureResult(70, err.Error())
	}

	if err := e.runner()(runCtx, CommandSpec{Name: args.Command, Args: args.Args}, func(stream domain.ExecutionStream, text string) error {
		return emitter.Emit(stream, text)
	}); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(runCtx.Err(), context.DeadlineExceeded):
			return timeoutResult("command timed out")
		default:
			var runErr *CommandRunError
			if errors.As(err, &runErr) {
				return failureResult(runErr.ExitCode, runErr.Error())
			}
			return failureResult(1, err.Error())
		}
	}

	exitCode := 0
	return ExecutionResult{
		Status:   domain.ExecutionStatusSuccess,
		ExitCode: &exitCode,
	}
}

func (e *LocalExecutor) runner() CommandRunner {
	if e.RunCommand != nil {
		return e.RunCommand
	}
	return streamLocalCommand
}

func (e *LocalExecutor) nodeLabel() string {
	if strings.TrimSpace(e.Hostname) != "" {
		return e.Hostname
	}
	if strings.TrimSpace(e.NodeID) != "" {
		return e.NodeID
	}
	return "unknown"
}

func streamLocalCommand(ctx context.Context, spec CommandSpec, emit func(stream domain.ExecutionStream, text string) error) error {
	cmd := exec.CommandContext(ctx, spec.Name, spec.Args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &CommandRunError{ExitCode: 1, Err: err}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return &CommandRunError{ExitCode: 1, Err: err}
	}

	if err := cmd.Start(); err != nil {
		exitCode := 1
		if errors.Is(err, exec.ErrNotFound) {
			exitCode = 127
		}
		return &CommandRunError{ExitCode: exitCode, Err: err}
	}

	var wg sync.WaitGroup
	var streamErr error
	var streamErrMu sync.Mutex
	recordStreamErr := func(err error) {
		if err == nil {
			return
		}
		streamErrMu.Lock()
		if streamErr == nil {
			streamErr = err
		}
		streamErrMu.Unlock()
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		recordStreamErr(copyStream(stdout, domain.ExecutionStreamStdout, emit))
	}()
	go func() {
		defer wg.Done()
		recordStreamErr(copyStream(stderr, domain.ExecutionStreamStderr, emit))
	}()

	waitErr := cmd.Wait()
	wg.Wait()
	if streamErr != nil {
		return streamErr
	}
	if waitErr != nil {
		if errors.Is(waitErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return &CommandRunError{ExitCode: exitErr.ExitCode(), Err: waitErr}
		}
		return &CommandRunError{ExitCode: 1, Err: waitErr}
	}
	return nil
}

func copyStream(reader io.Reader, stream domain.ExecutionStream, emit func(stream domain.ExecutionStream, text string) error) error {
	buf := make([]byte, 512)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if emitErr := emit(stream, string(buf[:n])); emitErr != nil {
				return emitErr
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

func failureResult(exitCode int, reason string) ExecutionResult {
	code := exitCode
	return ExecutionResult{
		Status:       domain.ExecutionStatusFailed,
		ExitCode:     &code,
		StatusReason: strPtr(reason),
	}
}

func timeoutResult(reason string) ExecutionResult {
	code := 124
	return ExecutionResult{
		Status:       domain.ExecutionStatusTimeout,
		ExitCode:     &code,
		StatusReason: strPtr(reason),
	}
}

func strPtr(v string) *string {
	return &v
}
