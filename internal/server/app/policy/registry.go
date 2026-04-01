package policy

import (
	"context"
	"fmt"

	"github.com/momaek/tolato/internal/server/agentapi"
)

type Option func(*Registry)

func WithExecutionStarter(starter ExecutionStarter) Option {
	return func(r *Registry) {
		r.execution = starter
	}
}

func WithExecutionWaiter(waiter ExecutionWaiter) Option {
	return func(r *Registry) {
		r.waiter = waiter
	}
}

func WithExecutionResultQuerier(querier ExecutionResultQuerier) Option {
	return func(r *Registry) {
		r.resultQuerier = querier
	}
}

type Registry struct {
	tools         map[string]Tool
	order         []string
	execution     ExecutionStarter
	waiter        ExecutionWaiter
	resultQuerier ExecutionResultQuerier
}

func NewRegistry(source NodeSource, options ...Option) *Registry {
	registry := &Registry{
		tools: make(map[string]Tool),
		order: make([]string, 0),
	}
	for _, option := range options {
		if option != nil {
			option(registry)
		}
	}

	tokens := NewConfirmTokenStore()

	tools := []Tool{
		NewListNodesTool(source),
		NewRunOnNodeTool(source, registry.execution, registry.waiter, registry.resultQuerier, tokens),
	}
	for _, tool := range tools {
		registry.tools[tool.Name()] = tool
		registry.order = append(registry.order, tool.Name())
	}
	return registry
}

func (r *Registry) Definitions() []agentapi.ToolSpec {
	definitions := make([]agentapi.ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		definitions = append(definitions, r.tools[name].Definition())
	}
	return definitions
}

func (r *Registry) Call(ctx context.Context, call agentapi.Item) (ToolResult, error) {
	tool, ok := r.tools[call.Name]
	if !ok {
		return ToolResult{}, fmt.Errorf("unknown tool %q", call.Name)
	}
	return tool.Call(ctx, call)
}
