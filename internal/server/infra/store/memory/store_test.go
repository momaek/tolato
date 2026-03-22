package memory

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

func TestSessionRepositoryCopiesAndLists(t *testing.T) {
	repo := NewSessionRepository()
	now := time.Unix(100, 0).UTC()
	currentTaskID := "task-1"
	pending := &domain.PendingAction{
		Type:    domain.PendingActionTypeApproval,
		Payload: json.RawMessage(`{"approve":true}`),
	}
	session := domain.Session{
		ID:                "sess-1",
		Title:             "title",
		Status:            domain.SessionStatusRunning,
		PendingAction:     pending,
		CurrentTaskID:     &currentTaskID,
		LastAgentState:    json.RawMessage(`{"step":1}`),
		ProviderStateBlob: json.RawMessage(`{"provider":"x"}`),
		Revision:          3,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := repo.Create(context.Background(), session); err != nil {
		t.Fatalf("create session: %v", err)
	}

	got, err := repo.Get(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	if got.ID != session.ID || got.Status != session.Status {
		t.Fatalf("unexpected session: %#v", got)
	}

	got.Title = "mutated"
	got.PendingAction.Payload[0] = '{'

	again, err := repo.Get(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("get session again: %v", err)
	}
	if again.Title != "title" {
		t.Fatalf("store was mutated through read copy")
	}
	if string(again.PendingAction.Payload) != `{"approve":true}` {
		t.Fatalf("pending action payload was mutated: %s", again.PendingAction.Payload)
	}

	session.Title = "updated"
	session.Status = domain.SessionStatusPausedWaitApproval
	session.Revision = 4
	if err := repo.Update(context.Background(), session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	list, err := repo.List(context.Background(), domain.SessionFilter{Statuses: []domain.SessionStatus{domain.SessionStatusPausedWaitApproval}})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(list) != 1 || list[0].Title != "updated" {
		t.Fatalf("unexpected list result: %#v", list)
	}
}

func TestThreadMessageAndTimelineCursorReads(t *testing.T) {
	msgRepo := NewThreadMessageRepository()
	timelineRepo := NewTimelineRepository()
	base := time.Unix(200, 0).UTC()

	for i := 1; i <= 4; i++ {
		id := string(rune('a' + i - 1))
		msg := domain.ThreadMessage{
			ID:        id,
			SessionID: "sess-1",
			Role:      domain.MessageRoleUser,
			Kind:      domain.ThreadMessageKindUserMessage,
			Content:   id,
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}
		if err := msgRepo.Append(context.Background(), msg); err != nil {
			t.Fatalf("append message %d: %v", i, err)
		}
		row := domain.TimelineRow{
			ID:        id,
			SessionID: "sess-1",
			Kind:      domain.TimelineRowKindUserMessage,
			Text:      id,
			CreatedAt: msg.CreatedAt,
		}
		if err := timelineRepo.Append(context.Background(), row); err != nil {
			t.Fatalf("append row %d: %v", i, err)
		}
	}

	msgs, err := msgRepo.ListBySession(context.Background(), "sess-1", domain.CursorPage{Limit: 2})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 || msgs[0].ID != "c" || msgs[1].ID != "d" {
		t.Fatalf("unexpected messages: %#v", msgs)
	}

	olderMsgs, err := msgRepo.ListBySession(context.Background(), "sess-1", domain.CursorPage{BeforeID: "d", Limit: 2})
	if err != nil {
		t.Fatalf("list older messages: %v", err)
	}
	if len(olderMsgs) != 2 || olderMsgs[0].ID != "b" || olderMsgs[1].ID != "c" {
		t.Fatalf("unexpected older messages: %#v", olderMsgs)
	}

	rows, err := timelineRepo.ListBySession(context.Background(), "sess-1", domain.CursorPage{BeforeID: "d", Limit: 3})
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	if len(rows) != 3 || rows[0].ID != "a" || rows[2].ID != "c" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}

func TestTaskExecutionAggregateAndAuditSettings(t *testing.T) {
	taskRepo := NewTaskRepository()
	execRepo := NewExecutionRepository()
	auditRepo := NewAuditRepository()
	settingsRepo := NewSettingsRepository()
	now := time.Unix(300, 0).UTC()
	summary := "done"
	task := domain.Task{
		ID:        "task-1",
		SessionID: "sess-1",
		InputText: "check",
		OperationTargetSnapshot: domain.TargetSnapshot{
			Scope:        domain.TargetScopeMulti,
			NodeIDs:      []string{"n1", "n2"},
			DisplayLabel: "nodes",
			Source:       domain.TargetSourceUserExplicit,
			Confirmed:    true,
			CapturedAt:   now,
		},
		Status:         domain.TaskStatusPlanned,
		ApprovalStatus: domain.ApprovalStatusPending,
		RiskLevel:      domain.RiskLevelMedium,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	task.Status = domain.TaskStatusApproved
	task.Summary = &summary
	if err := taskRepo.Update(context.Background(), task); err != nil {
		t.Fatalf("update task: %v", err)
	}
	gotTask, err := taskRepo.Get(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if gotTask.Status != domain.TaskStatusApproved || gotTask.Summary == nil || *gotTask.Summary != "done" {
		t.Fatalf("unexpected task: %#v", gotTask)
	}

	queued := now.Add(time.Second)
	running := now.Add(2 * time.Second)
	success := now.Add(3 * time.Second)
	timeout := now.Add(4 * time.Second)
	execs := []domain.Execution{
		{ID: "e1", TaskID: "task-1", SessionID: "sess-1", NodeID: "n1", Status: domain.ExecutionStatusQueued, CreatedAt: queued, UpdatedAt: queued},
		{ID: "e2", TaskID: "task-1", SessionID: "sess-1", NodeID: "n2", Status: domain.ExecutionStatusRunning, CreatedAt: running, UpdatedAt: running},
		{ID: "e3", TaskID: "task-1", SessionID: "sess-1", NodeID: "n3", Status: domain.ExecutionStatusSuccess, CreatedAt: success, UpdatedAt: success},
		{ID: "e4", TaskID: "task-1", SessionID: "sess-1", NodeID: "n4", Status: domain.ExecutionStatusTimeout, CreatedAt: timeout, UpdatedAt: timeout},
	}
	for _, exec := range execs {
		if err := execRepo.Create(context.Background(), exec); err != nil {
			t.Fatalf("create execution %s: %v", exec.ID, err)
		}
	}

	aggregate, err := execRepo.AggregateByTask(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if aggregate.Total != 4 || aggregate.Queued != 1 || aggregate.Running != 1 || aggregate.Success != 1 || aggregate.Timeout != 1 {
		t.Fatalf("unexpected aggregate: %#v", aggregate)
	}

	taskID := "task-1"
	record := domain.AuditRecord{
		ID:        "audit-1",
		SessionID: "sess-1",
		TaskID:    &taskID,
		ActorID:   "user-1",
		EventType: "approve",
		Payload:   json.RawMessage(`{"ok":true}`),
		CreatedAt: now,
	}
	if err := auditRepo.Append(context.Background(), record); err != nil {
		t.Fatalf("append audit: %v", err)
	}
	audits, err := auditRepo.ListByTask(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(audits) != 1 || audits[0].EventType != "approve" {
		t.Fatalf("unexpected audits: %#v", audits)
	}

	if err := settingsRepo.Put(context.Background(), domain.SettingRecord{
		UserID:    "user-1",
		Key:       domain.SettingKeyModelConfig,
		Value:     json.RawMessage(`{"provider":"openai"}`),
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("put setting: %v", err)
	}
	if err := settingsRepo.Put(context.Background(), domain.SettingRecord{
		UserID:    "user-1",
		Key:       domain.SettingKeyPreferences,
		Value:     json.RawMessage(`{"theme":"dark"}`),
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("put setting 2: %v", err)
	}

	settings, err := settingsRepo.ListByUser(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("list settings: %v", err)
	}
	if len(settings) != 2 {
		t.Fatalf("unexpected settings: %#v", settings)
	}

	gotSetting, err := settingsRepo.Get(context.Background(), "user-1", domain.SettingKeyPreferences)
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if string(gotSetting.Value) != `{"theme":"dark"}` {
		t.Fatalf("unexpected setting value: %s", gotSetting.Value)
	}
}

func TestConcurrentMessageAppends(t *testing.T) {
	repo := NewThreadMessageRepository()
	const count = 64

	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a'+(i%26))) + string(rune('a'+((i/26)%26)))
			_ = repo.Append(context.Background(), domain.ThreadMessage{
				ID:        id + string(rune('0'+rune(i%10))),
				SessionID: "sess-1",
				Role:      domain.MessageRoleUser,
				Kind:      domain.ThreadMessageKindUserMessage,
				Content:   "x",
				CreatedAt: time.Unix(int64(i), 0).UTC(),
			})
		}(i)
	}
	wg.Wait()

	msgs, err := repo.ListBySession(context.Background(), "sess-1", domain.CursorPage{})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != count {
		t.Fatalf("unexpected message count: %d", len(msgs))
	}
}
