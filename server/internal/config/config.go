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
	GeoIP    GeoIPConfig    `yaml:"geoip"`
}

type GeoIPConfig struct {
	// Enabled toggles GeoIP lookup at registration & periodic refresh.
	Enabled bool `yaml:"enabled"`
	// DataDir is where the .mmdb files are downloaded and read from.
	DataDir string `yaml:"data_dir"`
	// RefreshInterval is how often the .mmdb files are re-downloaded. 0 disables auto-refresh.
	RefreshInterval time.Duration `yaml:"refresh_interval"`
}

type ServerConfig struct {
	Host             string   `yaml:"host"`
	Port             int      `yaml:"port"`
	AllowedOrigins   []string `yaml:"allowed_origins"`    // WebSocket & CORS allowed origins, empty = same-origin only
	InstallScriptURL string   `yaml:"install_script_url"` // /install.sh redirects here (usually a GitHub raw URL)
	// SkillURL is where /skill.md redirects: the agent skill that teaches an
	// AI coding agent to drive the `tolato` CLI. The Settings page hands users
	// a prompt pointing at this server's /skill.md, so a deployment whose
	// users cannot reach github.com can repoint this at a reachable mirror.
	SkillURL string `yaml:"skill_url"`
	// PublicAddress is the externally reachable URL/host that agents and the
	// install command use to reach this server (e.g. "https://tolato.example.com").
	// Used when the server sits behind a reverse proxy (caddy/nginx) on a
	// different host/port than what it binds to. If empty, falls back to
	// host:port, which only works for same-host setups.
	PublicAddress string `yaml:"public_address"`
	// ReleaseProxyUpstream is the upstream that /releases/* proxies to,
	// streaming binaries through this server so agents in regions where
	// github.com is unreachable can still install. The server itself must be
	// able to reach this upstream. Empty disables the proxy (404).
	ReleaseProxyUpstream string `yaml:"release_proxy_upstream"`
	// SelfNode names the registered node (alias or hostname) that this server
	// itself runs on. Used only to pre-fill the in-app upgrade prompt so the AI
	// assistant knows which node to run `docker compose pull && up -d` on.
	// The server can't reliably auto-detect this (it lives in a container with
	// a random hostname), so it's an explicit, deterministic opt-in. Empty =>
	// the upgrade prompt stays generic ("on the node where Tolato runs").
	SelfNode string `yaml:"self_node"`
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
			Host:                 "0.0.0.0",
			Port:                 8080,
			InstallScriptURL:     "https://raw.githubusercontent.com/momaek/tolato/main/scripts/install.sh",
			SkillURL:             "https://raw.githubusercontent.com/momaek/tolato/main/skills/tolato/SKILL.md",
			PublicAddress:        "", // e.g. "https://tolato.example.com" when behind caddy/nginx
			ReleaseProxyUpstream: "https://github.com/momaek/tolato/releases",
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
		GeoIP: GeoIPConfig{
			Enabled:         true,
			DataDir:         "data/geoip",
			RefreshInterval: 7 * 24 * time.Hour,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
