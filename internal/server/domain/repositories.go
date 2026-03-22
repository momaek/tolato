package domain

import "context"

type CursorPage struct {
	BeforeID string
	Limit    int
}

type SessionFilter struct {
	Statuses []SessionStatus
	Limit    int
}

type SessionRepository interface {
	Create(ctx context.Context, session Session) error
	Get(ctx context.Context, sessionID string) (Session, error)
	List(ctx context.Context, filter SessionFilter) ([]Session, error)
	Update(ctx context.Context, session Session) error
}

type ThreadMessageRepository interface {
	Append(ctx context.Context, message ThreadMessage) error
	ListBySession(ctx context.Context, sessionID string, page CursorPage) ([]ThreadMessage, error)
}

type TimelineRepository interface {
	Append(ctx context.Context, row TimelineRow) error
	ListBySession(ctx context.Context, sessionID string, page CursorPage) ([]TimelineRow, error)
}

type ToolCallRepository interface {
	Append(ctx context.Context, call ToolCall) error
	ListBySession(ctx context.Context, sessionID string, page CursorPage) ([]ToolCall, error)
}

type ToolResultRepository interface {
	Append(ctx context.Context, result ToolResult) error
	ListBySession(ctx context.Context, sessionID string, page CursorPage) ([]ToolResult, error)
	ListByTask(ctx context.Context, taskID string) ([]ToolResult, error)
}

type TaskRepository interface {
	Create(ctx context.Context, task Task) error
	Get(ctx context.Context, taskID string) (Task, error)
	ListBySession(ctx context.Context, sessionID string) ([]Task, error)
	Update(ctx context.Context, task Task) error
}

type ExecutionRepository interface {
	Create(ctx context.Context, execution Execution) error
	Get(ctx context.Context, executionID string) (Execution, error)
	ListByTask(ctx context.Context, taskID string) ([]Execution, error)
	Update(ctx context.Context, execution Execution) error
	AggregateByTask(ctx context.Context, taskID string) (ExecutionAggregate, error)
}

type AuditRepository interface {
	Append(ctx context.Context, record AuditRecord) error
	ListByTask(ctx context.Context, taskID string) ([]AuditRecord, error)
}

type SettingsRepository interface {
	Put(ctx context.Context, record SettingRecord) error
	Get(ctx context.Context, userID string, key SettingKey) (SettingRecord, error)
	ListByUser(ctx context.Context, userID string) ([]SettingRecord, error)
}

type AgentProviderStateRepository interface {
	Append(ctx context.Context, state AgentProviderState) error
	ListBySession(ctx context.Context, sessionID string) ([]AgentProviderState, error)
	LatestBySession(ctx context.Context, sessionID string) (AgentProviderState, error)
}
