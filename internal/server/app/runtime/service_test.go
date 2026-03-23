package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

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
		outputs: []ModelTurnOutput{{AssistantText: strPtr("hello"), Done: true}},
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
	if gotSession.Status != domain.SessionStatusCompleted {
		t.Fatalf("session status = %q, want completed", gotSession.Status)
	}

	msgs, err := store.ThreadMessages.ListBySession(context.Background(), "sess-1", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(messages) error = %v", err)
	}
	if len(msgs) != 2 || msgs[1].Content != "hello" {
		t.Fatalf("messages = %#v, want assistant reply", msgs)
	}
	if len(events.sessionStatuses) != 2 || events.sessionStatuses[0] != domain.SessionStatusRunning || events.sessionStatuses[1] != domain.SessionStatusCompleted {
		t.Fatalf("session statuses = %#v, want running then completed", events.sessionStatuses)
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
		outputs: []ModelTurnOutput{{AssistantText: strPtr("hello"), Done: true, Streamed: true}},
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

func TestHandleUserMessageToolWaitsForTargetConfirmation(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 5, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"msg-1", "row-1", "toolcall-1", "row-2", "toolresult-1", "row-3", "row-4"}}
	events := &stubEventPublisher{}

	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:        "sess-2",
		Title:     "Session 2",
		Status:    domain.SessionStatusIdle,
		Revision:  2,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	targetCtx := domain.ActiveTargetContext{
		Status:       domain.TargetStatusPendingConfirmation,
		Scope:        domain.TargetScopeSingle,
		NodeIDs:      []string{"jp-tokyo-01"},
		DisplayLabel: "jp-tokyo-01",
		Source:       domain.TargetSourceAssistantResolved,
		Confidence:   0.88,
	}
	payload := mustRaw(t, map[string]any{"targetContext": targetCtx})
	llm := &fakeLLM{
		outputs: []ModelTurnOutput{{ToolCall: &ToolInvocation{Name: "request_target_confirmation", Args: mustRaw(t, map[string]string{"text": "tokyo"})}}},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
	}, llm, fakeTools{
		result: ToolResult{
			MetaText:             "target confirmation required",
			WaitForUser:          true,
			PendingActionType:    domain.PendingActionTypeTargetConfirmation,
			PendingActionPayload: payload,
		},
	}, clock, &idgen, WithEventPublisher(events))

	if err := rt.HandleUserMessage(context.Background(), "sess-2", "check tokyo", "client-2"); err != nil {
		t.Fatalf("HandleUserMessage() error = %v", err)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-2")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusPausedWaitTargetConfirmation {
		t.Fatalf("session status = %q, want paused_wait_target_confirmation", gotSession.Status)
	}
	if gotSession.PendingAction == nil || gotSession.PendingAction.Type != domain.PendingActionTypeTargetConfirmation {
		t.Fatalf("pending action = %#v, want target_confirmation", gotSession.PendingAction)
	}

	rows, err := store.Timelines.ListBySession(context.Background(), "sess-2", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(timelines) error = %v", err)
	}
	if len(rows) != 4 || rows[3].Kind != domain.TimelineRowKindTargetConfirmation {
		t.Fatalf("timeline rows = %#v, want target_confirmation tail row", rows)
	}
	if len(events.pendingTargets) != 1 || events.pendingTargets[0].DisplayLabel != "jp-tokyo-01" {
		t.Fatalf("pending targets = %#v, want jp-tokyo-01", events.pendingTargets)
	}
	if len(events.sessionStatuses) != 2 || events.sessionStatuses[0] != domain.SessionStatusRunning || events.sessionStatuses[1] != domain.SessionStatusPausedWaitTargetConfirmation {
		t.Fatalf("session statuses = %#v, want running then paused_wait_target_confirmation", events.sessionStatuses)
	}
}

func TestResumeAfterTargetConfirmationContinuesLoop(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 12, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"audit-1", "toolresult-1", "row-1", "msg-1", "row-2"}}
	events := &stubEventPublisher{}
	taskID := "task-1"
	pendingTarget := domain.ActiveTargetContext{
		Status:       domain.TargetStatusPendingConfirmation,
		Scope:        domain.TargetScopeSingle,
		NodeIDs:      []string{"jp-tokyo-01"},
		DisplayLabel: "jp-tokyo-01",
		Source:       domain.TargetSourceAssistantResolved,
		Confidence:   0.88,
	}

	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:             taskID,
		SessionID:      "sess-5",
		Status:         domain.TaskStatusPlanned,
		ApprovalStatus: domain.ApprovalStatusNotRequired,
		RiskLevel:      domain.RiskLevelLow,
		CreatedAt:      clock.Now(),
		UpdatedAt:      clock.Now(),
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:                  "sess-5",
		Title:               "Session 5",
		Status:              domain.SessionStatusPausedWaitTargetConfirmation,
		ActiveTargetContext: pendingTarget,
		PendingAction: &domain.PendingAction{
			Type:    domain.PendingActionTypeTargetConfirmation,
			Payload: mustRaw(t, map[string]any{"targetContext": pendingTarget}),
		},
		CurrentTaskID: &taskID,
		Revision:      3,
		CreatedAt:     clock.Now(),
		UpdatedAt:     clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	llm := &fakeLLM{
		outputs: []ModelTurnOutput{{AssistantText: strPtr("Proceeding with jp-tokyo-01"), Done: true}},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
		Tasks:       store.Tasks,
		Audits:      store.Audits,
	}, llm, fakeTools{}, clock, &idgen, WithEventPublisher(events))

	if err := rt.ResumeAfterTargetConfirmation(context.Background(), "sess-5", ConfirmTargetAction{
		NodeIDs:        []string{"jp-tokyo-01"},
		Scope:          string(domain.TargetScopeSingle),
		IdempotencyKey: "idem-1",
	}); err != nil {
		t.Fatalf("ResumeAfterTargetConfirmation() error = %v", err)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-5")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusCompleted {
		t.Fatalf("session status = %q, want completed", gotSession.Status)
	}
	if gotSession.PendingAction != nil {
		t.Fatalf("pending action = %#v, want nil", gotSession.PendingAction)
	}
	if gotSession.ActiveTargetContext.Status != domain.TargetStatusConfirmed {
		t.Fatalf("active target status = %q, want confirmed", gotSession.ActiveTargetContext.Status)
	}

	msgs, err := store.ThreadMessages.ListBySession(context.Background(), "sess-5", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(messages) error = %v", err)
	}
	if len(msgs) != 1 || msgs[0].Kind != domain.ThreadMessageKindAssistantText {
		t.Fatalf("messages = %#v, want one assistant_text and no new user_message", msgs)
	}

	rows, err := store.Timelines.ListBySession(context.Background(), "sess-5", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(timelines) error = %v", err)
	}
	if len(rows) != 2 || rows[0].Kind != domain.TimelineRowKindToolResultMeta || rows[0].Source != domain.TimelineRowSourceUserAction || rows[1].Kind != domain.TimelineRowKindAssistantText {
		t.Fatalf("timeline rows = %#v, want user_action tool_result then assistant_text", rows)
	}

	results, err := store.ToolResults.ListBySession(context.Background(), "sess-5", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(toolResults) error = %v", err)
	}
	if len(results) != 1 || results[0].ToolName != "target_confirmation" || results[0].Source != domain.TimelineRowSourceUserAction {
		t.Fatalf("tool results = %#v, want target_confirmation user_action", results)
	}

	audits, err := store.Audits.ListByTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("ListByTask(audits) error = %v", err)
	}
	if len(audits) != 1 || audits[0].EventType != "target_confirmation.confirmed" {
		t.Fatalf("audits = %#v, want target_confirmation.confirmed", audits)
	}
	if len(events.confirmedTargets) != 1 || events.confirmedTargets[0].Status != domain.TargetStatusConfirmed {
		t.Fatalf("confirmed targets = %#v, want confirmed target event", events.confirmedTargets)
	}
}

func TestClearTargetContextClearsPendingTarget(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 20, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"audit-2", "toolresult-2", "row-2"}}
	events := &stubEventPublisher{}
	pendingTarget := domain.ActiveTargetContext{
		Status:       domain.TargetStatusPendingConfirmation,
		Scope:        domain.TargetScopeSingle,
		NodeIDs:      []string{"jp-tokyo-01"},
		DisplayLabel: "jp-tokyo-01",
		Source:       domain.TargetSourceAssistantResolved,
	}

	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:                  "sess-clear",
		Title:               "Clear Session",
		Status:              domain.SessionStatusPausedWaitTargetConfirmation,
		ActiveTargetContext: pendingTarget,
		PendingAction: &domain.PendingAction{
			Type:    domain.PendingActionTypeTargetConfirmation,
			Payload: mustRaw(t, map[string]any{"targetContext": pendingTarget}),
		},
		Revision:  2,
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
		Audits:      store.Audits,
	}, &fakeLLM{}, fakeTools{}, clock, &idgen, WithEventPublisher(events))

	if err := rt.ClearTargetContext(context.Background(), "sess-clear", "idem-clear"); err != nil {
		t.Fatalf("ClearTargetContext() error = %v", err)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-clear")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusIdle {
		t.Fatalf("session status = %q, want idle", gotSession.Status)
	}
	if gotSession.PendingAction != nil {
		t.Fatalf("pending action = %#v, want nil", gotSession.PendingAction)
	}
	if gotSession.ActiveTargetContext.Status != domain.TargetStatusUnset || len(gotSession.ActiveTargetContext.NodeIDs) != 0 {
		t.Fatalf("active target context = %#v, want cleared target context", gotSession.ActiveTargetContext)
	}

	results, err := store.ToolResults.ListBySession(context.Background(), "sess-clear", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(toolResults) error = %v", err)
	}
	if len(results) != 1 || results[0].ToolName != "target_clear" || results[0].Text != "target context cleared" {
		t.Fatalf("tool results = %#v, want target_clear result", results)
	}

	if len(events.clearedTargets) != 1 || events.clearedTargets[0].Status != domain.TargetStatusUnset {
		t.Fatalf("cleared targets = %#v, want one cleared target event", events.clearedTargets)
	}
}

func TestHandleUserMessageToolWaitsForApproval(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 14, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"msg-1", "row-1", "toolcall-1", "row-2", "toolresult-1", "row-3", "row-4"}}
	events := &stubEventPublisher{}
	taskID := "task-approve-1"

	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:        "sess-6",
		Title:     "Session 6",
		Status:    domain.SessionStatusIdle,
		Revision:  1,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	llm := &fakeLLM{
		outputs: []ModelTurnOutput{{ToolCall: &ToolInvocation{Name: "request_approval", Args: mustRaw(t, map[string]any{"taskId": taskID})}}},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
	}, llm, fakeTools{
		result: ToolResult{
			MetaText:          "restart production nginx; requires explicit approval.",
			WaitForUser:       true,
			PendingActionType: domain.PendingActionTypeApproval,
			PendingActionPayload: mustRaw(t, map[string]any{
				"taskId":           taskID,
				"riskLevel":        domain.RiskLevelHigh,
				"message":          "restart production nginx; requires explicit approval.",
				"requiresApproval": true,
			}),
			TaskID:            taskID,
			AppendApprovalRow: true,
		},
	}, clock, &idgen, WithEventPublisher(events))

	if err := rt.HandleUserMessage(context.Background(), "sess-6", "restart nginx", "client-6"); err != nil {
		t.Fatalf("HandleUserMessage() error = %v", err)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-6")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusPausedWaitApproval {
		t.Fatalf("session status = %q, want paused_wait_approval", gotSession.Status)
	}
	if gotSession.CurrentTaskID == nil || *gotSession.CurrentTaskID != taskID {
		t.Fatalf("current task id = %#v, want %q", gotSession.CurrentTaskID, taskID)
	}

	rows, err := store.Timelines.ListBySession(context.Background(), "sess-6", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(timelines) error = %v", err)
	}
	if len(rows) != 4 || rows[3].Kind != domain.TimelineRowKindApproval || rows[3].TaskID == nil || *rows[3].TaskID != taskID {
		t.Fatalf("timeline rows = %#v, want approval tail row", rows)
	}
}

func TestResumeAfterApprovalApproveContinuesLoop(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 16, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"audit-1", "toolresult-1", "row-1", "msg-1", "row-2"}}
	events := &stubEventPublisher{}
	taskID := "task-approve-2"

	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:             taskID,
		SessionID:      "sess-7",
		Status:         domain.TaskStatusWaitingApproval,
		ApprovalStatus: domain.ApprovalStatusPending,
		RiskLevel:      domain.RiskLevelHigh,
		CreatedAt:      clock.Now(),
		UpdatedAt:      clock.Now(),
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:     "sess-7",
		Title:  "Session 7",
		Status: domain.SessionStatusPausedWaitApproval,
		PendingAction: &domain.PendingAction{
			Type:    domain.PendingActionTypeApproval,
			Payload: mustRaw(t, map[string]any{"taskId": taskID}),
		},
		CurrentTaskID: &taskID,
		Revision:      2,
		CreatedAt:     clock.Now(),
		UpdatedAt:     clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	llm := &fakeLLM{
		outputs: []ModelTurnOutput{{AssistantText: strPtr("Approved. Proceeding."), Done: true}},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
		Tasks:       store.Tasks,
		Audits:      store.Audits,
	}, llm, fakeTools{}, clock, &idgen, WithEventPublisher(events))

	if err := rt.ResumeAfterApproval(context.Background(), "sess-7", ApprovalAction{
		TaskID:         taskID,
		Approved:       true,
		IdempotencyKey: "approve-1",
	}); err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}

	gotTask, err := store.Tasks.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Get(task) error = %v", err)
	}
	if gotTask.ApprovalStatus != domain.ApprovalStatusApproved || gotTask.Status != domain.TaskStatusApproved {
		t.Fatalf("task = %#v, want approved state", gotTask)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-7")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusCompleted || gotSession.PendingAction != nil {
		t.Fatalf("session = %#v, want completed without pending action", gotSession)
	}

	rows, err := store.Timelines.ListBySession(context.Background(), "sess-7", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(timelines) error = %v", err)
	}
	if len(rows) != 2 || rows[0].Kind != domain.TimelineRowKindToolResultMeta || rows[0].Source != domain.TimelineRowSourceUserAction || rows[1].Kind != domain.TimelineRowKindAssistantText {
		t.Fatalf("timeline rows = %#v, want approval tool_result then assistant_text", rows)
	}

	msgs, err := store.ThreadMessages.ListBySession(context.Background(), "sess-7", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(messages) error = %v", err)
	}
	if len(msgs) != 1 || msgs[0].Kind != domain.ThreadMessageKindAssistantText {
		t.Fatalf("messages = %#v, want one assistant_text and no user_message", msgs)
	}

	audits, err := store.Audits.ListByTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("ListByTask(audits) error = %v", err)
	}
	if len(audits) != 1 || audits[0].EventType != "approval.approved" {
		t.Fatalf("audits = %#v, want approval.approved", audits)
	}
}

func TestResumeAfterApprovalRejectCompletesSession(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 18, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"audit-1", "toolresult-1", "row-1"}}
	taskID := "task-approve-3"
	reason := "too risky"

	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:             taskID,
		SessionID:      "sess-8",
		Status:         domain.TaskStatusWaitingApproval,
		ApprovalStatus: domain.ApprovalStatusPending,
		RiskLevel:      domain.RiskLevelHigh,
		CreatedAt:      clock.Now(),
		UpdatedAt:      clock.Now(),
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:     "sess-8",
		Title:  "Session 8",
		Status: domain.SessionStatusPausedWaitApproval,
		PendingAction: &domain.PendingAction{
			Type:    domain.PendingActionTypeApproval,
			Payload: mustRaw(t, map[string]any{"taskId": taskID}),
		},
		CurrentTaskID: &taskID,
		Revision:      2,
		CreatedAt:     clock.Now(),
		UpdatedAt:     clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
		Tasks:       store.Tasks,
		Audits:      store.Audits,
	}, &fakeLLM{}, fakeTools{}, clock, &idgen)

	if err := rt.ResumeAfterApproval(context.Background(), "sess-8", ApprovalAction{
		TaskID:         taskID,
		Approved:       false,
		Reason:         &reason,
		IdempotencyKey: "reject-1",
	}); err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}

	gotTask, err := store.Tasks.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Get(task) error = %v", err)
	}
	if gotTask.ApprovalStatus != domain.ApprovalStatusRejected || gotTask.Status != domain.TaskStatusCancelled {
		t.Fatalf("task = %#v, want rejected/cancelled", gotTask)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-8")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusCompleted || gotSession.PendingAction != nil {
		t.Fatalf("session = %#v, want completed without pending action", gotSession)
	}

	msgs, err := store.ThreadMessages.ListBySession(context.Background(), "sess-8", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(messages) error = %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("messages = %#v, want no new user_message or assistant_text", msgs)
	}

	rows, err := store.Timelines.ListBySession(context.Background(), "sess-8", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(timelines) error = %v", err)
	}
	if len(rows) != 1 || rows[0].Kind != domain.TimelineRowKindToolResultMeta || rows[0].Source != domain.TimelineRowSourceUserAction {
		t.Fatalf("timeline rows = %#v, want one user_action tool_result", rows)
	}
}

func TestHandleUserMessageToolStartsAsyncExecution(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 20, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"msg-1", "row-1", "toolcall-1", "row-2", "toolresult-1", "row-3", "row-4"}}
	events := &stubEventPublisher{}
	taskID := "task-exec-1"
	groupID := "group-exec-1"

	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:        "sess-9",
		Title:     "Session 9",
		Status:    domain.SessionStatusIdle,
		Revision:  1,
		CreatedAt: clock.Now(),
		UpdatedAt: clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	llm := &fakeLLM{
		outputs: []ModelTurnOutput{{ToolCall: &ToolInvocation{Name: "exec_on_nodes", Args: mustRaw(t, map[string]any{"inputText": "run diagnostics"})}}},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
	}, llm, fakeTools{
		result: ToolResult{
			MetaText:              "queued execution for 2 node(s)",
			AsyncExecutionStarted: true,
			TaskID:                taskID,
			ExecutionGroupID:      groupID,
			AppendExecutionRow:    true,
		},
	}, clock, &idgen, WithEventPublisher(events))

	if err := rt.HandleUserMessage(context.Background(), "sess-9", "run diagnostics", "client-9"); err != nil {
		t.Fatalf("HandleUserMessage() error = %v", err)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-9")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusWaitingAsyncExecution {
		t.Fatalf("session status = %q, want waiting_async_execution", gotSession.Status)
	}
	if gotSession.CurrentTaskID == nil || *gotSession.CurrentTaskID != taskID {
		t.Fatalf("current task id = %#v, want %q", gotSession.CurrentTaskID, taskID)
	}
	if gotSession.CurrentExecutionGroupID == nil || *gotSession.CurrentExecutionGroupID != groupID {
		t.Fatalf("current execution group id = %#v, want %q", gotSession.CurrentExecutionGroupID, groupID)
	}

	rows, err := store.Timelines.ListBySession(context.Background(), "sess-9", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(timelines) error = %v", err)
	}
	if len(rows) != 4 || rows[3].Kind != domain.TimelineRowKindExecution || rows[3].TaskID == nil || *rows[3].TaskID != taskID {
		t.Fatalf("timeline rows = %#v, want execution tail row", rows)
	}
}

func TestHandleUserMessageToolThenAssistant(t *testing.T) {
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
			{ToolCall: &ToolInvocation{Name: "list_nodes", Args: mustRaw(t, map[string]string{"region": "asia"})}},
			{AssistantText: strPtr("Found one node in asia"), Done: true},
		},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
	}, llm, fakeTools{
		result: ToolResult{
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
		outputs: []ModelTurnOutput{{AssistantText: strPtr("done"), Done: true}},
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

func TestHandleExecutionFinishedContinuesLoopWithSummary(t *testing.T) {
	store := memory.NewStore()
	clock := infra.FixedClock{Time: time.Date(2026, 3, 22, 14, 20, 0, 0, time.UTC)}
	idgen := stubIDGen{values: []string{"toolcall-1", "row-1", "toolresult-1", "row-2", "row-3", "msg-1", "row-4"}}
	events := &stubEventPublisher{}
	taskID := "task-11"
	groupID := "group-11"

	if err := store.Tasks.Create(context.Background(), domain.Task{
		ID:             taskID,
		SessionID:      "sess-11",
		Status:         domain.TaskStatusPartialFailed,
		ApprovalStatus: domain.ApprovalStatusApproved,
		RiskLevel:      domain.RiskLevelHigh,
		CreatedAt:      clock.Now(),
		UpdatedAt:      clock.Now(),
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}
	finishedAtOne := clock.Now()
	finishedAtTwo := clock.Now()
	for _, execution := range []domain.Execution{
		{ID: "exec-1", TaskID: taskID, SessionID: "sess-11", NodeID: "node-1", Status: domain.ExecutionStatusSuccess, FinishedAt: &finishedAtOne, CreatedAt: clock.Now(), UpdatedAt: clock.Now()},
		{ID: "exec-2", TaskID: taskID, SessionID: "sess-11", NodeID: "node-2", Status: domain.ExecutionStatusFailed, FinishedAt: &finishedAtTwo, CreatedAt: clock.Now(), UpdatedAt: clock.Now()},
	} {
		if err := store.Executions.Create(context.Background(), execution); err != nil {
			t.Fatalf("Create(execution) error = %v", err)
		}
	}
	if err := store.Sessions.Create(context.Background(), domain.Session{
		ID:                      "sess-11",
		Title:                   "Session 11",
		Status:                  domain.SessionStatusWaitingAsyncExecution,
		CurrentTaskID:           &taskID,
		CurrentExecutionGroupID: &groupID,
		Revision:                3,
		CreatedAt:               clock.Now(),
		UpdatedAt:               clock.Now(),
	}); err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	llm := &fakeLLM{
		outputs: []ModelTurnOutput{
			{ToolCall: &ToolInvocation{Name: "summarize_execution", Args: mustRaw(t, map[string]any{"taskId": taskID, "status": domain.TaskStatusPartialFailed})}},
			{AssistantText: strPtr("Execution completed with one failure."), Done: true},
		},
	}
	rt := NewService(Repositories{
		Sessions:    store.Sessions,
		Messages:    store.ThreadMessages,
		Timelines:   store.Timelines,
		ToolCalls:   store.ToolCalls,
		ToolResults: store.ToolResults,
		Tasks:       store.Tasks,
		Executions:  store.Executions,
	}, llm, fakeTools{
		result: ToolResult{
			MetaText:         "execution finished on tokyo batch with mixed results (1 succeeded, 1 failed, 0 timed out, 0 cancelled)",
			TaskID:           taskID,
			AppendSummaryRow: true,
			ToolMessage:      mustRaw(t, map[string]any{"taskId": taskID}),
		},
	}, clock, &idgen, WithEventPublisher(events))

	if err := rt.HandleExecutionFinished(context.Background(), "sess-11", taskID); err != nil {
		t.Fatalf("HandleExecutionFinished() error = %v", err)
	}

	gotTask, err := store.Tasks.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Get(task) error = %v", err)
	}
	if gotTask.Summary == nil || *gotTask.Summary == "" {
		t.Fatalf("task summary = %#v, want persisted summary", gotTask.Summary)
	}

	rows, err := store.Timelines.ListBySession(context.Background(), "sess-11", domain.CursorPage{})
	if err != nil {
		t.Fatalf("ListBySession(timelines) error = %v", err)
	}
	if len(rows) != 4 || rows[2].Kind != domain.TimelineRowKindSummary || rows[3].Kind != domain.TimelineRowKindAssistantText {
		t.Fatalf("timeline rows = %#v, want tool call/result, summary, assistant tail", rows)
	}

	gotSession, err := store.Sessions.Get(context.Background(), "sess-11")
	if err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if gotSession.Status != domain.SessionStatusCompleted {
		t.Fatalf("session status = %q, want completed", gotSession.Status)
	}
}

type fakeLLM struct {
	outputs []ModelTurnOutput
	index   int
}

func (f *fakeLLM) RunTurn(ctx context.Context, input ModelTurnInput, tools []ToolDefinition) (ModelTurnOutput, error) {
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
	result ToolResult
	err    error
}

func (f fakeTools) Definitions() []ToolDefinition {
	return []ToolDefinition{
		{Name: "request_target_confirmation", Description: "Request target confirmation"},
		{Name: "summarize_execution", Description: "Summarize completed execution"},
	}
}

func (f fakeTools) Call(ctx context.Context, input ToolCallInput) (ToolResult, error) {
	_ = ctx
	_ = input
	return f.result, f.err
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
	sessionStatuses  []domain.SessionStatus
	timelineKinds    []domain.TimelineRowKind
	pendingTargets   []domain.ActiveTargetContext
	confirmedTargets []domain.ActiveTargetContext
	clearedTargets   []domain.ActiveTargetContext
	llmSSEEvents     []stubLLMSSEEvent
	llmCompleted     []json.RawMessage
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

func (s *stubEventPublisher) ThreadTargetPending(_ context.Context, session domain.Session) error {
	s.pendingTargets = append(s.pendingTargets, session.ActiveTargetContext)
	return nil
}

func (s *stubEventPublisher) ThreadTargetConfirmed(_ context.Context, session domain.Session) error {
	s.confirmedTargets = append(s.confirmedTargets, session.ActiveTargetContext)
	return nil
}

func (s *stubEventPublisher) ThreadTargetCleared(_ context.Context, session domain.Session) error {
	s.clearedTargets = append(s.clearedTargets, session.ActiveTargetContext)
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

func (s *stubEventPublisher) ExecutionChunk(_ context.Context, _ string, _ string, _ domain.Execution, _ domain.ExecutionChunk) error {
	return nil
}

func (s *stubEventPublisher) ExecutionFinished(_ context.Context, _ string, _ string, _ domain.Execution) error {
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
