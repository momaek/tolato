package policy

import (
	"github.com/momaek/tolato/internal/server/agentapi"
)

func listNodesToolSpec() agentapi.ToolSpec {
	return agentapi.NewFunctionTool("list_nodes", "List all VPS nodes. Use for 'how many nodes', 'show nodes', etc. Returns node summaries with status, region, and metrics.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":  map[string]any{"type": "string", "description": "Search keyword to match hostname, region, or tags"},
			"status": map[string]any{"type": "string", "description": "Filter by status: online, offline"},
			"region": map[string]any{"type": "string", "description": "Filter by region"},
			"tag":    map[string]any{"type": "string", "description": "Filter by tag"},
		},
	})
}

func runOnNodeToolSpec() agentapi.ToolSpec {
	return agentapi.NewFunctionTool("run_on_node", "Run a command on target VPS node(s). Pass the user's target description as-is (e.g. '东京', 'all', 'sg-2'). The backend resolves the target and handles risk checks automatically.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target": map[string]any{
				"type":        "string",
				"description": "Natural language target description or exact node ID/hostname",
			},
			"command": map[string]any{
				"type": "string",
				"enum": []any{
					"system_status", "disk_usage", "memory_usage", "docker_ps",
					"service_status", "tail_log", "restart_service", "reload_service",
					"network_check",
				},
				"description": "The predefined command to run",
			},
			"args": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Additional arguments for the command",
			},
			"confirm_token": map[string]any{
				"type":        "string",
				"description": "Confirmation token for risky operations, returned by a previous call",
			},
		},
		"required": []string{"target", "command"},
	})
}
