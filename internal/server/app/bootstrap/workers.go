package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/momaek/tolato/internal/server/domain/audit"
	"github.com/momaek/tolato/internal/server/domain/outbox"
	"github.com/momaek/tolato/internal/server/domain/task"
	"github.com/momaek/tolato/internal/server/infra/dispatch"
	"github.com/momaek/tolato/internal/server/infra/idgen"
	"github.com/momaek/tolato/internal/server/infra/queue"
	"github.com/momaek/tolato/internal/shared/types"
	"go.uber.org/zap"
)

func runOutboxRelay(ctx context.Context, logger *zap.Logger, repo outbox.Repository, stream *queue.Stream) {
	if repo == nil || stream == nil {
		return
	}
	items, err := repo.ListPending(ctx, 32)
	if err != nil {
		logger.Warn("outbox relay failed", zap.Error(err))
		return
	}
	now := time.Now().UTC()
	for _, item := range items {
		if err := stream.Publish(ctx, item); err != nil {
			_ = repo.IncrementAttempts(ctx, item.ID)
			logger.Warn("queue publish failed", zap.String("outbox_id", item.ID), zap.Error(err))
			continue
		}
		_ = repo.MarkPublished(ctx, item.ID, now)
	}
}

func runDispatchWorker(ctx context.Context, logger *zap.Logger, stream *queue.Stream, dispatcher *dispatch.Manager, taskRepo task.Repository, auditRepo audit.Repository, idGen idgen.Generator) {
	if stream == nil || dispatcher == nil || taskRepo == nil {
		return
	}
	lastID := ""
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		items, nextID, err := stream.Consume(ctx, lastID, 16, 2*time.Second)
		if err != nil {
			logger.Warn("dispatch worker consume failed", zap.Error(err))
			continue
		}
		if nextID != "" {
			lastID = nextID
		}
		for _, item := range items {
			executions, err := taskRepo.ListExecutions(ctx, item.TaskID)
			if err != nil {
				logger.Warn("dispatch worker list executions failed", zap.Error(err), zap.String("task_id", item.TaskID))
				continue
			}
			execution := findExecutionByID(executions, item.ExecutionID)
			if execution == nil {
				continue
			}
			switch item.Topic {
			case "task.dispatch":
				steps := decodePlanSteps(item.Payload["steps"])
				timeoutSec := decodeTimeout(item.Payload["timeout_sec"])
				if err := dispatcher.DispatchTask(ctx, item.NodeID, item.TaskID, item.ExecutionID, steps, timeoutSec); err != nil {
					execution.Status = "failed"
					execution.StatusReason = fmt.Sprintf("dispatch failed: %v", err)
					execution.FinishedAt = time.Now().UTC()
					execution.ExitCode = 1
				} else {
					execution.Status = "dispatched"
					execution.StatusReason = "dispatched to agent"
				}
				_ = taskRepo.UpsertExecution(ctx, *execution)
				recordAuditEvent(ctx, auditRepo, idGen, item.TaskID, item.NodeID, execution)
			case "task.cancel":
				reason, _ := item.Payload["reason"].(string)
				_ = dispatcher.CancelTask(ctx, item.NodeID, item.TaskID, item.ExecutionID, reason)
			}
		}
	}
}

func findExecutionByID(executions []task.TaskExecution, executionID string) *task.TaskExecution {
	for idx := range executions {
		if executions[idx].ID == executionID {
			item := executions[idx]
			return &item
		}
	}
	return nil
}

func decodeTimeout(raw any) int {
	switch value := raw.(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 10
	}
}

func decodePlanSteps(raw any) []types.PlanStep {
	switch value := raw.(type) {
	case []types.PlanStep:
		return value
	case []any:
		payload, _ := json.Marshal(value)
		var steps []types.PlanStep
		_ = json.Unmarshal(payload, &steps)
		return steps
	default:
		return nil
	}
}

func recordAuditEvent(ctx context.Context, repo audit.Repository, idGen idgen.Generator, taskID, actorID string, execution *task.TaskExecution) {
	if repo == nil || execution == nil {
		return
	}
	eventType := "task_execution_dispatched"
	if execution.Status == "failed" {
		eventType = "task_execution_dispatch_failed"
	}
	_ = repo.Create(ctx, audit.AuditEvent{
		ID:        idGen.New(),
		TaskID:    taskID,
		ActorID:   actorID,
		EventType: eventType,
		Payload: map[string]any{
			"execution_id": execution.ID,
			"node_id":      execution.NodeID,
			"reason":       execution.StatusReason,
		},
		CreatedAt: time.Now().UTC(),
	})
}
