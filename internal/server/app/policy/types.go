package policy

import (
	"context"
	"encoding/json"

	"github.com/momaek/tolato/internal/server/agentapi"
	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
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

type ToolResult struct {
	OutputItem            agentapi.Item
	MetaText              string
	ToolMessage           json.RawMessage
	WaitForUser           bool
	PendingActionType     domain.PendingActionType
	PendingActionPayload  json.RawMessage
	AsyncExecutionStarted bool
	TaskID                string
	ExecutionGroupID      string
	AppendPlanRow         bool
	AppendApprovalRow     bool
	AppendExecutionRow    bool
	AppendSummaryRow      bool
}

type NodeSummary struct {
	ID       string   `json:"id"`
	Hostname string   `json:"hostname"`
	Region   string   `json:"region"`
	OS       string   `json:"os"`
	Version  string   `json:"version"`
	Tags     []string `json:"tags"`
	Status   string   `json:"status"`
	Busy     bool     `json:"busy"`
	LastSeen string   `json:"lastSeen"`
	Metrics  Metrics  `json:"metrics"`
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

type ResolveTargetNodesInput struct {
	Query                string                      `json:"query,omitempty"`
	CurrentTargetContext *domain.ActiveTargetContext `json:"currentTargetContext,omitempty"`
}

type ResolveTargetNodesOutput struct {
	Query         string                     `json:"query"`
	TargetContext domain.ActiveTargetContext `json:"targetContext"`
	Candidates    []domain.TargetCandidate   `json:"candidates,omitempty"`
	Nodes         []NodeSummary              `json:"nodes,omitempty"`
}

type RequestTargetConfirmationInput struct {
	TargetContext domain.ActiveTargetContext `json:"targetContext"`
	Message       string                     `json:"message,omitempty"`
}

type RequestTargetConfirmationOutput struct {
	TargetContext domain.ActiveTargetContext `json:"targetContext"`
	Message       string                     `json:"message"`
}

type RequestApprovalInput struct {
	TaskID           string           `json:"taskId"`
	RiskLevel        domain.RiskLevel `json:"riskLevel"`
	Message          string           `json:"message,omitempty"`
	Reason           string           `json:"reason,omitempty"`
	RequiresApproval *bool            `json:"requiresApproval,omitempty"`
}

type RequestApprovalOutput struct {
	TaskID           string           `json:"taskId"`
	RiskLevel        domain.RiskLevel `json:"riskLevel"`
	Message          string           `json:"message"`
	RequiresApproval bool             `json:"requiresApproval"`
}

type ExecOnNodesInput struct {
	SessionID     string                     `json:"sessionId"`
	InputText     string                     `json:"inputText"`
	Command       string                     `json:"command,omitempty"`
	CommandArgs   []string                   `json:"commandArgs,omitempty"`
	TargetContext domain.ActiveTargetContext `json:"targetContext"`
	RiskLevel     domain.RiskLevel           `json:"riskLevel,omitempty"`
}

type ExecOnNodesOutput struct {
	TaskID           string           `json:"taskId"`
	ExecutionGroupID string           `json:"executionGroupId"`
	NodeIDs          []string         `json:"nodeIds"`
	RiskLevel        domain.RiskLevel `json:"riskLevel"`
	Message          string           `json:"message"`
}

type SummarizeExecutionInput struct {
	TaskID      string                    `json:"taskId"`
	Status      domain.TaskStatus         `json:"status"`
	Aggregate   domain.ExecutionAggregate `json:"aggregate"`
	TargetLabel string                    `json:"targetLabel,omitempty"`
}

type SummarizeExecutionOutput struct {
	TaskID    string                    `json:"taskId"`
	Status    domain.TaskStatus         `json:"status"`
	Aggregate domain.ExecutionAggregate `json:"aggregate"`
	Summary   string                    `json:"summary"`
}

type PlanStep struct {
	Action           string           `json:"action"`
	Args             map[string]any   `json:"args,omitempty"`
	Risk             domain.RiskLevel `json:"risk"`
	TimeoutSec       int              `json:"timeoutSec,omitempty"`
	BroadcastAllowed bool             `json:"broadcastAllowed,omitempty"`
}

type ProposePlanInput struct {
	InputText        string                     `json:"inputText"`
	TargetContext    domain.ActiveTargetContext `json:"targetContext"`
	RiskLevel        domain.RiskLevel           `json:"riskLevel,omitempty"`
	RequiresApproval *bool                      `json:"requiresApproval,omitempty"`
	Steps            []PlanStep                 `json:"steps,omitempty"`
}

type ProposedPlan struct {
	TargetNodes      []string         `json:"targetNodes"`
	Summary          string           `json:"summary"`
	EstimatedImpact  string           `json:"estimatedImpact"`
	RiskLevel        domain.RiskLevel `json:"riskLevel"`
	RequiresApproval bool             `json:"requiresApproval"`
	Steps            []PlanStep       `json:"steps"`
	Metadata         map[string]any   `json:"metadata,omitempty"`
	CreatedAt        string           `json:"createdAt"`
}
