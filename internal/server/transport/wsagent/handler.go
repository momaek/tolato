package wsagent

import (
	"context"

	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

type AgentAuthenticator interface {
	AuthenticateAgent(ctx context.Context, client infraws.Client) error
}

type Handler struct {
	Auth       AgentAuthenticator
	Hub        infraws.Hub
	Dispatcher Dispatcher
}

func (h Handler) Connect(ctx context.Context, client infraws.Client) error {
	if h.Auth != nil {
		if err := h.Auth.AuthenticateAgent(ctx, client); err != nil {
			return err
		}
	}
	if h.Hub != nil {
		h.Hub.Register(client)
	}
	return nil
}

func (h Handler) Disconnect(clientID string) {
	if h.Dispatcher.Agents != nil {
		h.Dispatcher.Agents.ForgetClient(clientID)
	}
	if h.Hub != nil {
		h.Hub.Unregister(clientID)
	}
}

func (h Handler) Handle(ctx context.Context, clientID string, raw []byte) ([]byte, error) {
	return h.Dispatcher.Dispatch(WithClientID(ctx, clientID), raw)
}
