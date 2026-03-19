package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Server struct {
		Address string `yaml:"address"`
	} `yaml:"server"`
	Postgres struct {
		DSN string `yaml:"dsn"`
	} `yaml:"postgres"`
	Redis struct {
		Addr string `yaml:"addr"`
		DB   int    `yaml:"db"`
	} `yaml:"redis"`
	Auth struct {
		AdminUsername string `yaml:"admin_username"`
		AdminPassword string `yaml:"admin_password"`
	} `yaml:"auth"`
}

type AgentConfig struct {
	Agent struct {
		Hostname          string `yaml:"hostname"`
		Region            string `yaml:"region"`
		OS                string `yaml:"os"`
		Version           string `yaml:"version"`
		DataDir           string `yaml:"data_dir"`
		IdentityFile      string `yaml:"identity_file"`
		ServerBaseURL     string `yaml:"server_base_url"`
		HeartbeatInterval string `yaml:"heartbeat_interval"`
		ReconnectInterval string `yaml:"reconnect_interval"`
	} `yaml:"agent"`
}

func LoadServerConfig(path string) (ServerConfig, error) {
	var cfg ServerConfig
	if err := load(path, &cfg); err != nil {
		return ServerConfig{}, err
	}

	if cfg.Server.Address == "" {
		cfg.Server.Address = ":8080"
	}

	return cfg, nil
}

func LoadAgentConfig(path string) (AgentConfig, error) {
	var cfg AgentConfig
	if err := load(path, &cfg); err != nil {
		return AgentConfig{}, err
	}

	if cfg.Agent.HeartbeatInterval == "" {
		cfg.Agent.HeartbeatInterval = "5s"
	}

	if cfg.Agent.ReconnectInterval == "" {
		cfg.Agent.ReconnectInterval = "3s"
	}

	return cfg, nil
}

func (c AgentConfig) HeartbeatInterval() time.Duration {
	d, err := time.ParseDuration(c.Agent.HeartbeatInterval)
	if err != nil {
		return 5 * time.Second
	}
	return d
}

func (c AgentConfig) ReconnectInterval() time.Duration {
	d, err := time.ParseDuration(c.Agent.ReconnectInterval)
	if err != nil {
		return 3 * time.Second
	}
	return d
}

func load(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(raw, target)
}
