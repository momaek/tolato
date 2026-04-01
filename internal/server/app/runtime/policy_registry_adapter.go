package runtime

import (
	"context"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/policy"
)

type policyRegistryAdapter struct {
	registry policy.ToolRegistry
}

func NewPolicyToolRegistry(registry policy.ToolRegistry) ToolRegistry {
	return policyRegistryAdapter{registry: registry}
}

func (a policyRegistryAdapter) Definitions() []agentapi.ToolSpec {
	if a.registry == nil {
		return nil
	}
	return a.registry.Definitions()
}

func (a policyRegistryAdapter) Call(ctx context.Context, call agentapi.Item) (policy.ToolResult, error) {
	return a.registry.Call(ctx, call)
}
