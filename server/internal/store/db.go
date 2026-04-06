package store

import (
	"encoding/json"
	"fmt"

	"github.com/momaek/tolato/server/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the global database instance.
var DB *gorm.DB

// InitDB initializes the database connection and runs migrations.
func InitDB(dsn string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Auto-migrate all models
	if err := db.AutoMigrate(
		&model.Conversation{},
		&model.Message{},
		&model.Node{},
		&model.RegistrationToken{},
		&model.AuditLog{},
		&model.Setting{},
		&model.APIKey{},
		&model.ProbeLink{},
		&model.ProbeMetric{},
		&model.ProbeAlert{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}

	DB = db

	// Initialize default settings
	initDefaultSettings()

	return nil
}

func initDefaultSettings() {
	defaults := map[string]any{
		"llm.api_base_url":           "",
		"llm.api_key":                "",
		"llm.default_model":          "",
		"llm.max_rounds":             20,
		"llm.temperature":            0.7,
		"security.confirm_enabled":   true,
		"security.sensitive_keywords": []string{"rm -rf", "reboot", "shutdown", "mkfs", "dd if=", ":(){ :|:& };:"},
		"security.command_blacklist":  []string{},
		"agent.heartbeat_interval":   30,
		"agent.command_timeout":      60,
		"agent.output_max_lines":     10000,
		"chat.context_rounds":        20,
		"chat.output_truncate_lines": 100,
		"chat.custom_system_prompt":  "",
	}

	for key, val := range defaults {
		var count int64
		DB.Model(&model.Setting{}).Where("key = ?", key).Count(&count)
		if count == 0 {
			jsonVal, _ := json.Marshal(val)
			DB.Create(&model.Setting{
				Key:   key,
				Value: string(jsonVal),
			})
		}
	}
}
