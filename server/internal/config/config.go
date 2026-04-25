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
	Host             string   `yaml:"host"`
	Port             int      `yaml:"port"`
	AllowedOrigins   []string `yaml:"allowed_origins"`    // WebSocket & CORS allowed origins, empty = same-origin only
	InstallScriptURL string   `yaml:"install_script_url"` // /install.sh redirects here (usually a GitHub raw URL)
	// PublicAddress is the externally reachable URL/host that agents and the
	// install command use to reach this server (e.g. "https://tolato.example.com").
	// Used when the server sits behind a reverse proxy (caddy/nginx) on a
	// different host/port than what it binds to. If empty, falls back to
	// host:port, which only works for same-host setups.
	PublicAddress string `yaml:"public_address"`
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
			Host:             "0.0.0.0",
			Port:             8080,
			InstallScriptURL: "https://raw.githubusercontent.com/momaek/tolato/main/scripts/install.sh",
			PublicAddress:    "", // e.g. "https://tolato.example.com" when behind caddy/nginx
		},
		Database: DatabaseConfig{
			Driver: "postgres",
			DSN:    "host=localhost user=tolato password=tolato dbname=tolato port=5432 sslmode=disable",
		},
		Security: SecurityConfig{
			EncryptKey:       "tolato-default-encrypt-key-32b!",
			JWTSecret:        "tolato-jwt-secret-change-me",
			AgentTokenExpiry: 0, // 0 = never expires
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
