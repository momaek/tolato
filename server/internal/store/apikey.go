package store

import (
	"github.com/momaek/tolato/server/internal/model"
)

// CreateAPIKey creates a new API key record.
func CreateAPIKey(key *model.APIKey) error {
	return DB.Create(key).Error
}

// GetAPIKeyByHash finds an API key by its hash.
func GetAPIKeyByHash(keyHash string) (*model.APIKey, error) {
	var key model.APIKey
	if err := DB.Where("key_hash = ?", keyHash).First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// GetAPIKeyByID finds an API key by primary key.
func GetAPIKeyByID(id string) (*model.APIKey, error) {
	var key model.APIKey
	if err := DB.First(&key, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// ListAllAPIKeys returns every API key. Admin-only view.
func ListAllAPIKeys() ([]model.APIKey, error) {
	var keys []model.APIKey
	err := DB.Order("created_at DESC").Find(&keys).Error
	return keys, err
}

// ListAPIKeysByOwner returns the keys belonging to one user.
func ListAPIKeysByOwner(ownerUserID string) ([]model.APIKey, error) {
	var keys []model.APIKey
	err := DB.Where("owner_user_id = ?", ownerUserID).
		Order("created_at DESC").Find(&keys).Error
	return keys, err
}

// UpdateAPIKey updates API key fields.
func UpdateAPIKey(id string, updates map[string]any) error {
	return DB.Model(&model.APIKey{}).Where("id = ?", id).Updates(updates).Error
}
