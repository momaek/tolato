package session

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
	"github.com/momaek/tolato/internal/server/infra"
	"github.com/momaek/tolato/internal/server/infra/store/memory"
)

func TestCreateAndDeleteSession(t *testing.T) {
	store := memory.NewStore()
	svc := NewService(Repositories{
		Sessions:   store.Sessions,
		Timelines:  store.Timelines,
		Tasks:      store.Tasks,
		Executions: store.Executions,
	}, WithClock(infra.FixedClock{Time: time.Date(2026, 3, 22, 18, 0, 0, 0, time.UTC)}), WithIDGenerator(stubIDGen{id: "sess-new"}))

	sessionID, err := svc.CreateSession(context.Background(), "")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if sessionID != "sess-new" {
		t.Fatalf("session id = %q, want sess-new", sessionID)
	}

	created, err := store.Sessions.Get(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Get(created) error = %v", err)
	}
	if created.Title != "Console Session" {
		t.Fatalf("created title = %q, want Console Session", created.Title)
	}
	if created.Status != domain.SessionStatusIdle || created.Revision != 1 {
		t.Fatalf("created session = %#v, want idle revision 1", created)
	}

	if err := svc.DeleteSession(context.Background(), sessionID); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if _, err := store.Sessions.Get(context.Background(), sessionID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get(deleted) error = %v, want not found", err)
	}
}

func TestDeleteSessionRejectsRunningSession(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 22, 18, 10, 0, 0, time.UTC)
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:        "sess-running",
		Title:     "Running",
		Status:    domain.SessionStatusRunning,
		Revision:  2,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	svc := NewService(Repositories{
		Sessions:   store.Sessions,
		Timelines:  store.Timelines,
		Tasks:      store.Tasks,
		Executions: store.Executions,
	})

	err := svc.DeleteSession(context.Background(), "sess-running")
	if !errors.Is(err, domain.ErrSessionBusy) {
		t.Fatalf("DeleteSession() error = %v, want session busy", err)
	}
}

func TestBuildSnapshot(t *testing.T) {
	store := memory.NewStore()
	svc := NewService(Repositories{
		Sessions:   store.Sessions,
		Timelines:  store.Timelines,
		Tasks:      store.Tasks,
		Executions: store.Executions,
	})

	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	taskID := "task-1"
	groupID := "group-1"
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:     "sess-1",
		Title:  "Tokyo Session",
		Status: domain.SessionStatusWaitingAsyncExecution,
		ActiveTargetContext: domain.ActiveTargetContext{
			Status:       domain.TargetStatusConfirmed,
			Scope:        domain.TargetScopeSingle,
			NodeIDs:      []string{"jp-tokyo-01"},
			DisplayLabel: "jp-tokyo-01",
			Source:       domain.TargetSourceUserExplicit,
			Confidence:   1,
		},
		PendingAction:           nil,
		CurrentTaskID:           &taskID,
		CurrentExecutionGroupID: &groupID,
		Revision:                9,
		CreatedAt:               now,
		UpdatedAt:               now,
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}
	if err := store.Timelines.Append(context.Background(), domain.TimelineRow{
		ID:        "row-1",
		SessionID: "sess-1",
		Kind:      domain.TimelineRowKindUserMessage,
		Text:      "check nginx",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("Append(row-1) error = %v", err)
	}
	if err := store.Timelines.Append(context.Background(), domain.TimelineRow{
		ID:        "row-2",
		SessionID: "sess-1",
		Kind:      domain.TimelineRowKindExecution,
		Text:      "execution started",
		CreatedAt: now.Add(time.Second),
		TaskID:    &taskID,
	}); err != nil {
		t.Fatalf("Append(row-2) error = %v", err)
	}
	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:        taskID,
		SessionID: "sess-1",
		InputText: "check nginx",
		OperationTargetSnapshot: domain.TargetSnapshot{
			Scope:        domain.TargetScopeSingle,
			NodeIDs:      []string{"jp-tokyo-01"},
			DisplayLabel: "jp-tokyo-01",
			Source:       domain.TargetSourceUserExplicit,
			Confirmed:    true,
			CapturedAt:   now,
		},
		Status:         domain.TaskStatusRunning,
		ApprovalStatus: domain.ApprovalStatusNotRequired,
		RiskLevel:      domain.RiskLevelLow,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	if err := store.Executions.Create(context.Background(), domain.Execution{
		ID:        "exec-1",
		TaskID:    taskID,
		SessionID: "sess-1",
		NodeID:    "jp-tokyo-01",
		Status:    domain.ExecutionStatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create(exec-1) error = %v", err)
	}

	snapshot, err := svc.BuildSnapshot(context.Background(), "client-1", "sess-1")
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}

	if snapshot.Session.Revision != 9 {
		t.Fatalf("snapshot revision = %d, want 9", snapshot.Session.Revision)
	}
	if snapshot.HeaderState.ActiveTargetLabel != "Confirmed target: jp-tokyo-01" {
		t.Fatalf("active target label = %q", snapshot.HeaderState.ActiveTargetLabel)
	}
	if !snapshot.ComposerState.Disabled {
		t.Fatal("composer should be disabled while waiting async execution")
	}
	if snapshot.ExecutionState == nil || snapshot.ExecutionState.Aggregate == nil || snapshot.ExecutionState.Aggregate.Running != 1 {
		t.Fatalf("execution state = %#v, want running aggregate", snapshot.ExecutionState)
	}
	if len(snapshot.Timeline.Rows) != 2 {
		t.Fatalf("timeline rows = %d, want 2", len(snapshot.Timeline.Rows))
	}
}

func TestBuildSnapshotPendingActionAndListRows(t *testing.T) {
	store := memory.NewStore()
	svc := NewService(Repositories{
		Sessions:   store.Sessions,
		Timelines:  store.Timelines,
		Tasks:      store.Tasks,
		Executions: store.Executions,
	})

	now := time.Date(2026, 3, 22, 13, 0, 0, 0, time.UTC)
	payload := json.RawMessage(`{"taskId":"task-9"}`)
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:     "sess-2",
		Title:  "Review Session",
		Status: domain.SessionStatusPausedWaitApproval,
		PendingAction: &domain.PendingAction{
			Type:    domain.PendingActionTypeApproval,
			Payload: payload,
		},
		Revision:  3,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}
	for i, id := range []string{"row-a", "row-b", "row-c"} {
		if err := store.Timelines.Append(context.Background(), domain.TimelineRow{
			ID:        id,
			SessionID: "sess-2",
			Kind:      domain.TimelineRowKindAssistantText,
			Text:      id,
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("Append(%s) error = %v", id, err)
		}
	}

	snapshot, err := svc.BuildSnapshot(context.Background(), "client-1", "sess-2")
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	if snapshot.PendingAction == nil || snapshot.PendingAction.TaskID == nil || *snapshot.PendingAction.TaskID != "task-9" {
		t.Fatalf("pending action = %#v, want task-9", snapshot.PendingAction)
	}
	if !snapshot.ComposerState.Disabled {
		t.Fatal("composer should be disabled while waiting approval")
	}

	page, err := svc.ListRows(context.Background(), "sess-2", domain.CursorPage{Limit: 2})
	if err != nil {
		t.Fatalf("ListRows() error = %v", err)
	}
	if !page.HasMoreBefore || page.NextBeforeCursor == nil || *page.NextBeforeCursor != "row-b" {
		t.Fatalf("timeline page = %#v, want more-before cursor row-b", page)
	}
	if len(page.Rows) != 2 || page.Rows[0].ID != "row-b" || page.Rows[1].ID != "row-c" {
		t.Fatalf("rows = %#v, want [row-b row-c]", page.Rows)
	}
}

func TestUpdateSubscriptions(t *testing.T) {
	reg := &stubSubscriptions{}
	store := memory.NewStore()
	svc := NewService(Repositories{
		Sessions:      store.Sessions,
		Timelines:     store.Timelines,
		Tasks:         store.Tasks,
		Executions:    store.Executions,
		Subscriptions: reg,
	})

	err := svc.UpdateSubscriptions(context.Background(), "client-1", "sess-a", []string{"sess-b", "sess-c"})
	if err != nil {
		t.Fatalf("UpdateSubscriptions() error = %v", err)
	}
	if reg.clientID != "client-1" || reg.activeSession != "sess-a" {
		t.Fatalf("registry state = %#v", reg)
	}
	if len(reg.watchSessions) != 2 || reg.watchSessions[0] != "sess-b" || reg.watchSessions[1] != "sess-c" {
		t.Fatalf("watch sessions = %#v, want sess-b/sess-c", reg.watchSessions)
	}
}

func TestListSessionsIncludesUnreadCounts(t *testing.T) {
	reg := &stubSubscriptions{
		unreadBySession: map[string]int{
			"sess-a": 2,
			"sess-b": 1,
		},
	}
	store := memory.NewStore()
	now := time.Date(2026, 3, 22, 14, 0, 0, 0, time.UTC)
	for _, session := range []domain.Session{
		{ID: "sess-a", Title: "A", Status: domain.SessionStatusRunning, CreatedAt: now, UpdatedAt: now},
		{ID: "sess-b", Title: "B", Status: domain.SessionStatusCompleted, CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.Sessions.Create(context.Background(), session); err != nil {
			t.Fatalf("Create(session %s) error = %v", session.ID, err)
		}
	}

	svc := NewService(Repositories{
		Sessions:      store.Sessions,
		Timelines:     store.Timelines,
		Tasks:         store.Tasks,
		Executions:    store.Executions,
		Subscriptions: reg,
	})

	items, err := svc.ListSessions(context.Background(), "client-7")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v, want 2 sessions", items)
	}
	if items[0].Unread != 2 || items[1].Unread != 1 {
		t.Fatalf("unread counts = %#v, want [2 1]", items)
	}
}

func TestBuildSnapshotClearsUnread(t *testing.T) {
	reg := &stubSubscriptions{
		unreadBySession: map[string]int{"sess-clear": 3},
	}
	store := memory.NewStore()
	now := time.Date(2026, 3, 22, 14, 30, 0, 0, time.UTC)
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:        "sess-clear",
		Title:     "Clear",
		Status:    domain.SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	svc := NewService(Repositories{
		Sessions:      store.Sessions,
		Timelines:     store.Timelines,
		Tasks:         store.Tasks,
		Executions:    store.Executions,
		Subscriptions: reg,
	})

	if _, err := svc.BuildSnapshot(context.Background(), "client-9", "sess-clear"); err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}
	if reg.clearedClientID != "client-9" || reg.clearedSessionID != "sess-clear" {
		t.Fatalf("clear unread call = %#v, want client-9/sess-clear", reg)
	}
}

type stubSubscriptions struct {
	clientID         string
	activeSession    string
	watchSessions    []string
	clearedClientID  string
	clearedSessionID string
	unreadBySession  map[string]int
}

type stubIDGen struct {
	id string
}

func (s stubIDGen) NewID(prefix string) string {
	if s.id != "" {
		return s.id
	}
	return prefix + "-generated"
}

func (s *stubSubscriptions) SetActive(clientID string, sessionID string) {
	s.clientID = clientID
	s.activeSession = sessionID
}

func (s *stubSubscriptions) SetWatchSessions(clientID string, sessionIDs []string) {
	s.clientID = clientID
	s.watchSessions = append([]string(nil), sessionIDs...)
}

func (s *stubSubscriptions) ClearUnread(clientID string, sessionID string) (int, bool) {
	s.clearedClientID = clientID
	s.clearedSessionID = sessionID
	if s.unreadBySession == nil {
		return 0, false
	}
	if _, ok := s.unreadBySession[sessionID]; !ok {
		return 0, false
	}
	delete(s.unreadBySession, sessionID)
	return 0, true
}

func (s *stubSubscriptions) UnreadCount(clientID string, sessionID string) int {
	s.clientID = clientID
	if s.unreadBySession == nil {
		return 0
	}
	return s.unreadBySession[sessionID]
}
