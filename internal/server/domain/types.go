package domain

import (
	"encoding/json"
	"time"
)

type SessionStatus string

const (
	SessionStatusIdle                         SessionStatus = "idle"
	SessionStatusRunning                      SessionStatus = "running"
	SessionStatusPausedWaitTargetConfirmation SessionStatus = "paused_wait_target_confirmation"
	SessionStatusPausedWaitApproval           SessionStatus = "paused_wait_approval"
	SessionStatusWaitingAsyncExecution        SessionStatus = "waiting_async_execution"
	SessionStatusCompleted                    SessionStatus = "completed"
	SessionStatusFailed                       SessionStatus = "failed"
)

type PendingActionType string

const (
	PendingActionTypeTargetConfirmation PendingActionType = "target_confirmation"
	PendingActionTypeApproval           PendingActionType = "approval"
)

type TargetStatus string

const (
	TargetStatusUnset               TargetStatus = "unset"
	TargetStatusPendingConfirmation TargetStatus = "pending_confirmation"
	TargetStatusConfirmed           TargetStatus = "confirmed"
)

type TargetScope string

const (
	TargetScopeSingle    TargetScope = "single"
	TargetScopeMulti     TargetScope = "multi"
	TargetScopeAllOnline TargetScope = "all_online"
)

type TargetSource string

const (
	TargetSourceUserExplicit      TargetSource = "user_explicit"
	TargetSourceAssistantResolved TargetSource = "assistant_resolved"
	TargetSourceContextInherited  TargetSource = "context_inherited"
)

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

type ThreadMessageKind string

const (
	ThreadMessageKindUserMessage   ThreadMessageKind = "user_message"
	ThreadMessageKindAssistantText ThreadMessageKind = "assistant_text"
)

type TimelineRowKind string

const (
	TimelineRowKindUserMessage        TimelineRowKind = "user_message"
	TimelineRowKindAssistantText      TimelineRowKind = "assistant_text"
	TimelineRowKindTargetConfirmation TimelineRowKind = "target_confirmation"
	TimelineRowKindToolCallMeta       TimelineRowKind = "tool_call_meta"
	TimelineRowKindToolResultMeta     TimelineRowKind = "tool_result_meta"
	TimelineRowKindPlan               TimelineRowKind = "plan"
	TimelineRowKindApproval           TimelineRowKind = "approval"
	TimelineRowKindExecution          TimelineRowKind = "execution"
	TimelineRowKindSummary            TimelineRowKind = "summary"
)

type ToolResultStatus string

const (
	ToolResultStatusSucceeded ToolResultStatus = "succeeded"
	ToolResultStatusFailed    ToolResultStatus = "failed"
)

type TimelineRowSource string

const (
	TimelineRowSourceAgentLoop  TimelineRowSource = "agent_loop"
	TimelineRowSourceUserAction TimelineRowSource = "user_action"
)

type ToolCallSource string

const (
	ToolCallSourceAgentLoop ToolCallSource = "agent_loop"
)

type TaskStatus string

const (
	TaskStatusPlanned         TaskStatus = "planned"
	TaskStatusWaitingApproval TaskStatus = "waiting_approval"
	TaskStatusApproved        TaskStatus = "approved"
	TaskStatusQueued          TaskStatus = "queued"
	TaskStatusDispatched      TaskStatus = "dispatched"
	TaskStatusRunning         TaskStatus = "running"
	TaskStatusSuccess         TaskStatus = "success"
	TaskStatusFailed          TaskStatus = "failed"
	TaskStatusPartialFailed   TaskStatus = "partial_failed"
	TaskStatusTimeout         TaskStatus = "timeout"
	TaskStatusCancelled       TaskStatus = "cancelled"
)

type ApprovalStatus string

const (
	ApprovalStatusNotRequired ApprovalStatus = "not_required"
	ApprovalStatusPending     ApprovalStatus = "pending"
	ApprovalStatusApproved    ApprovalStatus = "approved"
	ApprovalStatusRejected    ApprovalStatus = "rejected"
	ApprovalStatusCancelled   ApprovalStatus = "cancelled"
)

type ExecutionStatus string

const (
	ExecutionStatusQueued     ExecutionStatus = "queued"
	ExecutionStatusDispatched ExecutionStatus = "dispatched"
	ExecutionStatusRunning    ExecutionStatus = "running"
	ExecutionStatusSuccess    ExecutionStatus = "success"
	ExecutionStatusFailed     ExecutionStatus = "failed"
	ExecutionStatusTimeout    ExecutionStatus = "timeout"
	ExecutionStatusCancelled  ExecutionStatus = "cancelled"
)

type ExecutionStream string

const (
	ExecutionStreamStdout ExecutionStream = "stdout"
	ExecutionStreamStderr ExecutionStream = "stderr"
)

type RiskLevel string

const (
	RiskLevelLow       RiskLevel = "low"
	RiskLevelMedium    RiskLevel = "medium"
	RiskLevelHigh      RiskLevel = "high"
	RiskLevelForbidden RiskLevel = "forbidden"
)

type TargetCandidate struct {
	NodeID    string
	Hostname  string
	Region    string
	MatchedBy string
	Reason    string
}

type ActiveTargetContext struct {
	Status          TargetStatus
	Scope           TargetScope
	NodeIDs         []string
	DisplayLabel    string
	Source          TargetSource
	Confidence      float64
	Candidates      []TargetCandidate
	SourceMessageID *string
	ConfirmedAt     *time.Time
}

type TargetSnapshot struct {
	Scope        TargetScope
	NodeIDs      []string
	DisplayLabel string
	Source       TargetSource
	Confirmed    bool
	ConfirmedAt  *time.Time
	CapturedAt   time.Time
}

type PendingAction struct {
	Type    PendingActionType
	Payload json.RawMessage
}

type Session struct {
	ID                      string
	Title                   string
	Status                  SessionStatus
	ActiveTargetContext     ActiveTargetContext
	PendingAction           *PendingAction
	CurrentOperationID      *string
	CurrentTaskID           *string
	CurrentExecutionGroupID *string
	LastAgentState          json.RawMessage
	ProviderStateBlob       json.RawMessage
	Revision                int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type AuthSession struct {
	UserID     string
	SessionID  string
	Token      string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

type ThreadMessage struct {
	ID              string
	SessionID       string
	ClientMessageID *string
	Role            MessageRole
	Kind            ThreadMessageKind
	Content         string
	CreatedAt       time.Time
}

type TimelineRow struct {
	ID            string
	SessionID     string
	Kind          TimelineRowKind
	CreatedAt     time.Time
	Text          string
	ToolName      string
	ToolStatus    ToolResultStatus
	Source        TimelineRowSource
	ArgsPreview   *string
	TaskID        *string
	TargetContext *ActiveTargetContext
}

type ToolCall struct {
	ID          string
	SessionID   string
	TaskID      *string
	MessageID   *string
	ToolName    string
	Arguments   json.RawMessage
	ArgsPreview *string
	Source      ToolCallSource
	CreatedAt   time.Time
}

type ToolResult struct {
	ID         string
	SessionID  string
	TaskID     *string
	ToolCallID *string
	ToolName   string
	Status     ToolResultStatus
	Text       string
	Source     TimelineRowSource
	Payload    json.RawMessage
	CreatedAt  time.Time
}

type Task struct {
	ID                      string
	SessionID               string
	InputText               string
	OperationTargetSnapshot TargetSnapshot
	Status                  TaskStatus
	ApprovalStatus          ApprovalStatus
	RiskLevel               RiskLevel
	Summary                 *string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type Execution struct {
	ID           string
	TaskID       string
	SessionID    string
	NodeID       string
	Status       ExecutionStatus
	StartedAt    *time.Time
	FinishedAt   *time.Time
	ExitCode     *int
	StdoutTail   string
	StderrTail   string
	StatusReason *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ExecutionChunk struct {
	Stream ExecutionStream `json:"stream"`
	Text   string          `json:"text"`
}

type ExecutionAggregate struct {
	Total      int
	Queued     int
	Dispatched int
	Running    int
	Success    int
	Failed     int
	Timeout    int
	Cancelled  int
}

type AuditRecord struct {
	ID        string
	SessionID string
	TaskID    *string
	ActorID   string
	EventType string
	Payload   json.RawMessage
	CreatedAt time.Time
}

type AgentProviderState struct {
	ID        string
	SessionID string
	Version   int64
	Payload   json.RawMessage
	CreatedAt time.Time
}

type SettingKey string

const (
	SettingKeyModelConfig     SettingKey = "model_config"
	SettingKeyAccountSecurity SettingKey = "account_security"
	SettingKeyPreferences     SettingKey = "preferences"
	SettingKeyAuthCredentials SettingKey = "auth_credentials"
)

type SettingRecord struct {
	UserID    string
	Key       SettingKey
	Value     json.RawMessage
	UpdatedAt time.Time
}
