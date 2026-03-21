package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/infra/memory"
	"github.com/momaek/tolato/internal/shared/types"
)

type staticIDGenerator struct{}

func (staticIDGenerator) New() string {
	return "audit-timeout"
}

func TestTimeoutTasksMarksStaleExecution(t *testing.T) {
	ctx := context.Background()
	_, _, taskRepo, auditRepo, _ := memory.NewStores()
	usecase := TimeoutTasks{
		TaskRepo:  taskRepo,
		AuditRepo: auditRepo,
		IDGen:     staticIDGenerator{},
	}

	startedAt := time.Now().UTC().Add(-45 * time.Second)
	err := taskRepo.Create(ctx, types.Task{
		ID:             "task-1",
		Mode:           "manual_command",
		InitiatorID:    "u_admin",
		Target:         []string{"sg-prod-01"},
		InputText:      "systemctl restart nginx",
		Plan:           types.Plan{TargetNodes: []string{"sg-prod-01"}, Summary: "restart", Steps: []types.PlanStep{{Action: "restart_service", TimeoutSec: 5}}},
		ApprovalStatus: "approved",
		FinalStatus:    "running",
		CreatedAt:      startedAt,
		UpdatedAt:      startedAt,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	err = taskRepo.UpsertExecution(ctx, types.TaskExecution{
		ID:        "exec-1",
		TaskID:    "task-1",
		NodeID:    "sg-prod-01",
		Status:    "running",
		Attempt:   1,
		StartedAt: startedAt,
	})
	if err != nil {
		t.Fatalf("UpsertExecution returned error: %v", err)
	}

	events, err := usecase.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 timeout event, got %d", len(events))
	}

	taskView, err := taskRepo.Get(ctx, "task-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if taskView.FinalStatus != "timeout" {
		t.Fatalf("expected task status timeout, got %q", taskView.FinalStatus)
	}

	executions, err := taskRepo.ListExecutions(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListExecutions returned error: %v", err)
	}
	if executions[0].Status != "timeout" {
		t.Fatalf("expected execution status timeout, got %q", executions[0].Status)
	}
	if executions[0].ExitCode != 124 {
		t.Fatalf("expected exit code 124, got %d", executions[0].ExitCode)
	}
}

func TestTimeoutTasksLeavesFreshExecutionRunning(t *testing.T) {
	ctx := context.Background()
	_, _, taskRepo, auditRepo, _ := memory.NewStores()
	usecase := TimeoutTasks{
		TaskRepo:  taskRepo,
		AuditRepo: auditRepo,
		IDGen:     staticIDGenerator{},
	}

	startedAt := time.Now().UTC().Add(-2 * time.Second)
	err := taskRepo.Create(ctx, types.Task{
		ID:             "task-2",
		Mode:           "ai_agent",
		InitiatorID:    "u_admin",
		Target:         []string{"sg-prod-01"},
		InputText:      "check nginx",
		Plan:           types.Plan{TargetNodes: []string{"sg-prod-01"}, Summary: "check", Steps: []types.PlanStep{{Action: "service_status", TimeoutSec: 10}}},
		ApprovalStatus: "not_required",
		FinalStatus:    "running",
		CreatedAt:      startedAt,
		UpdatedAt:      startedAt,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	err = taskRepo.UpsertExecution(ctx, types.TaskExecution{
		ID:        "exec-2",
		TaskID:    "task-2",
		NodeID:    "sg-prod-01",
		Status:    "running",
		Attempt:   1,
		StartedAt: startedAt,
	})
	if err != nil {
		t.Fatalf("UpsertExecution returned error: %v", err)
	}

	events, err := usecase.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no timeout events, got %d", len(events))
	}

	taskView, err := taskRepo.Get(ctx, "task-2")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if taskView.FinalStatus != "running" {
		t.Fatalf("expected task status running, got %q", taskView.FinalStatus)
	}
}
