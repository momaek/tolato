// Package auth holds credential primitives shared by the login handlers and
// the user-management API: password hashing/verification and the first-run
// bootstrap that turns the config credentials into the initial admin account.
package auth

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLength is enforced on every password set through the API. The
// bootstrap path deliberately skips it so an existing deployment with a short
// config password still upgrades cleanly.
const MinPasswordLength = 8

// ErrPasswordTooShort is returned by ValidatePassword.
var ErrPasswordTooShort = errors.New("password too short")

// HashPassword returns the bcrypt hash of a plaintext password.
func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// CheckPassword reports whether plain matches the stored bcrypt hash. An empty
// hash (OIDC-only accounts) never matches.
func CheckPassword(hash, plain string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// ValidatePassword enforces the minimum length rule.
func ValidatePassword(plain string) error {
	if utf8.RuneCountInString(plain) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}
