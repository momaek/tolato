package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootManager(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "service")
	m, err := New(dir)
	if os.Geteuid() != 0 {
		if err == nil {
			m.Close()
			t.Fatal("non-root manager enabled")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	st, err := os.Stat(dir)
	if err != nil || st.Mode().Perm() != 0700 {
		t.Fatal("service directory is not private")
	}
}
