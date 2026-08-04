package auth

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// Bootstrap seeds the very first admin from the config credentials and adopts
// pre-multi-user rows into it. It runs on every start and is idempotent: once
// any user exists, the config credentials are ignored entirely (login goes
// through the users table from then on).
//
// Adoption matters because conversations and API keys predate the ownership
// columns — without it, an upgraded deployment would show an empty chat list
// and orphaned keys.
func Bootstrap(username, password string) error {
	n, err := store.CountUsers()
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if n > 0 {
		return nil
	}

	if username == "" || password == "" {
		log.Printf("[auth] no users exist and config auth credentials are empty; " +
			"set auth.username/auth.password to create the first admin")
		return nil
	}

	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}

	admin := &model.User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: hash,
		DisplayName:  username,
		Role:         model.RoleAdmin,
		Status:       model.UserStatusActive,
		AuthSource:   model.AuthSourceLocal,
	}
	if err := store.CreateUser(admin); err != nil {
		return fmt.Errorf("create bootstrap admin: %w", err)
	}

	adopted, err := store.AdoptOwnerlessRows(admin.ID)
	if err != nil {
		return fmt.Errorf("adopt pre-existing rows: %w", err)
	}

	log.Printf("[auth] bootstrapped admin %q from config (adopted %d conversations, %d api keys). "+
		"Change the password in the UI; auth.username/auth.password in config.yaml is no longer used for login.",
		username, adopted.Conversations, adopted.APIKeys)
	return nil
}
