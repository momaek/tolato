package policy

import (
	"context"
	"encoding/json"

	"github.com/momaek/tolato/internal/server/agentapi"
	appexecution "github.com/momaek/tolato/internal/server/app/execution"
)

type Tool interface {
	Name() string
	Definition() agentapi.ToolSpec
	Call(ctx context.Context, call agentapi.Item) (ToolResult, error)
}

type ToolRegistry interface {
	Definitions() []agentapi.ToolSpec
	Call(ctx context.Context, call agentapi.Item) (ToolResult, error)
}

type ExecutionStarter = appexecution.Service

// ToolResult is pure data — no control flags, no runtime instructions.
type ToolResult struct {
	OutputItem  agentapi.Item
	MetaText    string
	ToolMessage json.RawMessage
}

type NodeSummary struct {
	ID        string   `json:"id"`
	Hostname  string   `json:"hostname"`
	Region    string   `json:"region"`
	OS        string   `json:"os"`
	Version   string   `json:"version"`
	IPAddress string   `json:"ipAddress,omitempty"`
	Tags      []string `json:"tags"`
	Status    string   `json:"status"`
	Busy      bool     `json:"busy"`
	LastSeen  string   `json:"lastSeen"`
	Metrics   Metrics  `json:"metrics"`
}

type Metrics struct {
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
	Disk   float64 `json:"disk"`
}

type ListNodesInput struct {
	Query  string `json:"query,omitempty"`
	Status string `json:"status,omitempty"`
	Busy   *bool  `json:"busy,omitempty"`
	Region string `json:"region,omitempty"`
	Tag    string `json:"tag,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type ListNodesOutput struct {
	Nodes []NodeSummary `json:"nodes"`
}

type RunOnNodeInput struct {
	Target       string   `json:"target"`
	Command      string   `json:"command"`
	Args         []string `json:"args,omitempty"`
	ConfirmToken string   `json:"confirmToken,omitempty"`
}

type RunOnNodeOutput struct {
	Status       string           `json:"status"` // "completed" | "no_match" | "ambiguous" | "needs_confirmation" | "error"
	Results      []NodeExecResult `json:"results,omitempty"`
	Candidates   []NodeSummary    `json:"candidates,omitempty"`
	ConfirmToken string           `json:"confirmToken,omitempty"`
	Message      string           `json:"message,omitempty"`
}

type NodeExecResult struct {
	NodeID   string `json:"nodeId"`
	Hostname string `json:"hostname"`
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
	Status   string `json:"status"` // "success" | "failed" | "timeout"
}

// NodeSource provides access to node inventory.
type NodeSource interface {
	ListNodes(ctx context.Context) ([]NodeSummary, error)
}
