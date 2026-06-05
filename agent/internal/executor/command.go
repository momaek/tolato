package executor

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"time"
)

// CommandResult holds the result of a shell command execution.
type CommandResult struct {
	ExitCode   int
	Stdout     string
	Stderr     string
	DurationMS int64
}

// Executor runs shell commands.
type Executor struct{}

// NewExecutor creates a new Executor.
func NewExecutor() *Executor {
	return &Executor{}
}

// waitDelay bounds how long Wait() blocks on the output pipes after the
// foreground process has exited (or been killed). This is what lets commands
// that spawn background processes (e.g. `foo & echo done`) return promptly:
// the background child inherits the stdout/stderr pipe, so without a bound,
// Wait() would block until that child also exits.
const waitDelay = 2 * time.Second

// Execute runs a shell command with the given timeout (in seconds).
// If timeout <= 0, a default of 300 seconds is used.
func (e *Executor) Execute(ctx context.Context, command string, timeout int) *CommandResult {
	if timeout <= 0 {
		timeout = 300
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}

	// Run in a dedicated process group so we can clean up the whole tree
	// (including backgrounded grandchildren) on timeout/cancel.
	setProcGroup(cmd)
	cmd.Cancel = func() error { return killProcessGroup(cmd) }
	// Bound the wait on inherited output pipes so a backgrounded child can't
	// keep us blocked indefinitely after the foreground command finishes.
	cmd.WaitDelay = waitDelay

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start).Milliseconds()

	result := &CommandResult{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMS: duration,
	}

	switch {
	case err == nil:
		result.ExitCode = 0
	case ctx.Err() == context.DeadlineExceeded:
		result.ExitCode = -1
		result.Stderr = "command timeout"
	case errors.Is(err, exec.ErrWaitDelay):
		// Foreground command completed; a backgrounded child kept the output
		// pipe open. Treat the foreground exit status as the result.
		if cmd.ProcessState != nil {
			result.ExitCode = cmd.ProcessState.ExitCode()
		}
	default:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			result.Stderr = err.Error()
		}
	}

	return result
}
