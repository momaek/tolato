package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

func TestPostgresRepositoriesRoundTrip(t *testing.T) {
	q := newFakeQuerier()
	store := NewStore(q)
	ctx := context.Background()
	now := time.Unix(1000, 0).UTC()

	currentTaskID := "task-1"
	currentExecutionGroupID := "group-1"
	session := domain.Session{
		ID:     "sess-1",
		Title:  "session",
		Status: domain.SessionStatusRunning,
		ActiveTargetContext: domain.ActiveTargetContext{
			Status:       domain.TargetStatusConfirmed,
			Scope:        domain.TargetScopeSingle,
			NodeIDs:      []string{"node-1"},
			DisplayLabel: "node-1",
			Source:       domain.TargetSourceUserExplicit,
			Confidence:   0.99,
		},
		CurrentTaskID:           &currentTaskID,
		CurrentExecutionGroupID: &currentExecutionGroupID,
		LastAgentState:          json.RawMessage(`{"step":1}`),
		ProviderStateBlob:       json.RawMessage(`{"provider":"openai"}`),
		Revision:                3,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := store.Sessions.Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}

	gotSession, err := store.Sessions.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if gotSession.Title != session.Title || gotSession.Status != session.Status {
		t.Fatalf("unexpected session: %#v", gotSession)
	}

	if err := store.ThreadMessages.Append(ctx, domain.ThreadMessage{
		ID:        "msg-1",
		SessionID: session.ID,
		Role:      domain.MessageRoleUser,
		Kind:      domain.ThreadMessageKindUserMessage,
		Content:   "hello",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}
	if err := store.Timelines.Append(ctx, domain.TimelineRow{
		ID:        "row-1",
		SessionID: session.ID,
		Kind:      domain.TimelineRowKindUserMessage,
		CreatedAt: now,
		Text:      "hello",
		Source:    domain.TimelineRowSourceUserAction,
	}); err != nil {
		t.Fatalf("append timeline: %v", err)
	}
	if err := store.ToolCalls.Append(ctx, domain.ToolCall{
		ID:          "call-1",
		SessionID:   session.ID,
		TaskID:      &currentTaskID,
		MessageID:   stringPtr("msg-1"),
		ToolName:    "list_nodes",
		Arguments:   json.RawMessage(`{"q":"all"}`),
		ArgsPreview: stringPtr(`{"q":"all"}`),
		Source:      domain.ToolCallSourceAgentLoop,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("append tool call: %v", err)
	}
	if err := store.ToolResults.Append(ctx, domain.ToolResult{
		ID:         "result-1",
		SessionID:  session.ID,
		TaskID:     &currentTaskID,
		ToolCallID: stringPtr("call-1"),
		ToolName:   "list_nodes",
		Status:     domain.ToolResultStatusSucceeded,
		Text:       "ok",
		Source:     domain.TimelineRowSourceAgentLoop,
		Payload:    json.RawMessage(`{"items":1}`),
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("append tool result: %v", err)
	}
	if err := store.Tasks.Create(ctx, domain.Task{
		ID:        currentTaskID,
		SessionID: session.ID,
		InputText: "check",
		OperationTargetSnapshot: domain.TargetSnapshot{
			Scope:        domain.TargetScopeSingle,
			NodeIDs:      []string{"node-1"},
			DisplayLabel: "node-1",
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
		t.Fatalf("create task: %v", err)
	}
	if err := store.Executions.Create(ctx, domain.Execution{
		ID:         "exec-1",
		TaskID:     currentTaskID,
		SessionID:  session.ID,
		NodeID:     "node-1",
		Status:     domain.ExecutionStatusSuccess,
		StdoutTail: "done",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("create execution: %v", err)
	}
	if err := store.Audits.Append(ctx, domain.AuditRecord{
		ID:        "audit-1",
		SessionID: session.ID,
		TaskID:    &currentTaskID,
		ActorID:   "user-1",
		EventType: "approve",
		Payload:   json.RawMessage(`{"ok":true}`),
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("append audit: %v", err)
	}
	if err := store.Settings.Put(ctx, domain.SettingRecord{
		UserID:    "user-1",
		Key:       domain.SettingKeyPreferences,
		Value:     json.RawMessage(`{"theme":"dark"}`),
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("put setting: %v", err)
	}
	if err := store.AgentProviderStates.Append(ctx, domain.AgentProviderState{
		ID:        "state-1",
		SessionID: session.ID,
		Version:   1,
		Payload:   json.RawMessage(`{"cursor":"abc"}`),
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("append provider state: %v", err)
	}

	sessions, err := store.Sessions.List(ctx, domain.SessionFilter{Statuses: []domain.SessionStatus{domain.SessionStatusRunning}})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}

	msgs, err := store.ThreadMessages.ListBySession(ctx, session.ID, domain.CursorPage{Limit: 1})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Fatalf("unexpected messages: %#v", msgs)
	}

	rows, err := store.Timelines.ListBySession(ctx, session.ID, domain.CursorPage{Limit: 1})
	if err != nil {
		t.Fatalf("list timeline: %v", err)
	}
	if len(rows) != 1 || rows[0].Text != "hello" {
		t.Fatalf("unexpected timeline rows: %#v", rows)
	}

	calls, err := store.ToolCalls.ListBySession(ctx, session.ID, domain.CursorPage{Limit: 1})
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(calls) != 1 || calls[0].ToolName != "list_nodes" {
		t.Fatalf("unexpected tool calls: %#v", calls)
	}

	results, err := store.ToolResults.ListByTask(ctx, currentTaskID)
	if err != nil {
		t.Fatalf("list tool results: %v", err)
	}
	if len(results) != 1 || results[0].Text != "ok" {
		t.Fatalf("unexpected tool results: %#v", results)
	}

	task, err := store.Tasks.Get(ctx, currentTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.InputText != "check" {
		t.Fatalf("unexpected task: %#v", task)
	}

	executionAgg, err := store.Executions.AggregateByTask(ctx, currentTaskID)
	if err != nil {
		t.Fatalf("aggregate executions: %v", err)
	}
	if executionAgg.Total != 1 || executionAgg.Success != 1 {
		t.Fatalf("unexpected aggregate: %#v", executionAgg)
	}

	latestState, err := store.AgentProviderStates.LatestBySession(ctx, session.ID)
	if err != nil {
		t.Fatalf("latest provider state: %v", err)
	}
	if latestState.Version != 1 {
		t.Fatalf("unexpected provider state: %#v", latestState)
	}
}

type fakeQuerier struct {
	sessions       map[string]domain.Session
	threadMessages map[string][]domain.ThreadMessage
	timelineRows   map[string][]domain.TimelineRow
	toolCalls      map[string][]domain.ToolCall
	toolResults    map[string][]domain.ToolResult
	tasks          map[string]domain.Task
	executions     map[string]domain.Execution
	audits         map[string][]domain.AuditRecord
	settings       map[string]map[string]domain.SettingRecord
	providerStates map[string][]domain.AgentProviderState
}

func newFakeQuerier() *fakeQuerier {
	return &fakeQuerier{
		sessions:       map[string]domain.Session{},
		threadMessages: map[string][]domain.ThreadMessage{},
		timelineRows:   map[string][]domain.TimelineRow{},
		toolCalls:      map[string][]domain.ToolCall{},
		toolResults:    map[string][]domain.ToolResult{},
		tasks:          map[string]domain.Task{},
		executions:     map[string]domain.Execution{},
		audits:         map[string][]domain.AuditRecord{},
		settings:       map[string]map[string]domain.SettingRecord{},
		providerStates: map[string][]domain.AgentProviderState{},
	}
}

func (q *fakeQuerier) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	switch {
	case strings.Contains(query, "INSERT INTO sessions"):
		session := sessionFromArgs(args)
		q.sessions[session.ID] = session
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "UPDATE sessions SET"):
		session := sessionFromArgs(args)
		if _, ok := q.sessions[session.ID]; !ok {
			return driver.RowsAffected(0), nil
		}
		q.sessions[session.ID] = session
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "INSERT INTO thread_messages"):
		message := threadMessageFromArgs(args)
		q.threadMessages[message.SessionID] = append(q.threadMessages[message.SessionID], message)
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "INSERT INTO timeline_rows"):
		row := timelineRowFromArgs(args)
		q.timelineRows[row.SessionID] = append(q.timelineRows[row.SessionID], row)
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "INSERT INTO tool_calls"):
		call := toolCallFromArgs(args)
		q.toolCalls[call.SessionID] = append(q.toolCalls[call.SessionID], call)
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "INSERT INTO tool_results"):
		result := toolResultFromArgs(args)
		q.toolResults[result.SessionID] = append(q.toolResults[result.SessionID], result)
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "INSERT INTO tasks"):
		task := taskFromArgs(args)
		q.tasks[task.ID] = task
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "UPDATE tasks SET"):
		task := taskFromArgs(args)
		if _, ok := q.tasks[task.ID]; !ok {
			return driver.RowsAffected(0), nil
		}
		q.tasks[task.ID] = task
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "INSERT INTO executions"):
		execution := executionFromArgs(args)
		q.executions[execution.ID] = execution
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "UPDATE executions SET"):
		execution := executionFromArgs(args)
		if _, ok := q.executions[execution.ID]; !ok {
			return driver.RowsAffected(0), nil
		}
		q.executions[execution.ID] = execution
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "INSERT INTO audits"):
		record := auditFromArgs(args)
		q.audits[auditTaskKey(record.TaskID)] = append(q.audits[auditTaskKey(record.TaskID)], record)
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "INSERT INTO settings"):
		record := settingFromArgs(args)
		if q.settings[record.UserID] == nil {
			q.settings[record.UserID] = map[string]domain.SettingRecord{}
		}
		q.settings[record.UserID][string(record.Key)] = record
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "INSERT INTO agent_provider_state"):
		state := providerStateFromArgs(args)
		q.providerStates[state.SessionID] = append(q.providerStates[state.SessionID], state)
		return driver.RowsAffected(1), nil
	default:
		return nil, fmt.Errorf("unexpected exec query: %s", query)
	}
}

func (q *fakeQuerier) QueryContext(_ context.Context, query string, args ...any) (Rows, error) {
	switch {
	case strings.Contains(query, "FROM sessions") && strings.Contains(query, "WHERE id = $1"):
		session, ok := q.sessions[asString(args[0])]
		if !ok {
			return &fakeRows{}, nil
		}
		return rowsFromSessions([]domain.Session{session}), nil
	case strings.Contains(query, "FROM sessions"):
		items := make([]domain.Session, 0, len(q.sessions))
		for _, session := range q.sessions {
			items = append(items, session)
		}
		sortSessions(items)
		return rowsFromSessions(items), nil
	case strings.Contains(query, "FROM thread_messages"):
		items := cloneThreadMessages(q.threadMessages[asString(args[0])])
		return rowsFromThreadMessages(items), nil
	case strings.Contains(query, "FROM timeline_rows"):
		items := cloneTimelineRows(q.timelineRows[asString(args[0])])
		return rowsFromTimelineRows(items), nil
	case strings.Contains(query, "FROM tool_calls"):
		items := cloneToolCalls(q.toolCalls[asString(args[0])])
		return rowsFromToolCalls(items), nil
	case strings.Contains(query, "FROM tool_results") && strings.Contains(query, "WHERE task_id = $1"):
		items := cloneToolResultsByTask(q.toolResults, asString(args[0]))
		return rowsFromToolResults(items), nil
	case strings.Contains(query, "FROM tool_results"):
		items := cloneToolResults(q.toolResults[asString(args[0])])
		return rowsFromToolResults(items), nil
	case strings.Contains(query, "FROM tasks") && strings.Contains(query, "WHERE id = $1"):
		task, ok := q.tasks[asString(args[0])]
		if !ok {
			return &fakeRows{}, nil
		}
		return rowsFromTasks([]domain.Task{task}), nil
	case strings.Contains(query, "FROM tasks"):
		items := make([]domain.Task, 0)
		for _, task := range q.tasks {
			if task.SessionID == asString(args[0]) {
				items = append(items, task)
			}
		}
		return rowsFromTasks(items), nil
	case strings.Contains(query, "FROM executions") && strings.Contains(query, "WHERE id = $1"):
		exec, ok := q.executions[asString(args[0])]
		if !ok {
			return &fakeRows{}, nil
		}
		return rowsFromExecutions([]domain.Execution{exec}), nil
	case strings.Contains(query, "FROM executions"):
		items := make([]domain.Execution, 0)
		for _, exec := range q.executions {
			if exec.TaskID == asString(args[0]) {
				items = append(items, exec)
			}
		}
		return rowsFromExecutions(items), nil
	case strings.Contains(query, "FROM audits"):
		items := cloneAudits(q.audits[asString(args[0])])
		return rowsFromAudits(items), nil
	case strings.Contains(query, "FROM settings") && strings.Contains(query, "AND key = $2"):
		record, ok := q.settings[asString(args[0])][asString(args[1])]
		if !ok {
			return &fakeRows{}, nil
		}
		return rowsFromSettings([]domain.SettingRecord{record}), nil
	case strings.Contains(query, "FROM settings"):
		items := make([]domain.SettingRecord, 0)
		for _, record := range q.settings[asString(args[0])] {
			items = append(items, record)
		}
		return rowsFromSettings(items), nil
	case strings.Contains(query, "FROM agent_provider_state"):
		items := cloneProviderStates(q.providerStates[asString(args[0])])
		return rowsFromProviderStates(items), nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

type fakeRows struct {
	values [][]any
	idx    int
	err    error
}

func (r *fakeRows) Next() bool {
	if r.idx >= len(r.values) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	row := r.values[r.idx-1]
	for i := range dest {
		if err := assign(dest[i], row[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *fakeRows) Close() error { return nil }
func (r *fakeRows) Err() error   { return r.err }

func assign(dest any, src any) error {
	switch d := dest.(type) {
	case *string:
		*d = asString(src)
	case *[]byte:
		switch v := src.(type) {
		case nil:
			*d = nil
		case []byte:
			*d = cloneBytes(v)
		case json.RawMessage:
			*d = cloneBytes(v)
		case string:
			*d = []byte(v)
		default:
			*d = []byte(asString(v))
		}
	case *time.Time:
		switch v := src.(type) {
		case time.Time:
			*d = v
		default:
			return fmt.Errorf("cannot assign %T to *time.Time", src)
		}
	case *sql.NullString:
		if src == nil {
			d.Valid = false
			d.String = ""
			return nil
		}
		d.Valid = true
		d.String = asString(src)
	case *sql.NullInt64:
		if src == nil {
			d.Valid = false
			d.Int64 = 0
			return nil
		}
		d.Valid = true
		switch v := src.(type) {
		case int64:
			d.Int64 = v
		case int:
			d.Int64 = int64(v)
		case *int64:
			if v == nil {
				d.Valid = false
				d.Int64 = 0
				return nil
			}
			d.Int64 = *v
		case *int:
			if v == nil {
				d.Valid = false
				d.Int64 = 0
				return nil
			}
			d.Int64 = int64(*v)
		default:
			return fmt.Errorf("cannot assign %T to *sql.NullInt64", src)
		}
	case *int64:
		switch v := src.(type) {
		case int64:
			*d = v
		case int:
			*d = int64(v)
		default:
			return fmt.Errorf("cannot assign %T to *int64", src)
		}
	case *int:
		switch v := src.(type) {
		case int:
			*d = v
		case int64:
			*d = int(v)
		default:
			return fmt.Errorf("cannot assign %T to *int", src)
		}
	case *sql.NullTime:
		if src == nil {
			d.Valid = false
			d.Time = time.Time{}
			return nil
		}
		d.Valid = true
		switch v := src.(type) {
		case time.Time:
			d.Time = v
		case *time.Time:
			if v == nil {
				d.Valid = false
				d.Time = time.Time{}
				return nil
			}
			d.Time = *v
		default:
			return fmt.Errorf("cannot assign %T to *sql.NullTime", src)
		}
	default:
		return fmt.Errorf("unsupported scan dest %T", dest)
	}
	return nil
}

func asString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case json.RawMessage:
		return string(x)
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func sessionFromArgs(args []any) domain.Session {
	session := domain.Session{
		ID:        asString(args[0]),
		Title:     asString(args[1]),
		Status:    domain.SessionStatus(asString(args[2])),
		Revision:  asInt64(args[11]),
		CreatedAt: asTime(args[12]),
		UpdatedAt: asTime(args[13]),
	}
	if raw := asBytes(args[3]); len(raw) > 0 {
		_ = json.Unmarshal(raw, &session.ActiveTargetContext)
	}
	if s := asStringPtr(args[4]); s != nil {
		session.PendingAction = &domain.PendingAction{Type: domain.PendingActionType(*s), Payload: asBytes(args[5])}
	}
	session.CurrentOperationID = asStringPtr(args[6])
	session.CurrentTaskID = asStringPtr(args[7])
	session.CurrentExecutionGroupID = asStringPtr(args[8])
	session.LastAgentState = asBytes(args[9])
	session.ProviderStateBlob = asBytes(args[10])
	return session
}

func threadMessageFromArgs(args []any) domain.ThreadMessage {
	return domain.ThreadMessage{
		ID:              asString(args[0]),
		SessionID:       asString(args[1]),
		ClientMessageID: asStringPtr(args[2]),
		Role:            domain.MessageRole(asString(args[3])),
		Kind:            domain.ThreadMessageKind(asString(args[4])),
		Content:         asString(args[5]),
		CreatedAt:       asTime(args[6]),
	}
}

func timelineRowFromArgs(args []any) domain.TimelineRow {
	row := domain.TimelineRow{
		ID:          asString(args[0]),
		SessionID:   asString(args[1]),
		Kind:        domain.TimelineRowKind(asString(args[2])),
		CreatedAt:   asTime(args[3]),
		Text:        asString(args[4]),
		ToolName:    asString(args[5]),
		ToolStatus:  domain.ToolResultStatus(asString(args[6])),
		Source:      domain.TimelineRowSource(asString(args[7])),
		ArgsPreview: asStringPtr(args[8]),
		TaskID:      asStringPtr(args[9]),
	}
	if raw := asBytes(args[10]); len(raw) > 0 {
		var ctx domain.ActiveTargetContext
		_ = json.Unmarshal(raw, &ctx)
		row.TargetContext = &ctx
	}
	return row
}

func toolCallFromArgs(args []any) domain.ToolCall {
	return domain.ToolCall{
		ID:          asString(args[0]),
		SessionID:   asString(args[1]),
		TaskID:      asStringPtr(args[2]),
		MessageID:   asStringPtr(args[3]),
		ToolName:    asString(args[4]),
		Arguments:   asBytes(args[5]),
		ArgsPreview: asStringPtr(args[6]),
		Source:      domain.ToolCallSource(asString(args[7])),
		CreatedAt:   asTime(args[8]),
	}
}

func toolResultFromArgs(args []any) domain.ToolResult {
	return domain.ToolResult{
		ID:         asString(args[0]),
		SessionID:  asString(args[1]),
		TaskID:     asStringPtr(args[2]),
		ToolCallID: asStringPtr(args[3]),
		ToolName:   asString(args[4]),
		Status:     domain.ToolResultStatus(asString(args[5])),
		Text:       asString(args[6]),
		Source:     domain.TimelineRowSource(asString(args[7])),
		Payload:    asBytes(args[8]),
		CreatedAt:  asTime(args[9]),
	}
}

func taskFromArgs(args []any) domain.Task {
	var snapshot domain.TargetSnapshot
	_ = json.Unmarshal(asBytes(args[3]), &snapshot)
	return domain.Task{
		ID:                      asString(args[0]),
		SessionID:               asString(args[1]),
		InputText:               asString(args[2]),
		OperationTargetSnapshot: snapshot,
		Status:                  domain.TaskStatus(asString(args[4])),
		ApprovalStatus:          domain.ApprovalStatus(asString(args[5])),
		RiskLevel:               domain.RiskLevel(asString(args[6])),
		Summary:                 asStringPtr(args[7]),
		CreatedAt:               asTime(args[8]),
		UpdatedAt:               asTime(args[9]),
	}
}

func executionFromArgs(args []any) domain.Execution {
	exec := domain.Execution{
		ID:           asString(args[0]),
		TaskID:       asString(args[1]),
		SessionID:    asString(args[2]),
		NodeID:       asString(args[3]),
		Status:       domain.ExecutionStatus(asString(args[4])),
		StartedAt:    asTimePtr(args[5]),
		FinishedAt:   asTimePtr(args[6]),
		StdoutTail:   asString(args[8]),
		StderrTail:   asString(args[9]),
		StatusReason: asStringPtr(args[10]),
		CreatedAt:    asTime(args[11]),
		UpdatedAt:    asTime(args[12]),
	}
	if v, ok := args[7].(int); ok {
		exec.ExitCode = intPtr(v)
	}
	return exec
}

func auditFromArgs(args []any) domain.AuditRecord {
	return domain.AuditRecord{
		ID:        asString(args[0]),
		SessionID: asString(args[1]),
		TaskID:    asStringPtr(args[2]),
		ActorID:   asString(args[3]),
		EventType: asString(args[4]),
		Payload:   asBytes(args[5]),
		CreatedAt: asTime(args[6]),
	}
}

func settingFromArgs(args []any) domain.SettingRecord {
	return domain.SettingRecord{
		UserID:    asString(args[0]),
		Key:       domain.SettingKey(asString(args[1])),
		Value:     asBytes(args[2]),
		UpdatedAt: asTime(args[3]),
	}
}

func providerStateFromArgs(args []any) domain.AgentProviderState {
	return domain.AgentProviderState{
		ID:        asString(args[0]),
		SessionID: asString(args[1]),
		Version:   asInt64(args[2]),
		Payload:   asBytes(args[3]),
		CreatedAt: asTime(args[4]),
	}
}

func asBytes(v any) []byte {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		return cloneBytes(x)
	case json.RawMessage:
		return cloneBytes(x)
	case string:
		return []byte(x)
	default:
		return []byte(fmt.Sprintf("%v", v))
	}
}

func asStringPtr(v any) *string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return stringPtr(x)
	case *string:
		return x
	case sql.NullString:
		if !x.Valid {
			return nil
		}
		return stringPtr(x.String)
	default:
		s := asString(v)
		return &s
	}
}

func asInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	default:
		return 0
	}
}

func asTime(v any) time.Time {
	switch x := v.(type) {
	case time.Time:
		return x.UTC()
	case *time.Time:
		if x == nil {
			return time.Time{}
		}
		return x.UTC()
	default:
		return time.Time{}
	}
}

func asTimePtr(v any) *time.Time {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		return &x
	case *time.Time:
		return x
	default:
		return nil
	}
}

func rowsFromSessions(items []domain.Session) Rows { return rowsFromSessionsValues(sessionRows(items)) }
func rowsFromThreadMessages(items []domain.ThreadMessage) Rows {
	return rowsFromValues(threadMessageRows(items))
}
func rowsFromTimelineRows(items []domain.TimelineRow) Rows {
	return rowsFromValues(timelineRows(items))
}
func rowsFromToolCalls(items []domain.ToolCall) Rows { return rowsFromValues(toolCallRows(items)) }
func rowsFromToolResults(items []domain.ToolResult) Rows {
	return rowsFromValues(toolResultRows(items))
}
func rowsFromTasks(items []domain.Task) Rows             { return rowsFromValues(taskRows(items)) }
func rowsFromExecutions(items []domain.Execution) Rows   { return rowsFromValues(executionRows(items)) }
func rowsFromAudits(items []domain.AuditRecord) Rows     { return rowsFromValues(auditRows(items)) }
func rowsFromSettings(items []domain.SettingRecord) Rows { return rowsFromValues(settingRows(items)) }
func rowsFromProviderStates(items []domain.AgentProviderState) Rows {
	return rowsFromValues(providerStateRows(items))
}

func rowsFromSessionsValues(values [][]any) Rows { return &fakeRows{values: values} }
func rowsFromValues(values [][]any) Rows         { return &fakeRows{values: values} }

func sessionRows(items []domain.Session) [][]any {
	values := make([][]any, 0, len(items))
	for _, item := range items {
		values = append(values, []any{
			item.ID,
			item.Title,
			string(item.Status),
			rawMessage(item.ActiveTargetContext),
			pendingActionType(item.PendingAction),
			pendingActionPayload(item.PendingAction),
			nullableString(item.CurrentOperationID),
			nullableString(item.CurrentTaskID),
			nullableString(item.CurrentExecutionGroupID),
			rawMessage(item.LastAgentState),
			rawMessage(item.ProviderStateBlob),
			item.Revision,
			item.CreatedAt,
			item.UpdatedAt,
		})
	}
	return values
}

func threadMessageRows(items []domain.ThreadMessage) [][]any {
	values := make([][]any, 0, len(items))
	for _, item := range items {
		values = append(values, []any{item.ID, item.SessionID, nullableString(item.ClientMessageID), string(item.Role), string(item.Kind), item.Content, item.CreatedAt})
	}
	return values
}

func timelineRows(items []domain.TimelineRow) [][]any {
	values := make([][]any, 0, len(items))
	for _, item := range items {
		values = append(values, []any{item.ID, item.SessionID, string(item.Kind), item.CreatedAt, item.Text, item.ToolName, string(item.ToolStatus), string(item.Source), item.ArgsPreview, item.TaskID, rawMessage(item.TargetContext)})
	}
	return values
}

func toolCallRows(items []domain.ToolCall) [][]any {
	values := make([][]any, 0, len(items))
	for _, item := range items {
		values = append(values, []any{item.ID, item.SessionID, item.TaskID, item.MessageID, item.ToolName, item.Arguments, item.ArgsPreview, string(item.Source), item.CreatedAt})
	}
	return values
}

func toolResultRows(items []domain.ToolResult) [][]any {
	values := make([][]any, 0, len(items))
	for _, item := range items {
		values = append(values, []any{item.ID, item.SessionID, item.TaskID, item.ToolCallID, item.ToolName, string(item.Status), item.Text, string(item.Source), item.Payload, item.CreatedAt})
	}
	return values
}

func taskRows(items []domain.Task) [][]any {
	values := make([][]any, 0, len(items))
	for _, item := range items {
		values = append(values, []any{item.ID, item.SessionID, item.InputText, rawMessage(item.OperationTargetSnapshot), string(item.Status), string(item.ApprovalStatus), string(item.RiskLevel), item.Summary, item.CreatedAt, item.UpdatedAt})
	}
	return values
}

func executionRows(items []domain.Execution) [][]any {
	values := make([][]any, 0, len(items))
	for _, item := range items {
		values = append(values, []any{item.ID, item.TaskID, item.SessionID, item.NodeID, string(item.Status), item.StartedAt, item.FinishedAt, item.ExitCode, item.StdoutTail, item.StderrTail, item.StatusReason, item.CreatedAt, item.UpdatedAt})
	}
	return values
}

func auditRows(items []domain.AuditRecord) [][]any {
	values := make([][]any, 0, len(items))
	for _, item := range items {
		values = append(values, []any{item.ID, item.SessionID, item.TaskID, item.ActorID, item.EventType, item.Payload, item.CreatedAt})
	}
	return values
}

func settingRows(items []domain.SettingRecord) [][]any {
	values := make([][]any, 0, len(items))
	for _, item := range items {
		values = append(values, []any{item.UserID, string(item.Key), item.Value, item.UpdatedAt})
	}
	return values
}

func providerStateRows(items []domain.AgentProviderState) [][]any {
	values := make([][]any, 0, len(items))
	for _, item := range items {
		values = append(values, []any{item.ID, item.SessionID, item.Version, item.Payload, item.CreatedAt})
	}
	return values
}

func cloneThreadMessages(items []domain.ThreadMessage) []domain.ThreadMessage {
	out := make([]domain.ThreadMessage, len(items))
	for i := range items {
		out[i] = items[i]
		if items[i].ClientMessageID != nil {
			v := *items[i].ClientMessageID
			out[i].ClientMessageID = &v
		}
	}
	return out
}

func cloneTimelineRows(items []domain.TimelineRow) []domain.TimelineRow {
	out := make([]domain.TimelineRow, len(items))
	copy(out, items)
	return out
}

func cloneToolCalls(items []domain.ToolCall) []domain.ToolCall {
	out := make([]domain.ToolCall, len(items))
	copy(out, items)
	return out
}

func cloneToolResults(items []domain.ToolResult) []domain.ToolResult {
	out := make([]domain.ToolResult, len(items))
	copy(out, items)
	return out
}

func cloneToolResultsByTask(all map[string][]domain.ToolResult, taskID string) []domain.ToolResult {
	out := make([]domain.ToolResult, 0)
	for _, items := range all {
		for _, item := range items {
			if item.TaskID != nil && *item.TaskID == taskID {
				out = append(out, item)
			}
		}
	}
	return out
}

func cloneAudits(items []domain.AuditRecord) []domain.AuditRecord {
	out := make([]domain.AuditRecord, len(items))
	copy(out, items)
	return out
}

func cloneProviderStates(items []domain.AgentProviderState) []domain.AgentProviderState {
	out := make([]domain.AgentProviderState, len(items))
	copy(out, items)
	return out
}

func auditTaskKey(taskID *string) string {
	if taskID == nil {
		return ""
	}
	return *taskID
}
