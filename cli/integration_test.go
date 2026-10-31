package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationCommands(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Error("missing key")
		}
		if r.URL.Path == "/api/v1/nodes" {
			w.Write([]byte(`[{"id":"n","name":"box","country_code":"JP"}]`))
			return
		}
		calls++
		switch r.URL.Path {
		case "/api/v1/nodes/n/commands":
			if r.URL.Query().Get("since") != "2026-10-31T00:00:00Z" {
				t.Error("since lost")
			}
			w.Write([]byte(`{"items":[],"next_cursor":"","checkpoint":"poll"}`))
		case "/api/v1/nodes/n/services/patato-node":
			if r.Method == http.MethodPost {
				var req struct {
					Action, SHA256 string
					Config         json.RawMessage
				}
				if json.NewDecoder(r.Body).Decode(&req) != nil || req.Action != "install" || len(req.Config) == 0 {
					t.Error("manifest not forwarded")
				}
			}
			w.Write([]byte(`{"enabled":true,"running":true}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c := newClient(Config{URL: srv.URL, APIKey: "k"})
	if err := runCommands(c, []string{"box", "--since", "2026-10-31T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := runServices(c, []string{"status", "box"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"panel":"https://panel.example","token":"test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runServices(c, []string{"install", "box", "--binary-url", "https://binary.example/bin", "--sha256", strings.Repeat("a", 64), "--config", path}); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatal("requests missing")
	}
	for _, args := range [][]string{{"status", "box", "--config", path}, {"install", "box"}, {"unknown", "box"}} {
		if err := runServices(c, args); err == nil {
			t.Fatal("invalid service flags accepted")
		}
	}
}
