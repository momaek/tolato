package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSplitRemote(t *testing.T) {
	cases := []struct {
		arg, node, path string
		remote          bool
	}{
		{"web-01:/etc/hosts", "web-01", "/etc/hosts", true},
		{"web-01:", "web-01", "", true},
		{"./a:b", "", "./a:b", false},
		{"dir/a:b", "", "dir/a:b", false},
		{":/x", "", ":/x", false},
		{"plain.txt", "", "plain.txt", false},
		{"-", "", "-", false},
	}
	for _, tc := range cases {
		node, path, remote := splitRemote(tc.arg)
		if node != tc.node || path != tc.path || remote != tc.remote {
			t.Errorf("splitRemote(%q) = %q, %q, %v; want %q, %q, %v", tc.arg, node, path, remote, tc.node, tc.path, tc.remote)
		}
	}
}

func TestCheckRemotePath(t *testing.T) {
	for _, p := range []string{"/etc/x", "/"} {
		if err := checkRemotePath(p, false); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	for _, p := range []string{"", "etc/x", "~/x", "./x"} {
		if err := checkRemotePath(p, false); err == nil {
			t.Errorf("%q: accepted a relative path", p)
		}
	}
	for _, p := range []string{`C:\x`, "C:/x", `\\host\share\x`} {
		if err := checkRemotePath(p, true); err != nil {
			t.Errorf("windows %q: %v", p, err)
		}
	}
	if err := checkRemotePath("/x", true); err == nil {
		t.Error("windows: accepted a path with no drive")
	}
}

// fakeNode stands in for the server plus one agent. Remote paths are real
// paths on this machine, file ops follow the agent's files package (including
// its 1 MiB read cap and its commit), and exec runs through sh -c — so a test
// exercises the same chunking, part file and final move that a real node
// would see.
type fakeNode struct {
	failWriteAt int64 // fail a write at this offset; <0 never
	corrupt     bool  // flip the first byte of every chunk written
	legacy      bool  // an agent from before the commit op
	writes      atomic.Int32
	execs       atomic.Int32
}

func newFakeServer(t *testing.T, fn *fakeNode) *client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Node{{ID: "n1", Name: "box", Status: "online", OS: "Ubuntu 24.04"}})
	})
	mux.HandleFunc("POST /api/v1/nodes/n1/execute", func(w http.ResponseWriter, r *http.Request) {
		fn.execs.Add(1)
		var req struct{ Command string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		var stderr bytes.Buffer
		cmd := exec.Command("sh", "-c", req.Command)
		cmd.Stderr = &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			code = 1
		}
		_ = json.NewEncoder(w).Encode(ExecResult{ExitCode: code, Stderr: stderr.String()})
	})
	mux.HandleFunc("POST /api/v1/nodes/n1/files", func(w http.ResponseWriter, r *http.Request) {
		var op FileOp
		_ = json.NewDecoder(r.Body).Decode(&op)
		_ = json.NewEncoder(w).Encode(fn.handle(op))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return newClient(Config{URL: srv.URL, APIKey: "k"})
}

func (fn *fakeNode) handle(op FileOp) FileResult {
	fail := func(err error) FileResult { return FileResult{Error: err.Error()} }
	switch op.Op {
	case "stat":
		st, err := os.Stat(op.Path)
		if err != nil {
			return fail(err)
		}
		return FileResult{OK: true, Stat: &FileEntry{Name: st.Name(), Size: st.Size(), Mode: uint32(st.Mode()), IsDir: st.IsDir()}}
	case "write":
		fn.writes.Add(1)
		if fn.failWriteAt >= 0 && op.Offset == fn.failWriteAt {
			return fail(errors.New("disk full"))
		}
		raw, _ := base64.StdEncoding.DecodeString(op.Data)
		if fn.corrupt && len(raw) > 0 {
			raw[0] ^= 0xff
		}
		flag := os.O_WRONLY | os.O_CREATE
		if op.Offset == 0 {
			flag |= os.O_TRUNC
		}
		f, err := os.OpenFile(op.Path, flag, 0o644)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		if _, err := f.WriteAt(raw, op.Offset); err != nil {
			return fail(err)
		}
		return FileResult{OK: true}
	case "read":
		if op.Length <= 0 || op.Length > 1<<20 {
			op.Length = 1 << 20
		}
		f, err := os.Open(op.Path)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		st, _ := f.Stat()
		buf := make([]byte, op.Length)
		n, err := f.ReadAt(buf, op.Offset)
		if err != nil && err != io.EOF {
			return fail(err)
		}
		return FileResult{OK: true, Data: base64.StdEncoding.EncodeToString(buf[:n]), EOF: op.Offset+int64(n) >= st.Size()}
	case "delete":
		if err := os.Remove(op.Path); err != nil {
			return fail(err)
		}
		return FileResult{OK: true}
	case "commit":
		if fn.legacy {
			break
		}
		data, err := os.ReadFile(op.From)
		if err != nil {
			return fail(err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != op.SHA256 {
			_ = os.Remove(op.From)
			return FileResult{Error: "sha256 mismatch: " + got, SHA256: got}
		}
		if op.Mode != 0 {
			if err := os.Chmod(op.From, os.FileMode(op.Mode).Perm()); err != nil {
				return fail(err)
			}
		}
		if err := os.Rename(op.From, op.Path); err != nil {
			return fail(err)
		}
		return FileResult{OK: true, SHA256: op.SHA256}
	}
	return fail(fmt.Errorf("unknown op: %q", op.Op))
}

func randomFile(t *testing.T, dir string, size int, perm os.FileMode) (string, []byte) {
	t.Helper()
	data := make([]byte, size)
	_, _ = rand.Read(data)
	p := filepath.Join(dir, "src.bin")
	if err := os.WriteFile(p, data, perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, perm); err != nil {
		t.Fatal(err)
	}
	return p, data
}

func assertFile(t *testing.T, p string, want []byte, perm os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: %d bytes differ from the %d sent", p, len(got), len(want))
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != perm {
		t.Errorf("%s: mode %o, want %o", p, st.Mode().Perm(), perm)
	}
	if _, err := os.Stat(p + partSuffix); !os.IsNotExist(err) {
		t.Errorf("%s: part file left behind", p)
	}
}

func TestCpRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake node execs through sh")
	}
	for _, legacy := range []bool{false, true} {
		fn := &fakeNode{failWriteAt: -1, legacy: legacy}
		testCpRoundTrip(t, newFakeServer(t, fn))
		// With commit, nothing about an upload goes through exec.
		if !legacy && fn.execs.Load() != 0 {
			t.Errorf("%d execs on an agent with commit", fn.execs.Load())
		}
	}
}

func testCpRoundTrip(t *testing.T, c *client) {
	for _, size := range []int{0, 1, cpChunk, 2*cpChunk + 17} {
		local, remote := t.TempDir(), t.TempDir()
		src, data := randomFile(t, local, size, 0o750)

		// Upload into a directory keeps the name and the mode.
		if err := runCp(c, []string{src, "box:" + remote}); err != nil {
			t.Fatalf("upload %d bytes: %v", size, err)
		}
		up := filepath.Join(remote, "src.bin")
		assertFile(t, up, data, 0o750)

		// Uploading again replaces the file in place.
		if err := runCp(c, []string{src, "box:" + up}); err != nil {
			t.Fatalf("re-upload %d bytes: %v", size, err)
		}
		assertFile(t, up, data, 0o750)

		// And back down, under a new name.
		down := filepath.Join(local, "back.bin")
		if err := runCp(c, []string{"box:" + up, down}); err != nil {
			t.Fatalf("download %d bytes: %v", size, err)
		}
		assertFile(t, down, data, 0o750)
		leftovers, _ := filepath.Glob(filepath.Join(local, ".*"+partSuffix+"*"))
		if len(leftovers) > 0 {
			t.Errorf("download left temp files: %v", leftovers)
		}
	}
}

func TestCpFailedUploadLeavesNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake node execs through sh")
	}
	c := newFakeServer(t, &fakeNode{failWriteAt: cpChunk})
	src, _ := randomFile(t, t.TempDir(), 3*cpChunk, 0o644)
	remote := t.TempDir()
	dst := filepath.Join(remote, "out.bin")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runCp(c, []string{src, "box:" + dst})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("got %v, want the agent's write error", err)
	}
	// The old file is untouched and the part file is gone.
	assertFile(t, dst, []byte("old"), 0o644)
}

// What arrives is checked against what was sent; a damaged upload fails and
// never replaces the destination.
func TestCpCorruptedUploadIsRefused(t *testing.T) {
	c := newFakeServer(t, &fakeNode{failWriteAt: -1, corrupt: true})
	src, _ := randomFile(t, t.TempDir(), 2*cpChunk, 0o644)
	dst := filepath.Join(t.TempDir(), "out.bin")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runCp(c, []string{src, "box:" + dst})
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("got %v, want a digest mismatch", err)
	}
	assertFile(t, dst, []byte("old"), 0o644)
}

func TestCpRejects(t *testing.T) {
	c := newFakeServer(t, &fakeNode{failWriteAt: -1})
	dir := t.TempDir()
	src, _ := randomFile(t, dir, 10, 0o644)
	cases := map[string][]string{
		"relative remote":  {src, "box:tmp/x"},
		"tilde remote":     {src, "box:~/x"},
		"both local":       {src, filepath.Join(dir, "y")},
		"both remote":      {"box:/a", "box:/b"},
		"directory source": {dir, "box:/tmp/x"},
		"unknown node":     {src, "nope:/tmp/x"},
	}
	for name, args := range cases {
		if err := runCp(c, args); err == nil {
			t.Errorf("%s: accepted %q", name, args)
		}
	}
	if err := runCp(c, []string{src}); !errors.Is(err, errUsage) {
		t.Errorf("one path: got %v, want usage", err)
	}
}
