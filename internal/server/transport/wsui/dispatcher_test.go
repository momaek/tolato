package wsui

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appruntime "github.com/momaek/tolato/internal/server/app/runtime"
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
	resp, err := dispatcher.Dispatch(context.Background(), raw)
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
	resp, err := dispatcher.Dispatch(context.Background(), raw)
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

func TestDispatcherSessionTargetClear(t *testing.T) {
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
	if resp.Type != TypeSessionActionAccepted {
		t.Fatalf("response type = %q, want %q", resp.Type, TypeSessionActionAccepted)
	}
	if rt.clearSessionID != "sess-clear" || rt.clearKey != "idem-clear" {
		t.Fatalf("runtime clear call = %#v, want sess-clear/idem-clear", rt)
	}
}

type fakeSessionService struct {
	items                       []appsession.SessionListItem
	snapshot                    appsession.Snapshot
	page                        appsession.TimelinePage
	subscriptionClientID        string
	subscriptionActiveSessionID string
	subscriptionWatchSessionIDs []string
}

type fakeRuntime struct {
	sessionID       string
	text            string
	clientMessageID string
	clearSessionID  string
	clearKey        string
	err             error
}

func (f *fakeRuntime) HandleUserMessage(ctx context.Context, sessionID string, text string, clientMessageID string) error {
	_ = ctx
	f.sessionID = sessionID
	f.text = text
	f.clientMessageID = clientMessageID
	return f.err
}

func (f *fakeRuntime) ResumeAfterTargetConfirmation(ctx context.Context, sessionID string, action appruntime.ConfirmTargetAction) error {
	_ = ctx
	_ = sessionID
	_ = action
	return nil
}

func (f *fakeRuntime) ClearTargetContext(ctx context.Context, sessionID string, idempotencyKey string) error {
	_ = ctx
	f.clearSessionID = sessionID
	f.clearKey = idempotencyKey
	return nil
}

func (f *fakeRuntime) ResumeAfterApproval(ctx context.Context, sessionID string, action appruntime.ApprovalAction) error {
	_ = ctx
	_ = sessionID
	_ = action
	return nil
}

func (f *fakeSessionService) ListSessions(context.Context) ([]appsession.SessionListItem, error) {
	return f.items, nil
}

func (f *fakeSessionService) BuildSnapshot(context.Context, string) (appsession.Snapshot, error) {
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
