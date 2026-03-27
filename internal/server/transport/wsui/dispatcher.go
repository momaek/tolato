package wsui

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	appruntime "github.com/momaek/tolato/internal/server/app/runtime"
	appsession "github.com/momaek/tolato/internal/server/app/session"
	"github.com/momaek/tolato/internal/server/domain"
)

type SessionService interface {
	CreateSession(ctx context.Context, title string) (string, error)
	DeleteSession(ctx context.Context, sessionID string) error
	ListSessions(ctx context.Context, clientID string) ([]appsession.SessionListItem, error)
	BuildSnapshot(ctx context.Context, clientID string, sessionID string) (appsession.Snapshot, error)
	ListRows(ctx context.Context, sessionID string, page domain.CursorPage) (appsession.TimelinePage, error)
	UpdateSubscriptions(ctx context.Context, clientID string, activeSessionID string, watchSessionIDs []string) error
}

type Runtime interface {
	HandleUserMessage(ctx context.Context, sessionID string, text string, clientMessageID string) error
	ResumeAfterTargetConfirmation(ctx context.Context, sessionID string, action appruntime.ConfirmTargetAction) error
	ClearTargetContext(ctx context.Context, sessionID string, idempotencyKey string) error
	ResumeAfterApproval(ctx context.Context, sessionID string, action appruntime.ApprovalAction) error
}

type ExecutionService interface {
	CancelTask(ctx context.Context, sessionID string, taskID string, idempotencyKey string) error
	SendShellInput(ctx context.Context, input appexecution.ShellInputInput) error
	ResizeShell(ctx context.Context, input appexecution.ShellResizeInput) error
}

type Dispatcher struct {
	Sessions  SessionService
	Runtime   Runtime
	Execution ExecutionService
	Now       func() time.Time
}

func (d Dispatcher) Dispatch(ctx context.Context, raw []byte) (ResponseEnvelope, error) {
	var req RequestEnvelope
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorResponse("", "bad_request", err.Error()), nil
	}

	now := d.now().UTC().Format(time.RFC3339)
	clientID, _ := ClientIDFromContext(ctx)
	switch req.Type {
	case TypeSessionCreate:
		if d.Sessions == nil {
			return ResponseEnvelope{}, errors.New("session service is not configured")
		}
		payload, err := DecodePayload[SessionCreateRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		sessionID, err := d.Sessions.CreateSession(ctx, payload.Title)
		if err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, sessionID, now), nil

	case TypeSessionDelete:
		if d.Sessions == nil {
			return ResponseEnvelope{}, errors.New("session service is not configured")
		}
		payload, err := DecodePayload[SessionDeleteRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		if err := d.Sessions.DeleteSession(ctx, payload.SessionID); err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, payload.SessionID, now), nil

	case TypeSessionsListRequest:
		items, err := d.Sessions.ListSessions(ctx, clientID)
		if err != nil {
			return ResponseEnvelope{}, err
		}
		return ResponseEnvelope{
			Type:      TypeSessionsListResponse,
			RequestID: req.RequestID,
			Payload:   SessionsListResponse{Items: items},
		}, nil

	case TypeSessionSnapshotRequest:
		payload, err := DecodePayload[SessionSnapshotRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		snapshot, err := d.Sessions.BuildSnapshot(ctx, clientID, payload.SessionID)
		if err != nil {
			return ResponseEnvelope{}, err
		}
		return ResponseEnvelope{
			Type:      TypeSessionSnapshotResponse,
			RequestID: req.RequestID,
			Payload:   SessionSnapshotResponse{Snapshot: snapshot},
		}, nil

	case TypeSessionRowsRequest:
		payload, err := DecodePayload[SessionRowsRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		page, err := d.Sessions.ListRows(ctx, payload.SessionID, domain.CursorPage{BeforeID: payload.Before, Limit: payload.Limit})
		if err != nil {
			return ResponseEnvelope{}, err
		}
		return ResponseEnvelope{
			Type:      TypeSessionRowsResponse,
			RequestID: req.RequestID,
			Payload:   SessionRowsResponse{Page: page},
		}, nil

	case TypeSessionMessageSubmit:
		if d.Runtime == nil {
			return ResponseEnvelope{}, errors.New("runtime is not configured")
		}
		payload, err := DecodePayload[SessionMessageSubmitRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		if err := d.Runtime.HandleUserMessage(ctx, payload.SessionID, payload.Text, payload.ClientMessageID); err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, payload.SessionID, now), nil

	case TypeSessionTargetConfirm:
		if d.Runtime == nil {
			return ResponseEnvelope{}, errors.New("runtime is not configured")
		}
		payload, err := DecodePayload[SessionTargetConfirmRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		if err := d.Runtime.ResumeAfterTargetConfirmation(ctx, payload.SessionID, appruntime.ConfirmTargetAction{
			NodeIDs:        payload.NodeIDs,
			Scope:          string(payload.Scope),
			IdempotencyKey: payload.IdempotencyKey,
		}); err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, payload.SessionID, now), nil

	case TypeSessionTargetClear:
		if d.Runtime == nil {
			return ResponseEnvelope{}, errors.New("runtime is not configured")
		}
		payload, err := DecodePayload[SessionTargetClearRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		if err := d.Runtime.ClearTargetContext(ctx, payload.SessionID, payload.IdempotencyKey); err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, payload.SessionID, now), nil

	case TypeSessionTargetReselect:
		if d.Runtime == nil {
			return ResponseEnvelope{}, errors.New("runtime is not configured")
		}
		payload, err := DecodePayload[SessionTargetReselectRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		if err := d.Runtime.ClearTargetContext(ctx, payload.SessionID, payload.IdempotencyKey); err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, payload.SessionID, now), nil

	case TypeSessionApprovalApprove:
		if d.Runtime == nil {
			return ResponseEnvelope{}, errors.New("runtime is not configured")
		}
		payload, err := DecodePayload[SessionApprovalApproveRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		if err := d.Runtime.ResumeAfterApproval(ctx, payload.SessionID, appruntime.ApprovalAction{
			TaskID:         payload.TaskID,
			Approved:       true,
			IdempotencyKey: payload.IdempotencyKey,
		}); err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, payload.SessionID, now), nil

	case TypeSessionApprovalReject:
		if d.Runtime == nil {
			return ResponseEnvelope{}, errors.New("runtime is not configured")
		}
		payload, err := DecodePayload[SessionApprovalRejectRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		if err := d.Runtime.ResumeAfterApproval(ctx, payload.SessionID, appruntime.ApprovalAction{
			TaskID:         payload.TaskID,
			Approved:       false,
			Reason:         payload.Reason,
			IdempotencyKey: payload.IdempotencyKey,
		}); err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, payload.SessionID, now), nil

	case TypeSessionOperationCancel:
		if d.Execution == nil {
			return ResponseEnvelope{}, errors.New("execution service is not configured")
		}
		payload, err := DecodePayload[SessionOperationCancelRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		if err := d.Execution.CancelTask(ctx, payload.SessionID, payload.TaskID, payload.IdempotencyKey); err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, payload.SessionID, now), nil

	case TypeSessionShellInput:
		if d.Execution == nil {
			return ResponseEnvelope{}, errors.New("execution service is not configured")
		}
		payload, err := DecodePayload[SessionShellInputRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		if err := d.Execution.SendShellInput(ctx, appexecution.ShellInputInput{
			SessionID:   payload.SessionID,
			ExecutionID: payload.ExecutionID,
			Data:        payload.Data,
		}); err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, payload.SessionID, now), nil

	case TypeSessionShellResize:
		if d.Execution == nil {
			return ResponseEnvelope{}, errors.New("execution service is not configured")
		}
		payload, err := DecodePayload[SessionShellResizeRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		if err := d.Execution.ResizeShell(ctx, appexecution.ShellResizeInput{
			SessionID:   payload.SessionID,
			ExecutionID: payload.ExecutionID,
			Rows:        payload.Rows,
			Cols:        payload.Cols,
		}); err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, payload.SessionID, now), nil

	case TypeSubscriptionsUpdate:
		if d.Sessions == nil {
			return ResponseEnvelope{}, errors.New("session service is not configured")
		}
		payload, err := DecodePayload[SubscriptionsUpdateRequest](req)
		if err != nil {
			return errorResponse(req.RequestID, "bad_request", err.Error()), nil
		}
		if clientID == "" {
			return ResponseEnvelope{}, errors.New("ws/ui client id is missing from context")
		}
		if err := d.Sessions.UpdateSubscriptions(ctx, clientID, payload.ActiveSessionID, payload.WatchSessionIDs); err != nil {
			return ResponseEnvelope{}, err
		}
		return accepted(req.RequestID, payload.ActiveSessionID, now), nil
	default:
		return errorResponse(req.RequestID, "unknown_type", "unsupported ws/ui message type"), nil
	}
}

func (d Dispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func accepted(requestID, sessionID, timestamp string) ResponseEnvelope {
	return ResponseEnvelope{
		Type:      TypeSessionActionAccepted,
		RequestID: requestID,
		Payload: SessionActionAccepted{
			SessionID: sessionID,
			Timestamp: timestamp,
		},
	}
}

func errorResponse(requestID, code, message string) ResponseEnvelope {
	return ResponseEnvelope{
		Type:      TypeError,
		RequestID: requestID,
		Error: &ErrorBody{
			Code:    code,
			Message: message,
		},
	}
}
