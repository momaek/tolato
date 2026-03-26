package mock

import (
	"context"
	"sync"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/runtime"
)

type Provider struct {
	mu      sync.Mutex
	outputs []runtime.ModelTurnOutput
}

func New(outputs []runtime.ModelTurnOutput) *Provider {
	cloned := make([]runtime.ModelTurnOutput, len(outputs))
	copy(cloned, outputs)
	return &Provider{outputs: cloned}
}

func (p *Provider) RunTurn(ctx context.Context, input runtime.ModelTurnInput, tools []agentapi.ToolSpec) (runtime.ModelTurnOutput, error) {
	_ = ctx
	_ = input
	_ = tools

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.outputs) == 0 {
		return runtime.ModelTurnOutput{}, runtime.ErrEmptyModelOutput
	}

	out := p.outputs[0]
	p.outputs = p.outputs[1:]
	return out, nil
}
