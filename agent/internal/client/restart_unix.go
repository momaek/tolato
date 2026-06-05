//go:build !windows

package client

import (
	"os"
	"syscall"
)

// restartSelf replaces the current process image with the (freshly updated)
// binary, preserving the original arguments and environment. The new process
// reuses the stored identity in the data dir and reconnects. This does not
// depend on a service manager to bring the agent back.
func restartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
