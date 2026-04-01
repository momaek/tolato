package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/server/agentapi"
	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
)

// ---------------------------------------------------------------------------
// list_nodes
// ---------------------------------------------------------------------------

type listNodesTool struct {
	source NodeSource
}

func NewListNodesTool(source NodeSource) Tool {
	return listNodesTool{source: source}
}

func (t listNodesTool) Name() string { return "list_nodes" }

func (t listNodesTool) Definition() agentapi.ToolSpec {
	return listNodesToolSpec()
}

func (t listNodesTool) Call(ctx context.Context, call agentapi.Item) (ToolResult, error) {
	if t.source == nil {
		return ToolResult{}, fmt.Errorf("node source is not configured")
	}

	var req ListNodesInput
	if args := agentapi.ArgumentsJSON(call); len(args) > 0 {
		if err := json.Unmarshal(args, &req); err != nil {
			return ToolResult{}, err
		}
	}

	nodes, err := t.source.ListNodes(ctx)
	if err != nil {
		return ToolResult{}, err
	}
	nodes = filterNodes(nodes, req)

	if req.Limit > 0 && len(nodes) > req.Limit {
		nodes = nodes[:req.Limit]
	}

	payload, err := json.Marshal(ListNodesOutput{Nodes: nodes})
	if err != nil {
		return ToolResult{}, err
	}

	output := string(payload)
	return ToolResult{
		OutputItem:  agentapi.FunctionCallOutput(call.CallID, output),
		MetaText:    fmt.Sprintf("listed %d nodes", len(nodes)),
		ToolMessage: payload,
	}, nil
}

// ---------------------------------------------------------------------------
// run_on_node
// ---------------------------------------------------------------------------

type contextKey string

const sessionIDKey contextKey = "sessionID"

// ContextWithSessionID attaches a session ID to a context for tool use.
func ContextWithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

func sessionIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(sessionIDKey).(string)
	return v
}

// ExecutionWaiter allows tools to block until async execution completes.
type ExecutionWaiter interface {
	Register(taskID string) <-chan domain.ExecutionResult
	Remove(taskID string)
}

// ExecutionResultQuerier fetches execution results after completion.
type ExecutionResultQuerier interface {
	ListByTask(ctx context.Context, taskID string) ([]domain.Execution, error)
}

type runOnNodeTool struct {
	source    NodeSource
	execution ExecutionStarter
	waiter    ExecutionWaiter
	results   ExecutionResultQuerier
	tokens    *ConfirmTokenStore
}

func NewRunOnNodeTool(
	source NodeSource,
	execution ExecutionStarter,
	waiter ExecutionWaiter,
	results ExecutionResultQuerier,
	tokens *ConfirmTokenStore,
) Tool {
	return &runOnNodeTool{
		source:    source,
		execution: execution,
		waiter:    waiter,
		results:   results,
		tokens:    tokens,
	}
}

func (t *runOnNodeTool) Name() string { return "run_on_node" }

func (t *runOnNodeTool) Definition() agentapi.ToolSpec {
	return runOnNodeToolSpec()
}

func (t *runOnNodeTool) Call(ctx context.Context, call agentapi.Item) (ToolResult, error) {
	if t.source == nil || t.execution == nil {
		return ToolResult{}, fmt.Errorf("run_on_node dependencies not configured")
	}

	var req RunOnNodeInput
	if args := agentapi.ArgumentsJSON(call); len(args) > 0 {
		if err := json.Unmarshal(args, &req); err != nil {
			return ToolResult{}, err
		}
	}
	if strings.TrimSpace(req.Target) == "" {
		return ToolResult{}, fmt.Errorf("target is required")
	}
	if strings.TrimSpace(req.Command) == "" {
		return ToolResult{}, fmt.Errorf("command is required")
	}

	// If a confirm token is provided, validate and execute directly.
	if req.ConfirmToken != "" {
		return t.executeWithToken(ctx, call, req)
	}

	// Resolve target.
	nodes, err := t.source.ListNodes(ctx)
	if err != nil {
		return ToolResult{}, err
	}

	matched := matchTargetNodes(req.Target, nodes)

	switch len(matched) {
	case 0:
		return t.outputResult(call, RunOnNodeOutput{
			Status:     "no_match",
			Candidates: nodes,
			Message:    fmt.Sprintf("No node matching %q found. Available nodes are listed in candidates.", req.Target),
		})

	case 1:
		// Single match — check risk.
		risk := inferRisk(req.Command + " " + strings.Join(req.Args, " "))
		if risk == domain.RiskLevelForbidden {
			return ToolResult{}, fmt.Errorf("blocked by policy: operation is forbidden")
		}
		if risk == domain.RiskLevelLow {
			return t.dispatchAndWait(ctx, call, req, matched)
		}
		// Medium/high risk — require confirmation.
		token := t.tokens.Generate(nodeIDs(matched), req.Command, req.Args)
		return t.outputResult(call, RunOnNodeOutput{
			Status:       "needs_confirmation",
			ConfirmToken: token,
			Candidates:   matched,
			Message:      fmt.Sprintf("Will execute %s on %s. This is a %s-risk operation. Pass confirm_token to proceed.", req.Command, matched[0].Hostname, risk),
		})

	default:
		// Check if target is "all" — treat as multi-node with confirmation.
		if isAllQuery(req.Target) {
			risk := inferRisk(req.Command + " " + strings.Join(req.Args, " "))
			if risk == domain.RiskLevelForbidden {
				return ToolResult{}, fmt.Errorf("blocked by policy: operation is forbidden")
			}
			if risk == domain.RiskLevelLow {
				return t.dispatchAndWait(ctx, call, req, matched)
			}
			token := t.tokens.Generate(nodeIDs(matched), req.Command, req.Args)
			return t.outputResult(call, RunOnNodeOutput{
				Status:       "needs_confirmation",
				ConfirmToken: token,
				Candidates:   matched,
				Message:      fmt.Sprintf("Will execute %s on %d nodes. This is a %s-risk operation. Pass confirm_token to proceed.", req.Command, len(matched), risk),
			})
		}

		return t.outputResult(call, RunOnNodeOutput{
			Status:     "ambiguous",
			Candidates: matched,
			Message:    fmt.Sprintf("Multiple nodes match %q. Please specify which one.", req.Target),
		})
	}
}

func (t *runOnNodeTool) executeWithToken(ctx context.Context, call agentapi.Item, req RunOnNodeInput) (ToolResult, error) {
	nodeIDList, command, args, ok := t.tokens.Validate(req.ConfirmToken)
	if !ok {
		return t.outputResult(call, RunOnNodeOutput{
			Status:  "error",
			Message: "Invalid or expired confirmation token. Please request the operation again.",
		})
	}

	// Reconstruct matched nodes from IDs.
	allNodes, err := t.source.ListNodes(ctx)
	if err != nil {
		return ToolResult{}, err
	}
	var matched []NodeSummary
	nodeSet := make(map[string]bool, len(nodeIDList))
	for _, id := range nodeIDList {
		nodeSet[id] = true
	}
	for _, n := range allNodes {
		if nodeSet[n.ID] {
			matched = append(matched, n)
		}
	}
	if len(matched) == 0 {
		return t.outputResult(call, RunOnNodeOutput{
			Status:  "error",
			Message: "Confirmed nodes are no longer available.",
		})
	}

	// Use the stored command/args from the token, not from the request.
	req.Command = command
	req.Args = args
	return t.dispatchAndWait(ctx, call, req, matched)
}

func (t *runOnNodeTool) dispatchAndWait(ctx context.Context, call agentapi.Item, req RunOnNodeInput, matched []NodeSummary) (ToolResult, error) {
	sessionID := sessionIDFromContext(ctx)
	if sessionID == "" {
		return ToolResult{}, fmt.Errorf("session ID not found in context")
	}

	ids := nodeIDs(matched)
	displayLabel := displayLabelForNodes(matched)

	// Build target context for execution service.
	targetCtx := domain.ActiveTargetContext{
		Status:       domain.TargetStatusConfirmed,
		Scope:        domain.TargetScopeSingle,
		NodeIDs:      ids,
		DisplayLabel: displayLabel,
		Source:       domain.TargetSourceAssistantResolved,
	}
	if len(ids) > 1 {
		targetCtx.Scope = domain.TargetScopeMulti
	}

	risk := inferRisk(req.Command + " " + strings.Join(req.Args, " "))
	if risk == domain.RiskLevelForbidden {
		risk = domain.RiskLevelHigh
	}

	dispatchInput := appexecution.StartDispatchInput{
		SessionID:     sessionID,
		InputText:     req.Command,
		Command:       req.Command,
		CommandArgs:   req.Args,
		TargetContext: targetCtx,
		RiskLevel:     risk,
	}

	dispatch, err := t.execution.StartDispatch(ctx, dispatchInput)
	if err != nil {
		return ToolResult{}, fmt.Errorf("dispatch failed: %w", err)
	}

	// Register waiter and wait for completion.
	var doneCh <-chan domain.ExecutionResult
	if t.waiter != nil {
		doneCh = t.waiter.Register(dispatch.TaskID)
	}

	timeout := 300 * time.Second
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if doneCh != nil {
		select {
		case result := <-doneCh:
			return t.buildCompletedResult(ctx, call, req, matched, dispatch.TaskID, result)
		case <-timeoutCtx.Done():
			if t.waiter != nil {
				t.waiter.Remove(dispatch.TaskID)
			}
			return t.outputResult(call, RunOnNodeOutput{
				Status:  "error",
				Message: fmt.Sprintf("Execution timed out after %v", timeout),
			})
		}
	}

	// No waiter available — return immediately with task info.
	return t.outputResult(call, RunOnNodeOutput{
		Status:  "completed",
		Message: fmt.Sprintf("Dispatched %s to %d node(s). Task ID: %s", req.Command, len(matched), dispatch.TaskID),
	})
}

func (t *runOnNodeTool) buildCompletedResult(ctx context.Context, call agentapi.Item, req RunOnNodeInput, matched []NodeSummary, taskID string, execResult domain.ExecutionResult) (ToolResult, error) {
	results := make([]NodeExecResult, 0, len(matched))

	if t.results != nil {
		executions, err := t.results.ListByTask(ctx, taskID)
		if err == nil {
			hostnameByID := make(map[string]string, len(matched))
			for _, n := range matched {
				hostnameByID[n.ID] = n.Hostname
			}
			for _, exec := range executions {
				status := "success"
				switch exec.Status {
				case domain.ExecutionStatusFailed:
					status = "failed"
				case domain.ExecutionStatusTimeout:
					status = "timeout"
				case domain.ExecutionStatusCancelled:
					status = "cancelled"
				}
				output := exec.StdoutTail
				if exec.StderrTail != "" {
					if output != "" {
						output += "\n--- stderr ---\n"
					}
					output += exec.StderrTail
				}
				exitCode := 0
				if exec.ExitCode != nil {
					exitCode = *exec.ExitCode
				}
				results = append(results, NodeExecResult{
					NodeID:   exec.NodeID,
					Hostname: hostnameByID[exec.NodeID],
					Output:   output,
					ExitCode: exitCode,
					Status:   status,
				})
			}
		}
	}

	if len(results) == 0 {
		// Fallback: use aggregate info.
		agg := execResult.Aggregate
		return t.outputResult(call, RunOnNodeOutput{
			Status:  "completed",
			Message: fmt.Sprintf("Execution finished: %d success, %d failed, %d timeout", agg.Success, agg.Failed, agg.Timeout),
		})
	}

	return t.outputResult(call, RunOnNodeOutput{
		Status:  "completed",
		Results: results,
	})
}

func (t *runOnNodeTool) outputResult(call agentapi.Item, output RunOnNodeOutput) (ToolResult, error) {
	payload, err := json.Marshal(output)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{
		OutputItem:  agentapi.FunctionCallOutput(call.CallID, string(payload)),
		MetaText:    output.Message,
		ToolMessage: payload,
	}, nil
}

// ---------------------------------------------------------------------------
// Target matching helpers (reused from previous implementation)
// ---------------------------------------------------------------------------

func matchTargetNodes(target string, nodes []NodeSummary) []NodeSummary {
	query := strings.ToLower(strings.TrimSpace(target))
	if query == "" {
		return nil
	}

	if isAllQuery(query) {
		return nodes
	}

	var matched []NodeSummary
	for _, node := range nodes {
		if matchBy, _ := matchNode(node, query); matchBy != "" {
			matched = append(matched, node)
		}
	}
	return matched
}

func isAllQuery(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	return q == "all" || q == "all online" || q == "all nodes" || q == "all hosts" ||
		q == "所有" || q == "所有节点" || q == "全部" || q == "全部节点"
}

func filterNodes(nodes []NodeSummary, req ListNodesInput) []NodeSummary {
	out := make([]NodeSummary, 0, len(nodes))
	query := strings.ToLower(strings.TrimSpace(req.Query))
	region := strings.ToLower(strings.TrimSpace(req.Region))
	tag := strings.ToLower(strings.TrimSpace(req.Tag))
	status := strings.ToLower(strings.TrimSpace(req.Status))

	for _, node := range nodes {
		if query != "" && !nodeMatchesQuery(node, query) {
			continue
		}
		if region != "" && strings.ToLower(node.Region) != region {
			continue
		}
		if tag != "" && !nodeHasTag(node, tag) {
			continue
		}
		if status != "" && strings.ToLower(node.Status) != status {
			continue
		}
		if req.Busy != nil && node.Busy != *req.Busy {
			continue
		}
		out = append(out, node)
	}
	return out
}

func nodeMatchesQuery(node NodeSummary, query string) bool {
	if query == "" {
		return true
	}
	if strings.Contains(strings.ToLower(node.ID), query) ||
		strings.Contains(strings.ToLower(node.Hostname), query) ||
		strings.Contains(strings.ToLower(node.Region), query) {
		return true
	}
	for _, tag := range node.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func matchNode(node NodeSummary, query string) (string, string) {
	if query == "" {
		return "", ""
	}
	if strings.EqualFold(node.ID, query) || strings.Contains(strings.ToLower(node.ID), query) {
		return "id", "matched node id"
	}
	if strings.Contains(strings.ToLower(node.Hostname), query) {
		return "hostname", "matched hostname"
	}
	if strings.Contains(strings.ToLower(node.Region), query) {
		return "region", "matched region"
	}
	for _, tag := range node.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return "tag", "matched tag"
		}
	}
	return "", ""
}

func nodeHasTag(node NodeSummary, tag string) bool {
	for _, item := range node.Tags {
		if strings.ToLower(item) == tag {
			return true
		}
	}
	return false
}

func nodeIDs(nodes []NodeSummary) []string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

func displayLabelForNodes(nodes []NodeSummary) string {
	switch len(nodes) {
	case 0:
		return "unknown target"
	case 1:
		return nodes[0].Hostname
	default:
		names := make([]string, 0, len(nodes))
		for _, node := range nodes {
			names = append(names, node.Hostname)
		}
		if len(names) > 3 {
			return fmt.Sprintf("%d nodes", len(names))
		}
		return strings.Join(names, ", ")
	}
}

func inferRisk(text string) domain.RiskLevel {
	lower := " " + strings.ToLower(text) + " "
	switch {
	case containsAny(lower, "rm -rf /", "mkfs", "dd if=/dev/zero", "drop database", "wipe disk"):
		return domain.RiskLevelForbidden
	case containsWordAny(lower, "restart", "reboot", "delete", "destroy", "shutdown") ||
		containsAny(lower, " rm ", " drop "):
		return domain.RiskLevelHigh
	case containsWordAny(lower, "reload", "scale", "migrate", "upgrade"):
		return domain.RiskLevelMedium
	default:
		return domain.RiskLevelLow
	}
}

func containsWordAny(text string, words ...string) bool {
	for _, word := range words {
		if strings.Contains(text, " "+word+" ") ||
			strings.Contains(text, " "+word+"\t") ||
			strings.Contains(text, " "+word+"\n") {
			return true
		}
	}
	return false
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
