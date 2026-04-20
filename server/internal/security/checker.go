package security

import (
	"log"
	"regexp"
	"strings"

	"github.com/momaek/tolato/server/internal/settings"
)

// Checker detects sensitive or blacklisted commands. It reads security
// settings through a cached accessor so this is cheap to call in the hot path.
type Checker struct {
	cache *settings.Cache
}

// NewChecker creates a Checker backed by the given settings cache.
func NewChecker(cache *settings.Cache) *Checker {
	return &Checker{cache: cache}
}

// IsSensitive reports whether `command` matches any sensitive keyword and
// confirmation is enabled. Fail-open: on cache/decode miss we return false,
// because a missed "please confirm" prompt is better than blocking the
// operator during a settings outage.
func (c *Checker) IsSensitive(command string) bool {
	if !c.cache.SecurityConfirmEnabled() {
		return false
	}
	keywords, _ := c.cache.SecurityList("security.sensitive_keywords")
	return matchAny(command, keywords)
}

// IsBlacklisted reports whether `command` hits the blacklist. Fail-closed: if
// the setting can't be read we return true so a transient DB issue can't
// silently strip the kill-switch. Operators see an obvious block with a log
// line.
func (c *Checker) IsBlacklisted(command string) bool {
	patterns, ok := c.cache.SecurityList("security.command_blacklist")
	if !ok {
		log.Printf("[security] blacklist unavailable — blocking command by default")
		return true
	}
	return matchAny(command, patterns)
}

// matchAny returns true if any pattern matches `command`.
//
// Matching is done per-segment: the command is split on shell separators
// (`;`, `|`, `&&`, `||`, newlines), each segment is trimmed, and patterns are
// tested against the segment. A pattern matches if:
//
//  1. it compiles as a Go regex and matches the segment (case-insensitive), or
//  2. it appears as a literal substring (case-insensitive fallback).
//
// Splitting on separators prevents `rm -rf /` from sneaking past a `rm`
// blacklist as a substring of e.g. `echo "rm -rf /"`. It's not a full shell
// parser — someone determined can still bypass with eval or base64 — but it
// closes the obvious holes. The blacklist is defense-in-depth, not the last
// line of defense.
func matchAny(command string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	segments := splitSegments(command)
	for _, p := range patterns {
		if p == "" {
			continue
		}
		re, isRegex := compilePattern(p)
		lowerP := strings.ToLower(p)
		for _, seg := range segments {
			if isRegex && re.MatchString(seg) {
				return true
			}
			if !isRegex && strings.Contains(strings.ToLower(seg), lowerP) {
				return true
			}
		}
	}
	return false
}

var segmentSplitRE = regexp.MustCompile(`(?:\|\||&&|;|\||\n)`)

func splitSegments(command string) []string {
	parts := segmentSplitRE.Split(command, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{strings.TrimSpace(command)}
	}
	return out
}

func compilePattern(p string) (*regexp.Regexp, bool) {
	re, err := regexp.Compile("(?i)" + p)
	if err != nil {
		return nil, false
	}
	return re, true
}
