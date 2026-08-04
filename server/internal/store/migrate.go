package store

import (
	"fmt"

	"github.com/momaek/tolato/server/internal/model"
)

// AdoptCounts reports how many pre-multi-user rows were assigned an owner.
type AdoptCounts struct {
	Conversations int64
	APIKeys       int64
}

// AdoptOwnerlessRows assigns ownership of rows created before the ownership
// columns existed to the given user. Called once from the bootstrap path, right
// after the first admin is created.
func AdoptOwnerlessRows(userID string) (AdoptCounts, error) {
	var counts AdoptCounts

	res := DB.Model(&model.Conversation{}).Where("user_id IS NULL OR user_id = ''").
		Update("user_id", userID)
	if res.Error != nil {
		return counts, fmt.Errorf("adopt conversations: %w", res.Error)
	}
	counts.Conversations = res.RowsAffected

	res = DB.Model(&model.APIKey{}).Where("owner_user_id IS NULL OR owner_user_id = ''").
		Update("owner_user_id", userID)
	if res.Error != nil {
		return counts, fmt.Errorf("adopt api keys: %w", res.Error)
	}
	counts.APIKeys = res.RowsAffected

	return counts, nil
}

// migrateAPIKeyPermissions collapses the retired three-tier key permissions
// (readonly/standard/admin) into the current two tiers. "standard" and "admin"
// both become "writable" — they could already run commands, so nothing is
// escalated. Idempotent; a no-op once every row has been converted.
func migrateAPIKeyPermissions() error {
	return DB.Model(&model.APIKey{}).
		Where("permission IN ?", []string{"standard", "admin"}).
		Update("permission", model.APIKeyWritable).Error
}
