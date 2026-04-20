//go:build !windows

package terminal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// Start launches a shell inside a new PTY with the given initial size.
// cwd / shell are optional — empty strings pick sensible defaults.
func Start(shell, cwd string, cols, rows uint16) (*Session, error) {
	if shell == "" {
		shell = pickShell()
	}
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	cmd := exec.Command(shell, "-l")
	if cwd != "" {
		cmd.Dir = cwd
	}
	// Preserve a clean environment for the shell.
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}

	s := &Session{
		pty:    ptmx,
		cmd:    cmd,
		output: make(chan []byte, 64),
		closed: make(chan struct{}),
	}

	// Reader goroutine: pump PTY output onto s.output until EOF / close.
	go func() {
		defer close(s.output)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				select {
				case s.output <- chunk:
				case <-s.closed:
					return
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					// Already closed or genuine read failure.
				}
				return
			}
		}
	}()

	// Waiter goroutine: capture exit, trigger close once child dies.
	go func() {
		err := cmd.Wait()
		s.exitMu.Lock()
		s.exitErr = err
		s.exitMu.Unlock()
		s.Close()
	}()

	return s, nil
}

// Resize updates the PTY window size.
func (s *Session) Resize(cols, rows uint16) error {
	f, ok := s.pty.(*os.File)
	if !ok {
		return errors.New("pty does not support resize")
	}
	return pty.Setsize(f, &pty.Winsize{Cols: cols, Rows: rows})
}
