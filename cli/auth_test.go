package main

import (
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// get performs a callback request the way the browser would.
func get(t *testing.T, rawURL string) int {
	t.Helper()
	resp, err := http.Get(rawURL)
	if err != nil {
		t.Fatalf("GET %s: %v", rawURL, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func listen(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln, fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)
}

// The loopback port is open to anything running on the machine, so a callback
// that does not carry our state must not be able to feed us a code — and must
// not end the wait, either, or a hostile local page could cancel a login.
func TestAwaitCallbackIgnoresWrongState(t *testing.T) {
	ln, base := listen(t)

	type out struct {
		code string
		err  error
	}
	done := make(chan out, 1)
	go func() {
		code, err := awaitCallback(ln, "the-real-state")
		done <- out{code, err}
	}()

	if got := get(t, base+"?state=guessed&code=attacker-code"); got != http.StatusBadRequest {
		t.Errorf("forged callback got %d, want %d", got, http.StatusBadRequest)
	}

	select {
	case res := <-done:
		t.Fatalf("forged callback ended the wait: code=%q err=%v", res.code, res.err)
	case <-time.After(200 * time.Millisecond):
	}

	// The genuine browser still gets through afterwards.
	if got := get(t, base+"?state=the-real-state&code=real-code"); got != http.StatusOK {
		t.Errorf("genuine callback got %d, want 200", got)
	}

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("genuine callback errored: %v", res.err)
		}
		if res.code != "real-code" {
			t.Errorf("code = %q, want %q", res.code, "real-code")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("genuine callback did not complete the wait")
	}
}

// Cancelling in the browser should surface as a prompt error, not a three
// minute wait for a code that is never coming.
func TestAwaitCallbackReportsDenial(t *testing.T) {
	ln, base := listen(t)

	done := make(chan error, 1)
	go func() {
		_, err := awaitCallback(ln, "s")
		done <- err
	}()

	get(t, base+"?state=s&error=access_denied")

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("err = %v, want an authorization-denied error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("denial did not end the wait")
	}
}

func TestResolveServerURLPrecedence(t *testing.T) {
	// An empty config file keeps the lookup from finding a real one.
	dir := t.TempDir()
	t.Setenv("TOLATO_CONFIG", filepath.Join(dir, "config.yaml"))

	t.Setenv("TOLATO_URL", "https://from-env.example.com")
	if got, _ := resolveServerURL("https://from-flag.example.com"); got != "https://from-flag.example.com" {
		t.Errorf("flag should win, got %q", got)
	}
	if got, _ := resolveServerURL(""); got != "https://from-env.example.com" {
		t.Errorf("env should win over config, got %q", got)
	}

	t.Setenv("TOLATO_URL", "")
	if _, err := resolveServerURL(""); err == nil {
		t.Error("want an error when nothing supplies a URL")
	}

	// Trailing slashes would double up when paths are appended.
	t.Setenv("TOLATO_URL", "https://x.example.com/")
	if got, _ := resolveServerURL(""); got != "https://x.example.com" {
		t.Errorf("trailing slash not trimmed: %q", got)
	}
}

// The file holds an API key, so its mode is part of the contract.
func TestWriteConfigFileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")
	t.Setenv("TOLATO_CONFIG", path)

	want := Config{URL: "https://tolato.example.com", APIKey: "tlat_secret"}
	if err := writeConfigFile(want); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != fs.FileMode(0o600) {
		t.Errorf("mode = %v, want 0600", perm)
	}

	got, err := readConfigFile()
	if err != nil {
		t.Fatalf("readConfigFile: %v", err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}

	// No temporary files left behind next to the config.
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".config-") {
			t.Errorf("temporary file %q survived the write", e.Name())
		}
	}
}

// The server composes "CLI on <label>", so the label must not say it too.
func TestMachineLabelIsBareIdentity(t *testing.T) {
	got := machineLabel()
	if got == "" {
		t.Fatal("machineLabel() is empty")
	}
	if strings.Contains(strings.ToLower(got), "cli on") {
		t.Errorf("machineLabel() = %q, should not carry the wording the server adds", got)
	}
}

func TestMaskKeyHidesTheSecret(t *testing.T) {
	key := "tlat_0123456789abcdef0123456789abcdef"
	got := maskKey(key)
	if strings.Contains(got, key[12:]) {
		t.Errorf("maskKey(%q) = %q, leaks the tail", key, got)
	}
	if !strings.HasPrefix(got, "tlat_") {
		t.Errorf("maskKey = %q, want a recognisable prefix", got)
	}
}
