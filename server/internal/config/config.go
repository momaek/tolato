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
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
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
			Driver: "sqlite",
			DSN:    "data/tolato.db",
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
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
