package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/server/agentapi"
	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
)

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

type resolveTargetNodesTool struct {
	source NodeSource
}

func NewResolveTargetNodesTool(source NodeSource) Tool {
	return resolveTargetNodesTool{source: source}
}

func (t resolveTargetNodesTool) Name() string { return "resolve_target_nodes" }

func (t resolveTargetNodesTool) Definition() agentapi.ToolSpec {
	return resolveTargetNodesToolSpec()
}

func (t resolveTargetNodesTool) Call(ctx context.Context, call agentapi.Item) (ToolResult, error) {
	if t.source == nil {
		return ToolResult{}, fmt.Errorf("node source is not configured")
	}

	var req ResolveTargetNodesInput
	if args := agentapi.ArgumentsJSON(call); len(args) > 0 {
		if err := json.Unmarshal(args, &req); err != nil {
			return ToolResult{}, err
		}
	}

	nodes, err := t.source.ListNodes(ctx)
	if err != nil {
		return ToolResult{}, err
	}

	resolved := resolveTargetNodes(req, nodes)
	payload, err := json.Marshal(resolved)
	if err != nil {
		return ToolResult{}, err
	}

	output := string(payload)
	return ToolResult{
		OutputItem:  agentapi.FunctionCallOutput(call.CallID, output),
		MetaText:    fmt.Sprintf("resolved %d target candidate(s)", len(resolved.Candidates)),
		ToolMessage: payload,
	}, nil
}

type requestTargetConfirmationTool struct{}

func NewRequestTargetConfirmationTool() Tool {
	return requestTargetConfirmationTool{}
}

func (t requestTargetConfirmationTool) Name() string { return "request_target_confirmation" }

func (t requestTargetConfirmationTool) Definition() agentapi.ToolSpec {
	return requestTargetConfirmationToolSpec()
}

func (t requestTargetConfirmationTool) Call(ctx context.Context, call agentapi.Item) (ToolResult, error) {
	var req RequestTargetConfirmationInput
	if args := agentapi.ArgumentsJSON(call); len(args) > 0 {
		if err := json.Unmarshal(args, &req); err != nil {
			return ToolResult{}, err
		}
	}
	if req.TargetContext.Status == "" {
		req.TargetContext.Status = domain.TargetStatusPendingConfirmation
	}
	if req.TargetContext.Source == "" {
		req.TargetContext.Source = domain.TargetSourceAssistantResolved
	}
	payload, err := json.Marshal(RequestTargetConfirmationOutput{
		TargetContext: req.TargetContext,
		Message:       confirmationMessage(req.TargetContext),
	})
	if err != nil {
		return ToolResult{}, err
	}

	output := string(payload)
	return ToolResult{
		OutputItem:           agentapi.FunctionCallOutput(call.CallID, output),
		MetaText:             confirmationMessage(req.TargetContext),
		ToolMessage:          payload,
		WaitForUser:          true,
		PendingActionType:    domain.PendingActionTypeTargetConfirmation,
		PendingActionPayload: payload,
	}, nil
}

type proposePlanTool struct{}

func NewProposePlanTool() Tool {
	return proposePlanTool{}
}

func (t proposePlanTool) Name() string { return "propose_plan" }

func (t proposePlanTool) Definition() agentapi.ToolSpec {
	return proposePlanToolSpec()
}

func (t proposePlanTool) Call(ctx context.Context, call agentapi.Item) (ToolResult, error) {
	var req ProposePlanInput
	if args := agentapi.ArgumentsJSON(call); len(args) > 0 {
		if err := json.Unmarshal(args, &req); err != nil {
			return ToolResult{}, err
		}
	}

	plan := buildPlan(req)
	payload, err := json.Marshal(plan)
	if err != nil {
		return ToolResult{}, err
	}

	output := string(payload)
	return ToolResult{
		OutputItem:    agentapi.FunctionCallOutput(call.CallID, output),
		MetaText:      plan.Summary,
		ToolMessage:   payload,
		AppendPlanRow: true,
	}, nil
}

type requestApprovalTool struct{}

func NewRequestApprovalTool() Tool {
	return requestApprovalTool{}
}

func (t requestApprovalTool) Name() string { return "request_approval" }

func (t requestApprovalTool) Definition() agentapi.ToolSpec {
	return requestApprovalToolSpec()
}

func (t requestApprovalTool) Call(ctx context.Context, call agentapi.Item) (ToolResult, error) {
	var req RequestApprovalInput
	if args := agentapi.ArgumentsJSON(call); len(args) > 0 {
		if err := json.Unmarshal(args, &req); err != nil {
			return ToolResult{}, err
		}
	}
	if req.TaskID == "" {
		return ToolResult{}, fmt.Errorf("taskId is required")
	}
	if req.RiskLevel == "" {
		req.RiskLevel = domain.RiskLevelMedium
	}

	requiresApproval := true
	if req.RequiresApproval != nil {
		requiresApproval = *req.RequiresApproval
	}
	message := approvalMessage(req)
	payload, err := json.Marshal(RequestApprovalOutput{
		TaskID:           req.TaskID,
		RiskLevel:        req.RiskLevel,
		Message:          message,
		RequiresApproval: requiresApproval,
	})
	if err != nil {
		return ToolResult{}, err
	}

	output := string(payload)
	return ToolResult{
		OutputItem:           agentapi.FunctionCallOutput(call.CallID, output),
		MetaText:             message,
		ToolMessage:          payload,
		WaitForUser:          requiresApproval,
		PendingActionType:    domain.PendingActionTypeApproval,
		PendingActionPayload: payload,
		TaskID:               req.TaskID,
		AppendApprovalRow:    requiresApproval,
	}, nil
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

func approvalMessage(req RequestApprovalInput) string {
	if req.Message != "" {
		return req.Message
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "potentially disruptive change"
	}
	return fmt.Sprintf("%s; requires explicit approval.", reason)
}

type execOnNodesTool struct {
	execution ExecutionStarter
}

func NewExecOnNodesTool(execution ExecutionStarter) Tool {
	return execOnNodesTool{execution: execution}
}

func (t execOnNodesTool) Name() string { return "exec_on_nodes" }

func (t execOnNodesTool) Definition() agentapi.ToolSpec {
	return execOnNodesToolSpec()
}

func (t execOnNodesTool) Call(ctx context.Context, call agentapi.Item) (ToolResult, error) {
	if t.execution == nil {
		return ToolResult{}, fmt.Errorf("execution starter is not configured")
	}

	var req ExecOnNodesInput
	if args := agentapi.ArgumentsJSON(call); len(args) > 0 {
		if err := json.Unmarshal(args, &req); err != nil {
			return ToolResult{}, err
		}
	}
	if req.SessionID == "" || len(req.TargetContext.NodeIDs) == 0 {
		return ToolResult{}, fmt.Errorf("sessionId and targetContext.nodeIds are required")
	}
	if strings.TrimSpace(req.Command) == "" {
		return ToolResult{}, fmt.Errorf("command is required")
	}
	if req.RiskLevel == "" {
		req.RiskLevel = inferRisk(req.InputText)
	}
	if req.RiskLevel == domain.RiskLevelForbidden {
		return ToolResult{}, fmt.Errorf("blocked by policy")
	}

	dispatch, err := t.execution.StartDispatch(ctx, appexecution.StartDispatchInput{
		SessionID:     req.SessionID,
		InputText:     req.InputText,
		Command:       req.Command,
		CommandArgs:   req.CommandArgs,
		TargetContext: req.TargetContext,
		RiskLevel:     req.RiskLevel,
	})
	if err != nil {
		return ToolResult{}, err
	}

	message := fmt.Sprintf("queued execution for %d node(s)", len(req.TargetContext.NodeIDs))
	payload, err := json.Marshal(ExecOnNodesOutput{
		TaskID:           dispatch.TaskID,
		ExecutionGroupID: dispatch.ExecutionGroupID,
		NodeIDs:          append([]string(nil), req.TargetContext.NodeIDs...),
		RiskLevel:        req.RiskLevel,
		Message:          message,
	})
	if err != nil {
		return ToolResult{}, err
	}

	output := string(payload)
	return ToolResult{
		OutputItem:            agentapi.FunctionCallOutput(call.CallID, output),
		MetaText:              message,
		ToolMessage:           payload,
		AsyncExecutionStarted: true,
		TaskID:                dispatch.TaskID,
		ExecutionGroupID:      dispatch.ExecutionGroupID,
		AppendExecutionRow:    true,
	}, nil
}

type summarizeExecutionTool struct{}

func NewSummarizeExecutionTool() Tool {
	return summarizeExecutionTool{}
}

func (t summarizeExecutionTool) Name() string { return "summarize_execution" }

func (t summarizeExecutionTool) Definition() agentapi.ToolSpec {
	return summarizeExecutionToolSpec()
}

func (t summarizeExecutionTool) Call(ctx context.Context, call agentapi.Item) (ToolResult, error) {
	_ = ctx

	var req SummarizeExecutionInput
	if args := agentapi.ArgumentsJSON(call); len(args) > 0 {
		if err := json.Unmarshal(args, &req); err != nil {
			return ToolResult{}, err
		}
	}
	if req.TaskID == "" {
		return ToolResult{}, fmt.Errorf("taskId is required")
	}

	summary := executionSummary(req)
	payload, err := json.Marshal(SummarizeExecutionOutput{
		TaskID:    req.TaskID,
		Status:    req.Status,
		Aggregate: req.Aggregate,
		Summary:   summary,
	})
	if err != nil {
		return ToolResult{}, err
	}

	output := string(payload)
	return ToolResult{
		OutputItem:       agentapi.FunctionCallOutput(call.CallID, output),
		MetaText:         summary,
		ToolMessage:      payload,
		TaskID:           req.TaskID,
		AppendSummaryRow: true,
	}, nil
}

func resolveTargetNodes(req ResolveTargetNodesInput, nodes []NodeSummary) ResolveTargetNodesOutput {
	query := strings.ToLower(strings.TrimSpace(req.Query))
	candidates := make([]domain.TargetCandidate, 0)
	matchedNodes := make([]NodeSummary, 0)

	if query == "" && req.CurrentTargetContext != nil {
		ctxValue := cloneTargetContext(*req.CurrentTargetContext)
		ctxValue.Status = domain.TargetStatusConfirmed
		return ResolveTargetNodesOutput{
			Query:         req.Query,
			TargetContext: ctxValue,
			Nodes:         matchedNodes,
		}
	}

	if isAllOnlineQuery(query) {
		for _, node := range nodes {
			matchedNodes = append(matchedNodes, node)
			candidates = append(candidates, candidateFromNode(node, "all_online", "matched all online nodes"))
		}
		return ResolveTargetNodesOutput{
			Query: req.Query,
			TargetContext: domain.ActiveTargetContext{
				Status:       domain.TargetStatusPendingConfirmation,
				Scope:        domain.TargetScopeAllOnline,
				NodeIDs:      nodeIDs(matchedNodes),
				DisplayLabel: "All online nodes",
				Source:       domain.TargetSourceAssistantResolved,
				Confidence:   0.75,
				Candidates:   candidates,
			},
			Candidates: candidates,
			Nodes:      matchedNodes,
		}
	}

	for _, node := range nodes {
		matchBy, reason := matchNode(node, query)
		if matchBy == "" {
			continue
		}
		matchedNodes = append(matchedNodes, node)
		candidates = append(candidates, candidateFromNode(node, matchBy, reason))
	}

	if len(matchedNodes) == 0 {
		for _, node := range nodes {
			candidates = append(candidates, candidateFromNode(node, "history", "no direct match, available for manual confirmation"))
		}
		display := req.Query
		if display == "" {
			display = "unknown target"
		}
		return ResolveTargetNodesOutput{
			Query: req.Query,
			TargetContext: domain.ActiveTargetContext{
				Status:       domain.TargetStatusPendingConfirmation,
				Scope:        domain.TargetScopeMulti,
				NodeIDs:      nil,
				DisplayLabel: display,
				Source:       domain.TargetSourceAssistantResolved,
				Confidence:   0.1,
				Candidates:   candidates,
			},
			Candidates: candidates,
			Nodes:      matchedNodes,
		}
	}

	scope := domain.TargetScopeSingle
	if len(matchedNodes) > 1 {
		scope = domain.TargetScopeMulti
	}

	confidence := 0.95
	if len(matchedNodes) > 1 {
		confidence = 0.75
	}

	return ResolveTargetNodesOutput{
		Query: req.Query,
		TargetContext: domain.ActiveTargetContext{
			Status:       domain.TargetStatusPendingConfirmation,
			Scope:        scope,
			NodeIDs:      nodeIDs(matchedNodes),
			DisplayLabel: displayLabelForNodes(matchedNodes),
			Source:       domain.TargetSourceAssistantResolved,
			Confidence:   confidence,
			Candidates:   candidates,
		},
		Candidates: candidates,
		Nodes:      matchedNodes,
	}
}

func executionSummary(req SummarizeExecutionInput) string {
	label := strings.TrimSpace(req.TargetLabel)
	if label == "" {
		label = "target nodes"
	}
	switch req.Status {
	case domain.TaskStatusSuccess:
		return fmt.Sprintf("execution completed successfully on %s (%d/%d succeeded)", label, req.Aggregate.Success, req.Aggregate.Total)
	case domain.TaskStatusFailed:
		return fmt.Sprintf("execution failed on %s (%d/%d failed)", label, req.Aggregate.Failed, req.Aggregate.Total)
	case domain.TaskStatusTimeout:
		return fmt.Sprintf("execution timed out on %s (%d timeout, %d succeeded)", label, req.Aggregate.Timeout, req.Aggregate.Success)
	case domain.TaskStatusCancelled:
		return fmt.Sprintf("execution was cancelled on %s", label)
	default:
		return fmt.Sprintf("execution finished on %s with mixed results (%d succeeded, %d failed, %d timed out, %d cancelled)", label, req.Aggregate.Success, req.Aggregate.Failed, req.Aggregate.Timeout, req.Aggregate.Cancelled)
	}
}

func buildPlan(req ProposePlanInput) ProposedPlan {
	nodes := req.TargetContext.NodeIDs
	if len(nodes) == 0 && req.TargetContext.DisplayLabel != "" {
		nodes = []string{req.TargetContext.DisplayLabel}
	}

	risk := req.RiskLevel
	if risk == "" {
		risk = inferRisk(req.InputText)
	}
	requiresApproval := risk == domain.RiskLevelMedium || risk == domain.RiskLevelHigh
	if req.RequiresApproval != nil {
		requiresApproval = *req.RequiresApproval
	}

	steps := req.Steps
	if len(steps) == 0 {
		steps = []PlanStep{
			{
				Action:           "inspect",
				Args:             map[string]any{"input_text": req.InputText},
				Risk:             risk,
				TimeoutSec:       30,
				BroadcastAllowed: req.TargetContext.Scope == domain.TargetScopeAllOnline,
			},
		}
	}

	return ProposedPlan{
		TargetNodes:      nodes,
		Summary:          summaryForPlan(req.InputText, req.TargetContext),
		EstimatedImpact:  impactForRisk(risk),
		RiskLevel:        risk,
		RequiresApproval: requiresApproval,
		Steps:            steps,
		Metadata: map[string]any{
			"targetScope": req.TargetContext.Scope,
			"source":      req.TargetContext.Source,
		},
		CreatedAt: time.Now().UTC().Format(timeLayout),
	}
}

func inferRisk(text string) domain.RiskLevel {
	lower := strings.ToLower(text)
	switch {
	case containsAny(lower, "rm -rf /", "mkfs", "dd if=/dev/zero", "drop database", "wipe disk"):
		return domain.RiskLevelForbidden
	case containsAny(lower, "restart", "reboot", "delete", "drop ", "rm ", "destroy", "shutdown"):
		return domain.RiskLevelHigh
	case containsAny(lower, "reload", "scale", "migrate", "upgrade"):
		return domain.RiskLevelMedium
	default:
		return domain.RiskLevelLow
	}
}

func summaryForPlan(text string, ctx domain.ActiveTargetContext) string {
	target := ctx.DisplayLabel
	if target == "" {
		target = "selected target"
	}
	if text == "" {
		return "Plan for " + target
	}
	return fmt.Sprintf("Plan for %s: %s", target, text)
}

func impactForRisk(risk domain.RiskLevel) string {
	switch risk {
	case domain.RiskLevelHigh:
		return "Potentially disruptive change; requires explicit approval."
	case domain.RiskLevelMedium:
		return "Moderate operational impact; verify carefully before proceeding."
	case domain.RiskLevelForbidden:
		return "Blocked by policy."
	default:
		return "Read-only or low-impact operation."
	}
}

func confirmationMessage(ctx domain.ActiveTargetContext) string {
	if ctx.DisplayLabel != "" {
		return "confirm target: " + ctx.DisplayLabel
	}
	return "confirm target selection"
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
		return "history", "matched node id"
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
		sort.Strings(names)
		if len(names) > 3 {
			return fmt.Sprintf("%d nodes", len(names))
		}
		return strings.Join(names, ", ")
	}
}

func candidateFromNode(node NodeSummary, matchedBy, reason string) domain.TargetCandidate {
	return domain.TargetCandidate{
		NodeID:    node.ID,
		Hostname:  node.Hostname,
		Region:    node.Region,
		MatchedBy: matchedBy,
		Reason:    reason,
	}
}

func cloneTargetContext(ctx domain.ActiveTargetContext) domain.ActiveTargetContext {
	out := ctx
	out.NodeIDs = append([]string(nil), ctx.NodeIDs...)
	out.Candidates = append([]domain.TargetCandidate(nil), ctx.Candidates...)
	return out
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func isAllOnlineQuery(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	return containsAny(query, "all online", "all nodes", "all hosts", "all")
}

const timeLayout = "2006-01-02T15:04:05Z07:00"
