package probe

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ProbeConfig holds the agent-side probe configuration.
type ProbeConfig struct {
	ServerURL string         `yaml:"server_url"`
	AuthToken string         `yaml:"auth_token"`
	NodeID    string         `yaml:"node_id"`
	NodeName  string         `yaml:"node_name"`
	Targets   []TargetConfig `yaml:"targets"`
}

// TargetConfig describes a single probe target.
type TargetConfig struct {
	ID           string `yaml:"id"`
	Name         string `yaml:"name"`
	Host         string `yaml:"host"`
	PingCount    int    `yaml:"ping_count"`
	TCPPort      int    `yaml:"tcp_port"`
	BandwidthURL string `yaml:"bandwidth_url"`
}

// LoadProbeConfig reads and parses a probe YAML config file.
func LoadProbeConfig(path string) (ProbeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProbeConfig{}, fmt.Errorf("read probe config: %w", err)
	}

	var cfg ProbeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ProbeConfig{}, fmt.Errorf("parse probe config: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return ProbeConfig{}, err
	}
	return cfg, nil
}

func (c *ProbeConfig) applyDefaults() {
	for i := range c.Targets {
		if c.Targets[i].PingCount <= 0 {
			c.Targets[i].PingCount = 10
		}
		if c.Targets[i].TCPPort <= 0 {
			c.Targets[i].TCPPort = 443
		}
	}
}

func (c *ProbeConfig) Validate() error {
	if c.ServerURL == "" {
		return fmt.Errorf("probe config: server_url is required")
	}
	if c.NodeID == "" {
		return fmt.Errorf("probe config: node_id is required")
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("probe config: at least one target is required")
	}
	for i, t := range c.Targets {
		if t.ID == "" {
			return fmt.Errorf("probe config: targets[%d].id is required", i)
		}
		if t.Host == "" {
			return fmt.Errorf("probe config: targets[%d].host is required", i)
		}
	}
	return nil
}
