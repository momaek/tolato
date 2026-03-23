package wsagent

import (
	"context"
	"encoding/json"
	"testing"

	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

func TestDispatchPublisherPublishesTaskDispatch(t *testing.T) {
	hub := infraws.NewMemoryHub()
	client := infraws.NewMemoryClient("agent-pub", infraws.ClientKindAgent, 2)
	hub.Register(client)

	registry := infraws.NewMemoryAgentRegistry(hub)
	registry.BindNode("node-pub", "agent-pub", infraws.AgentNodeMetadata{Hostname: "node-pub"})

	publisher := NewDispatchPublisher(registry)
	if err := publisher.DispatchToNode(context.Background(), "node-pub", appexecution.DispatchCommand{
		Type:        TypeTaskDispatch,
		SessionID:   "sess-pub",
		TaskID:      "task-pub",
		ExecutionID: "exec-pub",
		NodeID:      "node-pub",
		Action:      "run_command",
		RiskLevel:   domain.RiskLevelLow,
	}); err != nil {
		t.Fatalf("DispatchToNode() error = %v", err)
	}

	raw := <-client.Messages()
	var cmd DispatchCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if cmd.Type != TypeTaskDispatch || cmd.ExecutionID != "exec-pub" || cmd.Action != "run_command" {
		t.Fatalf("cmd = %#v", cmd)
	}
}
