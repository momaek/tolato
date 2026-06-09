package notify

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/momaek/tolato/server/internal/model"
)

func TestRenderBodyEscapesText(t *testing.T) {
	// A message containing a double-quote must not break the surrounding JSON.
	ev := Event{NodeName: `web-"01"`, NodeIP: "1.2.3.4", Type: EventOffline, At: time.Unix(0, 0).UTC()}
	vars := renderVars(map[string]string{"chat_id": "123"}, ev, "")
	out := renderBody(`{"chat_id":"{{chat_id}}","text":"{{message}}"}`, vars)

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("rendered body is not valid JSON: %v\n%s", err, out)
	}
	if got["chat_id"] != "123" {
		t.Errorf("chat_id = %v, want 123", got["chat_id"])
	}
}

func TestRenderPlainRawSubstitution(t *testing.T) {
	vars := renderVars(map[string]string{"bot_token": "abc:def"}, Event{}, "")
	got := renderPlain("https://api/bot{{bot_token}}/send", vars)
	want := "https://api/botabc:def/send"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderBodyNumericParamUnquoted(t *testing.T) {
	// WeCom needs agentid as a bare JSON number; params are substituted raw.
	ev := Event{NodeName: "n1", Type: EventOffline}
	vars := renderVars(map[string]string{"agentid": "1000002"}, ev, "tok")
	out := renderBody(`{"agentid":{{agentid}},"text":{"content":"{{message}}"}}`, vars)
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if f, ok := got["agentid"].(float64); !ok || int(f) != 1000002 {
		t.Errorf("agentid = %v (%T), want number 1000002", got["agentid"], got["agentid"])
	}
}

func TestMatchJSONIntFromConfig(t *testing.T) {
	// Config values arrive as float64 after JSON decode; response errcode is also
	// float64. Both must compare equal via the string-normalized path.
	var m model.NotifyJSONMatch
	if err := json.Unmarshal([]byte(`{"json_path":"errcode","values":[42001,40014]}`), &m); err != nil {
		t.Fatal(err)
	}
	if !matchJSON([]byte(`{"errcode":42001,"errmsg":"invalid"}`), &m) {
		t.Error("expected errcode 42001 to match")
	}
	if matchJSON([]byte(`{"errcode":0}`), &m) {
		t.Error("expected errcode 0 not to match")
	}
}

func TestMatchJSONBool(t *testing.T) {
	m := &model.NotifyJSONMatch{JSONPath: "ok", Values: []any{true}}
	if !matchJSON([]byte(`{"ok":true}`), m) {
		t.Error("expected ok:true to match")
	}
	if matchJSON([]byte(`{"ok":false}`), m) {
		t.Error("expected ok:false not to match")
	}
}

func TestGetPathNested(t *testing.T) {
	var data map[string]any
	_ = json.Unmarshal([]byte(`{"data":{"token":"xyz"}}`), &data)
	v, ok := getPath(data, "data.token")
	if !ok || v != "xyz" {
		t.Errorf("getPath = %v, %v; want xyz, true", v, ok)
	}
	if _, ok := getPath(data, "data.missing"); ok {
		t.Error("expected missing path to return false")
	}
}

func TestResolveSpecOverlay(t *testing.T) {
	// Preset provides defaults; custom webhook fields override non-empty ones.
	ch := model.NotifyChannel{
		Preset:  "telegram",
		Webhook: &model.NotifyWebhookSpec{BodyTemplate: `{"custom":true}`},
	}
	spec := resolveSpec(ch)
	if spec.URL == "" {
		t.Error("expected preset URL to survive overlay")
	}
	if spec.BodyTemplate != `{"custom":true}` {
		t.Errorf("body override not applied: %q", spec.BodyTemplate)
	}
}

func TestPresetSpecKnownAndUnknown(t *testing.T) {
	for _, p := range []string{"telegram", "discord", "slack", "wecom", "feishu"} {
		if _, ok := presetSpec(p); !ok {
			t.Errorf("preset %q should be known", p)
		}
	}
	if _, ok := presetSpec("custom"); ok {
		t.Error("custom should not have a built-in spec")
	}
}
