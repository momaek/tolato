// Package terminal wraps a locally-spawned PTY so it can be exposed over the
// agent WebSocket to the server.
package terminal

import (
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
)

// Session wraps a running PTY + the subprocess glued to it.
// Exactly one reader goroutine pumps bytes off the PTY onto OutputCh.
// Exactly one Close call tears everything down.
type Session struct {
	pty    ptyFile
	cmd    *exec.Cmd
	output chan []byte

	closeOnce sync.Once
	closed    chan struct{}

	exitErr error
	exitMu  sync.Mutex
}

// ptyFile is the subset of *os.File we use. Extracted for platform shims.
type ptyFile interface {
	io.ReadWriteCloser
}

// Output returns the read-only side of the PTY-output channel.
func (s *Session) Output() <-chan []byte {
	return s.output
}

// Closed returns a channel closed when the session ends (process exit or Close).
func (s *Session) Closed() <-chan struct{} {
	return s.closed
}

// Write feeds bytes into the PTY (stdin of the child).
func (s *Session) Write(p []byte) (int, error) {
	return s.pty.Write(p)
}

// Close tears down the PTY and process.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		_ = s.pty.Close()
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		close(s.closed)
	})
}

// ExitError returns the error reported by the subprocess after it exited (if any).
func (s *Session) ExitError() error {
	s.exitMu.Lock()
	defer s.exitMu.Unlock()
	return s.exitErr
}

// ExitCode extracts the child exit code from the (possibly-nil) exit error.
func (s *Session) ExitCode() int {
	err := s.ExitError()
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

// pickShell returns a sensible default shell for the current OS.
func pickShell() string {
	switch runtime.GOOS {
	case "windows":
		return "cmd"
	default:
		if s := os.Getenv("SHELL"); s != "" {
			return s
		}
		return "/bin/bash"
	}
}
