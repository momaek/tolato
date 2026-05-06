// Package settings provides a thin, TTL-based cache over store.GetSetting so
// high-frequency readers (LLM loop, security checker) don't hit the DB on
// every message/command.
//
// Writes invalidate via a hook registered on store.OnSettingChanged, so
// settings edited via the REST API take effect immediately. TTL exists only as
// a safety net for direct DB writes that bypass the hook (migrations, manual
// sqlite edits during dev).
package settings

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

const defaultTTL = 30 * time.Second

// Cache caches setting JSON payloads keyed by their setting key.
type Cache struct {
	ttl time.Duration
	mu  sync.RWMutex
	m   map[string]entry
}

type entry struct {
	value string
	exp   time.Time
	ok    bool // distinguish "DB miss" from "not fetched yet"
}

// New builds a cache and wires itself into store.OnSettingChanged so REST
// writes invalidate affected keys. Only one cache should be created per
// process.
func New() *Cache {
	c := &Cache{
		ttl: defaultTTL,
		m:   make(map[string]entry),
	}
	store.OnSettingChanged = c.Invalidate
	return c
}

// Invalidate drops cached entries for the given keys. Safe to call with zero
// keys (no-op) — used by store.OnSettingChanged after each write.
func (c *Cache) Invalidate(keys []string) {
	if len(keys) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.m, k)
	}
}

// InvalidateAll clears the cache. Useful for tests.
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = make(map[string]entry)
}

// Raw returns the JSON-encoded setting value for key, or ("", false) on DB
// miss or decode error. Callers should decode via GetJSON or the typed helpers
// below.
func (c *Cache) Raw(key string) (string, bool) {
	c.mu.RLock()
	e, hit := c.m[key]
	c.mu.RUnlock()
	if hit && time.Now().Before(e.exp) {
		return e.value, e.ok
	}

	s, err := store.GetSetting(key)
	ne := entry{exp: time.Now().Add(c.ttl)}
	if err == nil && s != nil {
		ne.value = s.Value
		ne.ok = true
	}

	c.mu.Lock()
	c.m[key] = ne
	c.mu.Unlock()

	return ne.value, ne.ok
}

// GetJSON decodes the cached setting at `key` into `out`. Returns true on
// success. On any error (missing, decode-failure) the caller's existing value
// of `out` is left untouched — so pass in a value pre-populated with the
// desired default and GetJSON just overwrites it on hit.
func GetJSON[T any](c *Cache, key string, out *T) bool {
	raw, ok := c.Raw(key)
	if !ok {
		return false
	}
	return json.Unmarshal([]byte(raw), out) == nil
}

// --- Typed helpers for the fixed schema -------------------------------------

// LLM returns LLM settings with built-in defaults. Never errors: missing
// entries just stay at their default values.
func (c *Cache) LLM() model.LLMSettings {
	s := model.LLMSettings{
		MaxRounds:   20,
		Temperature: 0.7,
	}
	GetJSON(c, "llm.api_base_url", &s.APIBaseURL)
	GetJSON(c, "llm.api_key", &s.APIKey)
	GetJSON(c, "llm.default_model", &s.DefaultModel)
	GetJSON(c, "llm.max_rounds", &s.MaxRounds)
	GetJSON(c, "llm.temperature", &s.Temperature)
	return s
}

// Chat returns chat settings with built-in defaults.
func (c *Cache) Chat() model.ChatSettings {
	s := model.ChatSettings{
		ContextRounds:       20,
		OutputTruncateLines: 100,
	}
	GetJSON(c, "chat.context_rounds", &s.ContextRounds)
	GetJSON(c, "chat.output_truncate_lines", &s.OutputTruncateLines)
	var prompt string
	if GetJSON(c, "chat.custom_system_prompt", &prompt) && prompt != "" {
		s.CustomSystemPrompt = &prompt
	}
	return s
}

// WebFetch returns web-fetch settings with built-in defaults. The web_fetch
// tool reads this on every invocation, so edits via the REST API take effect
// without a restart.
func (c *Cache) WebFetch() model.WebFetchSettings {
	s := model.WebFetchSettings{
		Mode:       "jina",
		TimeoutSec: 30,
		MaxKB:      1024,
	}
	GetJSON(c, "webfetch.mode", &s.Mode)
	GetJSON(c, "webfetch.jina_api_key", &s.JinaAPIKey)
	GetJSON(c, "webfetch.timeout_sec", &s.TimeoutSec)
	GetJSON(c, "webfetch.max_kb", &s.MaxKB)
	return s
}

// SecurityConfirmEnabled and SecurityList are used by security.Checker.
func (c *Cache) SecurityConfirmEnabled() bool {
	var enabled bool
	GetJSON(c, "security.confirm_enabled", &enabled)
	return enabled
}

// SecurityList returns (list, ok). ok=false when the setting is missing or
// malformed — callers then decide fail-open vs fail-closed.
func (c *Cache) SecurityList(key string) ([]string, bool) {
	var list []string
	ok := GetJSON(c, key, &list)
	return list, ok
}
