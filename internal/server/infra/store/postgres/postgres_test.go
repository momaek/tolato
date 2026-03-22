package postgres

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/momaek/tolato/internal/server/domain"
)

func TestSessionRepositoryGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 3, 22, 9, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "title", "status", "active_target_context", "pending_action_type", "pending_action_payload",
		"current_operation_id", "current_task_id", "current_execution_group_id", "last_agent_state",
		"provider_state_blob", "revision", "created_at", "updated_at",
	}).AddRow(
		"sess-1", "Test Session", "running", `{"status":"confirmed","scope":"single","nodeIds":["node-1"],"displayLabel":"node-1","source":"user_explicit","confidence":1}`,
		"approval", `{"taskId":"task-1"}`, "op-1", "task-1", "group-1", `{"step":"resume"}`, `{"provider":"openai"}`, int64(7), now, now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, title, status, active_target_context, pending_action_type, pending_action_payload,
       current_operation_id, current_task_id, current_execution_group_id, last_agent_state,
       provider_state_blob, revision, created_at, updated_at
FROM sessions
WHERE id = $1
`)).WithArgs("sess-1").WillReturnRows(rows)

	repo := NewSessionRepository(SQLDB{DB: db})
	got, err := repo.Get(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.ID != "sess-1" || got.Status != domain.SessionStatusRunning {
		t.Fatalf("unexpected session = %#v", got)
	}
	if got.PendingAction == nil || got.PendingAction.Type != domain.PendingActionTypeApproval {
		t.Fatalf("pending action = %#v, want approval", got.PendingAction)
	}
	if got.CurrentTaskID == nil || *got.CurrentTaskID != "task-1" {
		t.Fatalf("current task = %#v, want task-1", got.CurrentTaskID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestToolResultRepositoryListBySession(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	base := time.Date(2026, 3, 22, 9, 30, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "session_id", "task_id", "tool_call_id", "tool_name", "status", "text", "source", "payload", "created_at",
	}).AddRow(
		"res-1", "sess-1", "task-1", "call-1", "list_nodes", "succeeded", "ok", "agent_loop", `{"count":3}`, base,
	).AddRow(
		"res-2", "sess-1", nil, nil, "approval", "succeeded", "approved", "user_action", `{"approved":true}`, base.Add(time.Second),
	)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, session_id, task_id, tool_call_id, tool_name, status, text, source, payload, created_at
FROM tool_results
WHERE session_id = $1
ORDER BY created_at ASC, id ASC
`)).WithArgs("sess-1").WillReturnRows(rows)

	repo := NewToolResultRepository(SQLDB{DB: db})
	got, err := repo.ListBySession(context.Background(), "sess-1", domain.CursorPage{Limit: 10})
	if err != nil {
		t.Fatalf("ListBySession() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(got))
	}
	if got[1].Source != domain.TimelineRowSourceUserAction {
		t.Fatalf("result[1].Source = %q, want %q", got[1].Source, domain.TimelineRowSourceUserAction)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSettingsRepositoryPut(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO settings (user_id, key, value, updated_at)
VALUES ($1,$2,$3,$4)
ON CONFLICT (user_id, key)
DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at
`)).WithArgs("user-1", "preferences", []byte(`{"language":"zh-CN"}`), now).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewSettingsRepository(SQLDB{DB: db})
	err = repo.Put(context.Background(), domain.SettingRecord{
		UserID:    "user-1",
		Key:       domain.SettingKeyPreferences,
		Value:     []byte(`{"language":"zh-CN"}`),
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestInitialMigrationContainsFactTables(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "db", "migrations", "0001_initial.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	text := string(raw)
	required := []string{
		"CREATE TABLE IF NOT EXISTS sessions",
		"CREATE TABLE IF NOT EXISTS thread_messages",
		"CREATE TABLE IF NOT EXISTS timeline_rows",
		"CREATE TABLE IF NOT EXISTS tool_calls",
		"CREATE TABLE IF NOT EXISTS tool_results",
		"CREATE TABLE IF NOT EXISTS tasks",
		"CREATE TABLE IF NOT EXISTS executions",
		"CREATE TABLE IF NOT EXISTS audits",
		"CREATE TABLE IF NOT EXISTS settings",
		"CREATE TABLE IF NOT EXISTS agent_provider_state",
	}

	for _, fragment := range required {
		if !strings.Contains(text, fragment) {
			t.Fatalf("migration missing fragment %q", fragment)
		}
	}
}
