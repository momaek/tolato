package auth

import (
	"testing"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/momaek/tolato/server/internal/model"
)

// Without the openid scope the IdP runs a plain OAuth2 flow and never returns
// an ID token, so the whole sign-in silently has nothing to verify.
func TestNormalizeScopesAlwaysIncludesOpenID(t *testing.T) {
	tests := []struct {
		name string
		in   []string
	}{
		{"empty falls back to defaults", nil},
		{"caller omitted openid", []string{"profile", "email"}},
		{"caller already included it", []string{gooidc.ScopeOpenID, "groups"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeScopes(tt.in)
			found := false
			for _, s := range got {
				if s == gooidc.ScopeOpenID {
					found = true
				}
			}
			if !found {
				t.Errorf("normalizeScopes(%v) = %v, missing openid", tt.in, got)
			}
		})
	}
}

func TestNormalizeScopesKeepsCallerScopes(t *testing.T) {
	got := normalizeScopes([]string{"profile", "groups"})
	want := map[string]bool{gooidc.ScopeOpenID: false, "profile": false, "groups": false}
	for _, s := range got {
		if _, ok := want[s]; ok {
			want[s] = true
		}
	}
	for scope, seen := range want {
		if !seen {
			t.Errorf("scope %q was dropped: got %v", scope, got)
		}
	}
}

// Admin is granted by email, so an unverified email must never qualify —
// otherwise an IdP that lets users type any address hands out admin.
func TestOIDCRoleRequiresVerifiedEmail(t *testing.T) {
	s := model.OIDCSettings{AdminEmails: []string{"boss@example.com"}}

	tests := []struct {
		name     string
		identity *OIDCIdentity
		want     string
	}{
		{
			name:     "verified email on the list",
			identity: &OIDCIdentity{Email: "boss@example.com", EmailVerified: true},
			want:     model.RoleAdmin,
		},
		{
			name:     "same email but unverified",
			identity: &OIDCIdentity{Email: "boss@example.com", EmailVerified: false},
			want:     model.RoleMember,
		},
		{
			name:     "verified email not on the list",
			identity: &OIDCIdentity{Email: "someone@example.com", EmailVerified: true},
			want:     model.RoleMember,
		},
		{
			name:     "no email at all",
			identity: &OIDCIdentity{EmailVerified: true},
			want:     model.RoleMember,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := oidcRole(tt.identity, s); got != tt.want {
				t.Errorf("oidcRole() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Address comparison is case-insensitive and tolerant of stray whitespace in
// the admin's configured list.
func TestOIDCRoleMatchesEmailLoosely(t *testing.T) {
	s := model.OIDCSettings{AdminEmails: []string{"  Boss@Example.COM  "}}
	id := &OIDCIdentity{Email: "boss@example.com", EmailVerified: true}
	if got := oidcRole(id, s); got != model.RoleAdmin {
		t.Errorf("oidcRole() = %q, want admin for a case/whitespace variant", got)
	}
}

func TestPreferredUsername(t *testing.T) {
	tests := []struct {
		name string
		id   *OIDCIdentity
		want string
	}{
		{"preferred_username wins", &OIDCIdentity{Username: "Alice", Email: "a@b.com"}, "alice"},
		{"falls back to email local part", &OIDCIdentity{Email: "Bob.Smith@example.com"}, "bob.smith"},
		{"strips unsafe characters", &OIDCIdentity{Username: "al ice!@#"}, "alice"},
		{"nothing usable", &OIDCIdentity{}, "user"},
		// A CJK-only handle sanitizes to empty; falling through to "user" beats
		// creating an account with a blank username.
		{"non-ascii handle", &OIDCIdentity{Username: "张三"}, "user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := preferredUsername(tt.id); got != tt.want {
				t.Errorf("preferredUsername() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerifyOIDCConfigRequiresIssuer(t *testing.T) {
	if _, err := VerifyOIDCConfig(t.Context(), model.OIDCSettings{}, ""); err == nil {
		t.Error("VerifyOIDCConfig accepted an empty issuer")
	}
}

// A disabled or incompletely configured provider must not be reachable.
func TestOIDCManagerRejectsIncompleteConfig(t *testing.T) {
	m := NewOIDCManager()
	tests := []struct {
		name string
		s    model.OIDCSettings
	}{
		{"disabled", model.OIDCSettings{Issuer: "https://idp", ClientID: "id", ClientSecret: "sec"}},
		{"no issuer", model.OIDCSettings{Enabled: true, ClientID: "id", ClientSecret: "sec"}},
		{"no client id", model.OIDCSettings{Enabled: true, Issuer: "https://idp", ClientSecret: "sec"}},
		{"no secret", model.OIDCSettings{Enabled: true, Issuer: "https://idp", ClientID: "id"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := m.Get(t.Context(), tt.s, "https://app/callback"); err != ErrOIDCDisabled {
				t.Errorf("Get() error = %v, want ErrOIDCDisabled", err)
			}
		})
	}
}
