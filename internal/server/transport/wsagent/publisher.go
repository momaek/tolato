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

func (p *DispatchPublisher) SendShellInput(ctx context.Context, nodeID string, executionID string, data string) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("agent registry is not configured")
	}
	payload, err := json.Marshal(ShellInputPayload{
		ExecutionID: executionID,
		NodeID:      nodeID,
		Data:        data,
	})
	if err != nil {
		return err
	}
	raw, err := json.Marshal(Message{
		Type:    TypeShellInput,
		NodeID:  nodeID,
		Payload: payload,
	})
	if err != nil {
		return err
	}
	return p.Registry.PublishDispatch(nodeID, raw)
}

func (p *DispatchPublisher) SendShellResize(ctx context.Context, nodeID string, executionID string, rows, cols int) error {
	_ = ctx
	if p.Registry == nil {
		return errors.New("agent registry is not configured")
	}
	payload, err := json.Marshal(ShellResizePayload{
		ExecutionID: executionID,
		NodeID:      nodeID,
		Rows:        rows,
		Cols:        cols,
	})
	if err != nil {
		return err
	}
	raw, err := json.Marshal(Message{
		Type:    TypeShellResize,
		NodeID:  nodeID,
		Payload: payload,
	})
	if err != nil {
		return err
	}
	return p.Registry.PublishDispatch(nodeID, raw)
}
