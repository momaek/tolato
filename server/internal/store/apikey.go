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

// ListAPIKeys returns all API keys.
func ListAPIKeys() ([]model.APIKey, error) {
	var keys []model.APIKey
	err := DB.Order("created_at DESC").Find(&keys).Error
	return keys, err
}

// UpdateAPIKey updates API key fields.
func UpdateAPIKey(id string, updates map[string]any) error {
	return DB.Model(&model.APIKey{}).Where("id = ?", id).Updates(updates).Error
}
