package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/security"
	"github.com/momaek/tolato/server/internal/store"
	"github.com/momaek/tolato/server/internal/webfetch"
)

// callerContext is the per-request bag we hand each tool handler. It carries
// the API key identity already validated by middleware.APIKeyAuth, so tools
// can enforce permission checks and write provenance into audit logs.
type callerContext struct {
	APIKeyID   string
	Permission string // "readonly" | "standard" | "admin"
}

// catalog enumerates the tools the MCP server exposes. Keep descriptions
// short and instructive — Claude reads these when deciding which tool to call.
func catalog() []tool {
	return []tool{
		{
			Name:        "list_nodes",
			Description: "List all managed VPS nodes with current status and real-time metrics (CPU/memory/disk %, GeoIP region when available).",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "get_node",
			Description: "Get detailed information about a single node (hardware specs, OS/kernel, agent version, GeoIP, current metrics).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"node_id": map[string]any{
						"type":        "string",
						"description": "Node ID returned by list_nodes.",
					},
				},
				"required": []string{"node_id"},
			},
		},
		{
			Name: "execute_command",
			Description: "Run a shell command on a node and return stdout/stderr/exit_code. " +
				"Read-only API keys cannot call this. Commands flagged as sensitive (rm, reboot, etc.) " +
				"return a structured `needs_confirmation` response — re-invoke with `confirm=true` " +
				"after the user agrees. Blacklisted commands are rejected outright.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"node_id": map[string]any{
						"type":        "string",
						"description": "Target node ID.",
					},
					"command": map[string]any{
						"type":        "string",
						"description": "Shell command to execute on the node.",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Timeout in seconds (default 60).",
					},
					"confirm": map[string]any{
						"type":        "boolean",
						"description": "Set true to proceed past the sensitive-command guard. Use only after the user explicitly confirms.",
					},
				},
				"required": []string{"node_id", "command"},
			},
		},
		{
			Name: "web_fetch",
			Description: "Fetch a public HTTP(S) URL and return its content as Markdown via the server's configured Jina Reader. " +
				"Use this when you need to read a page (install scripts, API docs, RSS, etc.) before acting.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "Absolute http(s) URL to fetch.",
					},
				},
				"required": []string{"url"},
			},
		},
	}
}

// dispatch routes a tools/call to the matching handler. It returns the JSON
// payload to wrap in a text content block. Errors here are JSON-RPC errors
// (bad arguments, unknown tool); tool-level failures (permission denied,
// command failed) are returned as `isError: true` content via the handler.
func (s *server) dispatch(ctx context.Context, caller callerContext, params toolCallParams) (any, bool, error) {
	switch params.Name {
	case "list_nodes":
		return s.toolListNodes()
	case "get_node":
		return s.toolGetNode(params.Arguments)
	case "execute_command":
		return s.toolExecuteCommand(ctx, caller, params.Arguments)
	case "web_fetch":
		return s.toolWebFetch(ctx, params.Arguments)
	default:
		return nil, false, fmt.Errorf("unknown tool: %s", params.Name)
	}
}

func (s *server) toolListNodes() (any, bool, error) {
	nodes, _, err := store.ListNodes(1, 200, "")
	if err != nil {
		return errPayload("failed to list nodes: " + err.Error()), true, nil
	}
	items := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, nodeListItem(n, s.nodes))
	}
	return items, false, nil
}

func (s *server) toolGetNode(raw json.RawMessage) (any, bool, error) {
	var args struct {
		NodeID string `json:"node_id"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, false, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.NodeID == "" {
		return errPayload("node_id is required"), true, nil
	}
	n, err := store.GetNodeByID(args.NodeID)
	if err != nil {
		return errPayload("node not found: " + args.NodeID), true, nil
	}
	return nodeDetail(*n, s.nodes), false, nil
}

func (s *server) toolExecuteCommand(ctx context.Context, caller callerContext, raw json.RawMessage) (any, bool, error) {
	var args struct {
		NodeID  string `json:"node_id"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
		Confirm bool   `json:"confirm"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, false, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.NodeID == "" || args.Command == "" {
		return errPayload("node_id and command are required"), true, nil
	}
	if args.Timeout <= 0 {
		args.Timeout = 60
	}

	if caller.Permission == "readonly" {
		return errPayload("read-only API keys cannot execute commands"), true, nil
	}

	checker := security.NewChecker(s.settings)

	if checker.IsBlacklisted(args.Command) {
		return errPayload("command is blacklisted by server policy"), true, nil
	}

	// Sensitive operations require explicit confirm=true unless the key is admin.
	if caller.Permission != "admin" && checker.IsSensitive(args.Command) && !args.Confirm {
		return map[string]any{
			"status":     "needs_confirmation",
			"reason":     "command matches a sensitive pattern; ask the user to confirm before re-invoking",
			"retry_with": map[string]any{"confirm": true},
		}, false, nil
	}

	n, err := store.GetNodeByID(args.NodeID)
	if err != nil {
		return errPayload("node not found: " + args.NodeID), true, nil
	}

	apiKeyID := caller.APIKeyID
	confirmed := args.Confirm || caller.Permission == "admin"

	result, err := s.nodes.ExecuteCommand(ctx, args.NodeID, args.Command, args.Timeout)
	if err != nil {
		_ = store.CreateAuditLog(&model.AuditLog{
			NodeID:   args.NodeID,
			NodeName: n.Name,
			Command:  args.Command,
			Source:   "mcp",
			APIKeyID: &apiKeyID,
		})
		return errPayload("execution failed: " + err.Error()), true, nil
	}

	stdout := result.Stdout
	stderr := result.Stderr
	_ = store.CreateAuditLog(&model.AuditLog{
		NodeID:     args.NodeID,
		NodeName:   n.Name,
		Command:    args.Command,
		ExitCode:   &result.ExitCode,
		Stdout:     &stdout,
		Stderr:     &stderr,
		DurationMS: &result.DurationMS,
		Confirmed:  confirmed,
		Source:     "mcp",
		APIKeyID:   &apiKeyID,
	})

	return map[string]any{
		"node_id":     args.NodeID,
		"command":     args.Command,
		"exit_code":   result.ExitCode,
		"stdout":      result.Stdout,
		"stderr":      result.Stderr,
		"duration_ms": result.DurationMS,
	}, false, nil
}

func (s *server) toolWebFetch(ctx context.Context, raw json.RawMessage) (any, bool, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, false, fmt.Errorf("invalid arguments: %w", err)
	}
	res, err := webfetch.Fetch(ctx, s.settings, args.URL)
	if err != nil {
		return errPayload(err.Error()), true, nil
	}
	return res, false, nil
}

// --- helpers ---------------------------------------------------------------

func errPayload(msg string) map[string]any {
	return map[string]any{"error": msg}
}

func nodeListItem(n model.Node, nm *node.NodeManager) map[string]any {
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
	if n.CountryCode != "" {
		item["country_code"] = n.CountryCode
	}
	if n.City != "" {
		item["city"] = n.City
	}
	if n.ASN != "" {
		item["asn"] = n.ASN
	}
	if metrics := nm.GetMetrics(n.ID); metrics != nil {
		item["cpu"] = metrics.CPU
		item["memory"] = metrics.Memory
		item["disk"] = metrics.Disk
	}
	if n.LastHeartbeat != nil {
		item["last_heartbeat"] = n.LastHeartbeat
	}
	return item
}

func nodeDetail(n model.Node, nm *node.NodeManager) map[string]any {
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
		"created_at":      n.CreatedAt,
	}
	if n.Alias != nil {
		info["alias"] = *n.Alias
	}
	if n.CountryCode != "" {
		info["country_code"] = n.CountryCode
	}
	if n.City != "" {
		info["city"] = n.City
	}
	if n.ASN != "" {
		info["asn"] = n.ASN
	}
	if n.LastHeartbeat != nil {
		info["last_heartbeat"] = n.LastHeartbeat
	}
	if metrics := nm.GetMetrics(n.ID); metrics != nil {
		info["metrics"] = map[string]any{
			"cpu":      metrics.CPU,
			"memory":   metrics.Memory,
			"disk":     metrics.Disk,
			"uptime":   metrics.Uptime,
			"load_avg": metrics.LoadAvg,
		}
	}
	return info
}

