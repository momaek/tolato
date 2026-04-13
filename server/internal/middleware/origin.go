package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

// CheckOrigin returns a function suitable for websocket.Upgrader.CheckOrigin.
// If allowedOrigins is empty, only same-origin requests are allowed.
// Use "*" as a single element to allow all origins (not recommended for production).
func CheckOrigin(allowedOrigins []string) func(r *http.Request) bool {
	// Wildcard: allow all
	if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
		return func(r *http.Request) bool { return true }
	}

	// Build a set for fast lookup
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.TrimRight(o, "/")] = true
	}

	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			// No Origin header — likely a non-browser client (agent, curl), allow
			return true
		}

		// Same-origin check: compare origin with the Host header
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		if strings.EqualFold(u.Host, r.Host) {
			return true
		}

		// Check against explicit whitelist
		if len(allowed) > 0 {
			return allowed[strings.TrimRight(origin, "/")]
		}

		return false
	}
}
