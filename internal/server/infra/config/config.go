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
}
