package memory

import (
	"encoding/json"
	"sort"

	"github.com/momaek/tolato/internal/server/domain"
)

func cloneBytes(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
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

func clonePendingAction(in *domain.PendingAction) *domain.PendingAction {
	if in == nil {
		return nil
	}
	out := *in
	out.Payload = cloneBytes(in.Payload)
	return &out
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

func cloneSession(in domain.Session) domain.Session {
	out := in
	out.ActiveTargetContext = cloneActiveTargetContext(in.ActiveTargetContext)
	out.PendingAction = clonePendingAction(in.PendingAction)
	out.LastAgentState = cloneBytes(in.LastAgentState)
	out.ProviderStateBlob = cloneBytes(in.ProviderStateBlob)
	if in.CurrentOperationID != nil {
		v := *in.CurrentOperationID
		out.CurrentOperationID = &v
	}
	if in.CurrentTaskID != nil {
		v := *in.CurrentTaskID
		out.CurrentTaskID = &v
	}
	if in.CurrentExecutionGroupID != nil {
		v := *in.CurrentExecutionGroupID
		out.CurrentExecutionGroupID = &v
	}
	return out
}

func cloneThreadMessage(in domain.ThreadMessage) domain.ThreadMessage {
	out := in
	if in.ClientMessageID != nil {
		v := *in.ClientMessageID
		out.ClientMessageID = &v
	}
	return out
}

func cloneTimelineRow(in domain.TimelineRow) domain.TimelineRow {
	out := in
	if in.ArgsPreview != nil {
		v := *in.ArgsPreview
		out.ArgsPreview = &v
	}
	if in.TaskID != nil {
		v := *in.TaskID
		out.TaskID = &v
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
		v := *in.Summary
		out.Summary = &v
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
		v := *in.ExitCode
		out.ExitCode = &v
	}
	if in.StatusReason != nil {
		v := *in.StatusReason
		out.StatusReason = &v
	}
	return out
}

func cloneAuditRecord(in domain.AuditRecord) domain.AuditRecord {
	out := in
	out.Payload = cloneBytes(in.Payload)
	if in.TaskID != nil {
		v := *in.TaskID
		out.TaskID = &v
	}
	return out
}

func cloneSettingRecord(in domain.SettingRecord) domain.SettingRecord {
	out := in
	out.Value = cloneBytes(in.Value)
	return out
}

func cloneAuthSession(in domain.AuthSession) domain.AuthSession {
	return in
}

func cloneToolCall(in domain.ToolCall) domain.ToolCall {
	out := in
	out.Arguments = cloneBytes(in.Arguments)
	if in.TaskID != nil {
		v := *in.TaskID
		out.TaskID = &v
	}
	if in.MessageID != nil {
		v := *in.MessageID
		out.MessageID = &v
	}
	if in.CallID != nil {
		v := *in.CallID
		out.CallID = &v
	}
	if in.ArgsPreview != nil {
		v := *in.ArgsPreview
		out.ArgsPreview = &v
	}
	return out
}

func cloneToolResult(in domain.ToolResult) domain.ToolResult {
	out := in
	out.Payload = cloneBytes(in.Payload)
	if in.TaskID != nil {
		v := *in.TaskID
		out.TaskID = &v
	}
	if in.ToolCallID != nil {
		v := *in.ToolCallID
		out.ToolCallID = &v
	}
	if in.CallID != nil {
		v := *in.CallID
		out.CallID = &v
	}
	return out
}

func tailByLimit[T any](items []T, limit int) []T {
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

func sortByString(items []string) {
	sort.Strings(items)
}
