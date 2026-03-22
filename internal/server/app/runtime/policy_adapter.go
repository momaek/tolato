package runtime

import (
	"context"

	"github.com/momaek/tolato/internal/server/app/policy"
)

type policyRegistryAdapter struct {
	registry policy.ToolRegistry
}

func NewPolicyToolRegistry(registry policy.ToolRegistry) ToolRegistry {
	return policyRegistryAdapter{registry: registry}
}

func (a policyRegistryAdapter) Definitions() []ToolDefinition {
	defs := a.registry.Definitions()
	out := make([]ToolDefinition, 0, len(defs))
	for _, def := range defs {
		out = append(out, ToolDefinition{
			Name:        def.Name,
			Description: def.Description,
		})
	}
	return out
}

func (a policyRegistryAdapter) Call(ctx context.Context, input ToolCallInput) (ToolResult, error) {
	result, err := a.registry.Call(ctx, input.Name, input.Args)
	if err != nil {
		return ToolResult{}, err
	}

	return ToolResult{
		MetaText:              result.MetaText,
		ToolMessage:           result.ToolMessage,
		WaitForUser:           result.WaitForUser,
		PendingActionType:     result.PendingActionType,
		PendingActionPayload:  result.PendingActionPayload,
		AsyncExecutionStarted: result.AsyncExecutionStarted,
		AppendPlanRow:         result.AppendPlanRow,
		AppendApprovalRow:     result.AppendApprovalRow,
		AppendExecutionRow:    result.AppendExecutionRow,
		AppendSummaryRow:      result.AppendSummaryRow,
		TaskID:                result.TaskID,
		ExecutionGroupID:      result.ExecutionGroupID,
	}, nil
}
