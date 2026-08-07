package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/config"
)

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"short", "****"},
		{"12345678", "****"},
		{"abcdefghijklmnop", "abcd****mnop"},
	}
	for _, tt := range tests {
		if got := maskSecret(tt.in); got != tt.want {
			t.Errorf("maskSecret(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// The mask must never leak enough to reconstruct the secret: a 9-character
// value shows 4+4 characters, so the threshold below that falls back to a
// full mask.
func TestMaskSecretNeverRevealsWholeValue(t *testing.T) {
	for _, secret := range []string{"a", "abcd", "abcdefgh", "abcdefghi"} {
		got := maskSecret(secret)
		if got == secret {
			t.Errorf("maskSecret(%q) returned the secret verbatim", secret)
		}
	}
}

// The callback URL is what the admin registers with the IdP and what the token
// exchange is validated against, so a proxied deployment must derive it from
// the configured public address rather than the internal request host.
func TestOIDCRedirectURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		publicAddress string
		host          string
		forwardedFor  string
		want          string
	}{
		{
			name:          "public address wins over request host",
			publicAddress: "https://tolato.example.com",
			host:          "127.0.0.1:8080",
			want:          "https://tolato.example.com/api/auth/oidc/callback",
		},
		{
			name:          "trailing slash is not doubled",
			publicAddress: "https://tolato.example.com/",
			host:          "127.0.0.1:8080",
			want:          "https://tolato.example.com/api/auth/oidc/callback",
		},
		{
			name:          "falls back to the request host",
			publicAddress: "",
			host:          "tolato.local:8080",
			want:          "http://tolato.local:8080/api/auth/oidc/callback",
		},
		{
			name:          "honors the proxy's https header",
			publicAddress: "",
			host:          "tolato.local",
			forwardedFor:  "https",
			want:          "https://tolato.local/api/auth/oidc/callback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := &Deps{Config: &config.Config{}}
			deps.Config.Server.PublicAddress = tt.publicAddress

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/api/auth/oidc/login", nil)
			c.Request.Host = tt.host
			if tt.forwardedFor != "" {
				c.Request.Header.Set("X-Forwarded-Proto", tt.forwardedFor)
			}

			if got := oidcRedirectURL(deps, c); got != tt.want {
				t.Errorf("oidcRedirectURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
