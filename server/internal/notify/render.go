package notify

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/momaek/tolato/server/internal/model"
)

// textPlaceholders are the variables that get JSON-string-escaped when
// substituted into a body template, so a node name/message containing quotes or
// newlines can't break the surrounding JSON. URL and header rendering use raw
// substitution (tokens and ids are url-safe).
var textPlaceholders = map[string]bool{
	"message":    true,
	"node_name":  true,
	"node_alias": true,
	"node_ip":    true,
}

// jsonInner returns s escaped as JSON string *content* (no surrounding quotes),
// suitable for dropping inside an existing pair of quotes in a JSON template.
func jsonInner(s string) string {
	b, err := json.Marshal(s)
	if err != nil || len(b) < 2 {
		return s
	}
	return string(b[1 : len(b)-1])
}

// renderVars merges channel params with event-derived placeholders and the
// resolved token. Params take lower precedence so a stray "token" param can't
// shadow the real bearer token.
func renderVars(params map[string]string, ev Event, token string) map[string]string {
	vars := make(map[string]string, len(params)+10)
	for k, v := range params {
		vars[k] = v
	}
	vars["token"] = token
	vars["node_id"] = ev.NodeID
	vars["node_name"] = ev.NodeName
	vars["node_alias"] = ev.NodeAlias
	vars["node_ip"] = ev.NodeIP
	vars["type"] = string(ev.Type)
	vars["status"] = string(ev.Type)
	vars["message"] = ev.Message()
	vars["time"] = ev.At.Format("2006-01-02 15:04:05")
	if ev.LastHeartbeat != nil {
		vars["last_heartbeat"] = ev.LastHeartbeat.Format("2006-01-02 15:04:05")
	} else {
		vars["last_heartbeat"] = ""
	}
	return vars
}

// renderPlain substitutes {{key}} with raw values. Used for URLs and headers.
func renderPlain(tmpl string, vars map[string]string) string {
	return buildReplacer(vars, false).Replace(tmpl)
}

// renderBody substitutes {{key}} into a JSON body, JSON-escaping the free-text
// placeholders so they can't break the JSON.
func renderBody(tmpl string, vars map[string]string) string {
	return buildReplacer(vars, true).Replace(tmpl)
}

func buildReplacer(vars map[string]string, escapeText bool) *strings.Replacer {
	pairs := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		val := v
		if escapeText && textPlaceholders[k] {
			val = jsonInner(v)
		}
		pairs = append(pairs, "{{"+k+"}}", val)
	}
	return strings.NewReplacer(pairs...)
}

// getPath walks a dot-separated path into a decoded JSON object.
func getPath(data map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var cur any = data
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// matchJSON reports whether the value at m.JSONPath in body equals any of
// m.Values. Comparison is string-normalized so config numbers (decoded as
// float64) match response numbers regardless of formatting.
func matchJSON(body []byte, m *model.NotifyJSONMatch) bool {
	if m == nil || m.JSONPath == "" {
		return false
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return false
	}
	val, ok := getPath(data, m.JSONPath)
	if !ok {
		return false
	}
	got := normalize(val)
	for _, want := range m.Values {
		if got == normalize(want) {
			return true
		}
	}
	return false
}

// normalize renders a value to a stable string for comparison. Whole-number
// floats (JSON numbers like 0, 42001) are formatted without a trailing ".0".
func normalize(v any) string {
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%v", v)
}

func trunc(b []byte) string {
	const max = 300
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
