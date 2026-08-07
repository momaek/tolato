package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// CLI login (RFC 8252 loopback redirect + PKCE).
//
// `tolato auth login` listens on 127.0.0.1:<random port>, opens the browser at
// /cli-auth, and the user approves there. The approval mints a short-lived code
// that the browser carries back to the loopback listener; the CLI then trades
// that code — plus the verifier it never put in a URL — for the API key.
//
// The key deliberately does not travel in the redirect. A query string ends up
// in browser history, in any Referer the page emits, and in the access log of
// whatever proxy sits in front of us. A code that is single-use, expires in a
// minute, and is worthless without the verifier does not.

const (
	cliCodeTTL = time.Minute
	// A CLI key can only be as strong as the two tiers the rest of the system
	// knows about; the consent screen picks one.
	cliDefaultPermission = model.APIKeyReadonly
)

// pendingCLIAuth is one approved-but-not-yet-collected login.
type pendingCLIAuth struct {
	userID     string
	challenge  string // base64url(sha256(verifier)), from the CLI
	permission string
	label      string
	expiresAt  time.Time
}

// Held in memory rather than a table: these live for sixty seconds, and a
// server restart in the middle of a login is adequately handled by running the
// command again. Nothing here is worth a migration or an fsync.
var cliAuth = struct {
	sync.Mutex
	codes map[string]pendingCLIAuth
}{codes: map[string]pendingCLIAuth{}}

// sweepCLICodes drops expired entries. Called under the lock on every write, so
// an abandoned login cannot accumulate.
func sweepCLICodes(now time.Time) {
	for code, p := range cliAuth.codes {
		if now.After(p.expiresAt) {
			delete(cliAuth.codes, code)
		}
	}
}

type cliAuthorizeRequest struct {
	RedirectURI   string `json:"redirect_uri" binding:"required"`
	CodeChallenge string `json:"code_challenge" binding:"required"`
	Label         string `json:"label"`
	Permission    string `json:"permission"`
}

type cliAuthorizeResponse struct {
	Code string `json:"code"`
}

// CLIAuthorize handles POST /api/auth/cli/authorize.
//
// It is JWT-authenticated and must stay a POST driven by an explicit click in
// the consent screen. As a GET the browser would follow it automatically, and
// any page the user visited could mint a key out of their session.
func CLIAuthorize(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req cliAuthorizeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_request", Message: err.Error()})
			return
		}

		if err := validateLoopbackRedirect(req.RedirectURI); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_redirect_uri", Message: err.Error()})
			return
		}

		// A challenge we cannot verify later is the same as no PKCE at all.
		if len(req.CodeChallenge) < 43 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "invalid_request",
				Message: "code_challenge must be base64url(sha256(verifier))",
			})
			return
		}

		permission := req.Permission
		if permission == "" {
			permission = cliDefaultPermission
		}
		if permission != model.APIKeyReadonly && permission != model.APIKeyWritable {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_permission", Message: "Permission must be readonly or writable"})
			return
		}

		code, err := randomToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "Failed to generate code"})
			return
		}

		now := time.Now()
		cliAuth.Lock()
		sweepCLICodes(now)
		cliAuth.codes[code] = pendingCLIAuth{
			userID:     middleware.CurrentUserID(c),
			challenge:  req.CodeChallenge,
			permission: permission,
			label:      cliKeyName(req.Label),
			expiresAt:  now.Add(cliCodeTTL),
		}
		cliAuth.Unlock()

		c.JSON(http.StatusOK, cliAuthorizeResponse{Code: code})
	}
}

type cliExchangeRequest struct {
	Code         string `json:"code" binding:"required"`
	CodeVerifier string `json:"code_verifier" binding:"required"`
}

type cliExchangeResponse struct {
	APIKey     string `json:"api_key"`
	Name       string `json:"name"`
	Permission string `json:"permission"`
}

// CLIExchange handles POST /api/auth/cli/exchange.
//
// Public by necessity — the caller is a CLI that has no credential yet. What
// stands in for one is the code: single-use, a minute old at most, and only
// redeemable by whoever holds the verifier behind its challenge.
func CLIExchange(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req cliExchangeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_request", Message: err.Error()})
			return
		}

		// Consume the code whatever happens next: a failed verifier must not
		// leave it redeemable for a second attempt.
		cliAuth.Lock()
		pending, ok := cliAuth.codes[req.Code]
		delete(cliAuth.codes, req.Code)
		cliAuth.Unlock()

		if !ok || time.Now().After(pending.expiresAt) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_code", Message: "code is unknown, already used, or expired"})
			return
		}

		sum := sha256.Sum256([]byte(req.CodeVerifier))
		got := base64.RawURLEncoding.EncodeToString(sum[:])
		if subtle.ConstantTimeCompare([]byte(got), []byte(pending.challenge)) != 1 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_verifier", Message: "code_verifier does not match the challenge"})
			return
		}

		// The approving account may have been disabled between consent and
		// collection; a key must never outlive its owner.
		owner, err := store.GetUserByID(pending.userID)
		if err != nil || owner.Status != model.UserStatusActive {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{Error: "unauthorized", Message: "account is no longer active"})
			return
		}

		rawKey := make([]byte, 32)
		if _, err := rand.Read(rawKey); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "Failed to generate key"})
			return
		}
		keyStr := "tlat_" + hex.EncodeToString(rawKey)

		apiKey := &model.APIKey{
			ID:          uuid.New().String(),
			Name:        pending.label,
			OwnerUserID: owner.ID,
			KeyHash:     middleware.HashAPIKey(keyStr),
			KeyPrefix:   keyStr[:12],
			Permission:  pending.permission,
			Status:      "active",
		}
		if err := store.CreateAPIKey(apiKey); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: "Failed to create API key"})
			return
		}

		c.JSON(http.StatusOK, cliExchangeResponse{
			APIKey:     keyStr,
			Name:       apiKey.Name,
			Permission: apiKey.Permission,
		})
	}
}

// RevokeOwnAPIKey handles DELETE /api/v1/auth/key: the key presented by the
// caller revokes itself. `tolato auth logout` needs this — it holds an API key,
// not a session, so it cannot reach the JWT-guarded key management endpoints,
// and a logout that only deleted the local file would leave a live credential
// behind on the server.
func RevokeOwnAPIKey(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetString("api_key_id")
		if id == "" {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{Error: "unauthorized", Message: "no api key on this request"})
			return
		}
		if err := store.UpdateAPIKey(id, map[string]any{"status": "revoked"}); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: "Failed to revoke API key"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// validateLoopbackRedirect accepts only the shape RFC 8252 reserves for native
// apps: plain http, to a literal loopback address, on any port.
//
// The port has to stay free-form — the CLI binds :0 and learns which one it got
// — so the host is doing all of the work here. "localhost" is refused on
// purpose despite being the obvious spelling: it resolves through DNS, and a
// name that an attacker can influence is not a loopback guarantee.
func validateLoopbackRedirect(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errInvalidRedirect("not a URL")
	}
	if u.Scheme != "http" {
		return errInvalidRedirect("must be http on a loopback address")
	}
	if u.User != nil {
		return errInvalidRedirect("must not carry userinfo")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errInvalidRedirect("must not carry a query or fragment")
	}
	if u.Path != "/callback" {
		return errInvalidRedirect("path must be /callback")
	}

	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return errInvalidRedirect("must include a port")
	}
	if host != "127.0.0.1" && host != "::1" {
		return errInvalidRedirect("host must be 127.0.0.1 or ::1")
	}
	if p, err := net.LookupPort("tcp", port); err != nil || p == 0 {
		return errInvalidRedirect("port must be numeric")
	}
	return nil
}

type redirectError string

func (e redirectError) Error() string { return "redirect_uri " + string(e) }

func errInvalidRedirect(reason string) error { return redirectError(reason) }

// cliKeyName labels the key with whatever the CLI reported about its machine,
// so it is identifiable in Settings → API Keys and can be revoked on its own.
// The label is attacker-influenced only to the extent the user's own CLI is, but
// it is still displayed, so cap it.
func cliKeyName(label string) string {
	const (
		prefix = "CLI on "
		max    = 60 // bounds the whole name, prefix included
	)
	if label == "" {
		return "CLI"
	}
	if len(label) > max-len(prefix) {
		label = label[:max-len(prefix)]
	}
	return prefix + label
}
