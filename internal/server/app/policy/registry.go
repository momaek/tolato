package policy

import (
	"context"
	"encoding/json"
	"fmt"
)

type NodeSource interface {
	ListNodes(ctx context.Context) ([]NodeSummary, error)
}

type Option func(*Registry)

func WithExecutionStarter(starter ExecutionStarter) Option {
	return func(r *Registry) {
		r.execution = starter
	}
}

type Registry struct {
	tools     map[string]Tool
	order     []string
	execution ExecutionStarter
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

	tools := []Tool{
		NewListNodesTool(source),
		NewResolveTargetNodesTool(source),
		NewRequestTargetConfirmationTool(),
		NewProposePlanTool(),
		NewRequestApprovalTool(),
		NewExecOnNodesTool(registry.execution),
		NewSummarizeExecutionTool(),
	}
	for _, tool := range tools {
		registry.tools[tool.Name()] = tool
		registry.order = append(registry.order, tool.Name())
	}
	return registry
}

func (r *Registry) Definitions() []ToolDefinition {
	definitions := make([]ToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		definitions = append(definitions, r.tools[name].Definition())
	}
	return definitions
}

func (r *Registry) Call(ctx context.Context, name string, input json.RawMessage) (ToolResult, error) {
	tool, ok := r.tools[name]
	if !ok {
		return ToolResult{}, fmt.Errorf("unknown tool %q", name)
	}
	return tool.Call(ctx, input)
}
