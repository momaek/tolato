package usecase

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/server/domain/audit"
	"github.com/momaek/tolato/internal/server/domain/task"
	"github.com/momaek/tolato/internal/server/infra/idgen"
)

const watchdogTimeoutGrace = 15 * time.Second

type TimeoutEvent struct {
	TaskID     string
	TaskStatus string
	Execution  task.TaskExecution
}

type TimeoutTasks struct {
	TaskRepo  task.Repository
	AuditRepo audit.Repository
	IDGen     idgen.Generator
}

func (uc TimeoutTasks) Execute(ctx context.Context) ([]TimeoutEvent, error) {
	if uc.TaskRepo == nil {
		return nil, nil
	}

	tasks, err := uc.TaskRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	events := make([]TimeoutEvent, 0)
	for _, model := range tasks {
		if !isWatchdogCandidate(model) {
			continue
		}

		itemEvents, err := uc.timeoutTaskExecutions(ctx, &model)
		if err != nil {
			return nil, err
		}
		events = append(events, itemEvents...)
	}

	return events, nil
}

func (uc TimeoutTasks) timeoutTaskExecutions(ctx context.Context, model *task.Task) ([]TimeoutEvent, error) {
	executions, err := uc.TaskRepo.ListExecutions(ctx, model.ID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	timeoutAfter := time.Duration(maxStepTimeout(model.Plan.Steps))*time.Second + watchdogTimeoutGrace
	timedOut := make([]task.TaskExecution, 0)

	for _, execution := range executions {
		if !isExecutionActive(execution.Status) {
			continue
		}

		reference := timeoutReference(*model, execution)
		if reference.IsZero() || now.Sub(reference) <= timeoutAfter {
			continue
		}

		execution.Status = "timeout"
		execution.FinishedAt = now
		execution.ExitCode = 124
		execution.StatusReason = "watchdog timeout: no result received before deadline"
		if execution.StartedAt.IsZero() {
			execution.StartedAt = reference
		}

		if err := uc.TaskRepo.UpsertExecution(ctx, execution); err != nil {
			return nil, err
		}
		recordAudit(ctx, uc.AuditRepo, uc.IDGen, model.ID, "system", "task_execution_timeout", map[string]any{
			"execution_id": execution.ID,
			"node_id":      execution.NodeID,
			"reason":       execution.StatusReason,
		}, now)
		timedOut = append(timedOut, execution)
	}

	if len(timedOut) == 0 {
		return nil, nil
	}

	refreshed, err := uc.TaskRepo.ListExecutions(ctx, model.ID)
	if err != nil {
		return nil, err
	}

	aggregate := buildTaskAggregate(*model, refreshed)
	model.FinalStatus = finalTaskStatus(*model, refreshed, aggregate)
	model.StatusReason = buildTaskSummary(*model, aggregate)
	model.UpdatedAt = now
	if err := uc.TaskRepo.Update(ctx, *model); err != nil {
		return nil, err
	}

	events := make([]TimeoutEvent, 0, len(timedOut))
	for _, execution := range timedOut {
		events = append(events, TimeoutEvent{
			TaskID:     model.ID,
			TaskStatus: model.FinalStatus,
			Execution:  execution,
		})
	}

	return events, nil
}

func isWatchdogCandidate(model task.Task) bool {
	switch model.FinalStatus {
	case "approved", "queued", "dispatched", "running":
		return true
	default:
		return false
	}
}

func isExecutionActive(status string) bool {
	switch status {
	case "approved", "queued", "dispatched", "running":
		return true
	default:
		return false
	}
}

func timeoutReference(model task.Task, execution task.TaskExecution) time.Time {
	if !execution.StartedAt.IsZero() {
		return execution.StartedAt
	}
	if !model.UpdatedAt.IsZero() {
		return model.UpdatedAt
	}
	return model.CreatedAt
}
