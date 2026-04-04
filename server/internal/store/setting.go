package store

import (
	"github.com/momaek/tolato/server/internal/model"
)

// GetSetting returns a single setting by key.
func GetSetting(key string) (*model.Setting, error) {
	var s model.Setting
	if err := DB.First(&s, "key = ?", key).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// SetSetting upserts a setting key-value pair.
func SetSetting(key, value string) error {
	return DB.Save(&model.Setting{
		Key:   key,
		Value: value,
	}).Error
}

// GetSettingsGroup returns all settings matching a prefix (e.g., "llm.").
func GetSettingsGroup(prefix string) ([]model.Setting, error) {
	var settings []model.Setting
	err := DB.Where("key LIKE ?", prefix+"%").Find(&settings).Error
	return settings, err
}

// SetSettingsGroup saves multiple settings for a group.
func SetSettingsGroup(settings map[string]string) error {
	tx := DB.Begin()
	for key, value := range settings {
		if err := tx.Save(&model.Setting{Key: key, Value: value}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}
