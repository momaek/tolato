package files

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func stage(t *testing.T, dir string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, "app.tolato-part")
	r := Handle(Request{Op: OpWrite, Path: p, Data: base64.StdEncoding.EncodeToString(data)})
	if !r.OK {
		t.Fatal(r.Error)
	}
	return p
}

func perm(t *testing.T, p string) os.FileMode {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.Mode().Perm()
}

func TestCommitVerifiesAndMovesIntoPlace(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "app")
	data := []byte("#!/bin/sh\necho new\n")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	part := stage(t, dir, data)

	// Upper case is as good as lower: digests get pasted from all sorts of tools.
	r := Handle(Request{Op: OpCommit, From: part, Path: dst, SHA256: strings.ToUpper(digest(data)), Mode: 0o755})
	if !r.OK {
		t.Fatal(r.Error)
	}
	if r.SHA256 != digest(data) || r.Stat == nil || r.Stat.Size != int64(len(data)) {
		t.Errorf("reply sha256=%s stat=%+v", r.SHA256, r.Stat)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Errorf("destination holds %q", got)
	}
	if runtime.GOOS != "windows" && perm(t, dst) != 0o755 {
		t.Errorf("mode %o, want 755", perm(t, dst))
	}
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Error("staged file still there after commit")
	}
}

func TestCommitMismatchLeavesDestinationAlone(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "app")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	part := stage(t, dir, []byte("truncated"))

	r := Handle(Request{Op: OpCommit, From: part, Path: dst, SHA256: digest([]byte("the whole thing"))})
	if r.OK || !strings.Contains(r.Error, "sha256 mismatch") {
		t.Fatalf("ok=%v error=%q", r.OK, r.Error)
	}
	if r.SHA256 != digest([]byte("truncated")) {
		t.Errorf("reply should carry the digest it found, got %q", r.SHA256)
	}
	if got, _ := os.ReadFile(dst); string(got) != "old" {
		t.Errorf("destination changed to %q", got)
	}
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Error("mismatched staged file was kept")
	}
}

func TestCommitKeepsExistingModeByDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no unix permission bits")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "app")
	if err := os.WriteFile(dst, []byte("v1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dst, 0o750); err != nil {
		t.Fatal(err)
	}
	part := stage(t, dir, []byte("v2"))

	if r := Handle(Request{Op: OpCommit, From: part, Path: dst}); !r.OK {
		t.Fatal(r.Error)
	}
	if perm(t, dst) != 0o750 {
		t.Errorf("mode %o, want the replaced file's 750", perm(t, dst))
	}
}

func TestCommitRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	part := stage(t, dir, []byte("x"))
	dst := filepath.Join(dir, "app")
	for name, req := range map[string]Request{
		"no from":      {Op: OpCommit, Path: dst},
		"short digest": {Op: OpCommit, From: part, Path: dst, SHA256: "abc"},
		"not hex":      {Op: OpCommit, From: part, Path: dst, SHA256: strings.Repeat("z", 64)},
		"missing from": {Op: OpCommit, From: part + ".nope", Path: dst},
	} {
		if r := Handle(req); r.OK {
			t.Errorf("%s: accepted", name)
		}
	}
	// A bad request must not have consumed the staged file.
	if _, err := os.Stat(part); err != nil {
		t.Errorf("staged file gone after rejected commits: %v", err)
	}
}

func TestWriteAppliesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no unix permission bits")
	}
	p := filepath.Join(t.TempDir(), "tool")
	r := Handle(Request{Op: OpWrite, Path: p, Data: base64.StdEncoding.EncodeToString([]byte("x")), Mode: 0o755})
	if !r.OK {
		t.Fatal(r.Error)
	}
	// 0755 survives a 022 umask anyway; 0777 would not, so check that too.
	if perm(t, p) != 0o755 {
		t.Errorf("mode %o, want 755", perm(t, p))
	}
	r = Handle(Request{Op: OpWrite, Path: p, Offset: 1, Data: base64.StdEncoding.EncodeToString([]byte("y")), Mode: 0o777})
	if !r.OK {
		t.Fatal(r.Error)
	}
	if perm(t, p) != 0o777 {
		t.Errorf("mode %o, want 777 despite the umask", perm(t, p))
	}
}
