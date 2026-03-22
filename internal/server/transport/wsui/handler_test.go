package wsui

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appsession "github.com/momaek/tolato/internal/server/app/session"
	"github.com/momaek/tolato/internal/server/domain"
	"github.com/momaek/tolato/internal/server/infra/store/memory"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

func TestHandlerConnectRegistersClientAndReturnsReady(t *testing.T) {
	hub := infraws.NewMemoryHub()
	client := infraws.NewMemoryClient("client-1", infraws.ClientKindUI, 4)
	auth := &fakeAuthenticator{}
	handler := Handler{
		Auth: auth,
		Hub:  hub,
		Now:  func() time.Time { return time.Date(2026, 3, 22, 17, 0, 0, 0, time.UTC) },
	}

	raw, err := handler.Connect(context.Background(), client)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if _, ok := hub.Client("client-1"); !ok {
		t.Fatal("expected client to be registered in hub")
	}
	if auth.clientID != "client-1" {
		t.Fatalf("auth client = %q, want client-1", auth.clientID)
	}

	var ready ConnectionReady
	if err := json.Unmarshal(raw, &ready); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if ready.Type != TypeConnectionReady || ready.Timestamp != "2026-03-22T17:00:00Z" {
		t.Fatalf("ready = %#v, want connection.ready timestamp", ready)
	}
}

func TestHandlerHandleInjectsClientID(t *testing.T) {
	sessions := &fakeSessionService{}
	handler := Handler{
		Dispatcher: Dispatcher{
			Sessions: sessions,
		},
	}

	raw, err := handler.Handle(context.Background(), "client-9", mustJSON(t, RequestEnvelope{
		Type:      TypeSubscriptionsUpdate,
		RequestID: "req-9",
		Payload: mustPayload(t, SubscriptionsUpdateRequest{
			ActiveSessionID: "sess-a",
			WatchSessionIDs: []string{"sess-b"},
		}),
	}))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	var resp ResponseEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Type != TypeSessionActionAccepted {
		t.Fatalf("response type = %q, want %q", resp.Type, TypeSessionActionAccepted)
	}
	if sessions.subscriptionClientID != "client-9" {
		t.Fatalf("subscription client = %q, want client-9", sessions.subscriptionClientID)
	}
}

func TestHandlerHandleMapsDomainErrorsToErrorEnvelope(t *testing.T) {
	handler := Handler{
		Dispatcher: Dispatcher{
			Runtime: &fakeRuntime{err: domain.ErrSessionBusy},
		},
	}

	raw, err := handler.Handle(context.Background(), "client-2", mustJSON(t, RequestEnvelope{
		Type:      TypeSessionMessageSubmit,
		RequestID: "req-busy",
		Payload: mustPayload(t, SessionMessageSubmitRequest{
			SessionID:       "sess-2",
			Text:            "run check",
			ClientMessageID: "client-busy",
		}),
	}))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	var resp ResponseEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Type != TypeError || resp.Error == nil {
		t.Fatalf("response = %#v, want error envelope", resp)
	}
	if resp.Error.Code != "session_busy" {
		t.Fatalf("error code = %q, want session_busy", resp.Error.Code)
	}
}

func TestHandlerDisconnectClearsSubscriptionsBeforeReconnect(t *testing.T) {
	hub := infraws.NewMemoryHub()
	registry := infraws.NewMemorySessionRegistry(hub)
	store := memory.NewStore()
	sessions := appsession.NewService(appsession.Repositories{
		Sessions:      store.Sessions,
		Timelines:     store.Timelines,
		Tasks:         store.Tasks,
		Executions:    store.Executions,
		Subscriptions: registry,
	})
	handler := Handler{
		Hub:           hub,
		Subscriptions: registry,
		Dispatcher: Dispatcher{
			Sessions: sessions,
		},
		Now: func() time.Time { return time.Date(2026, 3, 22, 17, 10, 0, 0, time.UTC) },
	}

	firstClient := infraws.NewMemoryClient("client-1", infraws.ClientKindUI, 8)
	if _, err := handler.Connect(context.Background(), firstClient); err != nil {
		t.Fatalf("first Connect() error = %v", err)
	}
	if _, err := handler.Handle(context.Background(), "client-1", mustJSON(t, RequestEnvelope{
		Type:      TypeSubscriptionsUpdate,
		RequestID: "req-sub-1",
		Payload: mustPayload(t, SubscriptionsUpdateRequest{
			ActiveSessionID: "sess-a",
			WatchSessionIDs: []string{"sess-b"},
		}),
	})); err != nil {
		t.Fatalf("Handle(subscriptions.update) error = %v", err)
	}

	handler.Disconnect("client-1")

	secondClient := infraws.NewMemoryClient("client-1", infraws.ClientKindUI, 8)
	if _, err := handler.Connect(context.Background(), secondClient); err != nil {
		t.Fatalf("second Connect() error = %v", err)
	}

	registry.PublishToSession("sess-a", []byte("timeline"))
	registry.PublishSummary("sess-b", []byte("summary"))
	if got := drainQueue(secondClient.Messages()); len(got) != 0 {
		t.Fatalf("reconnected client should not inherit old subscriptions: %#v", got)
	}

	if _, err := handler.Handle(context.Background(), "client-1", mustJSON(t, RequestEnvelope{
		Type:      TypeSubscriptionsUpdate,
		RequestID: "req-sub-2",
		Payload: mustPayload(t, SubscriptionsUpdateRequest{
			ActiveSessionID: "sess-a",
			WatchSessionIDs: []string{"sess-b"},
		}),
	})); err != nil {
		t.Fatalf("Handle(subscriptions.update reconnect) error = %v", err)
	}

	registry.PublishToSession("sess-a", []byte("timeline-2"))
	registry.PublishSummary("sess-b", []byte("summary-2"))
	got := drainQueue(secondClient.Messages())
	if len(got) != 2 {
		t.Fatalf("reconnected client should receive events after resubscribe: %#v", got)
	}
}

func TestReconnectFlowRestoresSnapshotAfterListAndResubscribe(t *testing.T) {
	store := memory.NewStore()
	hub := infraws.NewMemoryHub()
	registry := infraws.NewMemorySessionRegistry(hub)
	now := time.Date(2026, 3, 22, 17, 20, 0, 0, time.UTC)
	taskID := "task-a"
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:     "sess-a",
		Title:  "Tokyo Session",
		Status: domain.SessionStatusPausedWaitApproval,
		ActiveTargetContext: domain.ActiveTargetContext{
			Status:       domain.TargetStatusConfirmed,
			Scope:        domain.TargetScopeSingle,
			NodeIDs:      []string{"jp-tokyo-01"},
			DisplayLabel: "jp-tokyo-01",
			Source:       domain.TargetSourceUserExplicit,
			Confidence:   1,
		},
		PendingAction: &domain.PendingAction{
			Type:    domain.PendingActionTypeApproval,
			Payload: json.RawMessage(`{"taskId":"task-a"}`),
		},
		CurrentTaskID: &taskID,
		Revision:      18,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}
	if err := store.Timelines.Append(context.Background(), domain.TimelineRow{
		ID:        "row-approval",
		SessionID: "sess-a",
		Kind:      domain.TimelineRowKindApproval,
		Text:      "approval pending",
		CreatedAt: now,
		TaskID:    &taskID,
	}); err != nil {
		t.Fatalf("Append(timeline) error = %v", err)
	}
	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:             taskID,
		SessionID:      "sess-a",
		InputText:      "restart nginx",
		Status:         domain.TaskStatusWaitingApproval,
		ApprovalStatus: domain.ApprovalStatusPending,
		RiskLevel:      domain.RiskLevelHigh,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}

	sessions := appsession.NewService(appsession.Repositories{
		Sessions:      store.Sessions,
		Timelines:     store.Timelines,
		Tasks:         store.Tasks,
		Executions:    store.Executions,
		Subscriptions: registry,
	})
	handler := Handler{
		Hub:           hub,
		Subscriptions: registry,
		Dispatcher: Dispatcher{
			Sessions: sessions,
		},
		Now: func() time.Time { return now },
	}

	client := infraws.NewMemoryClient("client-reconnect", infraws.ClientKindUI, 8)
	if _, err := handler.Connect(context.Background(), client); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	handler.Disconnect("client-reconnect")

	client = infraws.NewMemoryClient("client-reconnect", infraws.ClientKindUI, 8)
	if _, err := handler.Connect(context.Background(), client); err != nil {
		t.Fatalf("Reconnect() error = %v", err)
	}

	listRaw, err := handler.Handle(context.Background(), "client-reconnect", mustJSON(t, RequestEnvelope{
		Type:      TypeSessionsListRequest,
		RequestID: "req-list",
	}))
	if err != nil {
		t.Fatalf("Handle(sessions.list.request) error = %v", err)
	}
	var listResp ResponseEnvelope
	if err := json.Unmarshal(listRaw, &listResp); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	listPayload, ok := listResp.Payload.(map[string]any)
	if !ok {
		t.Fatalf("list payload = %#v, want object payload", listResp.Payload)
	}
	items, ok := listPayload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("list items = %#v, want one session item", listPayload["items"])
	}

	snapshotRaw, err := handler.Handle(context.Background(), "client-reconnect", mustJSON(t, RequestEnvelope{
		Type:      TypeSessionSnapshotRequest,
		RequestID: "req-snapshot",
		Payload:   mustPayload(t, SessionSnapshotRequest{SessionID: "sess-a"}),
	}))
	if err != nil {
		t.Fatalf("Handle(session.snapshot.request) error = %v", err)
	}
	var snapshotResp ResponseEnvelope
	if err := json.Unmarshal(snapshotRaw, &snapshotResp); err != nil {
		t.Fatalf("json.Unmarshal(snapshot) error = %v", err)
	}
	snapshotPayload, ok := snapshotResp.Payload.(map[string]any)
	if !ok {
		t.Fatalf("snapshot payload = %#v, want object payload", snapshotResp.Payload)
	}
	snapshot, ok := snapshotPayload["snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot body = %#v, want snapshot object", snapshotPayload["snapshot"])
	}
	sessionView, ok := snapshot["session"].(map[string]any)
	if !ok || sessionView["id"] != "sess-a" || sessionView["status"] != string(domain.SessionStatusPausedWaitApproval) {
		t.Fatalf("snapshot session = %#v, want paused approval session", snapshot["session"])
	}
	timelineView, ok := snapshot["timeline"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot timeline = %#v, want timeline object", snapshot["timeline"])
	}
	rows, ok := timelineView["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("timeline rows = %#v, want one approval row", timelineView["rows"])
	}

	if _, err := handler.Handle(context.Background(), "client-reconnect", mustJSON(t, RequestEnvelope{
		Type:      TypeSubscriptionsUpdate,
		RequestID: "req-subscribe",
		Payload: mustPayload(t, SubscriptionsUpdateRequest{
			ActiveSessionID: "sess-a",
			WatchSessionIDs: []string{"sess-b"},
		}),
	})); err != nil {
		t.Fatalf("Handle(subscriptions.update) error = %v", err)
	}
	registry.PublishToSession("sess-a", []byte("timeline-after-resubscribe"))
	if got := drainQueue(client.Messages()); len(got) != 1 || string(got[0]) != "timeline-after-resubscribe" {
		t.Fatalf("reconnected client should receive timeline after resubscribe: %#v", got)
	}
}

type fakeAuthenticator struct {
	clientID string
}

func (f *fakeAuthenticator) AuthenticateUI(_ context.Context, client infraws.Client) error {
	f.clientID = client.ID()
	return nil
}
