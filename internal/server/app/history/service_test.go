package history

import (
	"context"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
	"github.com/momaek/tolato/internal/server/infra/store/memory"
)

func TestServiceListTasks(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	summary := "execution completed successfully"

	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:        "sess-1",
		Title:     "Tokyo",
		Status:    domain.SessionStatusCompleted,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}
	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:        "task-1",
		SessionID: "sess-1",
		InputText: "restart nginx",
		OperationTargetSnapshot: domain.TargetSnapshot{
			DisplayLabel: "jp-tokyo-01",
		},
		Status:         domain.TaskStatusSuccess,
		ApprovalStatus: domain.ApprovalStatusApproved,
		RiskLevel:      domain.RiskLevelMedium,
		Summary:        &summary,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}

	svc := NewService(Repositories{
		Sessions: store.Sessions,
		Tasks:    store.Tasks,
	})

	items, err := svc.ListTasks(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "task-1" {
		t.Fatalf("items = %#v, want single task item", items)
	}
	if items[0].Summary != summary || items[0].TargetLabels[0] != "jp-tokyo-01" {
		t.Fatalf("item = %#v, want mapped summary and target labels", items[0])
	}
}

func TestServiceGetTaskDetail(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	summary := "execution completed successfully"
	taskID := "task-1"

	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:        taskID,
		SessionID: "sess-1",
		InputText: "restart nginx",
		OperationTargetSnapshot: domain.TargetSnapshot{
			DisplayLabel: "jp-tokyo-01",
		},
		Status:         domain.TaskStatusSuccess,
		ApprovalStatus: domain.ApprovalStatusApproved,
		RiskLevel:      domain.RiskLevelMedium,
		Summary:        &summary,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	if err := store.Executions.Create(context.Background(), domain.Execution{
		ID:         "exec-1",
		TaskID:     taskID,
		SessionID:  "sess-1",
		NodeID:     "jp-tokyo-01",
		Status:     domain.ExecutionStatusSuccess,
		StdoutTail: "nginx ok",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("Create(execution) error = %v", err)
	}
	if err := store.Audits.Append(context.Background(), domain.AuditRecord{
		ID:        "audit-1",
		SessionID: "sess-1",
		TaskID:    &taskID,
		ActorID:   "ui_user",
		EventType: "approval.approved",
		Payload:   []byte(`{"approved":true}`),
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("Append(audit) error = %v", err)
	}
	if err := store.ToolResults.Append(context.Background(), domain.ToolResult{
		ID:        "toolresult-1",
		SessionID: "sess-1",
		TaskID:    &taskID,
		ToolName:  "approval",
		Status:    domain.ToolResultStatusSucceeded,
		Text:      "approval recorded",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("Append(tool result) error = %v", err)
	}

	svc := NewService(Repositories{
		Tasks:       store.Tasks,
		Executions:  store.Executions,
		Audits:      store.Audits,
		ToolResults: store.ToolResults,
	})

	detail, err := svc.GetTaskDetail(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTaskDetail() error = %v", err)
	}
	if detail.ID != taskID || detail.AISummary != summary {
		t.Fatalf("detail = %#v, want mapped detail", detail)
	}
	if len(detail.Executions) != 1 || detail.Executions[0].Label != "jp-tokyo-01" {
		t.Fatalf("executions = %#v, want node execution summary", detail.Executions)
	}
	if len(detail.AuditEvents) != 1 || len(detail.ToolMeta) != 1 {
		t.Fatalf("detail = %#v, want audit and tool meta", detail)
	}
	if detail.Plan == nil {
		t.Fatalf("detail.Plan = nil, want fallback plan detail")
	}
}
