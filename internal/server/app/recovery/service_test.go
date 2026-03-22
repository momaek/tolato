package recovery

import (
	"context"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
	"github.com/momaek/tolato/internal/server/infra"
	"github.com/momaek/tolato/internal/server/infra/store/memory"
)

func TestScanFailsOrphanedRunningSessionAndWritesAudit(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 16, 0, 0, 0, time.UTC)}
	idgen := &stubIDGen{values: []string{"audit-1"}}
	taskID := "task-running"

	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:             taskID,
		SessionID:      "sess-running",
		Status:         domain.TaskStatusRunning,
		ApprovalStatus: domain.ApprovalStatusApproved,
		RiskLevel:      domain.RiskLevelLow,
		CreatedAt:      clock.Now(),
		UpdatedAt:      clock.Now(),
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:            "sess-running",
		Title:         "Running",
		Status:        domain.SessionStatusRunning,
		CurrentTaskID: &taskID,
		Revision:      3,
		CreatedAt:     clock.Now(),
		UpdatedAt:     clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	svc := NewService(Repositories{
		Sessions:   store.Sessions,
		Executions: store.Executions,
		Audits:     store.Audits,
	}, clock, idgen)

	report, err := svc.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(report.FailedRunning) != 1 || report.FailedRunning[0] != "sess-running" {
		t.Fatalf("failed running = %#v, want sess-running", report.FailedRunning)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-running")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusFailed {
		t.Fatalf("session status = %q, want failed", gotSession.Status)
	}
	if gotSession.Revision != 4 {
		t.Fatalf("session revision = %d, want 4", gotSession.Revision)
	}

	audits, err := store.Audits.ListByTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("ListByTask(audits) error = %v", err)
	}
	if len(audits) != 1 || audits[0].EventType != "session.recovery.failed_running" {
		t.Fatalf("audits = %#v, want recovery failed_running audit", audits)
	}
}

func TestScanKeepsPausedSessionsRecoverable(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 16, 5, 0, 0, time.UTC)}

	for _, session := range []domain.Session{
		{
			ID:        "sess-target",
			Title:     "Target",
			Status:    domain.SessionStatusPausedWaitTargetConfirmation,
			Revision:  1,
			CreatedAt: clock.Now(),
			UpdatedAt: clock.Now(),
		},
		{
			ID:        "sess-approval",
			Title:     "Approval",
			Status:    domain.SessionStatusPausedWaitApproval,
			Revision:  1,
			CreatedAt: clock.Now(),
			UpdatedAt: clock.Now(),
		},
	} {
		if err := store.Sessions.Create(context.Background(), session); err != nil {
			t.Fatalf("Create(session %s) error = %v", session.ID, err)
		}
	}

	svc := NewService(Repositories{
		Sessions:   store.Sessions,
		Executions: store.Executions,
		Audits:     store.Audits,
	}, clock, &stubIDGen{})

	report, err := svc.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(report.PausedWaiting) != 2 {
		t.Fatalf("paused waiting = %#v, want two paused sessions", report.PausedWaiting)
	}
}

func TestScanTriggersRuntimeResumeWhenWaitingAsyncTaskIsTerminal(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 16, 10, 0, 0, time.UTC)}
	taskID := "task-terminal"
	groupID := "group-terminal"

	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:                      "sess-terminal",
		Title:                   "Waiting Terminal",
		Status:                  domain.SessionStatusWaitingAsyncExecution,
		CurrentTaskID:           &taskID,
		CurrentExecutionGroupID: &groupID,
		Revision:                2,
		CreatedAt:               clock.Now(),
		UpdatedAt:               clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}
	for _, execution := range []domain.Execution{
		{ID: "exec-1", TaskID: taskID, SessionID: "sess-terminal", NodeID: "node-1", Status: domain.ExecutionStatusSuccess, CreatedAt: clock.Now(), UpdatedAt: clock.Now()},
		{ID: "exec-2", TaskID: taskID, SessionID: "sess-terminal", NodeID: "node-2", Status: domain.ExecutionStatusFailed, CreatedAt: clock.Now(), UpdatedAt: clock.Now()},
	} {
		if err := store.Executions.Create(context.Background(), execution); err != nil {
			t.Fatalf("Create(execution) error = %v", err)
		}
	}

	runtime := &stubRuntimeResumer{}
	svc := NewService(Repositories{
		Sessions:   store.Sessions,
		Executions: store.Executions,
		Audits:     store.Audits,
	}, clock, &stubIDGen{}, WithRuntimeResumer(runtime))

	report, err := svc.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(report.WaitingAsync) != 1 || !report.WaitingAsync[0].ResumeTriggered {
		t.Fatalf("waiting async = %#v, want resume triggered", report.WaitingAsync)
	}
	if runtime.sessionID != "sess-terminal" || runtime.taskID != taskID {
		t.Fatalf("runtime resume = %#v, want sess-terminal/%s", runtime, taskID)
	}
}

func TestScanLeavesWaitingAsyncSessionWaitingForCallback(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 16, 15, 0, 0, time.UTC)}
	taskID := "task-running"
	groupID := "group-running"

	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:                      "sess-waiting",
		Title:                   "Waiting Async",
		Status:                  domain.SessionStatusWaitingAsyncExecution,
		CurrentTaskID:           &taskID,
		CurrentExecutionGroupID: &groupID,
		Revision:                2,
		CreatedAt:               clock.Now(),
		UpdatedAt:               clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}
	if err := store.Executions.Create(context.Background(), domain.Execution{
		ID:        "exec-running",
		TaskID:    taskID,
		SessionID: "sess-waiting",
		NodeID:    "node-1",
		Status:    domain.ExecutionStatusRunning,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}); err != nil {
		t.Fatalf("Create(execution) error = %v", err)
	}

	runtime := &stubRuntimeResumer{}
	svc := NewService(Repositories{
		Sessions:   store.Sessions,
		Executions: store.Executions,
		Audits:     store.Audits,
	}, clock, &stubIDGen{}, WithRuntimeResumer(runtime))

	report, err := svc.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(report.WaitingAsync) != 1 || !report.WaitingAsync[0].WaitingForCallback || report.WaitingAsync[0].ResumeTriggered {
		t.Fatalf("waiting async = %#v, want waiting for callback", report.WaitingAsync)
	}
	if runtime.sessionID != "" {
		t.Fatalf("runtime should not resume, got %#v", runtime)
	}
}

type stubIDGen struct {
	values []string
	index  int
}

func (s *stubIDGen) NewID(prefix string) string {
	if s.index >= len(s.values) {
		return prefix + "-overflow"
	}
	value := s.values[s.index]
	s.index++
	return value
}

type stubRuntimeResumer struct {
	sessionID string
	taskID    string
}

func (s *stubRuntimeResumer) HandleExecutionFinished(_ context.Context, sessionID string, taskID string) error {
	s.sessionID = sessionID
	s.taskID = taskID
	return nil
}
