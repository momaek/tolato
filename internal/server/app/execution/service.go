package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

const userActionActorID = "ui_user"

type Service interface {
	StartDispatch(ctx context.Context, input StartDispatchInput) (StartDispatchResult, error)
	CancelTask(ctx context.Context, sessionID string, taskID string, idempotencyKey string) error
	RecordChunk(ctx context.Context, input RecordChunkInput) error
	FinishExecution(ctx context.Context, input FinishExecutionInput) error
}

type StartDispatchInput struct {
	SessionID     string
	InputText     string
	TargetContext domain.ActiveTargetContext
	RiskLevel     domain.RiskLevel
}

type StartDispatchResult struct {
	TaskID           string
	ExecutionGroupID string
	ExecutionIDs     []string
}

type RecordChunkInput struct {
	SessionID   string
	TaskID      string
	ExecutionID string
	NodeID      string
	Chunk       domain.ExecutionChunk
}

type FinishExecutionInput struct {
	SessionID    string
	TaskID       string
	ExecutionID  string
	NodeID       string
	Status       domain.ExecutionStatus
	ExitCode     *int
	StatusReason *string
}

type DispatchCommand struct {
	Type        string           `json:"type"`
	SessionID   string           `json:"sessionId"`
	TaskID      string           `json:"taskId"`
	ExecutionID string           `json:"executionId"`
	NodeID      string           `json:"nodeId"`
	Action      string           `json:"action"`
	Args        json.RawMessage  `json:"args,omitempty"`
	RiskLevel   domain.RiskLevel `json:"riskLevel,omitempty"`
	Timestamp   string           `json:"timestamp"`
}

type Repositories struct {
	Sessions    domain.SessionRepository
	Tasks       domain.TaskRepository
	Executions  domain.ExecutionRepository
	Timelines   domain.TimelineRepository
	ToolResults domain.ToolResultRepository
	Audits      domain.AuditRepository
}

type EventPublisher interface {
	SessionStateUpdated(ctx context.Context, session domain.Session) error
	TimelineRowAppended(ctx context.Context, session domain.Session, row domain.TimelineRow) error
	ExecutionChunk(ctx context.Context, sessionID string, taskID string, execution domain.Execution, chunk domain.ExecutionChunk) error
	ExecutionFinished(ctx context.Context, sessionID string, taskID string, execution domain.Execution) error
}

type AgentDispatchPublisher interface {
	DispatchToNode(ctx context.Context, nodeID string, cmd DispatchCommand) error
}

type CompletionHandler interface {
	HandleExecutionFinished(ctx context.Context, sessionID string, taskID string) error
}

type service struct {
	repos      Repositories
	clock      domain.Clock
	ids        domain.IDGenerator
	locks      domain.LockManager
	events     EventPublisher
	dispatcher AgentDispatchPublisher
	completion CompletionHandler
}

type Option func(*service)

func WithEventPublisher(events EventPublisher) Option {
	return func(s *service) {
		s.events = events
	}
}

func WithDispatchPublisher(dispatcher AgentDispatchPublisher) Option {
	return func(s *service) {
		s.dispatcher = dispatcher
	}
}

func WithCompletionHandler(completion CompletionHandler) Option {
	return func(s *service) {
		s.completion = completion
	}
}

func WithLockManager(locks domain.LockManager) Option {
	return func(s *service) {
		s.locks = locks
	}
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

func (s *service) StartDispatch(ctx context.Context, input StartDispatchInput) (StartDispatchResult, error) {
	if err := s.validateReady(); err != nil {
		return StartDispatchResult{}, err
	}
	if input.SessionID == "" || len(input.TargetContext.NodeIDs) == 0 {
		return StartDispatchResult{}, domain.ErrInvalidArgument
	}

	now := s.clock.Now()
	taskID := s.ids.NewID("task")
	groupID := s.ids.NewID("execgrp")
	task := domain.Task{
		ID:        taskID,
		SessionID: input.SessionID,
		InputText: input.InputText,
		OperationTargetSnapshot: domain.TargetSnapshot{
			Scope:        input.TargetContext.Scope,
			NodeIDs:      append([]string(nil), input.TargetContext.NodeIDs...),
			DisplayLabel: input.TargetContext.DisplayLabel,
			Source:       input.TargetContext.Source,
			Confirmed:    input.TargetContext.Status == domain.TargetStatusConfirmed,
			ConfirmedAt:  input.TargetContext.ConfirmedAt,
			CapturedAt:   now,
		},
		Status:         domain.TaskStatusQueued,
		ApprovalStatus: approvalStatusForDispatch(input.RiskLevel),
		RiskLevel:      input.RiskLevel,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repos.Tasks.Create(ctx, task); err != nil {
		return StartDispatchResult{}, err
	}

	executions := make([]domain.Execution, 0, len(input.TargetContext.NodeIDs))
	executionIDs := make([]string, 0, len(input.TargetContext.NodeIDs))
	for _, nodeID := range input.TargetContext.NodeIDs {
		executionID := s.ids.NewID("exec")
		executionIDs = append(executionIDs, executionID)
		execution := domain.Execution{
			ID:        executionID,
			TaskID:    taskID,
			SessionID: input.SessionID,
			NodeID:    nodeID,
			Status:    domain.ExecutionStatusQueued,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.repos.Executions.Create(ctx, execution); err != nil {
			return StartDispatchResult{}, err
		}
		executions = append(executions, execution)
	}

	if s.dispatcher != nil {
		for i := range executions {
			command, err := buildDispatchCommand(task, executions[i], input, now)
			if err != nil {
				return StartDispatchResult{}, err
			}
			if err := s.dispatcher.DispatchToNode(ctx, executions[i].NodeID, command); err != nil {
				return StartDispatchResult{}, err
			}
			executions[i].Status = domain.ExecutionStatusDispatched
			executions[i].UpdatedAt = now
			if err := s.repos.Executions.Update(ctx, executions[i]); err != nil {
				return StartDispatchResult{}, err
			}
		}
		task.Status = domain.TaskStatusDispatched
		task.UpdatedAt = now
		if err := s.repos.Tasks.Update(ctx, task); err != nil {
			return StartDispatchResult{}, err
		}
	}

	return StartDispatchResult{
		TaskID:           taskID,
		ExecutionGroupID: groupID,
		ExecutionIDs:     executionIDs,
	}, nil
}

func (s *service) CancelTask(ctx context.Context, sessionID string, taskID string, idempotencyKey string) error {
	return s.withSessionLock(ctx, sessionID, func(ctx context.Context) error {
		if err := s.validateReady(); err != nil {
			return err
		}
		if taskID == "" || idempotencyKey == "" {
			return domain.ErrInvalidArgument
		}

		duplicated, err := s.hasProcessedCancel(ctx, sessionID, idempotencyKey)
		if err != nil {
			return err
		}
		if duplicated {
			return nil
		}

		session, err := s.repos.Sessions.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		task, err := s.repos.Tasks.Get(ctx, taskID)
		if err != nil {
			return err
		}
		if task.SessionID != sessionID {
			return domain.ErrInvalidArgument
		}

		now := s.clock.Now()
		payload := mustMarshalJSON(map[string]any{
			"idempotencyKey": idempotencyKey,
			"action":         "cancel",
			"taskId":         taskID,
		})

		switch session.Status {
		case domain.SessionStatusPausedWaitApproval:
			if session.PendingAction == nil || session.PendingAction.Type != domain.PendingActionTypeApproval {
				return fmt.Errorf("session %s is not waiting for approval", sessionID)
			}
			task.Status = domain.TaskStatusCancelled
			task.ApprovalStatus = domain.ApprovalStatusCancelled
			task.UpdatedAt = now
			if err := s.repos.Tasks.Update(ctx, task); err != nil {
				return err
			}
		case domain.SessionStatusWaitingAsyncExecution:
			executions, err := s.repos.Executions.ListByTask(ctx, taskID)
			if err != nil {
				return err
			}
			for _, execution := range executions {
				if isExecutionTerminal(execution.Status) {
					continue
				}
				execution.Status = domain.ExecutionStatusCancelled
				execution.FinishedAt = timePtr(now)
				execution.StatusReason = strPtr("cancelled by user")
				execution.UpdatedAt = now
				if err := s.repos.Executions.Update(ctx, execution); err != nil {
					return err
				}
			}
			task.Status = domain.TaskStatusCancelled
			if task.ApprovalStatus == domain.ApprovalStatusPending {
				task.ApprovalStatus = domain.ApprovalStatusCancelled
			}
			task.UpdatedAt = now
			if err := s.repos.Tasks.Update(ctx, task); err != nil {
				return err
			}
		default:
			return fmt.Errorf("session %s is not in a cancellable state", sessionID)
		}

		if err := s.repos.Audits.Append(ctx, domain.AuditRecord{
			ID:        s.ids.NewID("audit"),
			SessionID: sessionID,
			TaskID:    &taskID,
			ActorID:   userActionActorID,
			EventType: "operation.cancelled",
			Payload:   payload,
			CreatedAt: now,
		}); err != nil {
			return err
		}

		row, err := s.appendToolResult(ctx, sessionID, taskID, payload)
		if err != nil {
			return err
		}

		session.PendingAction = nil
		session.Status = domain.SessionStatusCompleted
		session.CurrentTaskID = nil
		session.CurrentExecutionGroupID = nil
		if err := s.bumpSession(ctx, &session); err != nil {
			return err
		}
		if err := s.publishTimelineRow(ctx, session, row); err != nil {
			return err
		}
		return s.publishSessionState(ctx, session)
	})
}

func (s *service) RecordChunk(ctx context.Context, input RecordChunkInput) error {
	if err := s.validateReady(); err != nil {
		return err
	}
	if input.ExecutionID == "" || input.Chunk.Stream == "" || input.Chunk.Text == "" {
		return domain.ErrInvalidArgument
	}

	execution, err := s.repos.Executions.Get(ctx, input.ExecutionID)
	if err != nil {
		return err
	}
	if err := validateExecutionEnvelope(execution, input.SessionID, input.TaskID, input.NodeID); err != nil {
		return err
	}

	now := s.clock.Now()
	switch input.Chunk.Stream {
	case domain.ExecutionStreamStdout:
		execution.StdoutTail = appendExecutionTail(execution.StdoutTail, input.Chunk.Text)
	case domain.ExecutionStreamStderr:
		execution.StderrTail = appendExecutionTail(execution.StderrTail, input.Chunk.Text)
	default:
		return domain.ErrInvalidArgument
	}
	if !isExecutionTerminal(execution.Status) {
		execution.Status = domain.ExecutionStatusRunning
		if execution.StartedAt == nil {
			execution.StartedAt = timePtr(now)
		}
	}
	execution.UpdatedAt = now
	if err := s.repos.Executions.Update(ctx, execution); err != nil {
		return err
	}

	task, err := s.repos.Tasks.Get(ctx, execution.TaskID)
	if err != nil {
		return err
	}
	if task.Status == domain.TaskStatusQueued || task.Status == domain.TaskStatusDispatched {
		task.Status = domain.TaskStatusRunning
		task.UpdatedAt = now
		if err := s.repos.Tasks.Update(ctx, task); err != nil {
			return err
		}
	}

	return s.publishExecutionChunk(ctx, execution.SessionID, execution.TaskID, execution, input.Chunk)
}

func (s *service) FinishExecution(ctx context.Context, input FinishExecutionInput) error {
	if err := s.validateReady(); err != nil {
		return err
	}
	if input.ExecutionID == "" || input.Status == "" || !isExecutionTerminal(input.Status) {
		return domain.ErrInvalidArgument
	}

	execution, err := s.repos.Executions.Get(ctx, input.ExecutionID)
	if err != nil {
		return err
	}
	if err := validateExecutionEnvelope(execution, input.SessionID, input.TaskID, input.NodeID); err != nil {
		return err
	}

	now := s.clock.Now()
	if execution.StartedAt == nil {
		execution.StartedAt = timePtr(now)
	}
	execution.Status = input.Status
	execution.FinishedAt = timePtr(now)
	execution.ExitCode = intPtrValue(input.ExitCode)
	execution.StatusReason = cloneStringPtr(input.StatusReason)
	execution.UpdatedAt = now
	if err := s.repos.Executions.Update(ctx, execution); err != nil {
		return err
	}

	aggregate, err := s.repos.Executions.AggregateByTask(ctx, execution.TaskID)
	if err != nil {
		return err
	}
	task, err := s.repos.Tasks.Get(ctx, execution.TaskID)
	if err != nil {
		return err
	}
	task.Status = aggregateTaskStatus(aggregate)
	task.UpdatedAt = now
	if err := s.repos.Tasks.Update(ctx, task); err != nil {
		return err
	}

	if err := s.publishExecutionFinished(ctx, execution.SessionID, execution.TaskID, execution); err != nil {
		return err
	}
	if !allExecutionsTerminal(aggregate) || s.completion == nil {
		return nil
	}
	return s.completion.HandleExecutionFinished(ctx, execution.SessionID, execution.TaskID)
}

func (s *service) validateReady() error {
	if s.repos.Sessions == nil || s.repos.Tasks == nil || s.repos.Executions == nil || s.repos.Timelines == nil || s.repos.ToolResults == nil || s.repos.Audits == nil {
		return errors.New("execution repositories are incomplete")
	}
	return nil
}

func (s *service) withSessionLock(ctx context.Context, sessionID string, fn func(context.Context) error) error {
	if s.locks == nil {
		return fn(ctx)
	}
	unlock, err := s.locks.LockSession(ctx, sessionID)
	if err != nil {
		return err
	}
	defer unlock()
	return fn(ctx)
}

func (s *service) hasProcessedCancel(ctx context.Context, sessionID, idempotencyKey string) (bool, error) {
	results, err := s.repos.ToolResults.ListBySession(ctx, sessionID, domain.CursorPage{})
	if err != nil {
		return false, err
	}
	for _, result := range results {
		if result.ToolName != "cancel" || result.Source != domain.TimelineRowSourceUserAction {
			continue
		}
		var payload struct {
			IdempotencyKey string `json:"idempotencyKey"`
		}
		if err := json.Unmarshal(result.Payload, &payload); err != nil {
			continue
		}
		if payload.IdempotencyKey == idempotencyKey {
			return true, nil
		}
	}
	return false, nil
}

func (s *service) appendToolResult(ctx context.Context, sessionID, taskID string, payload json.RawMessage) (domain.TimelineRow, error) {
	now := s.clock.Now()
	if err := s.repos.ToolResults.Append(ctx, domain.ToolResult{
		ID:        s.ids.NewID("toolresult"),
		SessionID: sessionID,
		TaskID:    &taskID,
		ToolName:  "cancel",
		Status:    domain.ToolResultStatusSucceeded,
		Text:      "operation cancelled",
		Source:    domain.TimelineRowSourceUserAction,
		Payload:   cloneRaw(payload),
		CreatedAt: now,
	}); err != nil {
		return domain.TimelineRow{}, err
	}
	row := domain.TimelineRow{
		ID:         s.ids.NewID("row"),
		SessionID:  sessionID,
		Kind:       domain.TimelineRowKindToolResultMeta,
		CreatedAt:  now,
		Text:       "operation cancelled",
		ToolName:   "cancel",
		ToolStatus: domain.ToolResultStatusSucceeded,
		Source:     domain.TimelineRowSourceUserAction,
		TaskID:     &taskID,
	}
	return row, s.repos.Timelines.Append(ctx, row)
}

func (s *service) bumpSession(ctx context.Context, session *domain.Session) error {
	session.Revision++
	session.UpdatedAt = s.clock.Now()
	return s.repos.Sessions.Update(ctx, *session)
}

func (s *service) publishSessionState(ctx context.Context, session domain.Session) error {
	if s.events == nil {
		return nil
	}
	return s.events.SessionStateUpdated(ctx, session)
}

func (s *service) publishTimelineRow(ctx context.Context, session domain.Session, row domain.TimelineRow) error {
	if s.events == nil {
		return nil
	}
	return s.events.TimelineRowAppended(ctx, session, row)
}

func (s *service) publishExecutionChunk(ctx context.Context, sessionID string, taskID string, execution domain.Execution, chunk domain.ExecutionChunk) error {
	if s.events == nil {
		return nil
	}
	return s.events.ExecutionChunk(ctx, sessionID, taskID, execution, chunk)
}

func (s *service) publishExecutionFinished(ctx context.Context, sessionID string, taskID string, execution domain.Execution) error {
	if s.events == nil {
		return nil
	}
	return s.events.ExecutionFinished(ctx, sessionID, taskID, execution)
}

func isExecutionTerminal(status domain.ExecutionStatus) bool {
	switch status {
	case domain.ExecutionStatusSuccess, domain.ExecutionStatusFailed, domain.ExecutionStatusTimeout, domain.ExecutionStatusCancelled:
		return true
	default:
		return false
	}
}

func approvalStatusForDispatch(risk domain.RiskLevel) domain.ApprovalStatus {
	switch risk {
	case domain.RiskLevelMedium, domain.RiskLevelHigh:
		return domain.ApprovalStatusApproved
	default:
		return domain.ApprovalStatusNotRequired
	}
}

func buildDispatchCommand(task domain.Task, execution domain.Execution, input StartDispatchInput, now time.Time) (DispatchCommand, error) {
	args, err := json.Marshal(map[string]any{
		"inputText": input.InputText,
	})
	if err != nil {
		return DispatchCommand{}, err
	}
	return DispatchCommand{
		Type:        "task.dispatch",
		SessionID:   task.SessionID,
		TaskID:      task.ID,
		ExecutionID: execution.ID,
		NodeID:      execution.NodeID,
		Action:      "execute_task",
		Args:        args,
		RiskLevel:   input.RiskLevel,
		Timestamp:   now.UTC().Format(time.RFC3339),
	}, nil
}

func appendExecutionTail(existing, chunk string) string {
	const maxTailLen = 4096
	combined := existing + chunk
	if len(combined) <= maxTailLen {
		return combined
	}
	return combined[len(combined)-maxTailLen:]
}

func validateExecutionEnvelope(execution domain.Execution, sessionID, taskID, nodeID string) error {
	if sessionID != "" && execution.SessionID != sessionID {
		return domain.ErrInvalidArgument
	}
	if taskID != "" && execution.TaskID != taskID {
		return domain.ErrInvalidArgument
	}
	if nodeID != "" && execution.NodeID != nodeID {
		return domain.ErrInvalidArgument
	}
	return nil
}

func allExecutionsTerminal(aggregate domain.ExecutionAggregate) bool {
	return aggregate.Total > 0 && aggregate.Queued == 0 && aggregate.Dispatched == 0 && aggregate.Running == 0
}

func aggregateTaskStatus(aggregate domain.ExecutionAggregate) domain.TaskStatus {
	if aggregate.Total == 0 {
		return domain.TaskStatusQueued
	}
	switch {
	case aggregate.Running > 0:
		return domain.TaskStatusRunning
	case aggregate.Dispatched > 0:
		return domain.TaskStatusDispatched
	case aggregate.Queued > 0:
		return domain.TaskStatusQueued
	case aggregate.Cancelled == aggregate.Total:
		return domain.TaskStatusCancelled
	case aggregate.Success == aggregate.Total:
		return domain.TaskStatusSuccess
	case aggregate.Timeout == aggregate.Total:
		return domain.TaskStatusTimeout
	case aggregate.Failed+aggregate.Cancelled == aggregate.Total:
		return domain.TaskStatusFailed
	case aggregate.Success > 0 && (aggregate.Failed > 0 || aggregate.Timeout > 0 || aggregate.Cancelled > 0):
		return domain.TaskStatusPartialFailed
	case aggregate.Timeout > 0 && aggregate.Success == 0 && aggregate.Failed == 0 && aggregate.Cancelled == 0:
		return domain.TaskStatusTimeout
	case aggregate.Failed > 0 || aggregate.Timeout > 0 || aggregate.Cancelled > 0:
		return domain.TaskStatusPartialFailed
	default:
		return domain.TaskStatusSuccess
	}
}

func mustMarshalJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func cloneRaw(in []byte) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func cloneStringPtr(in *string) *string {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func strPtr(v string) *string { return &v }

func intPtrValue(in *int) *int {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func timePtr(v time.Time) *time.Time {
	out := v.UTC()
	return &out
}
