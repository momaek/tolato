//go:build windows

package terminal

import "errors"

// Start is not supported on Windows in this phase.
func Start(shell, cwd string, cols, rows uint16) (*Session, error) {
	return nil, errors.New("PTY not supported on Windows")
}

// Resize is a no-op on Windows.
func (s *Session) Resize(cols, rows uint16) error { return nil }
