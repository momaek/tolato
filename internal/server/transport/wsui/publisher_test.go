package wsui

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

func TestPublisherTimelineEventOnlyReachesActiveClients(t *testing.T) {
	hub := infraws.NewMemoryHub()
	activeClient := infraws.NewMemoryClient("client-active", infraws.ClientKindUI, 4)
	watchClient := infraws.NewMemoryClient("client-watch", infraws.ClientKindUI, 4)
	hub.Register(activeClient)
	hub.Register(watchClient)

	registry := infraws.NewMemorySessionRegistry(hub)
	registry.SetActive("client-active", "sess-1")
	registry.SetWatchSessions("client-watch", []string{"sess-1"})

	publisher := &Publisher{
		Registry: registry,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 18, 0, 0, 0, time.UTC) },
	}
	session := domain.Session{ID: "sess-1", Revision: 7}
	row := domain.TimelineRow{
		ID:        "row-1",
		SessionID: "sess-1",
		Kind:      domain.TimelineRowKindAssistantText,
		Text:      "done",
		CreatedAt: time.Date(2026, 3, 22, 18, 0, 0, 0, time.UTC),
	}

	if err := publisher.TimelineRowAppended(context.Background(), session, row); err != nil {
		t.Fatalf("TimelineRowAppended() error = %v", err)
	}

	activeMsg := mustDrainOne(t, activeClient.Messages())
	var event TimelineRowAppendedEvent
	if err := json.Unmarshal(activeMsg, &event); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if event.Type != "timeline.row.appended" || event.EventScope != EventScopeTimeline || event.SessionID != "sess-1" || event.Revision != 7 {
		t.Fatalf("event = %#v, want timeline event", event)
	}
	watchMsg := mustDrainOne(t, watchClient.Messages())
	var unread SessionUnreadUpdatedEvent
	if err := json.Unmarshal(watchMsg, &unread); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if unread.Type != "session.unread.updated" || unread.Unread != 1 {
		t.Fatalf("watch event = %#v, want unread update", unread)
	}
}

func TestPublisherTimelineRowAppendedPublishesUnreadUpdatedToWatchers(t *testing.T) {
	hub := infraws.NewMemoryHub()
	activeClient := infraws.NewMemoryClient("client-active", infraws.ClientKindUI, 4)
	watchClient := infraws.NewMemoryClient("client-watch", infraws.ClientKindUI, 4)
	hub.Register(activeClient)
	hub.Register(watchClient)

	registry := infraws.NewMemorySessionRegistry(hub)
	registry.SetActive("client-active", "sess-unread")
	registry.SetWatchSessions("client-watch", []string{"sess-unread"})

	publisher := &Publisher{
		Registry: registry,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 18, 5, 0, 0, time.UTC) },
	}
	session := domain.Session{ID: "sess-unread", Revision: 8}
	row := domain.TimelineRow{
		ID:        "row-2",
		SessionID: "sess-unread",
		Kind:      domain.TimelineRowKindAssistantText,
		Text:      "new",
		CreatedAt: time.Date(2026, 3, 22, 18, 5, 0, 0, time.UTC),
	}

	if err := publisher.TimelineRowAppended(context.Background(), session, row); err != nil {
		t.Fatalf("TimelineRowAppended() error = %v", err)
	}

	activeMsgs := drainQueue(activeClient.Messages())
	if len(activeMsgs) != 1 {
		t.Fatalf("active messages = %#v, want one timeline event", activeMsgs)
	}

	watchMsg := mustDrainOne(t, watchClient.Messages())
	var unread SessionUnreadUpdatedEvent
	if err := json.Unmarshal(watchMsg, &unread); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if unread.Type != "session.unread.updated" || unread.EventScope != EventScopeSummary || unread.Unread != 1 {
		t.Fatalf("unread event = %#v, want unread=1 summary event", unread)
	}
}

func TestPublisherSessionStateUpdatedPublishesTimelineAndSummary(t *testing.T) {
	hub := infraws.NewMemoryHub()
	activeClient := infraws.NewMemoryClient("client-active", infraws.ClientKindUI, 4)
	watchClient := infraws.NewMemoryClient("client-watch", infraws.ClientKindUI, 4)
	hub.Register(activeClient)
	hub.Register(watchClient)

	registry := infraws.NewMemorySessionRegistry(hub)
	registry.SetActive("client-active", "sess-2")
	registry.SetWatchSessions("client-watch", []string{"sess-2"})

	publisher := &Publisher{
		Registry: registry,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 19, 0, 0, 0, time.UTC) },
	}

	if err := publisher.SessionStateUpdated(context.Background(), domain.Session{
		ID:       "sess-2",
		Status:   domain.SessionStatusRunning,
		Revision: 11,
	}); err != nil {
		t.Fatalf("SessionStateUpdated() error = %v", err)
	}

	activeMsgs := drainQueue(activeClient.Messages())
	if len(activeMsgs) != 2 {
		t.Fatalf("active messages = %#v, want timeline + summary", activeMsgs)
	}
	var timeline SessionStateUpdatedEvent
	if err := json.Unmarshal(activeMsgs[0], &timeline); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if timeline.Type != "session.state.updated" || timeline.Status != domain.SessionStatusRunning || timeline.Revision != 11 {
		t.Fatalf("timeline event = %#v, want running revision 11", timeline)
	}

	var activeSummary SessionSummaryUpdatedEvent
	if err := json.Unmarshal(activeMsgs[1], &activeSummary); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if activeSummary.Type != "session.summary.updated" || activeSummary.EventScope != EventScopeSummary || activeSummary.Summary.Status != domain.SessionStatusRunning {
		t.Fatalf("summary event = %#v, want running summary", activeSummary)
	}

	watchMsg := mustDrainOne(t, watchClient.Messages())
	var watchSummary SessionSummaryUpdatedEvent
	if err := json.Unmarshal(watchMsg, &watchSummary); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if watchSummary.Type != "session.summary.updated" || watchSummary.Summary.Status != domain.SessionStatusRunning {
		t.Fatalf("watch summary = %#v, want running summary", watchSummary)
	}
}

func TestPublisherSessionFinishedPublishesSummaryScope(t *testing.T) {
	hub := infraws.NewMemoryHub()
	activeClient := infraws.NewMemoryClient("client-active", infraws.ClientKindUI, 4)
	watchClient := infraws.NewMemoryClient("client-watch", infraws.ClientKindUI, 4)
	hub.Register(activeClient)
	hub.Register(watchClient)

	registry := infraws.NewMemorySessionRegistry(hub)
	registry.SetActive("client-active", "sess-finished")
	registry.SetWatchSessions("client-watch", []string{"sess-finished"})

	publisher := &Publisher{
		Registry: registry,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 19, 5, 0, 0, time.UTC) },
	}

	if err := publisher.SessionStateUpdated(context.Background(), domain.Session{
		ID:        "sess-finished",
		Title:     "Finished Session",
		Status:    domain.SessionStatusCompleted,
		Revision:  12,
		UpdatedAt: time.Date(2026, 3, 22, 19, 5, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SessionStateUpdated() error = %v", err)
	}

	activeMsgs := drainQueue(activeClient.Messages())
	if len(activeMsgs) != 2 {
		t.Fatalf("active messages = %#v, want timeline + finished", activeMsgs)
	}
	var finished SessionFinishedEvent
	if err := json.Unmarshal(activeMsgs[1], &finished); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if finished.Type != "session.finished" || finished.EventScope != EventScopeSummary || finished.Summary.Status != domain.SessionStatusCompleted {
		t.Fatalf("finished event = %#v, want completed summary", finished)
	}

	watchMsg := mustDrainOne(t, watchClient.Messages())
	var watchFinished SessionFinishedEvent
	if err := json.Unmarshal(watchMsg, &watchFinished); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if watchFinished.Type != "session.finished" || watchFinished.Summary.Title != "Finished Session" {
		t.Fatalf("watch finished = %#v, want finished session summary", watchFinished)
	}
}

func TestPublisherSessionRequiresAttentionPublishesSummaryScope(t *testing.T) {
	hub := infraws.NewMemoryHub()
	activeClient := infraws.NewMemoryClient("client-active", infraws.ClientKindUI, 4)
	watchClient := infraws.NewMemoryClient("client-watch", infraws.ClientKindUI, 4)
	hub.Register(activeClient)
	hub.Register(watchClient)

	registry := infraws.NewMemorySessionRegistry(hub)
	registry.SetActive("client-active", "sess-attention")
	registry.SetWatchSessions("client-watch", []string{"sess-attention"})

	publisher := &Publisher{
		Registry: registry,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 19, 15, 0, 0, time.UTC) },
	}

	if err := publisher.SessionStateUpdated(context.Background(), domain.Session{
		ID:        "sess-attention",
		Title:     "Attention Session",
		Status:    domain.SessionStatusPausedWaitApproval,
		Revision:  13,
		UpdatedAt: time.Date(2026, 3, 22, 19, 15, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SessionStateUpdated() error = %v", err)
	}

	activeMsgs := drainQueue(activeClient.Messages())
	if len(activeMsgs) != 3 {
		t.Fatalf("active messages = %#v, want timeline + summary + requires_attention", activeMsgs)
	}

	var attention SessionRequiresAttentionEvent
	if err := json.Unmarshal(activeMsgs[2], &attention); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if attention.Type != "session.requires_attention" || attention.EventScope != EventScopeSummary || attention.Reason != "approval" {
		t.Fatalf("attention event = %#v, want approval attention", attention)
	}

	watchMsgs := drainQueue(watchClient.Messages())
	if len(watchMsgs) != 2 {
		t.Fatalf("watch messages = %#v, want summary + requires_attention", watchMsgs)
	}
	if err := json.Unmarshal(watchMsgs[1], &attention); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if attention.Reason != "approval" {
		t.Fatalf("watch attention = %#v, want approval reason", attention)
	}
}

func TestPublisherExecutionChunkUsesTimelineScope(t *testing.T) {
	hub := infraws.NewMemoryHub()
	activeClient := infraws.NewMemoryClient("client-active", infraws.ClientKindUI, 4)
	hub.Register(activeClient)

	registry := infraws.NewMemorySessionRegistry(hub)
	registry.SetActive("client-active", "sess-3")

	publisher := &Publisher{
		Registry: registry,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 19, 30, 0, 0, time.UTC) },
	}

	if err := publisher.ExecutionChunk(context.Background(), "sess-3", "task-3", domain.Execution{
		ID:     "exec-3",
		NodeID: "node-3",
		Status: domain.ExecutionStatusRunning,
	}, domain.ExecutionChunk{
		Stream: domain.ExecutionStreamStdout,
		Text:   "line\n",
	}); err != nil {
		t.Fatalf("ExecutionChunk() error = %v", err)
	}

	msg := mustDrainOne(t, activeClient.Messages())
	var event ExecutionChunkEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if event.Type != "execution.chunk" || event.TaskID != "task-3" || event.ExecutionID != "exec-3" || event.Chunk.Text != "line\n" {
		t.Fatalf("event = %#v", event)
	}
}

func TestPublisherThreadTargetClearedUsesTimelineScope(t *testing.T) {
	hub := infraws.NewMemoryHub()
	activeClient := infraws.NewMemoryClient("client-active", infraws.ClientKindUI, 4)
	hub.Register(activeClient)

	registry := infraws.NewMemorySessionRegistry(hub)
	registry.SetActive("client-active", "sess-clear")

	publisher := &Publisher{
		Registry: registry,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 19, 40, 0, 0, time.UTC) },
	}

	if err := publisher.ThreadTargetCleared(context.Background(), domain.Session{
		ID:       "sess-clear",
		Revision: 8,
		ActiveTargetContext: domain.ActiveTargetContext{
			Status: domain.TargetStatusUnset,
		},
	}); err != nil {
		t.Fatalf("ThreadTargetCleared() error = %v", err)
	}

	msg := mustDrainOne(t, activeClient.Messages())
	var event ThreadTargetClearedEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if event.Type != "thread.target.cleared" || event.EventScope != EventScopeTimeline || event.Revision != 8 || event.TargetContext.Status != domain.TargetStatusUnset {
		t.Fatalf("event = %#v", event)
	}
}

func TestPublisherExecutionFinishedUsesTimelineScope(t *testing.T) {
	hub := infraws.NewMemoryHub()
	activeClient := infraws.NewMemoryClient("client-active", infraws.ClientKindUI, 4)
	hub.Register(activeClient)

	registry := infraws.NewMemorySessionRegistry(hub)
	registry.SetActive("client-active", "sess-4")

	publisher := &Publisher{
		Registry: registry,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 19, 45, 0, 0, time.UTC) },
	}

	if err := publisher.ExecutionFinished(context.Background(), "sess-4", "task-4", domain.Execution{
		ID:     "exec-4",
		NodeID: "node-4",
		Status: domain.ExecutionStatusFailed,
	}); err != nil {
		t.Fatalf("ExecutionFinished() error = %v", err)
	}

	msg := mustDrainOne(t, activeClient.Messages())
	var event ExecutionFinishedEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if event.Type != "execution.finished" || event.TaskID != "task-4" || event.ExecutionID != "exec-4" || event.Status != domain.ExecutionStatusFailed {
		t.Fatalf("event = %#v", event)
	}
}

func TestPublisherLLMSSEEventUsesTimelineScope(t *testing.T) {
	hub := infraws.NewMemoryHub()
	activeClient := infraws.NewMemoryClient("client-active", infraws.ClientKindUI, 4)
	hub.Register(activeClient)

	registry := infraws.NewMemorySessionRegistry(hub)
	registry.SetActive("client-active", "sess-llm")

	publisher := &Publisher{
		Registry: registry,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 20, 0, 0, 0, time.UTC) },
	}

	if err := publisher.LLMSSEEvent(context.Background(), "sess-llm", "resp-1", 3, "response.output_text.delta", json.RawMessage(`{"delta":"hi"}`)); err != nil {
		t.Fatalf("LLMSSEEvent() error = %v", err)
	}

	msg := mustDrainOne(t, activeClient.Messages())
	var event LLMSSEEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if event.Type != "llm.sse.event" || event.EventScope != EventScopeTimeline || event.ResponseID != "resp-1" || event.SequenceNumber != 3 || event.UpstreamEventType != "response.output_text.delta" {
		t.Fatalf("event = %#v", event)
	}
}

func TestPublisherLLMResponseCompletedUsesTimelineScope(t *testing.T) {
	hub := infraws.NewMemoryHub()
	activeClient := infraws.NewMemoryClient("client-active", infraws.ClientKindUI, 4)
	hub.Register(activeClient)

	registry := infraws.NewMemorySessionRegistry(hub)
	registry.SetActive("client-active", "sess-llm-complete")

	publisher := &Publisher{
		Registry: registry,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 20, 5, 0, 0, time.UTC) },
	}

	if err := publisher.LLMResponseCompleted(context.Background(), "sess-llm-complete", "resp-2", json.RawMessage(`{"id":"resp-2","output_text":"done"}`)); err != nil {
		t.Fatalf("LLMResponseCompleted() error = %v", err)
	}

	msg := mustDrainOne(t, activeClient.Messages())
	var event LLMResponseCompletedEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if event.Type != "llm.response.completed" || event.EventScope != EventScopeTimeline || event.ResponseID != "resp-2" {
		t.Fatalf("event = %#v", event)
	}
}

func mustDrainOne(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	got := drainQueue(ch)
	if len(got) != 1 {
		t.Fatalf("drained messages = %#v, want exactly one", got)
	}
	return got[0]
}

func drainQueue(ch <-chan []byte) [][]byte {
	out := make([][]byte, 0)
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, append([]byte(nil), msg...))
		default:
			return out
		}
	}
}
