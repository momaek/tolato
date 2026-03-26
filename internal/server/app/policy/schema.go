package policy

import (
	"sort"

	"github.com/momaek/tolato/internal/server/agentapi"
)

func listNodesToolSpec() agentapi.ToolSpec {
	return agentapi.NewFunctionTool("list_nodes", "List node summaries for search, filtering, and target resolution.", objectSchema(map[string]any{
		"query":  nullableStringSchema(),
		"status": nullableStringSchema(),
		"busy":   nullableBooleanSchema(),
		"region": nullableStringSchema(),
		"tag":    nullableStringSchema(),
		"limit":  nullableIntegerSchema(),
	}))
}

func resolveTargetNodesToolSpec() agentapi.ToolSpec {
	return agentapi.NewFunctionTool("resolve_target_nodes", "Resolve a natural-language target description into candidate nodes and a tentative target context.", objectSchema(map[string]any{
		"query":                nullableStringSchema(),
		"currentTargetContext": nullableObjectSchema(targetContextSchema()),
	}))
}

func requestTargetConfirmationToolSpec() agentapi.ToolSpec {
	return agentapi.NewFunctionTool("request_target_confirmation", "Pause the loop and ask the user to confirm the resolved target context.", objectSchema(map[string]any{
		"targetContext": targetContextSchema(),
		"message":       nullableStringSchema(),
	}))
}

func proposePlanToolSpec() agentapi.ToolSpec {
	return agentapi.NewFunctionTool("propose_plan", "Propose a compact plan for the selected target nodes and task text.", objectSchema(map[string]any{
		"inputText":        nullableStringSchema(),
		"targetContext":    targetContextSchema(),
		"riskLevel":        nullableEnumSchema("low", "medium", "high"),
		"requiresApproval": nullableBooleanSchema(),
		"steps": map[string]any{
			"type": []string{"array", "null"},
			"items": objectSchema(map[string]any{
				"action":           nullableStringSchema(),
				"args":             nullableFreeformObjectSchema(),
				"risk":             nullableEnumSchema("low", "medium", "high"),
				"timeoutSec":       nullableIntegerSchema(),
				"broadcastAllowed": nullableBooleanSchema(),
			}),
		},
	}))
}

func requestApprovalToolSpec() agentapi.ToolSpec {
	return agentapi.NewFunctionTool("request_approval", "Pause the loop and request explicit user approval for a planned task.", objectSchema(map[string]any{
		"taskId":           nullableStringSchema(),
		"riskLevel":        nullableEnumSchema("low", "medium", "high"),
		"message":          nullableStringSchema(),
		"reason":           nullableStringSchema(),
		"requiresApproval": nullableBooleanSchema(),
	}))
}

func execOnNodesToolSpec() agentapi.ToolSpec {
	return agentapi.NewFunctionTool("exec_on_nodes", "Create task and execution records for node dispatch and pause the loop for async execution.", objectSchema(map[string]any{
		"sessionId":     nullableStringSchema(),
		"inputText":     nullableStringSchema(),
		"command":       nullableStringSchema(),
		"commandArgs":   nullableStringArraySchema(),
		"targetContext": targetContextSchema(),
		"riskLevel":     nullableEnumSchema("low", "medium", "high"),
	}))
}

func summarizeExecutionToolSpec() agentapi.ToolSpec {
	return agentapi.NewFunctionTool("summarize_execution", "Summarize the completed execution aggregate into a final summary row.", objectSchema(map[string]any{
		"taskId":      nullableStringSchema(),
		"status":      nullableStringSchema(),
		"aggregate":   executionAggregateSchema(),
		"targetLabel": nullableStringSchema(),
	}))
}

func targetContextSchema() map[string]any {
	return objectSchema(map[string]any{
		"status":          nullableStringSchema(),
		"scope":           nullableStringSchema(),
		"nodeIds":         nullableStringArraySchema(),
		"displayLabel":    nullableStringSchema(),
		"source":          nullableStringSchema(),
		"confidence":      nullableNumberSchema(),
		"candidates":      nullableCandidateArraySchema(),
		"sourceMessageId": nullableStringSchema(),
		"confirmedAt":     nullableStringSchema(),
	})
}

func executionAggregateSchema() map[string]any {
	return objectSchema(map[string]any{
		"total":      nullableIntegerSchema(),
		"queued":     nullableIntegerSchema(),
		"dispatched": nullableIntegerSchema(),
		"running":    nullableIntegerSchema(),
		"success":    nullableIntegerSchema(),
		"failed":     nullableIntegerSchema(),
		"timeout":    nullableIntegerSchema(),
		"cancelled":  nullableIntegerSchema(),
	})
}

func nullableCandidateArraySchema() map[string]any {
	return map[string]any{
		"type": []string{"array", "null"},
		"items": objectSchema(map[string]any{
			"nodeId":    nullableStringSchema(),
			"hostname":  nullableStringSchema(),
			"region":    nullableStringSchema(),
			"matchedBy": nullableStringSchema(),
			"reason":    nullableStringSchema(),
		}),
	}
}

func objectSchema(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for key := range properties {
		required = append(required, key)
	}
	sort.Strings(required)
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func nullableStringSchema() map[string]any {
	return map[string]any{"type": []string{"string", "null"}}
}

func nullableBooleanSchema() map[string]any {
	return map[string]any{"type": []string{"boolean", "null"}}
}

func nullableIntegerSchema() map[string]any {
	return map[string]any{"type": []string{"integer", "null"}}
}

func nullableNumberSchema() map[string]any {
	return map[string]any{"type": []string{"number", "null"}}
}

func nullableStringArraySchema() map[string]any {
	return map[string]any{
		"type":  []string{"array", "null"},
		"items": nullableStringSchema(),
	}
}

func nullableFreeformObjectSchema() map[string]any {
	return map[string]any{
		"type":                 []string{"object", "null"},
		"additionalProperties": true,
	}
}

func nullableObjectSchema(properties map[string]any) map[string]any {
	return map[string]any{
		"anyOf": []any{
			properties,
			map[string]any{"type": "null"},
		},
	}
}

func nullableEnumSchema(values ...string) map[string]any {
	enum := make([]any, 0, len(values)+1)
	for _, value := range values {
		enum = append(enum, value)
	}
	enum = append(enum, nil)
	return map[string]any{
		"type": []string{"string", "null"},
		"enum": enum,
	}
}
