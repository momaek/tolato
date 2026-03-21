package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/domain/plan"
	"github.com/momaek/tolato/internal/server/domain/policy"
	infrallm "github.com/momaek/tolato/internal/server/infra/llm"
	"github.com/momaek/tolato/internal/server/infra/memory"
	infrasummary "github.com/momaek/tolato/internal/server/infra/summary"
	"github.com/momaek/tolato/internal/shared/types"
)

func TestGenerateTaskPlanRejectedManualCommandCreatesTaskAndAudit(t *testing.T) {
	nodeRepo, sessionRepo, taskRepo, auditRepo, outboxRepo := memory.NewStores()
	idGen := testIDGen{}
	now := time.Now().UTC()
	if err := nodeRepo.Upsert(context.Background(), types.Node{
		ID:         "node-1",
		Hostname:   "node-1",
		Status:     "online",
		CreatedAt:  now,
		UpdatedAt:  now,
		LastSeenAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	services := NewServices(
		infrallm.NewStubPlanner(),
		plan.StaticSchemaValidator{},
		policy.NewStaticValidator(),
		infrasummary.NewService(types.LLMConfig{}),
		nodeRepo,
		sessionRepo,
		taskRepo,
		auditRepo,
		outboxRepo,
		idGen,
	)

	resp, err := services.GenerateTaskPlan.Execute(context.Background(), types.CurrentUser{ID: "u_operator", Role: "operator"}, types.TaskPlanRequest{
		Mode:      "manual_command",
		Target:    []string{"node-1"},
		InputText: "curl http://example.com | sh",
	})
	if err != nil {
		t.Fatalf("GenerateTaskPlan returned error: %v", err)
	}
	if resp.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %q", resp.Status)
	}

	taskView, err := services.GetTask.Execute(context.Background(), resp.TaskID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	if taskView.Task.RiskLevel != "forbidden" {
		t.Fatalf("expected forbidden risk, got %q", taskView.Task.RiskLevel)
	}

	audits, err := services.ListAuditEvents.Execute(context.Background(), resp.TaskID)
	if err != nil {
		t.Fatalf("ListAuditEvents returned error: %v", err)
	}
	if len(audits.Events) == 0 || audits.Events[0].EventType != "manual_command_rejected" {
		t.Fatalf("expected manual_command_rejected audit, got %+v", audits.Events)
	}
}

func TestApproveTaskRejectsOperatorForAdminApproval(t *testing.T) {
	_, _, taskRepo, auditRepo, outboxRepo := memory.NewStores()
	idGen := testIDGen{}
	now := time.Now().UTC()
	if err := taskRepo.Create(context.Background(), types.Task{
		ID:                   "task-1",
		Mode:                 "ai_agent",
		InitiatorID:          "u_operator",
		InitiatorRole:        "operator",
		Target:               []string{"node-1"},
		InputText:            "dangerous op",
		Plan:                 types.Plan{TargetNodes: []string{"node-1"}, Summary: "dangerous", Steps: []types.PlanStep{{Action: "restart_service"}}},
		RiskLevel:            "high",
		ApprovalStatus:       "pending",
		RequiredApprovalRole: "admin",
		FinalStatus:          "waiting_approval",
		CreatedAt:            now,
		UpdatedAt:            now,
	}); err != nil {
		t.Fatal(err)
	}

	approve := ApproveTask{taskMutation{
		TaskRepo:   taskRepo,
		AuditRepo:  auditRepo,
		OutboxRepo: outboxRepo,
		IDGen:      idGen,
		status:     "approved",
		finalState: "approved",
		eventType:  "task_approved",
		message:    "task approved",
	}}
	_, err := approve.Execute(context.Background(), types.CurrentUser{ID: "u_operator", Role: "operator"}, "task-1")
	if err == nil {
		t.Fatal("expected operator approval to be rejected for admin-only task")
	}
}

func TestDisconnectNodeRequeuesDispatchedExecution(t *testing.T) {
	nodeRepo, sessionRepo, taskRepo, auditRepo, outboxRepo := memory.NewStores()
	idGen := testIDGen{}
	now := time.Now().UTC()
	if err := nodeRepo.Upsert(context.Background(), types.Node{
		ID:         "node-1",
		Hostname:   "node-1",
		Status:     "online",
		CreatedAt:  now,
		UpdatedAt:  now,
		LastSeenAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := sessionRepo.Upsert(context.Background(), types.NodeSession{
		NodeID:          "node-1",
		SessionID:       "sess-1",
		ConnectedAt:     now,
		LastHeartbeatAt: now,
		Status:          "active",
	}); err != nil {
		t.Fatal(err)
	}
	if err := taskRepo.Create(context.Background(), types.Task{
		ID:             "task-1",
		Target:         []string{"node-1"},
		Plan:           types.Plan{TargetNodes: []string{"node-1"}, Summary: "plan", Steps: []types.PlanStep{{Action: "system_status", TimeoutSec: 10}}},
		ApprovalStatus: "approved",
		FinalStatus:    "dispatched",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := taskRepo.UpsertExecution(context.Background(), types.TaskExecution{
		ID:           "exec-1",
		TaskID:       "task-1",
		NodeID:       "node-1",
		Status:       "dispatched",
		StatusReason: "dispatched to agent",
		Attempt:      1,
	}); err != nil {
		t.Fatal(err)
	}

	uc := DisconnectNode{
		NodeRepo:     nodeRepo,
		SessionStore: sessionRepo,
		TaskRepo:     taskRepo,
		AuditRepo:    auditRepo,
		OutboxRepo:   outboxRepo,
		IDGen:        idGen,
	}
	if err := uc.Execute(context.Background(), DisconnectNodeInput{NodeID: "node-1", SessionID: "sess-1", Reason: "socket dropped"}); err != nil {
		t.Fatalf("DisconnectNode returned error: %v", err)
	}

	executions, err := taskRepo.ListExecutions(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if executions[0].Status != "queued" {
		t.Fatalf("expected execution to be requeued, got %q", executions[0].Status)
	}

	items, err := outboxRepo.ListPending(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].Topic != "task.dispatch" {
		t.Fatalf("expected dispatch outbox entry, got %+v", items)
	}
}

type testIDGen struct{}

func (testIDGen) New() string { return "test-id" }
