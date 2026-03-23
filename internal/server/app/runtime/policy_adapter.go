package runtime

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/momaek/tolato/internal/server/app/policy"
	"github.com/momaek/tolato/internal/server/domain"
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
	args, err := augmentToolArgs(input)
	if err != nil {
		return ToolResult{}, err
	}

	result, err := a.registry.Call(ctx, input.Name, args)
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

func augmentToolArgs(input ToolCallInput) (json.RawMessage, error) {
	if input.Name != "exec_on_nodes" {
		return input.Args, nil
	}

	req := policy.ExecOnNodesInput{}
	if len(input.Args) > 0 {
		if err := json.Unmarshal(input.Args, &req); err != nil {
			return nil, err
		}
	}

	var legacy struct {
		NodeIDs  []string `json:"node_ids"`
		TaskText string   `json:"task_text"`
	}
	if len(input.Args) > 0 {
		if err := json.Unmarshal(input.Args, &legacy); err != nil {
			return nil, err
		}
	}

	if strings.TrimSpace(req.SessionID) == "" {
		req.SessionID = input.SessionID
	}
	if len(req.TargetContext.NodeIDs) == 0 {
		req.TargetContext = mergeTargetContext(input.ActiveTargetContext, legacy.NodeIDs)
	}
	if strings.TrimSpace(req.InputText) == "" && strings.TrimSpace(legacy.TaskText) != "" {
		req.InputText = strings.TrimSpace(legacy.TaskText)
	}
	if strings.TrimSpace(req.Command) == "" && strings.TrimSpace(legacy.TaskText) != "" {
		req.Command = "bash"
		if len(req.CommandArgs) == 0 {
			req.CommandArgs = []string{"-lc", strings.TrimSpace(legacy.TaskText)}
		}
	}

	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func mergeTargetContext(current domain.ActiveTargetContext, fallbackNodeIDs []string) domain.ActiveTargetContext {
	out := current
	if len(out.NodeIDs) == 0 && len(fallbackNodeIDs) > 0 {
		out.NodeIDs = append([]string(nil), fallbackNodeIDs...)
		if strings.TrimSpace(out.DisplayLabel) == "" && len(fallbackNodeIDs) == 1 {
			out.DisplayLabel = fallbackNodeIDs[0]
		}
	}
	return out
}
