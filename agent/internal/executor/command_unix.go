//go:build !windows

package executor

import (
	"os/exec"
	"syscall"
)

// setProcGroup puts the command in its own process group so the entire tree
// (including backgrounded children) can be signalled together.
func setProcGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup SIGKILLs the command's whole process group, falling back to
// killing just the process if the group id can't be resolved.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
		// Negative pid targets the process group.
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	return cmd.Process.Kill()
}
