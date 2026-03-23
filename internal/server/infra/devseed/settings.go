package devseed

import (
	"context"
	"encoding/json"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
	"github.com/momaek/tolato/internal/server/infra/store/memory"
)

func SeedSettingsStore(ctx context.Context, store *memory.Store, userID string, now time.Time) error {
	if store == nil {
		return domain.ErrInvalidArgument
	}
	if userID == "" {
		userID = "admin"
	}

	records := []struct {
		key   domain.SettingKey
		value any
	}{
		{
			key: domain.SettingKeyModelConfig,
			value: map[string]any{
				"provider":     "OpenAI",
				"model":        "gpt-5.4",
				"endpoint":     "https://api.openai.com/v1",
				"apiKey":       "sk-dev-placeholder",
				"temperature":  0.2,
				"maxTokens":    2048,
				"timeoutSec":   60,
				"approvalMode": "balanced",
			},
		},
		{
			key: domain.SettingKeyAccountSecurity,
			value: map[string]any{
				"username":           userID,
				"lastLoginAt":        now.UTC().Add(-4 * time.Hour).Format(time.RFC3339),
				"mfaEnabled":         true,
				"auditRetentionDays": 90,
			},
		},
		{
			key: domain.SettingKeyPreferences,
			value: map[string]any{
				"preferredRegion": "Tokyo",
				"defaultMode":     "ai_agent",
				"locale":          "zh-CN",
				"compactTimeline": false,
				"streamMarkdown":  true,
			},
		},
	}

	for _, item := range records {
		raw, err := json.Marshal(item.value)
		if err != nil {
			return err
		}
		if err := store.Settings.Put(ctx, domain.SettingRecord{
			UserID:    userID,
			Key:       item.key,
			Value:     raw,
			UpdatedAt: now.UTC(),
		}); err != nil {
			return err
		}
	}

	return nil
}
