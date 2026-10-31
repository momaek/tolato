package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestServiceLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix helper executable")
	}
	dir := filepath.Join(t.TempDir(), "service")
	m, err := open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	binary := "#!/bin/sh\nexec sleep 60\n"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(binary)) }))
	defer server.Close()
	m.http = server.Client()
	digest := func() string { sum := sha256.Sum256([]byte(binary)); return hex.EncodeToString(sum[:]) }
	config := json.RawMessage(`{"panel":"https://patato.example","token":"machine-token-must-not-leak"}`)
	install := Request{Action: "install", BinaryURL: server.URL, SHA256: digest(), Config: config}
	s, err := m.Apply(context.Background(), install)
	if err != nil || !s.Running || s.PID == 0 {
		t.Fatalf("install: %+v %v", s, err)
	}
	firstPID := s.PID
	for _, path := range []string{"config.json", "manifest.json"} {
		st, err := os.Stat(filepath.Join(dir, path))
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatalf("%s is not private", path)
		}
	}
	// A corrupt download is rejected while the old process is still running.
	bad := install
	bad.Action = "upgrade"
	bad.SHA256 = strings.Repeat("0", 64)
	bad.Config = nil
	s, err = m.Apply(context.Background(), bad)
	if err == nil || s.PID != firstPID || !s.Running {
		t.Fatal("bad digest interrupted old service")
	}
	// A correctly hashed non-executable payload cannot replace the running app.
	binary = "not an executable\n"
	bad.SHA256 = digest()
	s, err = m.Apply(context.Background(), bad)
	if err == nil || !s.Running || s.SHA256 != install.SHA256 {
		t.Fatalf("failed upgrade did not restore previous version: %+v %v", s, err)
	}
	binary = "#!/bin/sh\nexec sleep 61\n"
	bad.SHA256 = digest()
	s, err = m.Apply(context.Background(), bad)
	if err != nil || !s.Running || s.SHA256 != bad.SHA256 {
		t.Fatalf("upgrade failed %+v %v", s, err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	if string(raw) != string(config) {
		t.Fatal("upgrade lost enrollment")
	}
	pid := s.PID
	s, err = m.Apply(context.Background(), Request{Action: "restart"})
	if err != nil || s.PID == pid || !s.Running {
		t.Fatal("restart failed")
	}
	resume := m.PrepareExec()
	if m.Status().Running {
		t.Fatal("exec handoff left a duplicate child")
	}
	resume()
	if !m.Status().Running {
		t.Fatal("failed exec could not restore supervision")
	}
	m.Close()
	// Reopening simulates an agent restart. Config and desired running state
	// persist independently from the server WebSocket.
	m, err = open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if !m.Status().Running {
		t.Fatal("agent restart did not restore service")
	}
	s, err = m.Apply(context.Background(), Request{Action: "stop"})
	if err != nil || s.Running || s.Enabled {
		t.Fatal("stop failed")
	}
	m.Close()
	m, err = open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.Status().Running || m.Status().Enabled {
		t.Fatal("stopped service restarted")
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "token") || strings.Contains(string(b), "patato.example") {
		t.Fatal("status exposed config")
	}
}

func TestServiceCrashRestartsAndValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix helper executable")
	}
	m, err := open(filepath.Join(t.TempDir(), "service"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	body := "#!/bin/sh\nexit 1\n"
	sum := sha256.Sum256([]byte(body))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
	defer server.Close()
	m.http = server.Client()
	req := Request{Action: "install", BinaryURL: server.URL, SHA256: hex.EncodeToString(sum[:]), Config: json.RawMessage(`{"panel":"https://panel.example","token":"test"}`)}
	for _, change := range []func(*Request){func(r *Request) { r.SHA256 = "bad" }, func(r *Request) { r.BinaryURL = "http://insecure.example" }, func(r *Request) { r.Config = json.RawMessage(`{"panel":"https://panel.example"}`) }} {
		bad := req
		change(&bad)
		if _, err := m.Apply(context.Background(), bad); err == nil {
			t.Fatal("invalid installation accepted")
		}
	}
	if _, err := m.Apply(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if m.Status().Restarts >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("crashed service did not restart")
}
