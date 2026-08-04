package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/auth"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/settings"
	"github.com/momaek/tolato/server/internal/store"
)

// Cookies holding the one-attempt CSRF state and replay nonce. They are set
// just before the redirect to the IdP and cleared on return.
const (
	oidcStateCookie  = "tolato_oidc_state"
	oidcNonceCookie  = "tolato_oidc_nonce"
	oidcCookieMaxAge = 600 // seconds; a login attempt that takes longer has failed
)

// oidcCallbackPath is where the IdP sends the browser back. Also shown in the
// settings UI so the admin can register it with the provider.
const oidcCallbackPath = "/api/auth/oidc/callback"

// loadOIDCSettings reads the stored config, applying defaults.
func loadOIDCSettings(deps *Deps) model.OIDCSettings {
	s := model.OIDCSettings{}
	settings.GetJSON(deps.Settings, auth.OIDCSettingKey, &s)
	return s
}

// oidcRedirectURL is the absolute callback URL registered with the IdP. It is
// derived from the configured public address so it stays correct behind a
// reverse proxy, where the request's own host may be the internal one.
func oidcRedirectURL(deps *Deps, c *gin.Context) string {
	base := strings.TrimSuffix(deps.Config.Server.PublicAddress, "/")
	if base == "" {
		scheme := "http"
		if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		base = scheme + "://" + c.Request.Host
	}
	return base + oidcCallbackPath
}

// OIDCStatus handles GET /api/auth/oidc/status. Unauthenticated by necessity —
// the login page needs it before anyone has signed in — so it reveals only
// whether the SSO button should be shown.
func OIDCStatus(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := loadOIDCSettings(deps)
		c.JSON(http.StatusOK, model.OIDCStatusResponse{
			Enabled: s.Enabled && s.Issuer != "" && s.ClientID != "" && s.ClientSecret != "",
		})
	}
}

// OIDCLogin handles GET /api/auth/oidc/login by redirecting to the IdP.
func OIDCLogin(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := loadOIDCSettings(deps)
		provider, err := deps.OIDC.Get(c.Request.Context(), s, oidcRedirectURL(deps, c))
		if err != nil {
			log.Printf("[oidc] login unavailable: %v", err)
			redirectToLoginError(c, "sso_unavailable")
			return
		}

		state, err := randomToken()
		if err != nil {
			redirectToLoginError(c, "sso_failed")
			return
		}
		nonce, err := randomToken()
		if err != nil {
			redirectToLoginError(c, "sso_failed")
			return
		}

		setOIDCCookie(c, oidcStateCookie, state)
		setOIDCCookie(c, oidcNonceCookie, nonce)

		c.Redirect(http.StatusFound, provider.AuthCodeURL(state, nonce))
	}
}

// OIDCCallback handles GET /api/auth/oidc/callback: it validates the round
// trip, resolves the local account, and hands the session token to the SPA.
func OIDCCallback(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Clear the one-attempt cookies whatever the outcome, so a failed or
		// replayed attempt can't reuse them.
		defer func() {
			clearOIDCCookie(c, oidcStateCookie)
			clearOIDCCookie(c, oidcNonceCookie)
		}()

		if errParam := c.Query("error"); errParam != "" {
			log.Printf("[oidc] provider returned error: %s", errParam)
			redirectToLoginError(c, "sso_denied")
			return
		}

		// The state cookie is what proves this callback belongs to a login this
		// browser started. Without the comparison, an attacker could feed their
		// own authorization code to a victim's browser and sign them into the
		// attacker's account.
		state, err := c.Cookie(oidcStateCookie)
		if err != nil || state == "" || state != c.Query("state") {
			log.Printf("[oidc] state mismatch or missing")
			redirectToLoginError(c, "sso_state")
			return
		}
		nonce, err := c.Cookie(oidcNonceCookie)
		if err != nil || nonce == "" {
			redirectToLoginError(c, "sso_state")
			return
		}

		code := c.Query("code")
		if code == "" {
			redirectToLoginError(c, "sso_failed")
			return
		}

		s := loadOIDCSettings(deps)
		provider, err := deps.OIDC.Get(c.Request.Context(), s, oidcRedirectURL(deps, c))
		if err != nil {
			log.Printf("[oidc] callback unavailable: %v", err)
			redirectToLoginError(c, "sso_unavailable")
			return
		}

		identity, err := provider.Exchange(c.Request.Context(), code, nonce)
		if err != nil {
			log.Printf("[oidc] exchange failed: %v", err)
			redirectToLoginError(c, "sso_failed")
			return
		}

		user, err := auth.ResolveOIDCUser(identity, s)
		if err != nil {
			log.Printf("[oidc] could not resolve subject %s: %v", identity.Subject, err)
			switch {
			case errors.Is(err, auth.ErrOIDCSignupDisabled):
				redirectToLoginError(c, "sso_no_account")
			case errors.Is(err, auth.ErrOIDCAccountDisabled):
				redirectToLoginError(c, "sso_disabled")
			default:
				redirectToLoginError(c, "sso_failed")
			}
			return
		}

		token, expiresAt, err := middleware.GenerateToken(user)
		if err != nil {
			redirectToLoginError(c, "sso_failed")
			return
		}
		touchLastLogin(user)

		// The token travels in the URL fragment, not the query string: fragments
		// are never sent to a server, kept out of access logs, and stripped from
		// the Referer header. The SPA reads it, stores it, and clears the hash.
		frag := url.Values{}
		frag.Set("token", token)
		frag.Set("expires_at", expiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"))
		c.Redirect(http.StatusFound, "/login#"+frag.Encode())
	}
}

// touchLastLogin records the sign-in time, best-effort.
func touchLastLogin(user *model.User) {
	now := time.Now()
	if err := store.UpdateUser(user.ID, map[string]any{"last_login_at": &now}); err != nil {
		log.Printf("[oidc] failed to record last_login_at for %s: %v", user.Username, err)
	}
}

// redirectToLoginError sends the browser back to the login page with a code the
// SPA turns into a localized message. The code is a fixed enum, never the
// underlying error, which stays in the server log.
func redirectToLoginError(c *gin.Context, code string) {
	c.Redirect(http.StatusFound, "/login?sso_error="+url.QueryEscape(code))
}

func setOIDCCookie(c *gin.Context, name, value string) {
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	// SameSite=Lax still sends the cookie on the IdP's top-level GET redirect
	// back to us, while keeping it off cross-site subrequests.
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, oidcCookieMaxAge, "/", "", secure, true)
}

func clearOIDCCookie(c *gin.Context, name string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, "", -1, "/", "", false, true)
}

// randomToken returns 32 bytes of URL-safe randomness for state/nonce values.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// --- Settings API ---------------------------------------------------------

// GetOIDCSettings handles GET /api/settings/oidc (admin only).
func GetOIDCSettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := loadOIDCSettings(deps)
		s.ClientSecret = maskSecret(s.ClientSecret)
		c.JSON(http.StatusOK, model.OIDCSettingsResponse{
			OIDCSettings: s,
			RedirectURL:  oidcRedirectURL(deps, c),
		})
	}
}

// PutOIDCSettings handles PUT /api/settings/oidc (admin only).
func PutOIDCSettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.OIDCSettings
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "invalid request body",
			})
			return
		}

		prev := loadOIDCSettings(deps)
		// A secret still carrying the mask means "unchanged" — the form was
		// saved without retyping it.
		if strings.Contains(req.ClientSecret, "****") || req.ClientSecret == "" {
			req.ClientSecret = prev.ClientSecret
		}
		req.Issuer = strings.TrimSpace(req.Issuer)
		req.ClientID = strings.TrimSpace(req.ClientID)

		if req.Enabled && (req.Issuer == "" || req.ClientID == "" || req.ClientSecret == "") {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "issuer, client id and client secret are required to enable single sign-on",
			})
			return
		}

		raw, err := json.Marshal(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to encode settings",
			})
			return
		}
		if err := store.SetSetting(auth.OIDCSettingKey, string(raw)); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to save settings",
			})
			return
		}

		// store.SetSetting already invalidates the settings cache; the provider
		// cache is separate and has to be dropped too so the next sign-in
		// rediscovers against the new issuer without a restart.
		deps.OIDC.Reset()

		c.JSON(http.StatusOK, gin.H{"message": "updated"})
	}
}

// VerifyOIDCSettings handles POST /api/settings/oidc/verify (admin only). It
// runs discovery against the submitted issuer without saving anything.
func VerifyOIDCSettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.OIDCSettings
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "invalid request body",
			})
			return
		}
		if strings.Contains(req.ClientSecret, "****") || req.ClientSecret == "" {
			req.ClientSecret = loadOIDCSettings(deps).ClientSecret
		}

		authURL, err := auth.VerifyOIDCConfig(c.Request.Context(), req, oidcRedirectURL(deps, c))
		if err != nil {
			c.JSON(http.StatusOK, model.VerifyOIDCResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, model.VerifyOIDCResponse{
			Success:               true,
			Issuer:                strings.TrimSuffix(req.Issuer, "/"),
			AuthorizationEndpoint: authURL,
		})
	}
}

// maskSecret renders a stored secret for display without revealing it.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
