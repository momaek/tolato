package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/policy"
	"github.com/momaek/tolato/internal/server/domain"
	"github.com/momaek/tolato/internal/server/infra"
	infralock "github.com/momaek/tolato/internal/server/infra/lock"
	"github.com/momaek/tolato/internal/server/infra/store/memory"
)

func TestHandleUserMessageAssistantText(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 0, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"msg-1", "row-1", "msg-2", "row-2"}}
	events := &stubEventPublisher{}

	session := domain.Session{
		ID:        "sess-1",
		Title:     "Session 1",
		Status:    domain.SessionStatusIdle,
		Revision:  1,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}
	if err := store.Sessions.Create(context.Background(), session); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	llm := &fakeLLM{
		outputs: []ModelTurnOutput{assistantTurn("hello", true, false)},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
	}, llm, fakeTools{}, clock, &idgen, WithEventPublisher(events))

	if err := rt.HandleUserMessage(context.Background(), "sess-1", "ping", "client-1"); err != nil {
		t.Fatalf("HandleUserMessage() error = %v", err)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusIdle {
		t.Fatalf("session status = %q, want idle", gotSession.Status)
	}

	msgs, err := store.ThreadMessages.ListBySession(context.Background(), "sess-1", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(messages) error = %v", err)
	}
	if len(msgs) != 2 || msgs[1].Content != "hello" {
		t.Fatalf("messages = %#v, want assistant reply", msgs)
	}
	if len(events.sessionStatuses) != 2 || events.sessionStatuses[0] != domain.SessionStatusRunning || events.sessionStatuses[1] != domain.SessionStatusIdle {
		t.Fatalf("session statuses = %#v, want running then idle", events.sessionStatuses)
	}
	if len(events.timelineKinds) != 2 || events.timelineKinds[0] != domain.TimelineRowKindUserMessage || events.timelineKinds[1] != domain.TimelineRowKindAssistantText {
		t.Fatalf("timeline kinds = %#v, want user_message then assistant_text", events.timelineKinds)
	}
	if len(events.llmSSEEvents) == 0 || len(events.llmCompleted) != 1 {
		t.Fatalf("llm events = %#v completed = %#v, want streamed output and one completion", events.llmSSEEvents, events.llmCompleted)
	}
}

func TestHandleUserMessageAssistantTextSkipsSyntheticStreamWhenAlreadyStreamed(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 1, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"msg-1", "row-1", "msg-2", "row-2"}}
	events := &stubEventPublisher{
		llmSSEEvents: []stubLLMSSEEvent{{
			sessionID:         "sess-streamed",
			responseID:        "resp-1",
			sequenceNumber:    1,
			upstreamEventType: "response.output_text.delta",
			rawEvent:          json.RawMessage(`{"delta":"hello"}`),
		}},
		llmCompleted: []json.RawMessage{json.RawMessage(`{"id":"resp-1"}`)},
	}

	session := domain.Session{
		ID:        "sess-streamed",
		Title:     "Session streamed",
		Status:    domain.SessionStatusIdle,
		Revision:  1,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}
	if err := store.Sessions.Create(context.Background(), session); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	llm := &fakeLLM{
		outputs: []ModelTurnOutput{assistantTurn("hello", true, true)},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
	}, llm, fakeTools{}, clock, &idgen, WithEventPublisher(events))

	if err := rt.HandleUserMessage(context.Background(), "sess-streamed", "ping", "client-streamed"); err != nil {
		t.Fatalf("HandleUserMessage() error = %v", err)
	}

	if len(events.llmSSEEvents) != 1 || len(events.llmCompleted) != 1 {
		t.Fatalf("llm events = %#v completed = %#v, want no extra synthetic stream", events.llmSSEEvents, events.llmCompleted)
	}
}

func TestHandleUserMessageToolCallThenAssistant(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 8, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"msg-1", "row-1", "toolcall-1", "row-2", "toolresult-1", "row-3", "msg-2", "row-4"}}

	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:        "sess-4",
		Title:     "Session 4",
		Status:    domain.SessionStatusIdle,
		Revision:  1,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	llm := &fakeLLM{
		outputs: []ModelTurnOutput{
			toolTurn("list_nodes", map[string]string{"region": "asia"}),
			assistantTurn("Found one node in asia", true, false),
		},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
	}, llm, fakeTools{
		result: policy.ToolResult{
			MetaText:    "listed 1 nodes",
			ToolMessage: mustRaw(t, map[string]any{"nodes": []string{"jp-tokyo-01"}}),
		},
	}, clock, &idgen)

	if err := rt.HandleUserMessage(context.Background(), "sess-4", "list asia nodes", "client-4"); err != nil {
		t.Fatalf("HandleUserMessage() error = %v", err)
	}

	rows, err := store.Timelines.ListBySession(context.Background(), "sess-4", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(timelines) error = %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("timeline rows = %d, want 4", len(rows))
	}
	if rows[1].Kind != domain.TimelineRowKindToolCallMeta || rows[2].Kind != domain.TimelineRowKindToolResultMeta || rows[3].Kind != domain.TimelineRowKindAssistantText {
		t.Fatalf("unexpected timeline sequence = %#v", rows)
	}
}

func TestHandleUserMessageToolCallErrorContinuesLoop(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 9, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"msg-1", "row-1", "toolcall-1", "row-2", "toolresult-1", "row-3", "msg-2", "row-4"}}

	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:        "sess-err",
		Title:     "Session err",
		Status:    domain.SessionStatusIdle,
		Revision:  1,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	llm := &fakeLLM{
		outputs: []ModelTurnOutput{
			toolTurn("run_on_node", map[string]string{"target": "nonexistent", "command": "system_status"}),
			assistantTurn("Sorry, that node was not found.", true, false),
		},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
	}, llm, fakeTools{
		err: errors.New("node not found"),
	}, clock, &idgen)

	// Tool error should NOT fail the runtime; it should feed the error back to the LLM.
	if err := rt.HandleUserMessage(context.Background(), "sess-err", "check nonexistent", "client-err"); err != nil {
		t.Fatalf("HandleUserMessage() error = %v", err)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-err")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusIdle {
		t.Fatalf("session status = %q, want idle (error fed back to LLM)", gotSession.Status)
	}
}

func TestHandleUserMessageRejectsBusySession(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 10, 0, 0, time.UTC)}
	idgen := stubIDGen{}
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:        "sess-3",
		Title:     "Busy Session",
		Status:    domain.SessionStatusRunning,
		Revision:  1,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
	}, &fakeLLM{}, fakeTools{}, clock, &idgen)

	err := rt.HandleUserMessage(context.Background(), "sess-3", "hello", "client-3")
	if !errors.Is(err, domain.ErrSessionBusy) {
		t.Fatalf("HandleUserMessage() error = %v, want ErrSessionBusy", err)
	}
}

func TestHandleUserMessageRejectsWhenSessionLockIsHeld(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 12, 0, 0, time.UTC)}
	idgen := stubIDGen{}
	locks := infralock.NewMemoryLockManager()
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:        "sess-lock",
		Title:     "Locked Session",
		Status:    domain.SessionStatusIdle,
		Revision:  1,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	unlock, err := locks.LockSession(context.Background(), "sess-lock")
	if err != nil {
		t.Fatalf("LockSession() error = %v", err)
	}
	defer unlock()

	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
	}, &fakeLLM{}, fakeTools{}, clock, &idgen, WithLockManager(locks))

	err = rt.HandleUserMessage(context.Background(), "sess-lock", "hello", "client-lock")
	if !errors.Is(err, domain.ErrSessionBusy) {
		t.Fatalf("HandleUserMessage() error = %v, want ErrSessionBusy", err)
	}
}

func TestHandleUserMessageDeduplicatesClientMessageID(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 14, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"msg-1", "row-1", "msg-2", "row-2"}}
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:        "sess-dedupe",
		Title:     "Dedupe Session",
		Status:    domain.SessionStatusIdle,
		Revision:  1,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	llm := &fakeLLM{
		outputs: []ModelTurnOutput{assistantTurn("done", true, false)},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
	}, llm, fakeTools{}, clock, &idgen)

	if err := rt.HandleUserMessage(context.Background(), "sess-dedupe", "ping", "client-dedupe"); err != nil {
		t.Fatalf("first HandleUserMessage() error = %v", err)
	}
	if err := rt.HandleUserMessage(context.Background(), "sess-dedupe", "ping", "client-dedupe"); err != nil {
		t.Fatalf("second HandleUserMessage() error = %v", err)
	}

	msgs, err := store.ThreadMessages.ListBySession(context.Background(), "sess-dedupe", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(messages) error = %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("message count = %d, want 2 without duplicate user message", len(msgs))
	}
	if msgs[0].ClientMessageID == nil || *msgs[0].ClientMessageID != "client-dedupe" {
		t.Fatalf("client message id = %#v, want persisted client-dedupe", msgs[0].ClientMessageID)
	}

	rows, err := store.Timelines.ListBySession(context.Background(), "sess-dedupe", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(timelines) error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("timeline row count = %d, want 2 without duplicate row", len(rows))
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type fakeLLM struct {
	outputs []ModelTurnOutput
	index   int
}

func (f *fakeLLM) RunTurn(ctx context.Context, input ModelTurnInput, tools []agentapi.ToolSpec) (ModelTurnOutput, error) {
	_ = ctx
	_ = input
	_ = tools
	if f.index >= len(f.outputs) {
		return ModelTurnOutput{}, ErrEmptyModelOutput
	}
	out := f.outputs[f.index]
	f.index++
	return out, nil
}

type fakeTools struct {
	result policy.ToolResult
	err    error
}

func (f fakeTools) Definitions() []agentapi.ToolSpec {
	return []agentapi.ToolSpec{
		agentapi.NewFunctionTool("list_nodes", "List nodes", map[string]any{}),
		agentapi.NewFunctionTool("run_on_node", "Run on node", map[string]any{}),
	}
}

func (f fakeTools) Call(ctx context.Context, input agentapi.Item) (policy.ToolResult, error) {
	_ = ctx
	if f.err != nil {
		return policy.ToolResult{}, f.err
	}
	out := f.result
	if out.OutputItem.CallID == "" {
		out.OutputItem = agentapi.FunctionCallOutput(input.CallID, string(out.ToolMessage))
	}
	return out, nil
}

func assistantTurn(text string, done bool, streamed bool) ModelTurnOutput {
	raw, err := json.Marshal([]agentapi.ContentPart{{
		Type: "output_text",
		Text: text,
	}})
	if err != nil {
		panic(err)
	}
	return ModelTurnOutput{
		Items: []agentapi.Item{{
			Type:    "message",
			Role:    "assistant",
			Content: raw,
		}},
		Done:     done,
		Streamed: streamed,
	}
}

func toolTurn(name string, args any) ModelTurnOutput {
	raw, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return ModelTurnOutput{
		Items: []agentapi.Item{{
			Type:      "function_call",
			Name:      name,
			Arguments: string(raw),
			CallID:    "call_" + name,
		}},
		Done: false,
	}
}

type stubIDGen struct {
	values []string
	index  int
}

func (s *stubIDGen) NewID(prefix string) string {
	if s.index >= len(s.values) {
		return prefix + "-overflow"
	}
	value := s.values[s.index]
	s.index++
	return value
}

type stubEventPublisher struct {
	sessionStatuses []domain.SessionStatus
	timelineKinds   []domain.TimelineRowKind
	llmSSEEvents    []stubLLMSSEEvent
	llmCompleted    []json.RawMessage
}

type stubLLMSSEEvent struct {
	sessionID         string
	responseID        string
	sequenceNumber    int
	upstreamEventType string
	rawEvent          json.RawMessage
}

func (s *stubEventPublisher) SessionStateUpdated(_ context.Context, session domain.Session) error {
	s.sessionStatuses = append(s.sessionStatuses, session.Status)
	return nil
}

func (s *stubEventPublisher) TimelineRowAppended(_ context.Context, _ domain.Session, row domain.TimelineRow) error {
	s.timelineKinds = append(s.timelineKinds, row.Kind)
	return nil
}

func (s *stubEventPublisher) LLMSSEEvent(_ context.Context, sessionID string, responseID string, sequenceNumber int, upstreamEventType string, rawEvent json.RawMessage) error {
	s.llmSSEEvents = append(s.llmSSEEvents, stubLLMSSEEvent{
		sessionID:         sessionID,
		responseID:        responseID,
		sequenceNumber:    sequenceNumber,
		upstreamEventType: upstreamEventType,
		rawEvent:          append(json.RawMessage(nil), rawEvent...),
	})
	return nil
}

func (s *stubEventPublisher) LLMResponseCompleted(_ context.Context, _ string, _ string, rawResponse json.RawMessage) error {
	s.llmCompleted = append(s.llmCompleted, append(json.RawMessage(nil), rawResponse...))
	return nil
}

func mustRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}
