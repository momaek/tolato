package services

import (
	"os/exec"
	"syscall"
	"time"
)

func configureCommand(cmd *exec.Cmd) {
	// The child dies if the agent is killed unexpectedly. Its own process group
	// receives SIGTERM on normal shutdown, with a bounded grace period.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
}
