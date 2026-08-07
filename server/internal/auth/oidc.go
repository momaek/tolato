package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/momaek/tolato/server/internal/model"
	"golang.org/x/oauth2"
)

// OIDCSettingKey is where the single-provider config lives in the settings
// table. Admins edit it from the UI; nothing OIDC-related is read from
// config.yaml.
const OIDCSettingKey = "oidc.config"

// oidcDiscoveryTimeout caps the discovery request. An unreachable IdP must fail
// the login attempt promptly rather than hanging the request.
const oidcDiscoveryTimeout = 10 * time.Second

// ErrOIDCDisabled is returned when an OIDC endpoint is reached while SSO is off
// or incompletely configured.
var ErrOIDCDisabled = errors.New("single sign-on is not enabled")

// OIDCProvider wraps a discovered IdP plus the OAuth2 config derived from it.
type OIDCProvider struct {
	provider *gooidc.Provider
	verifier *gooidc.IDTokenVerifier
	oauth    *oauth2.Config
}

// OIDCManager lazily builds an OIDCProvider from the stored settings and caches
// it, rediscovering only when the settings that define the provider change.
//
// Discovery is a network round trip, so doing it per login would put the IdP on
// the critical path of every sign-in; caching keeps that to once per config.
type OIDCManager struct {
	mu       sync.Mutex
	cached   *OIDCProvider
	cacheKey string // settings fingerprint the cached provider was built from
}

// NewOIDCManager returns an empty manager. Providers are built on first use.
func NewOIDCManager() *OIDCManager { return &OIDCManager{} }

// Get returns a provider for the given settings, reusing the cached one when
// the relevant fields are unchanged. Returns ErrOIDCDisabled when SSO is off or
// the config is incomplete.
func (m *OIDCManager) Get(ctx context.Context, s model.OIDCSettings, redirectURL string) (*OIDCProvider, error) {
	if !s.Enabled || s.Issuer == "" || s.ClientID == "" || s.ClientSecret == "" || redirectURL == "" {
		return nil, ErrOIDCDisabled
	}

	key := strings.Join([]string{s.Issuer, s.ClientID, s.ClientSecret, redirectURL,
		strings.Join(s.Scopes, ",")}, "\x00")

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cached != nil && m.cacheKey == key {
		return m.cached, nil
	}

	p, err := buildOIDCProvider(ctx, s, redirectURL)
	if err != nil {
		return nil, err
	}
	m.cached, m.cacheKey = p, key
	return p, nil
}

// Reset drops the cached provider so the next Get rediscovers. Called after the
// settings are saved, making edits take effect without a restart.
func (m *OIDCManager) Reset() {
	m.mu.Lock()
	m.cached, m.cacheKey = nil, ""
	m.mu.Unlock()
}

// buildOIDCProvider runs discovery against the issuer and assembles the OAuth2
// config. Exported behavior is also what the settings "verify" button exercises.
func buildOIDCProvider(ctx context.Context, s model.OIDCSettings, redirectURL string) (*OIDCProvider, error) {
	ctx, cancel := context.WithTimeout(ctx, oidcDiscoveryTimeout)
	defer cancel()

	provider, err := gooidc.NewProvider(ctx, strings.TrimSuffix(s.Issuer, "/"))
	if err != nil {
		return nil, fmt.Errorf("discover issuer: %w", err)
	}

	return &OIDCProvider{
		provider: provider,
		verifier: provider.Verifier(&gooidc.Config{ClientID: s.ClientID}),
		oauth: &oauth2.Config{
			ClientID:     s.ClientID,
			ClientSecret: s.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  redirectURL,
			Scopes:       normalizeScopes(s.Scopes),
		},
	}, nil
}

// VerifyOIDCConfig performs discovery only, so the admin can check an issuer
// before turning SSO on. Returns the resolved authorization endpoint.
func VerifyOIDCConfig(ctx context.Context, s model.OIDCSettings, redirectURL string) (string, error) {
	// Discovery itself doesn't need the client credentials, so verify works on a
	// half-filled form — but not without an issuer to discover.
	if s.Issuer == "" {
		return "", errors.New("issuer is required")
	}
	probe := s
	probe.Enabled = true
	if probe.ClientID == "" {
		probe.ClientID = "verify-only"
	}
	if probe.ClientSecret == "" {
		probe.ClientSecret = "verify-only"
	}
	p, err := buildOIDCProvider(ctx, probe, orDefault(redirectURL, "http://localhost/callback"))
	if err != nil {
		return "", err
	}
	return p.oauth.Endpoint.AuthURL, nil
}

// AuthCodeURL builds the IdP redirect for one login attempt. The nonce is
// echoed back inside the ID token and checked, binding the token to this
// attempt so a token captured elsewhere can't be replayed into it.
func (p *OIDCProvider) AuthCodeURL(state, nonce string) string {
	return p.oauth.AuthCodeURL(state, gooidc.Nonce(nonce))
}

// OIDCIdentity is what a verified ID token tells us about the person.
type OIDCIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Username      string // preferred_username, when the IdP supplies one

	// Groups holds the IdP group names read from the configured claim.
	// GroupsPresent distinguishes "the IdP said this user is in no groups"
	// from "the token carried no group claim at all" — the first is an
	// instruction to remove memberships, the second means we know nothing and
	// must not touch them.
	Groups        []string
	GroupsPresent bool
}

// Exchange trades the authorization code for tokens, verifies the ID token
// signature and audience, checks the nonce, and returns the claims we use.
//
// groupClaim names the claim carrying the user's IdP groups; empty skips
// group extraction.
func (p *OIDCProvider) Exchange(ctx context.Context, code, nonce, groupClaim string) (*OIDCIdentity, error) {
	ctx, cancel := context.WithTimeout(ctx, oidcDiscoveryTimeout)
	defer cancel()

	tok, err := p.oauth.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return nil, errors.New("provider response carried no id_token")
	}

	idToken, err := p.verifier.Verify(ctx, rawID)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}
	if idToken.Nonce != nonce {
		return nil, errors.New("id_token nonce mismatch")
	}

	var claims struct {
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}
	if idToken.Subject == "" {
		return nil, errors.New("id_token carried no subject")
	}

	identity := &OIDCIdentity{
		Subject:       idToken.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		Name:          claims.Name,
		Username:      claims.PreferredUsername,
	}

	// The group claim's key is configurable, so it can't be a struct field —
	// decode the whole claim set again and look the key up by name.
	if groupClaim != "" {
		var raw map[string]json.RawMessage
		if err := idToken.Claims(&raw); err != nil {
			return nil, fmt.Errorf("decode claims: %w", err)
		}
		if v, ok := raw[groupClaim]; ok {
			groups, err := parseGroupsClaim(v)
			if err != nil {
				return nil, fmt.Errorf("claim %q: %w", groupClaim, err)
			}
			identity.Groups = groups
			identity.GroupsPresent = true
		}
	}

	return identity, nil
}

// parseGroupsClaim accepts the two shapes IdPs actually send: a list of names,
// or a single name as a bare string. A JSON null is treated as an empty list —
// the claim was present, so it still means "no groups".
func parseGroupsClaim(raw json.RawMessage) ([]string, error) {
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if single == "" {
			return nil, nil
		}
		return []string{single}, nil
	}
	var isNull any
	if err := json.Unmarshal(raw, &isNull); err == nil && isNull == nil {
		return nil, nil
	}
	return nil, errors.New("expected a string or an array of strings")
}

// normalizeScopes guarantees "openid" is present — without it the IdP runs a
// plain OAuth2 flow and returns no ID token.
func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{gooidc.ScopeOpenID, "profile", "email"}
	}
	for _, s := range scopes {
		if s == gooidc.ScopeOpenID {
			return scopes
		}
	}
	return append([]string{gooidc.ScopeOpenID}, scopes...)
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
