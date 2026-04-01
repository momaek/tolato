package wsui

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appsession "github.com/momaek/tolato/internal/server/app/session"
	"github.com/momaek/tolato/internal/server/domain"
)

func TestDispatcherSessionsList(t *testing.T) {
	sessions := &fakeSessionService{
		items: []appsession.SessionListItem{{
			SessionID:           "sess-1",
			Title:               "Tokyo",
			Status:              domain.SessionStatusRunning,
			UpdatedAt:           "2026-03-22T10:00:00Z",
			ActiveTargetSummary: "jp-tokyo-01",
		}},
	}
	dispatcher := Dispatcher{
		Sessions: sessions,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC) },
	}

	raw := mustJSON(t, RequestEnvelope{Type: TypeSessionsListRequest, RequestID: "req-1"})
	resp, err := dispatcher.Dispatch(WithClientID(context.Background(), "client-list"), raw)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if resp.Type != TypeSessionsListResponse {
		t.Fatalf("response type = %q, want %q", resp.Type, TypeSessionsListResponse)
	}
	payload, ok := resp.Payload.(SessionsListResponse)
	if !ok || len(payload.Items) != 1 {
		t.Fatalf("payload = %#v, want one session item", resp.Payload)
	}
	if sessions.listClientID != "client-list" {
		t.Fatalf("list client id = %q, want client-list", sessions.listClientID)
	}
}

func TestDispatcherSessionCreate(t *testing.T) {
	sessions := &fakeSessionService{
		createSessionID: "sess-new",
	}
	dispatcher := Dispatcher{
		Sessions: sessions,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 10, 5, 0, 0, time.UTC) },
	}

	raw := mustJSON(t, RequestEnvelope{
		Type:      TypeSessionCreate,
		RequestID: "req-create",
		Payload:   mustPayload(t, SessionCreateRequest{Title: "Custom Session"}),
	})
	resp, err := dispatcher.Dispatch(context.Background(), raw)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if resp.Type != TypeSessionActionAccepted {
		t.Fatalf("response type = %q, want %q", resp.Type, TypeSessionActionAccepted)
	}
	if sessions.createdSessionTitle != "Custom Session" {
		t.Fatalf("created session title = %q, want Custom Session", sessions.createdSessionTitle)
	}
}

func TestDispatcherSessionDelete(t *testing.T) {
	sessions := &fakeSessionService{}
	dispatcher := Dispatcher{
		Sessions: sessions,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 10, 6, 0, 0, time.UTC) },
	}

	raw := mustJSON(t, RequestEnvelope{
		Type:      TypeSessionDelete,
		RequestID: "req-delete",
		Payload:   mustPayload(t, SessionDeleteRequest{SessionID: "sess-delete"}),
	})
	resp, err := dispatcher.Dispatch(context.Background(), raw)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if resp.Type != TypeSessionActionAccepted {
		t.Fatalf("response type = %q, want %q", resp.Type, TypeSessionActionAccepted)
	}
	if sessions.deletedSessionID != "sess-delete" {
		t.Fatalf("deleted session id = %q, want sess-delete", sessions.deletedSessionID)
	}
}

func TestDispatcherSessionSnapshot(t *testing.T) {
	sessions := &fakeSessionService{
		snapshot: appsession.Snapshot{
			Session: appsession.SnapshotSession{
				ID:       "sess-1",
				Title:    "Tokyo",
				Status:   domain.SessionStatusPausedWaitApproval,
				Revision: 4,
			},
		},
	}
	dispatcher := Dispatcher{
		Sessions: sessions,
	}

	raw := mustJSON(t, RequestEnvelope{
		Type:      TypeSessionSnapshotRequest,
		RequestID: "req-2",
		Payload:   mustPayload(t, SessionSnapshotRequest{SessionID: "sess-1"}),
	})
	resp, err := dispatcher.Dispatch(WithClientID(context.Background(), "client-snapshot"), raw)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if resp.Type != TypeSessionSnapshotResponse {
		t.Fatalf("response type = %q, want %q", resp.Type, TypeSessionSnapshotResponse)
	}
	payload, ok := resp.Payload.(SessionSnapshotResponse)
	if !ok || payload.Snapshot.Session.ID != "sess-1" {
		t.Fatalf("payload = %#v, want sess-1 snapshot", resp.Payload)
	}
	if sessions.snapshotClientID != "client-snapshot" {
		t.Fatalf("snapshot client id = %q, want client-snapshot", sessions.snapshotClientID)
	}
}

func TestDispatcherSessionRows(t *testing.T) {
	sessions := &fakeSessionService{
		page: appsession.TimelinePage{
			Rows: []domain.TimelineRow{{ID: "row-1"}, {ID: "row-2"}},
		},
	}
	dispatcher := Dispatcher{
		Sessions: sessions,
	}

	raw := mustJSON(t, RequestEnvelope{
		Type:      TypeSessionRowsRequest,
		RequestID: "req-3",
		Payload:   mustPayload(t, SessionRowsRequest{SessionID: "sess-1", Before: "row-9", Limit: 20}),
	})
	resp, err := dispatcher.Dispatch(context.Background(), raw)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if resp.Type != TypeSessionRowsResponse {
		t.Fatalf("response type = %q, want %q", resp.Type, TypeSessionRowsResponse)
	}
	payload, ok := resp.Payload.(SessionRowsResponse)
	if !ok || len(payload.Page.Rows) != 2 {
		t.Fatalf("payload = %#v, want two rows", resp.Payload)
	}
}

func TestDispatcherSessionMessageSubmit(t *testing.T) {
	rt := &fakeRuntime{}
	dispatcher := Dispatcher{
		Runtime: rt,
		Now:     func() time.Time { return time.Date(2026, 3, 22, 15, 0, 0, 0, time.UTC) },
	}

	raw := mustJSON(t, RequestEnvelope{
		Type:      TypeSessionMessageSubmit,
		RequestID: "req-4",
		Payload: mustPayload(t, SessionMessageSubmitRequest{
			SessionID:       "sess-1",
			Text:            "check nginx",
			ClientMessageID: "client-1",
		}),
	})
	resp, err := dispatcher.Dispatch(context.Background(), raw)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if resp.Type != TypeSessionActionAccepted {
		t.Fatalf("response type = %q, want %q", resp.Type, TypeSessionActionAccepted)
	}
	if rt.sessionID != "sess-1" || rt.text != "check nginx" || rt.clientMessageID != "client-1" {
		t.Fatalf("runtime call = %#v, want submit payload", rt)
	}
}

func TestDispatcherSubscriptionsUpdate(t *testing.T) {
	sessions := &fakeSessionService{}
	dispatcher := Dispatcher{
		Sessions: sessions,
		Now:      func() time.Time { return time.Date(2026, 3, 22, 16, 0, 0, 0, time.UTC) },
	}

	raw := mustJSON(t, RequestEnvelope{
		Type:      TypeSubscriptionsUpdate,
		RequestID: "req-5",
		Payload: mustPayload(t, SubscriptionsUpdateRequest{
			ActiveSessionID: "sess-a",
			WatchSessionIDs: []string{"sess-b", "sess-c"},
		}),
	})
	resp, err := dispatcher.Dispatch(WithClientID(context.Background(), "client-42"), raw)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if resp.Type != TypeSessionActionAccepted {
		t.Fatalf("response type = %q, want %q", resp.Type, TypeSessionActionAccepted)
	}
	if sessions.subscriptionClientID != "client-42" || sessions.subscriptionActiveSessionID != "sess-a" {
		t.Fatalf("subscription call = %#v, want client-42/sess-a", sessions)
	}
	if len(sessions.subscriptionWatchSessionIDs) != 2 || sessions.subscriptionWatchSessionIDs[0] != "sess-b" || sessions.subscriptionWatchSessionIDs[1] != "sess-c" {
		t.Fatalf("watch session ids = %#v, want sess-b/sess-c", sessions.subscriptionWatchSessionIDs)
	}
}

func TestDispatcherSessionTargetClearReturnsDeprecated(t *testing.T) {
	rt := &fakeRuntime{}
	dispatcher := Dispatcher{
		Runtime: rt,
		Now:     func() time.Time { return time.Date(2026, 3, 22, 16, 10, 0, 0, time.UTC) },
	}

	raw := mustJSON(t, RequestEnvelope{
		Type:      TypeSessionTargetClear,
		RequestID: "req-clear",
		Payload: mustPayload(t, SessionTargetClearRequest{
			SessionID:      "sess-clear",
			IdempotencyKey: "idem-clear",
		}),
	})
	resp, err := dispatcher.Dispatch(context.Background(), raw)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if resp.Type != TypeError {
		t.Fatalf("response type = %q, want %q (deprecated)", resp.Type, TypeError)
	}
	if resp.Error == nil || resp.Error.Code != "deprecated" {
		t.Fatalf("error = %#v, want deprecated", resp.Error)
	}
}

type fakeSessionService struct {
	items                       []appsession.SessionListItem
	snapshot                    appsession.Snapshot
	page                        appsession.TimelinePage
	createSessionID             string
	createdSessionTitle         string
	deletedSessionID            string
	listClientID                string
	snapshotClientID            string
	subscriptionClientID        string
	subscriptionActiveSessionID string
	subscriptionWatchSessionIDs []string
}

type fakeRuntime struct {
	sessionID       string
	text            string
	clientMessageID string
	err             error
}

func (f *fakeRuntime) HandleUserMessage(ctx context.Context, sessionID string, text string, clientMessageID string) error {
	_ = ctx
	f.sessionID = sessionID
	f.text = text
	f.clientMessageID = clientMessageID
	return f.err
}



func (f *fakeSessionService) ListSessions(_ context.Context, clientID string) ([]appsession.SessionListItem, error) {
	f.listClientID = clientID
	return f.items, nil
}

func (f *fakeSessionService) CreateSession(_ context.Context, title string) (string, error) {
	f.createdSessionTitle = title
	if f.createSessionID != "" {
		return f.createSessionID, nil
	}
	return "sess-created", nil
}

func (f *fakeSessionService) DeleteSession(_ context.Context, sessionID string) error {
	f.deletedSessionID = sessionID
	return nil
}

func (f *fakeSessionService) BuildSnapshot(_ context.Context, clientID string, _ string) (appsession.Snapshot, error) {
	f.snapshotClientID = clientID
	return f.snapshot, nil
}

func (f *fakeSessionService) ListRows(context.Context, string, domain.CursorPage) (appsession.TimelinePage, error) {
	return f.page, nil
}

func (f *fakeSessionService) UpdateSubscriptions(_ context.Context, clientID string, activeSessionID string, watchSessionIDs []string) error {
	f.subscriptionClientID = clientID
	f.subscriptionActiveSessionID = activeSessionID
	f.subscriptionWatchSessionIDs = append([]string(nil), watchSessionIDs...)
	return nil
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}

func mustPayload(t *testing.T, value any) json.RawMessage {
	t.Helper()
	return json.RawMessage(mustJSON(t, value))
}
