package wsui

import (
	"encoding/json"

	"github.com/momaek/tolato/internal/server/app/session"
	"github.com/momaek/tolato/internal/server/domain"
)

const (
	TypeConnectionReady         = "connection.ready"
	TypeSessionsListRequest     = "sessions.list.request"
	TypeSessionsListResponse    = "sessions.list.response"
	TypeSessionSnapshotRequest  = "session.snapshot.request"
	TypeSessionSnapshotResponse = "session.snapshot.response"
	TypeSessionRowsRequest      = "session.rows.request"
	TypeSessionRowsResponse     = "session.rows.response"
	TypeSessionMessageSubmit    = "session.message.submit"
	TypeSessionTargetConfirm    = "session.target.confirm"
	TypeSessionTargetClear      = "session.target.clear"
	TypeSessionApprovalApprove  = "session.approval.approve"
	TypeSessionApprovalReject   = "session.approval.reject"
	TypeSessionOperationCancel  = "session.operation.cancel"
	TypeSubscriptionsUpdate     = "subscriptions.update"
	TypeSessionActionAccepted   = "session.action.accepted"
	TypeError                   = "error"
)

type RequestEnvelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"requestId,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type ResponseEnvelope struct {
	Type      string     `json:"type"`
	RequestID string     `json:"requestId,omitempty"`
	Payload   any        `json:"payload,omitempty"`
	Error     *ErrorBody `json:"error,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SessionSnapshotRequest struct {
	SessionID string `json:"sessionId"`
}

type SessionRowsRequest struct {
	SessionID string `json:"sessionId"`
	Before    string `json:"before,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type SessionMessageSubmitRequest struct {
	SessionID       string `json:"sessionId"`
	Text            string `json:"text"`
	ClientMessageID string `json:"clientMessageId"`
}

type SessionTargetConfirmRequest struct {
	SessionID      string             `json:"sessionId"`
	NodeIDs        []string           `json:"nodeIds"`
	Scope          domain.TargetScope `json:"scope"`
	IdempotencyKey string             `json:"idempotencyKey"`
}

type SessionTargetClearRequest struct {
	SessionID      string `json:"sessionId"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type SessionApprovalApproveRequest struct {
	SessionID      string `json:"sessionId"`
	TaskID         string `json:"taskId"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type SessionApprovalRejectRequest struct {
	SessionID      string  `json:"sessionId"`
	TaskID         string  `json:"taskId"`
	Reason         *string `json:"reason,omitempty"`
	IdempotencyKey string  `json:"idempotencyKey"`
}

type SessionOperationCancelRequest struct {
	SessionID      string `json:"sessionId"`
	TaskID         string `json:"taskId"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type SubscriptionsUpdateRequest struct {
	ActiveSessionID string   `json:"activeSessionId"`
	WatchSessionIDs []string `json:"watchSessionIds"`
}

type SessionActionAccepted struct {
	SessionID string `json:"sessionId"`
	Timestamp string `json:"timestamp"`
}

type SessionsListResponse struct {
	Items []session.SessionListItem `json:"items"`
}

type SessionSnapshotResponse struct {
	Snapshot session.Snapshot `json:"snapshot"`
}

type SessionRowsResponse struct {
	Page session.TimelinePage `json:"page"`
}

func DecodePayload[T any](env RequestEnvelope) (T, error) {
	var out T
	if len(env.Payload) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(env.Payload, &out); err != nil {
		return out, err
	}
	return out, nil
}
