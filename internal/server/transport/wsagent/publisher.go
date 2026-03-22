package wsagent

import (
	"context"
	"encoding/json"
	"errors"

	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

type DispatchPublisher struct {
	Registry infraws.AgentRegistry
}

func NewDispatchPublisher(registry infraws.AgentRegistry) *DispatchPublisher {
	return &DispatchPublisher{Registry: registry}
}

func (p *DispatchPublisher) DispatchToNode(ctx context.Context, nodeID string, cmd appexecution.DispatchCommand) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("agent registry is not configured")
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	return p.Registry.PublishDispatch(nodeID, raw)
}
