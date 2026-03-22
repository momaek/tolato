package postgres

import "github.com/momaek/tolato/internal/server/domain"

type Store struct {
	Sessions            domain.SessionRepository
	ThreadMessages      domain.ThreadMessageRepository
	Timelines           domain.TimelineRepository
	ToolCalls           domain.ToolCallRepository
	ToolResults         domain.ToolResultRepository
	Tasks               domain.TaskRepository
	Executions          domain.ExecutionRepository
	Audits              domain.AuditRepository
	AgentProviderStates domain.AgentProviderStateRepository
	Settings            domain.SettingsRepository
}

func NewStore(q Queryer) *Store {
	return &Store{
		Sessions:            NewSessionRepository(q),
		ThreadMessages:      NewThreadMessageRepository(q),
		Timelines:           NewTimelineRepository(q),
		ToolCalls:           NewToolCallRepository(q),
		ToolResults:         NewToolResultRepository(q),
		Tasks:               NewTaskRepository(q),
		Executions:          NewExecutionRepository(q),
		Audits:              NewAuditRepository(q),
		AgentProviderStates: NewAgentProviderStateRepository(q),
		Settings:            NewSettingsRepository(q),
	}
}
