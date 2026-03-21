package types

import "time"

type NodeMetrics struct {
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
	Disk   float64 `json:"disk"`
}

type Node struct {
	ID                string      `json:"id"`
	Hostname          string      `json:"hostname"`
	Region            string      `json:"region"`
	OS                string      `json:"os"`
	Version           string      `json:"version"`
	Tags              []string    `json:"tags"`
	Status            string      `json:"status"`
	LastSeenAt        time.Time   `json:"last_seen_at"`
	AuthSecretVersion int         `json:"auth_secret_version"`
	AgentSecret       string      `json:"-"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
	Busy              bool        `json:"busy"`
	Metrics           NodeMetrics `json:"metrics"`
}

type NodeSession struct {
	NodeID          string    `json:"node_id"`
	SessionID       string    `json:"session_id"`
	ConnectedAt     time.Time `json:"connected_at"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	DisconnectedAt  time.Time `json:"disconnected_at,omitempty"`
	RemoteAddr      string    `json:"remote_addr"`
	Capabilities    []string  `json:"capabilities"`
	Status          string    `json:"status"`
}

type Plan struct {
	TargetNodes          []string          `json:"target_nodes"`
	Summary              string            `json:"summary"`
	EstimatedImpact      string            `json:"estimated_impact"`
	RiskLevel            string            `json:"risk_level"`
	RequiresApproval     bool              `json:"requires_approval"`
	RequiredApprovalRole string            `json:"required_approval_role,omitempty"`
	Steps                []PlanStep        `json:"steps"`
	Metadata             map[string]string `json:"metadata,omitempty"`
}

type PlanDraft = Plan

type PlanStep struct {
	Action           string         `json:"action"`
	Args             map[string]any `json:"args"`
	Risk             string         `json:"risk"`
	TimeoutSec       int            `json:"timeout_sec"`
	BroadcastAllowed bool           `json:"broadcast_allowed,omitempty"`
}

type Task struct {
	ID                   string        `json:"id"`
	ParentTaskID         string        `json:"parent_task_id,omitempty"`
	Mode                 string        `json:"mode"`
	InitiatorID          string        `json:"initiator_id"`
	InitiatorRole        string        `json:"initiator_role,omitempty"`
	Target               []string      `json:"target"`
	InputText            string        `json:"input_text"`
	Plan                 Plan          `json:"plan"`
	RiskLevel            string        `json:"risk_level"`
	ApprovalStatus       string        `json:"approval_status"`
	RequiredApprovalRole string        `json:"required_approval_role,omitempty"`
	ApproverID           string        `json:"approver_id,omitempty"`
	FinalStatus          string        `json:"final_status"`
	StatusReason         string        `json:"status_reason"`
	ResultSummary        string        `json:"result_summary,omitempty"`
	FailureNodeIDs       []string      `json:"failure_node_ids,omitempty"`
	SummarySource        string        `json:"summary_source,omitempty"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
	Aggregate            TaskAggregate `json:"aggregate,omitempty"`
	Summary              string        `json:"summary,omitempty"`
}

type TaskExecution struct {
	ID           string    `json:"id"`
	TaskID       string    `json:"task_id"`
	NodeID       string    `json:"node_id"`
	Status       string    `json:"status"`
	Attempt      int       `json:"attempt"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	ExitCode     int       `json:"exit_code"`
	StdoutTail   string    `json:"stdout_tail"`
	StderrTail   string    `json:"stderr_tail"`
	StatusReason string    `json:"status_reason"`
}

type AuditEvent struct {
	ID        string         `json:"id"`
	TaskID    string         `json:"task_id"`
	ActorID   string         `json:"actor_id"`
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

type ActionSpec struct {
	Name             string         `json:"name"`
	RiskLevel        string         `json:"risk_level"`
	ApprovalRequired bool           `json:"approval_required"`
	BroadcastAllowed bool           `json:"broadcast_allowed"`
	TimeoutSec       int            `json:"timeout_sec"`
	ArgSchema        map[string]any `json:"arg_schema,omitempty"`
	Executor         string         `json:"executor,omitempty"`
	ResultShape      map[string]any `json:"result_shape,omitempty"`
}

type CurrentUser struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	User  CurrentUser `json:"user"`
	Token string      `json:"token,omitempty"`
}

type TaskPlanRequest struct {
	Mode      string   `json:"mode"`
	Target    []string `json:"target"`
	InputText string   `json:"input_text"`
}

type TaskPlanResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
	Plan   Plan   `json:"plan"`
}

type TaskAggregate struct {
	Total          int `json:"total"`
	Success        int `json:"success"`
	Failed         int `json:"failed"`
	OfflineSkipped int `json:"offline_skipped"`
	Running        int `json:"running"`
}

type TaskMutationResponse struct {
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type ListNodesResponse struct {
	Nodes []Node `json:"nodes"`
}

type ListTasksResponse struct {
	Tasks []TaskResponseItem `json:"tasks"`
}

type TaskResponse struct {
	Task      Task          `json:"task"`
	Aggregate TaskAggregate `json:"aggregate"`
	Summary   string        `json:"summary"`
}

type TaskResponseItem struct {
	Task      Task          `json:"task"`
	Aggregate TaskAggregate `json:"aggregate"`
	Summary   string        `json:"summary"`
}

type TaskExecutionsResponse struct {
	Executions []TaskExecution `json:"executions"`
}

type AuditEventsResponse struct {
	Events []AuditEvent `json:"events"`
}

type LLMConfig struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	Model    string `json:"model,omitempty"`
}

type OutboxMessage struct {
	ID          string         `json:"id"`
	Topic       string         `json:"topic"`
	TaskID      string         `json:"task_id"`
	ExecutionID string         `json:"execution_id,omitempty"`
	NodeID      string         `json:"node_id,omitempty"`
	Payload     map[string]any `json:"payload"`
	CreatedAt   time.Time      `json:"created_at"`
	PublishedAt time.Time      `json:"published_at,omitempty"`
	Attempts    int            `json:"attempts"`
}

type EnrollRequest struct {
	Hostname string   `json:"hostname"`
	Region   string   `json:"region"`
	OS       string   `json:"os"`
	Version  string   `json:"version"`
	Tags     []string `json:"tags"`
}

type EnrollResponse struct {
	NodeID string `json:"node_id"`
	Secret string `json:"secret"`
}

type AgentIdentity struct {
	NodeID   string `json:"node_id"`
	Secret   string `json:"secret"`
	Hostname string `json:"hostname"`
	Region   string `json:"region"`
	OS       string `json:"os"`
	Version  string `json:"version"`
}
