package executor

import (
	"bytes"
	"context"
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

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
			result.Stderr = "command timeout"
			return result
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			result.Stderr = err.Error()
		}
	}

	return result
}
