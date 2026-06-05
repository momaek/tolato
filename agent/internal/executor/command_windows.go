//go:build windows

package executor

import "os/exec"

// setProcGroup is a no-op on Windows; process-group semantics differ and the
// context-based kill plus WaitDelay are sufficient here.
func setProcGroup(cmd *exec.Cmd) {}

// killProcessGroup kills the command's process.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
