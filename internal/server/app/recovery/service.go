package recovery

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/momaek/tolato/internal/server/domain"
)

const recoveryActorID = "system_recovery"

type Service interface {
	Scan(ctx context.Context) (ScanReport, error)
}

type RuntimeResumer interface {
	HandleExecutionFinished(ctx context.Context, sessionID string, taskID string) error
}

type Repositories struct {
	Sessions   domain.SessionRepository
	Executions domain.ExecutionRepository
	Audits     domain.AuditRepository
}

type service struct {
	repos   Repositories
	clock   domain.Clock
	ids     domain.IDGenerator
	runtime RuntimeResumer
}

type Option func(*service)

type ScanReport struct {
	FailedRunning []string
	PausedWaiting []string
	WaitingAsync  []WaitingAsyncBinding
}

type WaitingAsyncBinding struct {
	SessionID          string
	TaskID             string
	ExecutionGroupID   *string
	Aggregate          domain.ExecutionAggregate
	ResumeTriggered    bool
	WaitingForCallback bool
}

func NewService(repos Repositories, clock domain.Clock, ids domain.IDGenerator, options ...Option) Service {
	svc := &service{
		repos: repos,
		clock: clock,
		ids:   ids,
	}
	for _, option := range options {
		if option != nil {
			option(svc)
		}
	}
	return svc
}

func WithRuntimeResumer(runtime RuntimeResumer) Option {
	return func(s *service) {
		s.runtime = runtime
	}
}

func (s *service) Scan(ctx context.Context) (ScanReport, error) {
	if err := s.validateReady(); err != nil {
		return ScanReport{}, err
	}

	sessions, err := s.repos.Sessions.List(ctx, domain.SessionFilter{
		Statuses: []domain.SessionStatus{
			domain.SessionStatusRunning,
			domain.SessionStatusPausedWaitTargetConfirmation,
			domain.SessionStatusPausedWaitApproval,
			domain.SessionStatusWaitingAsyncExecution,
		},
	})
	if err != nil {
		return ScanReport{}, err
	}

	report := ScanReport{
		FailedRunning: make([]string, 0),
		PausedWaiting: make([]string, 0),
		WaitingAsync:  make([]WaitingAsyncBinding, 0),
	}
	for _, session := range sessions {
		switch session.Status {
		case domain.SessionStatusRunning:
			if err := s.failRunningSession(ctx, &session); err != nil {
				return ScanReport{}, err
			}
			report.FailedRunning = append(report.FailedRunning, session.ID)
		case domain.SessionStatusPausedWaitTargetConfirmation, domain.SessionStatusPausedWaitApproval:
			report.PausedWaiting = append(report.PausedWaiting, session.ID)
		case domain.SessionStatusWaitingAsyncExecution:
			binding, err := s.recoverWaitingAsync(ctx, session)
			if err != nil {
				return ScanReport{}, err
			}
			report.WaitingAsync = append(report.WaitingAsync, binding)
		}
	}
	return report, nil
}

func (s *service) validateReady() error {
	if s.repos.Sessions == nil || s.repos.Executions == nil || s.repos.Audits == nil {
		return errors.New("recovery repositories are incomplete")
	}
	return nil
}

func (s *service) failRunningSession(ctx context.Context, session *domain.Session) error {
	now := s.clock.Now()
	payload, err := json.Marshal(map[string]any{
		"reason": "orphaned_running_session_on_startup",
	})
	if err != nil {
		return err
	}

	session.Status = domain.SessionStatusFailed
	session.Revision++
	session.UpdatedAt = now
	if err := s.repos.Sessions.Update(ctx, *session); err != nil {
		return err
	}

	return s.repos.Audits.Append(ctx, domain.AuditRecord{
		ID:        s.ids.NewID("audit"),
		SessionID: session.ID,
		TaskID:    cloneStringPtr(session.CurrentTaskID),
		ActorID:   recoveryActorID,
		EventType: "session.recovery.failed_running",
		Payload:   payload,
		CreatedAt: now,
	})
}

func (s *service) recoverWaitingAsync(ctx context.Context, session domain.Session) (WaitingAsyncBinding, error) {
	binding := WaitingAsyncBinding{
		SessionID:        session.ID,
		ExecutionGroupID: cloneStringPtr(session.CurrentExecutionGroupID),
	}
	if session.CurrentTaskID == nil || *session.CurrentTaskID == "" {
		binding.WaitingForCallback = true
		return binding, nil
	}
	binding.TaskID = *session.CurrentTaskID

	aggregate, err := s.repos.Executions.AggregateByTask(ctx, *session.CurrentTaskID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			binding.WaitingForCallback = true
			return binding, nil
		}
		return WaitingAsyncBinding{}, err
	}
	binding.Aggregate = aggregate

	if allExecutionsTerminal(aggregate) && s.runtime != nil {
		if err := s.runtime.HandleExecutionFinished(ctx, session.ID, *session.CurrentTaskID); err != nil {
			return WaitingAsyncBinding{}, err
		}
		binding.ResumeTriggered = true
		return binding, nil
	}

	binding.WaitingForCallback = true
	return binding, nil
}

func allExecutionsTerminal(aggregate domain.ExecutionAggregate) bool {
	if aggregate.Total == 0 {
		return false
	}
	return aggregate.Queued == 0 && aggregate.Dispatched == 0 && aggregate.Running == 0
}

func cloneStringPtr(in *string) *string {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}
