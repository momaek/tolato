package execution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
	"github.com/momaek/tolato/internal/server/infra"
	infralock "github.com/momaek/tolato/internal/server/infra/lock"
	"github.com/momaek/tolato/internal/server/infra/store/memory"
)

func TestCancelTaskWhileWaitingApproval(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 15, 0, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"audit-1", "toolresult-1", "row-1"}}
	taskID := "task-1"

	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:             taskID,
		SessionID:      "sess-1",
		Status:         domain.TaskStatusWaitingApproval,
		ApprovalStatus: domain.ApprovalStatusPending,
		RiskLevel:      domain.RiskLevelHigh,
		CreatedAt:      clock.Now(),
		UpdatedAt:      clock.Now(),
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:     "sess-1",
		Title:  "Session 1",
		Status: domain.SessionStatusPausedWaitApproval,
		PendingAction: &domain.PendingAction{
			Type:    domain.PendingActionTypeApproval,
			Payload: []byte(`{"taskId":"task-1"}`),
		},
		CurrentTaskID: &taskID,
		Revision:      1,
		CreatedAt:     clock.Now(),
		UpdatedAt:     clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	svc := NewService(Repositories{
		Sessions:    store.Sessions,
		Tasks:       store.Tasks,
		Executions:  store.Executions,
		Timelines:   store.Timelines,
		ToolResults: store.ToolResults,
		Audits:      store.Audits,
	}, clock, &idgen)

	if err := svc.CancelTask(context.Background(), "sess-1", taskID, "cancel-1"); err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}

	gotTask, err := store.Tasks.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Get(task) error = %v", err)
	}
	if gotTask.Status != domain.TaskStatusCancelled || gotTask.ApprovalStatus != domain.ApprovalStatusCancelled {
		t.Fatalf("task = %#v, want cancelled/cancelled", gotTask)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusCompleted || gotSession.PendingAction != nil || gotSession.CurrentTaskID != nil {
		t.Fatalf("session = %#v, want completed and cleared", gotSession)
	}
}

func TestCancelTaskRejectsWhenSessionLockIsHeld(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 15, 2, 0, 0, time.UTC)}
	idgen := stubIDGen{}
	locks := infralock.NewMemoryLockManager()
	taskID := "task-lock"

	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:             taskID,
		SessionID:      "sess-lock",
		Status:         domain.TaskStatusWaitingApproval,
		ApprovalStatus: domain.ApprovalStatusPending,
		RiskLevel:      domain.RiskLevelHigh,
		CreatedAt:      clock.Now(),
		UpdatedAt:      clock.Now(),
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:     "sess-lock",
		Title:  "Locked Session",
		Status: domain.SessionStatusPausedWaitApproval,
		PendingAction: &domain.PendingAction{
			Type:    domain.PendingActionTypeApproval,
			Payload: []byte(`{"taskId":"task-lock"}`),
		},
		CurrentTaskID: &taskID,
		Revision:      1,
		CreatedAt:     clock.Now(),
		UpdatedAt:     clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	unlock, err := locks.LockSession(context.Background(), "sess-lock")
	if err != nil {
		t.Fatalf("LockSession() error = %v", err)
	}
	defer unlock()

	svc := NewService(Repositories{
		Sessions:    store.Sessions,
		Tasks:       store.Tasks,
		Executions:  store.Executions,
		Timelines:   store.Timelines,
		ToolResults: store.ToolResults,
		Audits:      store.Audits,
	}, clock, &idgen, WithLockManager(locks))

	err = svc.CancelTask(context.Background(), "sess-lock", taskID, "cancel-lock")
	if !errors.Is(err, domain.ErrSessionBusy) {
		t.Fatalf("CancelTask() error = %v, want ErrSessionBusy", err)
	}
}

func TestStartDispatchCreatesTaskExecutionsAndDispatches(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 55, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"task-1", "execgrp-1", "exec-1", "exec-2"}}
	dispatcher := &stubDispatchPublisher{}

	svc := NewService(Repositories{
		Sessions:    store.Sessions,
		Tasks:       store.Tasks,
		Executions:  store.Executions,
		Timelines:   store.Timelines,
		ToolResults: store.ToolResults,
		Audits:      store.Audits,
	}, clock, &idgen, WithDispatchPublisher(dispatcher))

	result, err := svc.StartDispatch(context.Background(), StartDispatchInput{
		SessionID: "sess-0",
		InputText: "run diagnostics",
		Command:   "uptime",
		TargetContext: domain.ActiveTargetContext{
			Status:       domain.TargetStatusConfirmed,
			Scope:        domain.TargetScopeMulti,
			NodeIDs:      []string{"node-1", "node-2"},
			DisplayLabel: "2 targets",
			Source:       domain.TargetSourceUserExplicit,
		},
		RiskLevel: domain.RiskLevelLow,
	})
	if err != nil {
		t.Fatalf("StartDispatch() error = %v", err)
	}
	if result.TaskID != "task-1" || result.ExecutionGroupID != "execgrp-1" || len(result.ExecutionIDs) != 2 {
		t.Fatalf("result = %#v", result)
	}

	task, err := store.Tasks.Get(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Get(task) error = %v", err)
	}
	if task.Status != domain.TaskStatusDispatched || task.ApprovalStatus != domain.ApprovalStatusNotRequired {
		t.Fatalf("task = %#v, want dispatched/not_required", task)
	}

	executions, err := store.Executions.ListByTask(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("ListByTask(executions) error = %v", err)
	}
	if len(executions) != 2 || executions[0].Status != domain.ExecutionStatusDispatched || executions[1].Status != domain.ExecutionStatusDispatched {
		t.Fatalf("executions = %#v, want 2 dispatched executions", executions)
	}
	if len(dispatcher.commands) != 2 || dispatcher.commands[0].Type != "task.dispatch" {
		t.Fatalf("commands = %#v, want dispatch commands", dispatcher.commands)
	}
	if dispatcher.commands[0].Action != "run_command" {
		t.Fatalf("command action = %q, want run_command", dispatcher.commands[0].Action)
	}
	var args RunCommandArgs
	if err := json.Unmarshal(dispatcher.commands[0].Args, &args); err != nil {
		t.Fatalf("json.Unmarshal(args) error = %v", err)
	}
	if args.Command != "uptime" || len(args.Args) != 0 {
		t.Fatalf("dispatch args = %#v, want explicit command", args)
	}
}

func TestStartDispatchRejectsMissingCommand(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 55, 0, 0, time.UTC)}
	idgen := stubIDGen{}

	svc := NewService(Repositories{
		Sessions:    store.Sessions,
		Tasks:       store.Tasks,
		Executions:  store.Executions,
		Timelines:   store.Timelines,
		ToolResults: store.ToolResults,
		Audits:      store.Audits,
	}, clock, &idgen)

	_, err := svc.StartDispatch(context.Background(), StartDispatchInput{
		SessionID: "sess-0",
		TargetContext: domain.ActiveTargetContext{
			Status:  domain.TargetStatusConfirmed,
			NodeIDs: []string{"node-1"},
		},
		RiskLevel: domain.RiskLevelLow,
	})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("StartDispatch() error = %v, want ErrInvalidArgument", err)
	}
}

func TestRecordChunkMarksExecutionRunningAndPublishesEvent(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 15, 10, 0, 0, time.UTC)}
	events := &stubExecutionEvents{}

	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:             "task-3",
		SessionID:      "sess-3",
		Status:         domain.TaskStatusDispatched,
		ApprovalStatus: domain.ApprovalStatusApproved,
		CreatedAt:      clock.Now(),
		UpdatedAt:      clock.Now(),
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	if err := store.Executions.Create(context.Background(), domain.Execution{
		ID:        "exec-3",
		TaskID:    "task-3",
		SessionID: "sess-3",
		NodeID:    "node-3",
		Status:    domain.ExecutionStatusDispatched,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}); err != nil {
		t.Fatalf("Create(execution) error = %v", err)
	}

	svc := NewService(Repositories{
		Sessions:    store.Sessions,
		Tasks:       store.Tasks,
		Executions:  store.Executions,
		Timelines:   store.Timelines,
		ToolResults: store.ToolResults,
		Audits:      store.Audits,
	}, clock, &stubIDGen{}, WithEventPublisher(events))

	if err := svc.RecordChunk(context.Background(), RecordChunkInput{
		SessionID:   "sess-3",
		TaskID:      "task-3",
		ExecutionID: "exec-3",
		NodeID:      "node-3",
		Chunk: domain.ExecutionChunk{
			Stream: domain.ExecutionStreamStdout,
			Text:   "line-1\n",
		},
	}); err != nil {
		t.Fatalf("RecordChunk() error = %v", err)
	}

	execution, err := store.Executions.Get(context.Background(), "exec-3")
	if err != nil {
		t.Fatalf("Get(execution) error = %v", err)
	}
	if execution.Status != domain.ExecutionStatusRunning || execution.StdoutTail != "line-1\n" {
		t.Fatalf("execution = %#v, want running with stdout tail", execution)
	}
	if len(events.chunks) != 1 || events.chunks[0].Chunk.Text != "line-1\n" {
		t.Fatalf("events = %#v, want chunk event", events.chunks)
	}
}

func TestFinishExecutionAggregatesTaskAndTriggersCompletion(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 15, 15, 0, 0, time.UTC)}
	completion := &stubCompletionHandler{}
	events := &stubExecutionEvents{}

	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:             "task-4",
		SessionID:      "sess-4",
		Status:         domain.TaskStatusRunning,
		ApprovalStatus: domain.ApprovalStatusApproved,
		CreatedAt:      clock.Now(),
		UpdatedAt:      clock.Now(),
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	for _, execution := range []domain.Execution{
		{ID: "exec-4a", TaskID: "task-4", SessionID: "sess-4", NodeID: "node-a", Status: domain.ExecutionStatusSuccess, FinishedAt: timePtr(clock.Now()), CreatedAt: clock.Now(), UpdatedAt: clock.Now()},
		{ID: "exec-4b", TaskID: "task-4", SessionID: "sess-4", NodeID: "node-b", Status: domain.ExecutionStatusRunning, StartedAt: timePtr(clock.Now()), CreatedAt: clock.Now(), UpdatedAt: clock.Now()},
	} {
		if err := store.Executions.Create(context.Background(), execution); err != nil {
			t.Fatalf("Create(execution) error = %v", err)
		}
	}

	svc := NewService(Repositories{
		Sessions:    store.Sessions,
		Tasks:       store.Tasks,
		Executions:  store.Executions,
		Timelines:   store.Timelines,
		ToolResults: store.ToolResults,
		Audits:      store.Audits,
	}, clock, &stubIDGen{}, WithEventPublisher(events), WithCompletionHandler(completion))

	if err := svc.FinishExecution(context.Background(), FinishExecutionInput{
		SessionID:   "sess-4",
		TaskID:      "task-4",
		ExecutionID: "exec-4b",
		NodeID:      "node-b",
		Status:      domain.ExecutionStatusFailed,
		ExitCode:    intPtr(2),
	}); err != nil {
		t.Fatalf("FinishExecution() error = %v", err)
	}

	task, err := store.Tasks.Get(context.Background(), "task-4")
	if err != nil {
		t.Fatalf("Get(task) error = %v", err)
	}
	if task.Status != domain.TaskStatusPartialFailed {
		t.Fatalf("task status = %q, want partial_failed", task.Status)
	}
	if len(completion.calls) != 1 || completion.calls[0].TaskID != "task-4" {
		t.Fatalf("completion = %#v, want task-4 callback", completion.calls)
	}
	if len(events.finished) != 1 || events.finished[0].ExecutionID != "exec-4b" {
		t.Fatalf("finished events = %#v, want exec-4b", events.finished)
	}
}

func TestCancelTaskWhileWaitingAsyncExecution(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 15, 5, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"audit-1", "toolresult-1", "row-1"}}
	taskID := "task-2"
	groupID := "group-2"

	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:             taskID,
		SessionID:      "sess-2",
		Status:         domain.TaskStatusRunning,
		ApprovalStatus: domain.ApprovalStatusApproved,
		RiskLevel:      domain.RiskLevelHigh,
		CreatedAt:      clock.Now(),
		UpdatedAt:      clock.Now(),
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	for _, exec := range []domain.Execution{
		{ID: "exec-1", TaskID: taskID, SessionID: "sess-2", NodeID: "node-1", Status: domain.ExecutionStatusRunning, CreatedAt: clock.Now(), UpdatedAt: clock.Now()},
		{ID: "exec-2", TaskID: taskID, SessionID: "sess-2", NodeID: "node-2", Status: domain.ExecutionStatusQueued, CreatedAt: clock.Now(), UpdatedAt: clock.Now()},
	} {
		if err := store.Executions.Create(context.Background(), exec); err != nil {
			t.Fatalf("Create(execution) error = %v", err)
		}
	}
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:                      "sess-2",
		Title:                   "Session 2",
		Status:                  domain.SessionStatusWaitingAsyncExecution,
		CurrentTaskID:           &taskID,
		CurrentExecutionGroupID: &groupID,
		Revision:                4,
		CreatedAt:               clock.Now(),
		UpdatedAt:               clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	svc := NewService(Repositories{
		Sessions:    store.Sessions,
		Tasks:       store.Tasks,
		Executions:  store.Executions,
		Timelines:   store.Timelines,
		ToolResults: store.ToolResults,
		Audits:      store.Audits,
	}, clock, &idgen)

	if err := svc.CancelTask(context.Background(), "sess-2", taskID, "cancel-2"); err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}

	gotTask, err := store.Tasks.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Get(task) error = %v", err)
	}
	if gotTask.Status != domain.TaskStatusCancelled {
		t.Fatalf("task status = %q, want cancelled", gotTask.Status)
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

type stubDispatchPublisher struct {
	commands []DispatchCommand
}

func (s *stubDispatchPublisher) DispatchToNode(ctx context.Context, nodeID string, cmd DispatchCommand) error {
	_ = ctx
	if nodeID != cmd.NodeID {
		return domain.ErrInvalidArgument
	}
	s.commands = append(s.commands, cmd)
	return nil
}

func (s *stubDispatchPublisher) SendShellInput(ctx context.Context, nodeID string, executionID string, data string) error {
	return nil
}

func (s *stubDispatchPublisher) SendShellResize(ctx context.Context, nodeID string, executionID string, rows, cols int) error {
	return nil
}

type stubCompletionHandler struct {
	calls []struct {
		SessionID string
		TaskID    string
	}
}

func (s *stubCompletionHandler) HandleExecutionFinished(ctx context.Context, sessionID string, taskID string) error {
	_ = ctx
	s.calls = append(s.calls, struct {
		SessionID string
		TaskID    string
	}{SessionID: sessionID, TaskID: taskID})
	return nil
}

type stubExecutionEvents struct {
	chunks []struct {
		SessionID   string
		TaskID      string
		ExecutionID string
		Chunk       domain.ExecutionChunk
	}
	finished []struct {
		SessionID   string
		TaskID      string
		ExecutionID string
		Status      domain.ExecutionStatus
	}
}

func (s *stubExecutionEvents) SessionStateUpdated(ctx context.Context, session domain.Session) error {
	_ = ctx
	_ = session
	return nil
}

func (s *stubExecutionEvents) TimelineRowAppended(ctx context.Context, session domain.Session, row domain.TimelineRow) error {
	_ = ctx
	_ = session
	_ = row
	return nil
}

func (s *stubExecutionEvents) ExecutionChunk(ctx context.Context, sessionID string, taskID string, execution domain.Execution, chunk domain.ExecutionChunk) error {
	_ = ctx
	s.chunks = append(s.chunks, struct {
		SessionID   string
		TaskID      string
		ExecutionID string
		Chunk       domain.ExecutionChunk
	}{SessionID: sessionID, TaskID: taskID, ExecutionID: execution.ID, Chunk: chunk})
	return nil
}

func (s *stubExecutionEvents) ExecutionFinished(ctx context.Context, sessionID string, taskID string, execution domain.Execution) error {
	_ = ctx
	s.finished = append(s.finished, struct {
		SessionID   string
		TaskID      string
		ExecutionID string
		Status      domain.ExecutionStatus
	}{SessionID: sessionID, TaskID: taskID, ExecutionID: execution.ID, Status: execution.Status})
	return nil
}

func intPtr(v int) *int { return &v }
