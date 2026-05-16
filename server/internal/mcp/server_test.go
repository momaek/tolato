package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/settings"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestServer wires the MCP handler with empty deps. The store-backed tools
// (list_nodes, get_node, execute_command) need a real DB, so those are
// covered in integration tests — this suite focuses on protocol framing.
func newTestServer(t *testing.T) *gin.Engine {
	t.Helper()
	r := gin.New()
	r.POST("/mcp", Handler(node.NewNodeManager(), settings.New()))
	return r
}

func doRPC(t *testing.T, r *gin.Engine, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Body.Len() == 0 {
		return rec.Code, nil
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %s — %v", rec.Body.String(), err)
	}
	return rec.Code, out
}

func TestInitialize(t *testing.T) {
	r := newTestServer(t)
	status, out := doRPC(t, r, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if status != http.StatusOK {
		t.Fatalf("status=%d, want 200", status)
	}
	if out["jsonrpc"] != "2.0" {
		t.Errorf("missing jsonrpc=2.0, got %v", out["jsonrpc"])
	}
	result, ok := out["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result object: %v", out)
	}
	if result["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v, want %v", result["protocolVersion"], protocolVersion)
	}
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "tolato" {
		t.Errorf("serverInfo.name = %v, want tolato", info["name"])
	}
}

func TestNotificationGets202(t *testing.T) {
	r := newTestServer(t)
	status, out := doRPC(t, r, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if status != http.StatusAccepted {
		t.Errorf("status=%d, want 202", status)
	}
	if out != nil {
		t.Errorf("expected empty body, got %v", out)
	}
}

func TestToolsList(t *testing.T) {
	r := newTestServer(t)
	_, out := doRPC(t, r, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	result, _ := out["result"].(map[string]any)
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools is not an array: %v", result)
	}
	want := map[string]bool{"list_nodes": false, "get_node": false, "execute_command": false, "web_fetch": false}
	for _, raw := range tools {
		tm := raw.(map[string]any)
		if _, present := want[tm["name"].(string)]; present {
			want[tm["name"].(string)] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("missing tool %s in catalog", name)
		}
	}
}

func TestUnknownMethod(t *testing.T) {
	r := newTestServer(t)
	_, out := doRPC(t, r, `{"jsonrpc":"2.0","id":3,"method":"does/not/exist"}`)
	errObj, ok := out["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %v", out)
	}
	if code, _ := errObj["code"].(float64); int(code) != -32601 {
		t.Errorf("error.code = %v, want -32601", errObj["code"])
	}
}

func TestPing(t *testing.T) {
	r := newTestServer(t)
	_, out := doRPC(t, r, `{"jsonrpc":"2.0","id":4,"method":"ping"}`)
	if _, ok := out["result"]; !ok {
		t.Errorf("ping should return a result, got %v", out)
	}
}

func TestParseError(t *testing.T) {
	r := newTestServer(t)
	_, out := doRPC(t, r, `not json`)
	errObj, ok := out["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error, got %v", out)
	}
	if code, _ := errObj["code"].(float64); int(code) != -32700 {
		t.Errorf("error.code = %v, want -32700", errObj["code"])
	}
}

func TestGetReturns405(t *testing.T) {
	r := gin.New()
	h := Handler(node.NewNodeManager(), settings.New())
	r.GET("/mcp", h)
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status=%d, want 405", rec.Code)
	}
}
