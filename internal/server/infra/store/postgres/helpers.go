package postgres

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

func rawMessage(in any) json.RawMessage {
	switch v := in.(type) {
	case nil:
		return json.RawMessage("null")
	case json.RawMessage:
		return cloneBytes(v)
	case []byte:
		return cloneBytes(v)
	case string:
		return cloneBytes([]byte(v))
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return json.RawMessage("null")
		}
		return b
	}
}

func cloneBytes(in []byte) []byte {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func nullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	out := v.String
	return &out
}

func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	out := v.Time.UTC()
	return &out
}

func stringPtr(v string) *string {
	out := v
	return &out
}

func nullableString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return *v
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneTargetCandidates(in []domain.TargetCandidate) []domain.TargetCandidate {
	if in == nil {
		return nil
	}
	out := make([]domain.TargetCandidate, len(in))
	copy(out, in)
	return out
}

func cloneActiveTargetContext(in domain.ActiveTargetContext) domain.ActiveTargetContext {
	out := in
	out.NodeIDs = cloneStringSlice(in.NodeIDs)
	out.Candidates = cloneTargetCandidates(in.Candidates)
	if in.SourceMessageID != nil {
		v := *in.SourceMessageID
		out.SourceMessageID = &v
	}
	if in.ConfirmedAt != nil {
		v := *in.ConfirmedAt
		out.ConfirmedAt = &v
	}
	return out
}

func cloneTargetSnapshot(in domain.TargetSnapshot) domain.TargetSnapshot {
	out := in
	out.NodeIDs = cloneStringSlice(in.NodeIDs)
	if in.ConfirmedAt != nil {
		v := *in.ConfirmedAt
		out.ConfirmedAt = &v
	}
	return out
}

func clonePendingAction(in *domain.PendingAction) *domain.PendingAction {
	if in == nil {
		return nil
	}
	out := *in
	out.Payload = cloneBytes(in.Payload)
	return &out
}

func cloneSession(in domain.Session) domain.Session {
	out := in
	out.ActiveTargetContext = cloneActiveTargetContext(in.ActiveTargetContext)
	out.PendingAction = clonePendingAction(in.PendingAction)
	out.LastAgentState = cloneBytes(in.LastAgentState)
	out.ProviderStateBlob = cloneBytes(in.ProviderStateBlob)
	if in.CurrentOperationID != nil {
		out.CurrentOperationID = stringPtr(*in.CurrentOperationID)
	}
	if in.CurrentTaskID != nil {
		out.CurrentTaskID = stringPtr(*in.CurrentTaskID)
	}
	if in.CurrentExecutionGroupID != nil {
		out.CurrentExecutionGroupID = stringPtr(*in.CurrentExecutionGroupID)
	}
	return out
}

func cloneTimelineRow(in domain.TimelineRow) domain.TimelineRow {
	out := in
	if in.ArgsPreview != nil {
		out.ArgsPreview = stringPtr(*in.ArgsPreview)
	}
	if in.TaskID != nil {
		out.TaskID = stringPtr(*in.TaskID)
	}
	if in.TargetContext != nil {
		v := cloneActiveTargetContext(*in.TargetContext)
		out.TargetContext = &v
	}
	return out
}

func cloneTask(in domain.Task) domain.Task {
	out := in
	out.OperationTargetSnapshot = cloneTargetSnapshot(in.OperationTargetSnapshot)
	if in.Summary != nil {
		out.Summary = stringPtr(*in.Summary)
	}
	return out
}

func cloneExecution(in domain.Execution) domain.Execution {
	out := in
	if in.StartedAt != nil {
		v := *in.StartedAt
		out.StartedAt = &v
	}
	if in.FinishedAt != nil {
		v := *in.FinishedAt
		out.FinishedAt = &v
	}
	if in.ExitCode != nil {
		out.ExitCode = intPtr(*in.ExitCode)
	}
	if in.StatusReason != nil {
		out.StatusReason = stringPtr(*in.StatusReason)
	}
	return out
}

func cloneAuditRecord(in domain.AuditRecord) domain.AuditRecord {
	out := in
	out.Payload = cloneBytes(in.Payload)
	if in.TaskID != nil {
		out.TaskID = stringPtr(*in.TaskID)
	}
	return out
}

func cloneSettingRecord(in domain.SettingRecord) domain.SettingRecord {
	out := in
	out.Value = cloneBytes(in.Value)
	return out
}

func intPtr(v int) *int {
	out := v
	return &out
}

func reverse[T any](items []T) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func tail[T any](items []T, limit int) []T {
	if limit <= 0 || len(items) <= limit {
		out := make([]T, len(items))
		copy(out, items)
		return out
	}
	start := len(items) - limit
	out := make([]T, limit)
	copy(out, items[start:])
	return out
}

func filterBeforeID[T any](items []T, getID func(T) string, beforeID string) ([]T, error) {
	if beforeID == "" {
		return items, nil
	}
	for idx, item := range items {
		if getID(item) == beforeID {
			return items[:idx], nil
		}
	}
	return nil, domain.ErrNotFound
}

func filterStatuses(items []domain.Session, statuses []domain.SessionStatus) []domain.Session {
	if len(statuses) == 0 {
		return items
	}
	allowed := make(map[domain.SessionStatus]struct{}, len(statuses))
	for _, status := range statuses {
		allowed[status] = struct{}{}
	}
	out := make([]domain.Session, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.Status]; ok {
			out = append(out, item)
		}
	}
	return out
}

func sortSessions(items []domain.Session) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
}

func requireRowsAffected(result sql.Result, notFound error) error {
	if result == nil {
		return notFound
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return notFound
	}
	return nil
}

func wrapNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func cloneToolCall(in domain.ToolCall) domain.ToolCall {
	out := in
	out.Arguments = cloneBytes(in.Arguments)
	if in.TaskID != nil {
		out.TaskID = stringPtr(*in.TaskID)
	}
	if in.MessageID != nil {
		out.MessageID = stringPtr(*in.MessageID)
	}
	if in.ArgsPreview != nil {
		out.ArgsPreview = stringPtr(*in.ArgsPreview)
	}
	return out
}

func cloneToolResult(in domain.ToolResult) domain.ToolResult {
	out := in
	out.Payload = cloneBytes(in.Payload)
	if in.TaskID != nil {
		out.TaskID = stringPtr(*in.TaskID)
	}
	if in.ToolCallID != nil {
		out.ToolCallID = stringPtr(*in.ToolCallID)
	}
	return out
}

func cloneAgentProviderState(in domain.AgentProviderState) domain.AgentProviderState {
	out := in
	out.Payload = cloneBytes(in.Payload)
	return out
}
