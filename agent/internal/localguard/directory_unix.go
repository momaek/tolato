//go:build !windows

// Package localguard checks the ownership of privileged local state paths.
package localguard

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func PrivateDirectory(dir string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", errors.New("private directory path must be absolute")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("private directory must have mode 0700")
	}
	// Resolve aliases such as /var/run, then check each ancestor. A sticky
	// world-writable directory (e.g. /tmp) cannot replace a root-owned child.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	for p := resolved; ; p = filepath.Dir(p) {
		st, err := os.Stat(p)
		if err != nil {
			return "", err
		}
		owner, ok := st.Sys().(*syscall.Stat_t)
		if !ok || owner.Uid != 0 || st.Mode().Perm()&0022 != 0 && st.Mode()&os.ModeSticky == 0 {
			return "", errors.New("private directory and its ancestors must be owned by root and not writable by others")
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return resolved, nil
}
