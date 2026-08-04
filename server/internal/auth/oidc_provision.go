package auth

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
	"gorm.io/gorm"
)

// ErrOIDCSignupDisabled is returned when an unknown subject signs in and
// auto-provisioning is off.
var ErrOIDCSignupDisabled = errors.New("no local account for this identity")

// ErrOIDCAccountDisabled is returned when the matched account is disabled.
var ErrOIDCAccountDisabled = errors.New("account is disabled")

// usernameSanitizer strips whatever the IdP sends down to a login-safe handle.
var usernameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// ResolveOIDCUser maps a verified identity onto a local account, creating one
// when signup is allowed.
//
// Matching is by `sub`, never by email: an email address can be reassigned to a
// different person at the IdP, and matching on it would hand them the old
// account. The subject is the IdP's stable, non-reusable identifier.
func ResolveOIDCUser(id *OIDCIdentity, s model.OIDCSettings) (*model.User, error) {
	user, err := store.GetUserByOIDCSubject(id.Subject)
	switch {
	case err == nil:
		if user.Status != model.UserStatusActive {
			return nil, ErrOIDCAccountDisabled
		}
		syncOIDCProfile(user, id)
		return user, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, fmt.Errorf("look up oidc subject: %w", err)
	}

	// First sign-in for this subject. Adopt a local account that already owns
	// this verified email, so an existing admin who switches to SSO keeps their
	// data instead of getting a second, empty account. An unverified email is
	// not proof of ownership and is never adopted on.
	if id.EmailVerified && id.Email != "" {
		if existing, err := store.GetUserByEmail(id.Email); err == nil {
			if existing.AuthSource == model.AuthSourceLocal {
				if existing.Status != model.UserStatusActive {
					return nil, ErrOIDCAccountDisabled
				}
				sub := id.Subject
				updates := map[string]any{
					"auth_source":   model.AuthSourceOIDC,
					"oidc_subject":  &sub,
					"password_hash": "", // the local password stops being a way in
				}
				if err := store.UpdateUser(existing.ID, updates); err != nil {
					return nil, fmt.Errorf("link oidc subject: %w", err)
				}
				return store.GetUserByID(existing.ID)
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("look up email: %w", err)
		}
	}

	if !s.AllowSignup {
		return nil, ErrOIDCSignupDisabled
	}

	username, err := uniqueUsername(preferredUsername(id))
	if err != nil {
		return nil, err
	}
	sub := id.Subject
	u := &model.User{
		ID:          uuid.New().String(),
		Username:    username,
		DisplayName: orDefault(id.Name, username),
		Email:       id.Email,
		Role:        oidcRole(id, s),
		Status:      model.UserStatusActive,
		AuthSource:  model.AuthSourceOIDC,
		OIDCSubject: &sub,
	}
	if err := store.CreateUser(u); err != nil {
		return nil, fmt.Errorf("create oidc user: %w", err)
	}
	return u, nil
}

// oidcRole grants admin only to a verified email on the configured list.
// Requiring verification matters: an IdP that lets users set an arbitrary
// unverified email would otherwise let anyone claim an admin address.
func oidcRole(id *OIDCIdentity, s model.OIDCSettings) string {
	if !id.EmailVerified || id.Email == "" {
		return model.RoleMember
	}
	for _, e := range s.AdminEmails {
		if strings.EqualFold(strings.TrimSpace(e), id.Email) {
			return model.RoleAdmin
		}
	}
	return model.RoleMember
}

// syncOIDCProfile refreshes the display fields the IdP owns. The role is
// deliberately left alone: it is administered in Tolato, so a local promotion
// isn't undone on next sign-in.
func syncOIDCProfile(user *model.User, id *OIDCIdentity) {
	updates := map[string]any{}
	if name := orDefault(id.Name, id.Username); name != "" && name != user.DisplayName {
		updates["display_name"] = name
	}
	if id.Email != "" && id.Email != user.Email {
		updates["email"] = id.Email
	}
	if len(updates) == 0 {
		return
	}
	if err := store.UpdateUser(user.ID, updates); err == nil {
		if v, ok := updates["display_name"].(string); ok {
			user.DisplayName = v
		}
		if v, ok := updates["email"].(string); ok {
			user.Email = v
		}
	}
}

// preferredUsername picks the nicest available handle for a new account.
func preferredUsername(id *OIDCIdentity) string {
	for _, candidate := range []string{id.Username, strings.SplitN(id.Email, "@", 2)[0]} {
		if clean := usernameSanitizer.ReplaceAllString(candidate, ""); clean != "" {
			return strings.ToLower(clean)
		}
	}
	return "user"
}

// uniqueUsername appends a numeric suffix until the handle is free, so two
// IdP accounts that prefer the same name can both sign in.
func uniqueUsername(base string) (string, error) {
	candidate := base
	for i := 2; i < 100; i++ {
		_, err := store.GetUserByUsername(candidate)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("check username: %w", err)
		}
		candidate = fmt.Sprintf("%s%d", base, i)
	}
	return "", errors.New("could not derive a free username")
}
