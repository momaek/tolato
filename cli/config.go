package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is what the CLI needs to reach a server.
type Config struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

// loadConfig reads the environment first, then the config file for anything
// still missing. Environment wins so a single shell can target a different
// server without editing files.
func loadConfig() (Config, error) {
	cfg := Config{
		URL:    strings.TrimSpace(os.Getenv("TOLATO_URL")),
		APIKey: strings.TrimSpace(os.Getenv("TOLATO_API_KEY")),
	}

	if cfg.URL == "" || cfg.APIKey == "" {
		fileCfg, err := readConfigFile()
		if err != nil {
			return cfg, err
		}
		if cfg.URL == "" {
			cfg.URL = fileCfg.URL
		}
		if cfg.APIKey == "" {
			cfg.APIKey = fileCfg.APIKey
		}
	}

	if cfg.URL == "" {
		return cfg, fmt.Errorf("no server URL: set TOLATO_URL or `url` in %s", configPath())
	}
	if cfg.APIKey == "" {
		return cfg, fmt.Errorf("no API key: set TOLATO_API_KEY or `api_key` in %s", configPath())
	}
	cfg.URL = strings.TrimSuffix(cfg.URL, "/")
	return cfg, nil
}

func configPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "tolato", "config.yaml")
	}
	return "~/.config/tolato/config.yaml"
}

func readConfigFile() (Config, error) {
	var cfg Config
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // absent file is fine; the environment may supply everything
		}
		return cfg, fmt.Errorf("read %s: %w", configPath(), err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", configPath(), err)
	}
	return cfg, nil
}
