package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Security SecurityConfig `yaml:"security"`
	Defaults DefaultsConfig `yaml:"defaults"`
	Auth     AuthConfig     `yaml:"auth"`
	Probe    ProbeConfig    `yaml:"probe"`
}

type ProbeConfig struct {
	Enabled       bool             `yaml:"enabled"`
	RetentionDays int              `yaml:"retention_days"`
	Telegram      TelegramConfig   `yaml:"telegram"`
	AlertRules    AlertRulesConfig `yaml:"alert_rules"`
}

type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}

type AlertRulesConfig struct {
	LatencyThresholdMS     float64 `yaml:"latency_threshold_ms"`
	PacketLossThresholdPct float64 `yaml:"packet_loss_threshold_percent"`
	TCPConnectThresholdMS  float64 `yaml:"tcp_connect_threshold_ms"`
	BandwidthThresholdMbps float64 `yaml:"bandwidth_threshold_mbps"`
	OfflineTimeoutSeconds  int     `yaml:"offline_timeout_seconds"`
	RecoveryCount          int     `yaml:"recovery_count"`
}

type ServerConfig struct {
	Host           string   `yaml:"host"`
	Port           int      `yaml:"port"`
	AllowedOrigins []string `yaml:"allowed_origins"` // WebSocket & CORS allowed origins, empty = same-origin only
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type SecurityConfig struct {
	EncryptKey       string        `yaml:"encrypt_key"`
	JWTSecret        string        `yaml:"jwt_secret"`
	AgentTokenExpiry time.Duration `yaml:"agent_token_expiry"`
}

type DefaultsConfig struct {
	HeartbeatInterval   int `yaml:"heartbeat_interval"`
	CommandTimeout      int `yaml:"command_timeout"`
	MaxRounds           int `yaml:"max_rounds"`
	ContextRounds       int `yaml:"context_rounds"`
	OutputTruncateLines int `yaml:"output_truncate_lines"`
}

type AuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Load reads a YAML config file and returns a Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Database: DatabaseConfig{
			Driver: "postgres",
			DSN:    "host=localhost user=tolato password=tolato dbname=tolato port=5432 sslmode=disable",
		},
		Security: SecurityConfig{
			EncryptKey:       "tolato-default-encrypt-key-32b!",
			JWTSecret:        "tolato-jwt-secret-change-me",
			AgentTokenExpiry: 24 * time.Hour,
		},
		Defaults: DefaultsConfig{
			HeartbeatInterval:   30,
			CommandTimeout:      60,
			MaxRounds:           20,
			ContextRounds:       20,
			OutputTruncateLines: 100,
		},
		Auth: AuthConfig{
			Username: "admin",
			Password: "admin",
		},
		Probe: ProbeConfig{
			Enabled:       true,
			RetentionDays: 30,
			AlertRules: AlertRulesConfig{
				LatencyThresholdMS:     200,
				PacketLossThresholdPct: 5,
				TCPConnectThresholdMS:  500,
				BandwidthThresholdMbps: 10,
				OfflineTimeoutSeconds:  180,
				RecoveryCount:          3,
			},
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
