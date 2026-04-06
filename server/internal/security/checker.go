package security

import (
	"encoding/json"
	"strings"

	"github.com/momaek/tolato/server/internal/store"
)

// Checker detects sensitive or blacklisted commands.
type Checker struct{}

// NewChecker creates a new Checker.
func NewChecker() *Checker {
	return &Checker{}
}

// IsSensitive checks if a command matches any sensitive keyword.
// Returns true if security.confirm_enabled is true AND the command contains
// any sensitive keyword. Settings are read fresh from the database on every
// call. Returns false on any DB or unmarshal error (fail open).
func (c *Checker) IsSensitive(command string) bool {
	if !c.confirmEnabled() {
		return false
	}
	keywords := c.loadStringList("security.sensitive_keywords")
	return containsAny(command, keywords)
}

// IsBlacklisted checks if a command matches any blacklisted pattern.
// Returns true if the command contains any blacklist keyword.
// Returns false on any DB or unmarshal error (fail open).
func (c *Checker) IsBlacklisted(command string) bool {
	patterns := c.loadStringList("security.command_blacklist")
	return containsAny(command, patterns)
}

func (c *Checker) confirmEnabled() bool {
	s, err := store.GetSetting("security.confirm_enabled")
	if err != nil {
		return false
	}
	var enabled bool
	if err := json.Unmarshal([]byte(s.Value), &enabled); err != nil {
		return false
	}
	return enabled
}

func (c *Checker) loadStringList(key string) []string {
	s, err := store.GetSetting(key)
	if err != nil {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(s.Value), &list); err != nil {
		return nil
	}
	return list
}

// containsAny returns true if command contains any of the given patterns
// (case-insensitive).
func containsAny(command string, patterns []string) bool {
	lower := strings.ToLower(command)
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}
