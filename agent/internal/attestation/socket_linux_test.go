package attestation

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSocketPermissions(t *testing.T) {
	if path := os.Getenv("TOLATO_SOCKET_TEST_CHILD"); path != "" {
		conn, err := net.Dial("unix", path)
		if err == nil {
			conn.Close()
			os.Exit(1)
		}
		os.Exit(0)
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root; exercised in Linux integration checks")
	}
	path := filepath.Join(t.TempDir(), "private", "agent.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("socket is not root-only")
	}
	if other, err := Listen(path); err == nil {
		other.Close()
		t.Fatal("active socket was replaced")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSocketPermissions$")
	cmd.Env = append(os.Environ(), "TOLATO_SOCKET_TEST_CHILD="+path)
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("non-root could connect: %v %s", err, out)
	}
	l.Close()
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if l, err := Listen(path); err == nil {
		l.Close()
		t.Fatal("regular file replaced")
	}
	// Root must not bind in a directory that a non-root owner can rename.
	dir := t.TempDir()
	if err := os.Chown(dir, 65534, 65534); err != nil {
		t.Fatal(err)
	}
	if l, err := Listen(filepath.Join(dir, "bad.sock")); err == nil {
		l.Close()
		t.Fatal("non-root-owned directory accepted")
	}
}
