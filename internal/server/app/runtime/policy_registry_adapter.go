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

func (a policyRegistryAdapter) Call(ctx context.Context, call agentapi.Item) (ToolResult, error) {
	result, err := a.registry.Call(ctx, call)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{
		OutputItem:            result.OutputItem,
		MetaText:              result.MetaText,
		ToolMessage:           result.ToolMessage,
		WaitForUser:           result.WaitForUser,
		PendingActionType:     result.PendingActionType,
		PendingActionPayload:  result.PendingActionPayload,
		AsyncExecutionStarted: result.AsyncExecutionStarted,
		TaskID:                result.TaskID,
		ExecutionGroupID:      result.ExecutionGroupID,
		AppendPlanRow:         result.AppendPlanRow,
		AppendApprovalRow:     result.AppendApprovalRow,
		AppendExecutionRow:    result.AppendExecutionRow,
		AppendSummaryRow:      result.AppendSummaryRow,
	}, nil
}
