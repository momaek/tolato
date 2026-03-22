package wsagent

import (
	"context"
	"testing"

	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

func TestHandlerConnectRegistersAgentClient(t *testing.T) {
	hub := infraws.NewMemoryHub()
	client := infraws.NewMemoryClient("agent-1", infraws.ClientKindAgent, 4)
	auth := &fakeAgentAuthenticator{}
	handler := Handler{
		Auth: auth,
		Hub:  hub,
	}

	if err := handler.Connect(context.Background(), client); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if _, ok := hub.Client("agent-1"); !ok {
		t.Fatal("expected agent client to be registered")
	}
	if auth.clientID != "agent-1" {
		t.Fatalf("auth client = %q, want agent-1", auth.clientID)
	}
}

func TestHandlerDisconnectClosesClient(t *testing.T) {
	hub := infraws.NewMemoryHub()
	client := infraws.NewMemoryClient("agent-4", infraws.ClientKindAgent, 2)
	handler := Handler{Hub: hub}
	if err := handler.Connect(context.Background(), client); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	handler.Disconnect(client.ID())
	if !client.Closed() {
		t.Fatal("client should be closed after disconnect")
	}
}

type fakeAgentAuthenticator struct {
	clientID string
}

func (f *fakeAgentAuthenticator) AuthenticateAgent(_ context.Context, client infraws.Client) error {
	f.clientID = client.ID()
	return nil
}
