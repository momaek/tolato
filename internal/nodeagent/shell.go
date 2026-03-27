package nodeagent

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/momaek/tolato/internal/server/domain"
)

// ShellSession manages a single PTY shell session bound to an execution.
type ShellSession struct {
	ptmx    *os.File
	cmd     *exec.Cmd
	emitter ChunkEmitter

	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

// StartShell allocates a PTY, starts the given shell, and begins streaming
// output to the emitter. It blocks until the shell process exits or ctx is
// cancelled. The caller should run this in a goroutine.
func StartShell(ctx context.Context, shell string, rows, cols int, emitter ChunkEmitter) (*ShellSession, error) {
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
	}

	cmd := exec.CommandContext(ctx, shell)
	cmd.Env = os.Environ()

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}

	s := &ShellSession{
		ptmx:    ptmx,
		cmd:     cmd,
		emitter: emitter,
		done:    make(chan struct{}),
	}

	go s.readLoop()
	return s, nil
}

// Write sends user input to the shell's PTY.
func (s *ShellSession) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("shell session closed")
	}
	_, err := s.ptmx.Write(data)
	return err
}

// WriteBase64 decodes base64 data and writes it to the shell.
func (s *ShellSession) WriteBase64(encoded string) error {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}
	return s.Write(data)
}

// Resize changes the PTY window size.
func (s *ShellSession) Resize(rows, cols int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("shell session closed")
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

// Wait blocks until the shell process exits and returns the result.
func (s *ShellSession) Wait() ExecutionResult {
	<-s.done

	err := s.cmd.Wait()
	s.Close()

	if err == nil {
		exitCode := 0
		return ExecutionResult{
			Status:   domain.ExecutionStatusSuccess,
			ExitCode: &exitCode,
		}
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return failureResult(exitErr.ExitCode(), err.Error())
	}
	return failureResult(1, err.Error())
}

// Close terminates the PTY and shell process.
func (s *ShellSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	_ = s.ptmx.Close()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

func (s *ShellSession) readLoop() {
	defer close(s.done)

	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			// Encode as base64 to safely transport binary terminal output.
			encoded := base64.StdEncoding.EncodeToString(buf[:n])
			if emitErr := s.emitter.Emit(domain.ExecutionStreamStdout, encoded); emitErr != nil {
				return
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				_ = s.emitter.Emit(domain.ExecutionStreamStderr, err.Error())
			}
			return
		}
	}
}
