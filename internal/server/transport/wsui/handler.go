package wsui

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

type UIAuthenticator interface {
	AuthenticateUI(ctx context.Context, client infraws.Client) error
}

type Handler struct {
	Auth          UIAuthenticator
	Hub           infraws.Hub
	Subscriptions infraws.SessionRegistry
	Dispatcher    Dispatcher
	Now           func() time.Time
}

func (h Handler) Connect(ctx context.Context, client infraws.Client) ([]byte, error) {
	if h.Auth != nil {
		if err := h.Auth.AuthenticateUI(ctx, client); err != nil {
			return nil, err
		}
	}
	if h.Hub != nil {
		h.Hub.Register(client)
	}
	return json.Marshal(ConnectionReady{
		Type:      TypeConnectionReady,
		Timestamp: h.now().UTC().Format(time.RFC3339),
	})
}

func (h Handler) Disconnect(clientID string) {
	if h.Subscriptions != nil {
		h.Subscriptions.ForgetClient(clientID)
	}
	if h.Hub != nil {
		h.Hub.Unregister(clientID)
	}
}

func (h Handler) Handle(ctx context.Context, clientID string, raw []byte) ([]byte, error) {
	resp, err := h.Dispatcher.Dispatch(WithClientID(ctx, clientID), raw)
	if err != nil {
		return json.Marshal(errorResponse("", mapErrorCode(err), err.Error()))
	}
	return json.Marshal(resp)
}

func (h Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func mapErrorCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrSessionBusy):
		return "session_busy"
	case errors.Is(err, domain.ErrInvalidArgument):
		return "invalid_argument"
	case errors.Is(err, domain.ErrNotFound):
		return "not_found"
	case errors.Is(err, domain.ErrRevisionConflict):
		return "conflict"
	case errors.Is(err, domain.ErrDuplicateAction):
		return "duplicate_action"
	default:
		return "conflict"
	}
}
