package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	Store  StoreConfig  `yaml:"store"`
	Auth   AuthConfig   `yaml:"auth"`
	Probe  ProbeConfig  `yaml:"probe"`
}

// ProbeConfig configures the NodeProbe link monitoring subsystem.
type ProbeConfig struct {
	Enabled        bool            `yaml:"enabled"`
	RetentionDays  int             `yaml:"retention_days"`
	Telegram       TelegramConfig  `yaml:"telegram"`
	AlertRules     AlertThresholds `yaml:"alert_rules"`
}

type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}

type AlertThresholds struct {
	LatencyMs      float64 `yaml:"latency_threshold_ms"`
	PacketLossPct  float64 `yaml:"packet_loss_threshold_percent"`
	TCPConnectMs   float64 `yaml:"tcp_connect_threshold_ms"`
	BandwidthMbps  float64 `yaml:"bandwidth_threshold_mbps"`
	OfflineSeconds int     `yaml:"offline_timeout_seconds"`
	RecoveryCount  int     `yaml:"recovery_count"`
}

type ServerConfig struct {
	HTTPAddress string `yaml:"http_address"`
	UIWSPath    string `yaml:"ui_ws_path"`
	AgentWSPath string `yaml:"agent_ws_path"`
}

type StoreConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type AuthConfig struct {
	AdminUsername string `yaml:"admin_username"`
	AdminPassword string `yaml:"admin_password"`
	AgentToken    string `yaml:"agent_token"`
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	return Parse(raw)
}

func Parse(raw []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if c.Server.HTTPAddress == "" {
		return errors.New("server.http_address is required")
	}
	if c.Store.Driver == "" {
		return errors.New("store.driver is required")
	}
	switch c.Store.Driver {
	case "memory":
	case "postgres":
		if c.Store.Driver == "postgres" && c.Store.DSN == "" {
			return errors.New("store.dsn is required when store.driver=postgres")
		}
	default:
		return errors.New("store.driver must be one of: memory, postgres")
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.Server.UIWSPath == "" {
		c.Server.UIWSPath = "/ws/ui"
	}
	if c.Server.AgentWSPath == "" {
		c.Server.AgentWSPath = "/ws/agent"
	}

	// Probe defaults
	if c.Probe.RetentionDays <= 0 {
		c.Probe.RetentionDays = 30
	}
	if c.Probe.AlertRules.LatencyMs <= 0 {
		c.Probe.AlertRules.LatencyMs = 200
	}
	if c.Probe.AlertRules.PacketLossPct <= 0 {
		c.Probe.AlertRules.PacketLossPct = 5
	}
	if c.Probe.AlertRules.TCPConnectMs <= 0 {
		c.Probe.AlertRules.TCPConnectMs = 500
	}
	if c.Probe.AlertRules.BandwidthMbps <= 0 {
		c.Probe.AlertRules.BandwidthMbps = 10
	}
	if c.Probe.AlertRules.OfflineSeconds <= 0 {
		c.Probe.AlertRules.OfflineSeconds = 180
	}
	if c.Probe.AlertRules.RecoveryCount <= 0 {
		c.Probe.AlertRules.RecoveryCount = 3
	}
}
