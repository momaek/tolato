//go:build windows

package client

import "os"

// restartSelf exits the process so the service manager restarts it with the
// updated binary. Windows has no exec() equivalent, so we rely on the
// supervisor (e.g. a Windows service / NSSM) to bring the agent back.
func restartSelf() error {
	os.Exit(0)
	return nil
}
