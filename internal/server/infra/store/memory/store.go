package memory

import "github.com/momaek/tolato/internal/server/domain"

type Store struct {
	Sessions       domain.SessionRepository
	ThreadMessages domain.ThreadMessageRepository
	Timelines      domain.TimelineRepository
	ToolCalls      domain.ToolCallRepository
	ToolResults    domain.ToolResultRepository
	Tasks          domain.TaskRepository
	Executions     domain.ExecutionRepository
	Audits         domain.AuditRepository
	Settings       domain.SettingsRepository
}

func NewStore() *Store {
	return &Store{
		Sessions:       NewSessionRepository(),
		ThreadMessages: NewThreadMessageRepository(),
		Timelines:      NewTimelineRepository(),
		ToolCalls:      NewToolCallRepository(),
		ToolResults:    NewToolResultRepository(),
		Tasks:          NewTaskRepository(),
		Executions:     NewExecutionRepository(),
		Audits:         NewAuditRepository(),
		Settings:       NewSettingsRepository(),
	}
}
