package wsui

import (
	"encoding/json"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type EventScope string

const (
	EventScopeTimeline EventScope = "timeline"
	EventScopeSummary  EventScope = "summary"
)

type ConnectionReady struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
}

type SessionStateUpdatedEvent struct {
	Type       string               `json:"type"`
	EventScope EventScope           `json:"eventScope"`
	SessionID  string               `json:"sessionId"`
	Status     domain.SessionStatus `json:"status"`
	Revision   int64                `json:"revision"`
	Timestamp  string               `json:"timestamp"`
}

type TimelineRowAppendedEvent struct {
	Type       string          `json:"type"`
	EventScope EventScope      `json:"eventScope"`
	SessionID  string          `json:"sessionId"`
	Revision   int64           `json:"revision"`
	Timestamp  string          `json:"timestamp"`
	Row        TimelineRowView `json:"row"`
}

type ThreadTargetPendingEvent struct {
	Type          string                  `json:"type"`
	EventScope    EventScope              `json:"eventScope"`
	SessionID     string                  `json:"sessionId"`
	Revision      int64                   `json:"revision"`
	Timestamp     string                  `json:"timestamp"`
	TargetContext ActiveTargetContextView `json:"targetContext"`
}

type ThreadTargetConfirmedEvent struct {
	Type          string                  `json:"type"`
	EventScope    EventScope              `json:"eventScope"`
	SessionID     string                  `json:"sessionId"`
	Revision      int64                   `json:"revision"`
	Timestamp     string                  `json:"timestamp"`
	TargetContext ActiveTargetContextView `json:"targetContext"`
}

type ThreadTargetClearedEvent struct {
	Type          string                  `json:"type"`
	EventScope    EventScope              `json:"eventScope"`
	SessionID     string                  `json:"sessionId"`
	Revision      int64                   `json:"revision"`
	Timestamp     string                  `json:"timestamp"`
	TargetContext ActiveTargetContextView `json:"targetContext"`
}

type ExecutionChunkEvent struct {
	Type        string             `json:"type"`
	EventScope  EventScope         `json:"eventScope"`
	SessionID   string             `json:"sessionId"`
	TaskID      string             `json:"taskId"`
	ExecutionID string             `json:"executionId"`
	NodeID      string             `json:"nodeId"`
	Chunk       ExecutionChunkView `json:"chunk"`
	Timestamp   string             `json:"timestamp"`
}

type ExecutionFinishedEvent struct {
	Type        string                 `json:"type"`
	EventScope  EventScope             `json:"eventScope"`
	SessionID   string                 `json:"sessionId"`
	TaskID      string                 `json:"taskId"`
	ExecutionID string                 `json:"executionId"`
	NodeID      string                 `json:"nodeId"`
	Status      domain.ExecutionStatus `json:"status"`
	Timestamp   string                 `json:"timestamp"`
}

type LLMSSEEvent struct {
	Type              string          `json:"type"`
	EventScope        EventScope      `json:"eventScope"`
	SessionID         string          `json:"sessionId"`
	ResponseID        string          `json:"responseId,omitempty"`
	SequenceNumber    int             `json:"sequenceNumber,omitempty"`
	UpstreamEventType string          `json:"upstreamEventType"`
	RawEvent          json.RawMessage `json:"rawEvent"`
	Timestamp         string          `json:"timestamp"`
}

type LLMResponseCompletedEvent struct {
	Type        string          `json:"type"`
	EventScope  EventScope      `json:"eventScope"`
	SessionID   string          `json:"sessionId"`
	ResponseID  string          `json:"responseId,omitempty"`
	RawResponse json.RawMessage `json:"rawResponse"`
	Timestamp   string          `json:"timestamp"`
}

type SessionSummary struct {
	Title               string               `json:"title"`
	Status              domain.SessionStatus `json:"status"`
	UpdatedAt           string               `json:"updatedAt"`
	ActiveTargetSummary string               `json:"activeTargetSummary,omitempty"`
	Unread              int                  `json:"unread"`
}

type SessionSummaryUpdatedEvent struct {
	Type       string         `json:"type"`
	EventScope EventScope     `json:"eventScope"`
	SessionID  string         `json:"sessionId"`
	Timestamp  string         `json:"timestamp"`
	Summary    SessionSummary `json:"summary"`
}

type SessionRequiresAttentionEvent struct {
	Type       string     `json:"type"`
	EventScope EventScope `json:"eventScope"`
	SessionID  string     `json:"sessionId"`
	Timestamp  string     `json:"timestamp"`
	Reason     string     `json:"reason"`
}

type SessionUnreadUpdatedEvent struct {
	Type       string     `json:"type"`
	EventScope EventScope `json:"eventScope"`
	SessionID  string     `json:"sessionId"`
	Timestamp  string     `json:"timestamp"`
	Unread     int        `json:"unread"`
}

type SessionFinishedEvent struct {
	Type       string         `json:"type"`
	EventScope EventScope     `json:"eventScope"`
	SessionID  string         `json:"sessionId"`
	Timestamp  string         `json:"timestamp"`
	Summary    SessionSummary `json:"summary"`
}

type TimelineRowView struct {
	ID            string                   `json:"id"`
	Kind          domain.TimelineRowKind   `json:"kind"`
	CreatedAt     string                   `json:"createdAt"`
	Text          string                   `json:"text,omitempty"`
	ToolName      string                   `json:"toolName,omitempty"`
	ToolStatus    domain.ToolResultStatus  `json:"toolStatus,omitempty"`
	Source        domain.TimelineRowSource `json:"source,omitempty"`
	ArgsPreview   *string                  `json:"argsPreview,omitempty"`
	TaskID        *string                  `json:"taskId,omitempty"`
	TargetContext *ActiveTargetContextView `json:"targetContext,omitempty"`
}

type ActiveTargetContextView struct {
	Status          domain.TargetStatus   `json:"status"`
	Scope           domain.TargetScope    `json:"scope"`
	NodeIDs         []string              `json:"nodeIds,omitempty"`
	DisplayLabel    string                `json:"displayLabel,omitempty"`
	Source          domain.TargetSource   `json:"source,omitempty"`
	Confidence      float64               `json:"confidence,omitempty"`
	Candidates      []TargetCandidateView `json:"candidates,omitempty"`
	SourceMessageID *string               `json:"sourceMessageId,omitempty"`
	ConfirmedAt     *string               `json:"confirmedAt,omitempty"`
}

type TargetCandidateView struct {
	NodeID    string `json:"nodeId"`
	Hostname  string `json:"hostname,omitempty"`
	Region    string `json:"region,omitempty"`
	MatchedBy string `json:"matchedBy,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type ExecutionChunkView struct {
	Stream domain.ExecutionStream `json:"stream"`
	Text   string                 `json:"text"`
}

func newTimelineRowView(row domain.TimelineRow) TimelineRowView {
	view := TimelineRowView{
		ID:          row.ID,
		Kind:        row.Kind,
		CreatedAt:   row.CreatedAt.UTC().Format(time.RFC3339),
		Text:        row.Text,
		ToolName:    row.ToolName,
		ToolStatus:  row.ToolStatus,
		Source:      row.Source,
		ArgsPreview: row.ArgsPreview,
		TaskID:      row.TaskID,
	}
	if row.TargetContext != nil {
		target := newActiveTargetContextView(*row.TargetContext)
		view.TargetContext = &target
	}
	return view
}

func newActiveTargetContextView(ctx domain.ActiveTargetContext) ActiveTargetContextView {
	view := ActiveTargetContextView{
		Status:          ctx.Status,
		Scope:           ctx.Scope,
		NodeIDs:         append([]string(nil), ctx.NodeIDs...),
		DisplayLabel:    ctx.DisplayLabel,
		Source:          ctx.Source,
		Confidence:      ctx.Confidence,
		SourceMessageID: ctx.SourceMessageID,
	}
	if len(ctx.Candidates) > 0 {
		view.Candidates = make([]TargetCandidateView, 0, len(ctx.Candidates))
		for _, candidate := range ctx.Candidates {
			view.Candidates = append(view.Candidates, TargetCandidateView{
				NodeID:    candidate.NodeID,
				Hostname:  candidate.Hostname,
				Region:    candidate.Region,
				MatchedBy: candidate.MatchedBy,
				Reason:    candidate.Reason,
			})
		}
	}
	if ctx.ConfirmedAt != nil {
		ts := ctx.ConfirmedAt.UTC().Format(time.RFC3339)
		view.ConfirmedAt = &ts
	}
	return view
}

func newExecutionChunkView(chunk domain.ExecutionChunk) ExecutionChunkView {
	return ExecutionChunkView{
		Stream: chunk.Stream,
		Text:   chunk.Text,
	}
}

func newSessionSummary(session domain.Session, unread int) SessionSummary {
	return SessionSummary{
		Title:               session.Title,
		Status:              session.Status,
		UpdatedAt:           session.UpdatedAt.UTC().Format(time.RFC3339),
		ActiveTargetSummary: session.ActiveTargetContext.DisplayLabel,
		Unread:              unread,
	}
}
