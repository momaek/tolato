package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/momaek/tolato/server/internal/llm"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/security"
	"github.com/momaek/tolato/server/internal/store"
)

// ToolExecutor handles the execution of AI tool calls.
type ToolExecutor struct {
	nodeManager     *node.NodeManager
	securityChecker *security.Checker
	truncateLines   int
}

// NewToolExecutor creates a new ToolExecutor.
func NewToolExecutor(nm *node.NodeManager, sc *security.Checker, truncateLines int) *ToolExecutor {
	return &ToolExecutor{
		nodeManager:     nm,
		securityChecker: sc,
		truncateLines:   truncateLines,
	}
}

// ToolDefs returns the LLM tool definitions for the AI.
func ToolDefs() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		{
			Name:        "list_nodes",
			Description: "List all registered VPS nodes and their current status.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "get_node_info",
			Description: "Get detailed system information and real-time metrics for a specific node.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"node_id": map[string]any{
						"type":        "string",
						"description": "The ID of the node to get info for.",
					},
				},
				"required": []string{"node_id"},
			},
		},
		{
			Name:        "execute_command",
			Description: "Execute a shell command on a specified VPS node.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"node_id": map[string]any{
						"type":        "string",
						"description": "The target node ID.",
					},
					"command": map[string]any{
						"type":        "string",
						"description": "The shell command to execute.",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Timeout in seconds (default 60).",
					},
				},
				"required": []string{"node_id", "command"},
			},
		},
	}
}

// ExecuteToolCalls executes tool calls in parallel.
// For each call, it checks if it requires confirmation first.
// Returns a map of toolCallID -> result.
func (te *ToolExecutor) ExecuteToolCalls(ctx context.Context, calls []llm.ToolCall) map[string]*model.ToolResultItem {
	results := make(map[string]*model.ToolResultItem, len(calls))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, call := range calls {
		wg.Add(1)
		go func(tc llm.ToolCall) {
			defer wg.Done()
			result := te.executeSingle(ctx, tc)
			mu.Lock()
			results[tc.ID] = result
			mu.Unlock()
		}(call)
	}

	wg.Wait()
	return results
}

// NeedConfirmation checks if any tool call requires user confirmation.
// Returns the first tool call that needs confirmation, or nil.
func (te *ToolExecutor) NeedConfirmation(calls []llm.ToolCall) *llm.ToolCall {
	for i := range calls {
		if calls[i].Name == "execute_command" {
			command, _ := calls[i].Args["command"].(string)
			if command != "" && te.securityChecker.IsSensitive(command) {
				return &calls[i]
			}
		}
	}
	return nil
}

// IsBlacklisted checks if any tool call contains a blacklisted command.
// Returns the blacklisted tool call and true if found.
func (te *ToolExecutor) IsBlacklisted(calls []llm.ToolCall) (*llm.ToolCall, bool) {
	for i := range calls {
		if calls[i].Name == "execute_command" {
			command, _ := calls[i].Args["command"].(string)
			if command != "" && te.securityChecker.IsBlacklisted(command) {
				return &calls[i], true
			}
		}
	}
	return nil, false
}

func (te *ToolExecutor) executeSingle(ctx context.Context, tc llm.ToolCall) *model.ToolResultItem {
	switch tc.Name {
	case "list_nodes":
		return te.executeListNodes()
	case "get_node_info":
		nodeID, _ := tc.Args["node_id"].(string)
		return te.executeGetNodeInfo(nodeID)
	case "execute_command":
		nodeID, _ := tc.Args["node_id"].(string)
		command, _ := tc.Args["command"].(string)
		timeout := 60
		if t, ok := tc.Args["timeout"].(float64); ok && t > 0 {
			timeout = int(t)
		}
		return te.executeCommand(ctx, nodeID, command, timeout)
	default:
		errMsg := fmt.Sprintf("unknown tool: %s", tc.Name)
		return &model.ToolResultItem{Data: map[string]any{"error": errMsg}}
	}
}

func (te *ToolExecutor) executeListNodes() *model.ToolResultItem {
	nodes, _, err := store.ListNodes(1, 100, "")
	if err != nil {
		return &model.ToolResultItem{Data: map[string]any{"error": err.Error()}}
	}

	items := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		item := map[string]any{
			"id":     n.ID,
			"name":   n.Name,
			"ip":     n.IP,
			"os":     n.OS,
			"status": n.Status,
		}
		if n.Alias != nil {
			item["alias"] = *n.Alias
		}
		// Attach cached metrics if online
		if metrics := te.nodeManager.GetMetrics(n.ID); metrics != nil {
			item["cpu"] = metrics.CPU
			item["memory"] = metrics.Memory
			item["disk"] = metrics.Disk
		}
		items = append(items, item)
	}
	return &model.ToolResultItem{Data: items}
}

func (te *ToolExecutor) executeGetNodeInfo(nodeID string) *model.ToolResultItem {
	if nodeID == "" {
		return &model.ToolResultItem{Data: map[string]any{"error": "node_id is required"}}
	}

	n, err := store.GetNodeByID(nodeID)
	if err != nil {
		return &model.ToolResultItem{Data: map[string]any{"error": fmt.Sprintf("node not found: %s", nodeID)}}
	}

	info := map[string]any{
		"id":              n.ID,
		"name":            n.Name,
		"ip":              n.IP,
		"os":              n.OS,
		"kernel":          n.Kernel,
		"agent_version":   n.AgentVersion,
		"cpu_cores":       n.CPUCores,
		"memory_total_mb": n.MemoryTotalMB,
		"disk_total_gb":   n.DiskTotalGB,
		"status":          n.Status,
	}
	if n.Alias != nil {
		info["alias"] = *n.Alias
	}
	if metrics := te.nodeManager.GetMetrics(n.ID); metrics != nil {
		info["metrics"] = map[string]any{
			"cpu":      metrics.CPU,
			"memory":   metrics.Memory,
			"disk":     metrics.Disk,
			"uptime":   metrics.Uptime,
			"load_avg": metrics.LoadAvg,
		}
	}
	return &model.ToolResultItem{Data: info}
}

func (te *ToolExecutor) executeCommand(ctx context.Context, nodeID, command string, timeout int) *model.ToolResultItem {
	if nodeID == "" || command == "" {
		return &model.ToolResultItem{Data: map[string]any{"error": "node_id and command are required"}}
	}

	// Get node info for audit log
	n, err := store.GetNodeByID(nodeID)
	nodeName := nodeID
	if err == nil {
		nodeName = n.Name
	}

	// Execute command via NodeManager
	result, err := te.nodeManager.ExecuteCommand(ctx, nodeID, command, timeout)
	if err != nil {
		// Log failed execution
		store.CreateAuditLog(&model.AuditLog{
			NodeID:   nodeID,
			NodeName: nodeName,
			Command:  command,
			Source:   "webui",
		})
		errMsg := fmt.Sprintf("command execution failed: %s", err.Error())
		return &model.ToolResultItem{Data: map[string]any{"error": errMsg}}
	}

	// Truncate output if needed
	stdout := truncateOutput(result.Stdout, te.truncateLines)
	stderr := truncateOutput(result.Stderr, te.truncateLines)

	// Write audit log
	store.CreateAuditLog(&model.AuditLog{
		NodeID:     nodeID,
		NodeName:   nodeName,
		Command:    command,
		ExitCode:   &result.ExitCode,
		Stdout:     &stdout,
		Stderr:     &stderr,
		DurationMS: &result.DurationMS,
		Confirmed:  true,
		Source:     "webui",
	})

	return &model.ToolResultItem{
		ExitCode:   &result.ExitCode,
		Stdout:     &stdout,
		Stderr:     &stderr,
		DurationMS: &result.DurationMS,
	}
}

// truncateOutput keeps the first N/2 and last N/2 lines if total exceeds N.
func truncateOutput(output string, maxLines int) string {
	if maxLines <= 0 || output == "" {
		return output
	}
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}
	half := maxLines / 2
	head := lines[:half]
	tail := lines[len(lines)-half:]
	omitted := len(lines) - maxLines
	result := make([]string, 0, maxLines+1)
	result = append(result, head...)
	result = append(result, fmt.Sprintf("\n... (%d lines omitted) ...\n", omitted))
	result = append(result, tail...)
	return strings.Join(result, "\n")
}

// ResultToJSON converts a ToolResultItem to a JSON string for LLM message content.
func ResultToJSON(result *model.ToolResultItem) string {
	data, err := json.Marshal(result)
	if err != nil {
		return `{"error":"failed to serialize result"}`
	}
	return string(data)
}
