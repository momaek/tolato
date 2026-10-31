//go:build !linux

package services

import "os/exec"

// Managed services are Linux-only; this permits other agent targets and tests.
func configureCommand(cmd *exec.Cmd) {}
