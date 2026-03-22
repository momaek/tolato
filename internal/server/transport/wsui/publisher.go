package wsui

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

type UIEventPublisher interface {
	SessionStateUpdated(ctx context.Context, session domain.Session) error
	TimelineRowAppended(ctx context.Context, session domain.Session, row domain.TimelineRow) error
	ThreadTargetPending(ctx context.Context, session domain.Session) error
	ThreadTargetConfirmed(ctx context.Context, session domain.Session) error
	ThreadTargetCleared(ctx context.Context, session domain.Session) error
	ExecutionChunk(ctx context.Context, sessionID string, taskID string, execution domain.Execution, chunk domain.ExecutionChunk) error
	ExecutionFinished(ctx context.Context, sessionID string, taskID string, execution domain.Execution) error
	LLMSSEEvent(ctx context.Context, sessionID string, responseID string, sequenceNumber int, upstreamEventType string, rawEvent json.RawMessage) error
	LLMResponseCompleted(ctx context.Context, sessionID string, responseID string, rawResponse json.RawMessage) error
}

type Publisher struct {
	Registry infraws.SessionRegistry
	Now      func() time.Time
}

func NewPublisher(registry infraws.SessionRegistry) UIEventPublisher {
	return &Publisher{Registry: registry}
}

func (p *Publisher) SessionStateUpdated(ctx context.Context, session domain.Session) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("session registry is not configured")
	}
	timelineRaw, err := json.Marshal(SessionStateUpdatedEvent{
		Type:       "session.state.updated",
		EventScope: EventScopeTimeline,
		SessionID:  session.ID,
		Status:     session.Status,
		Revision:   session.Revision,
		Timestamp:  p.now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	p.Registry.PublishToSession(session.ID, timelineRaw)

	summaryRaw, err := json.Marshal(p.summaryEvent(session))
	if err != nil {
		return err
	}
	p.Registry.PublishSummary(session.ID, summaryRaw)

	if reason, ok := attentionReason(session.Status); ok {
		attentionRaw, err := json.Marshal(SessionRequiresAttentionEvent{
			Type:       "session.requires_attention",
			EventScope: EventScopeSummary,
			SessionID:  session.ID,
			Timestamp:  p.now().UTC().Format(time.RFC3339),
			Reason:     reason,
		})
		if err != nil {
			return err
		}
		p.Registry.PublishSummary(session.ID, attentionRaw)
	}
	return nil
}

func (p *Publisher) TimelineRowAppended(ctx context.Context, session domain.Session, row domain.TimelineRow) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("session registry is not configured")
	}
	raw, err := json.Marshal(TimelineRowAppendedEvent{
		Type:       "timeline.row.appended",
		EventScope: EventScopeTimeline,
		SessionID:  session.ID,
		Revision:   session.Revision,
		Timestamp:  p.now().UTC().Format(time.RFC3339),
		Row:        newTimelineRowView(row),
	})
	if err != nil {
		return err
	}
	p.Registry.PublishToSession(session.ID, raw)
	return nil
}

func (p *Publisher) ThreadTargetPending(ctx context.Context, session domain.Session) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("session registry is not configured")
	}
	raw, err := json.Marshal(ThreadTargetPendingEvent{
		Type:          "thread.target.pending",
		EventScope:    EventScopeTimeline,
		SessionID:     session.ID,
		Revision:      session.Revision,
		Timestamp:     p.now().UTC().Format(time.RFC3339),
		TargetContext: newActiveTargetContextView(session.ActiveTargetContext),
	})
	if err != nil {
		return err
	}
	p.Registry.PublishToSession(session.ID, raw)
	return nil
}

func (p *Publisher) ThreadTargetConfirmed(ctx context.Context, session domain.Session) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("session registry is not configured")
	}
	raw, err := json.Marshal(ThreadTargetConfirmedEvent{
		Type:          "thread.target.confirmed",
		EventScope:    EventScopeTimeline,
		SessionID:     session.ID,
		Revision:      session.Revision,
		Timestamp:     p.now().UTC().Format(time.RFC3339),
		TargetContext: newActiveTargetContextView(session.ActiveTargetContext),
	})
	if err != nil {
		return err
	}
	p.Registry.PublishToSession(session.ID, raw)
	return nil
}

func (p *Publisher) ThreadTargetCleared(ctx context.Context, session domain.Session) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("session registry is not configured")
	}
	raw, err := json.Marshal(ThreadTargetClearedEvent{
		Type:          "thread.target.cleared",
		EventScope:    EventScopeTimeline,
		SessionID:     session.ID,
		Revision:      session.Revision,
		Timestamp:     p.now().UTC().Format(time.RFC3339),
		TargetContext: newActiveTargetContextView(session.ActiveTargetContext),
	})
	if err != nil {
		return err
	}
	p.Registry.PublishToSession(session.ID, raw)
	return nil
}

func (p *Publisher) ExecutionChunk(ctx context.Context, sessionID string, taskID string, execution domain.Execution, chunk domain.ExecutionChunk) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("session registry is not configured")
	}
	raw, err := json.Marshal(ExecutionChunkEvent{
		Type:        "execution.chunk",
		EventScope:  EventScopeTimeline,
		SessionID:   sessionID,
		TaskID:      taskID,
		ExecutionID: execution.ID,
		NodeID:      execution.NodeID,
		Chunk:       newExecutionChunkView(chunk),
		Timestamp:   p.now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	p.Registry.PublishToSession(sessionID, raw)
	return nil
}

func (p *Publisher) ExecutionFinished(ctx context.Context, sessionID string, taskID string, execution domain.Execution) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("session registry is not configured")
	}
	raw, err := json.Marshal(ExecutionFinishedEvent{
		Type:        "execution.finished",
		EventScope:  EventScopeTimeline,
		SessionID:   sessionID,
		TaskID:      taskID,
		ExecutionID: execution.ID,
		NodeID:      execution.NodeID,
		Status:      execution.Status,
		Timestamp:   p.now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	p.Registry.PublishToSession(sessionID, raw)
	return nil
}

func (p *Publisher) LLMSSEEvent(ctx context.Context, sessionID string, responseID string, sequenceNumber int, upstreamEventType string, rawEvent json.RawMessage) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("session registry is not configured")
	}
	raw, err := json.Marshal(LLMSSEEvent{
		Type:              "llm.sse.event",
		EventScope:        EventScopeTimeline,
		SessionID:         sessionID,
		ResponseID:        responseID,
		SequenceNumber:    sequenceNumber,
		UpstreamEventType: upstreamEventType,
		RawEvent:          cloneRawMessage(rawEvent),
		Timestamp:         p.now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	p.Registry.PublishToSession(sessionID, raw)
	return nil
}

func (p *Publisher) LLMResponseCompleted(ctx context.Context, sessionID string, responseID string, rawResponse json.RawMessage) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("session registry is not configured")
	}
	raw, err := json.Marshal(LLMResponseCompletedEvent{
		Type:        "llm.response.completed",
		EventScope:  EventScopeTimeline,
		SessionID:   sessionID,
		ResponseID:  responseID,
		RawResponse: cloneRawMessage(rawResponse),
		Timestamp:   p.now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	p.Registry.PublishToSession(sessionID, raw)
	return nil
}

func (p *Publisher) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Publisher) summaryEvent(session domain.Session) any {
	summary := newSessionSummary(session)
	timestamp := p.now().UTC().Format(time.RFC3339)

	switch session.Status {
	case domain.SessionStatusCompleted, domain.SessionStatusFailed:
		return SessionFinishedEvent{
			Type:       "session.finished",
			EventScope: EventScopeSummary,
			SessionID:  session.ID,
			Timestamp:  timestamp,
			Summary:    summary,
		}
	default:
		return SessionSummaryUpdatedEvent{
			Type:       "session.summary.updated",
			EventScope: EventScopeSummary,
			SessionID:  session.ID,
			Timestamp:  timestamp,
			Summary:    summary,
		}
	}
}

func attentionReason(status domain.SessionStatus) (string, bool) {
	switch status {
	case domain.SessionStatusPausedWaitTargetConfirmation:
		return "target_confirmation", true
	case domain.SessionStatusPausedWaitApproval:
		return "approval", true
	case domain.SessionStatusFailed:
		return "failed", true
	default:
		return "", false
	}
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
