package store

import (
	"github.com/momaek/tolato/server/internal/model"
)

// OnSettingChanged is called after any successful setting write with the keys
// that were changed. The settings cache registers itself here on startup so it
// can invalidate its entries without this package importing the cache package.
// Nil-safe: if unset, writes just succeed silently.
var OnSettingChanged func(keys []string)

func notifySettingChange(keys ...string) {
	if OnSettingChanged != nil {
		OnSettingChanged(keys)
	}
}

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
	if err := DB.Save(&model.Setting{
		Key:   key,
		Value: value,
	}).Error; err != nil {
		return err
	}
	notifySettingChange(key)
	return nil
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
	keys := make([]string, 0, len(settings))
	for key, value := range settings {
		if err := tx.Save(&model.Setting{Key: key, Value: value}).Error; err != nil {
			tx.Rollback()
			return err
		}
		keys = append(keys, key)
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	notifySettingChange(keys...)
	return nil
}
