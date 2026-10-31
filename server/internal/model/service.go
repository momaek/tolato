package model

import "encoding/json"

// AgentServiceRequest is the version 1 agent service protocol.
type AgentServiceRequest struct {
	Service   string          `json:"service"`
	Action    string          `json:"action"`
	BinaryURL string          `json:"binary_url,omitempty"`
	SHA256    string          `json:"sha256,omitempty"`
	Config    json.RawMessage `json:"config,omitempty"`
}

type AgentServiceResult struct {
	OK     bool          `json:"ok"`
	Error  string        `json:"error,omitempty"`
	Status ServiceStatus `json:"status"`
}

type ServiceStatus struct {
	Enabled  bool   `json:"enabled"`
	Running  bool   `json:"running"`
	PID      int    `json:"pid,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Restarts int    `json:"restarts"`
	Error    string `json:"error,omitempty"`
}
