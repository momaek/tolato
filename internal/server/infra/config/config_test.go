package config

import "testing"

func TestParseAppliesDefaults(t *testing.T) {
	cfg, err := Parse([]byte(`
server:
  http_address: ":8080"
store:
  driver: memory
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.Server.UIWSPath != "/ws/ui" {
		t.Fatalf("UIWSPath = %q, want %q", cfg.Server.UIWSPath, "/ws/ui")
	}
	if cfg.Server.AgentWSPath != "/ws/agent" {
		t.Fatalf("AgentWSPath = %q, want %q", cfg.Server.AgentWSPath, "/ws/agent")
	}
}

func TestParseRequiresCoreFields(t *testing.T) {
	_, err := Parse([]byte(`
server:
  ui_ws_path: /ws/ui
store:
  driver: memory
`))
	if err == nil {
		t.Fatal("Parse() error = nil, want validation error")
	}
}
