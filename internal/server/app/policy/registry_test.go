package policy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/agentapi"
	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
)

func TestRegistryDefinitions(t *testing.T) {
	registry := NewRegistry(fakeNodeSource{nodes: sampleNodes()})
	defs := registry.Definitions()

	want := []string{"list_nodes", "resolve_target_nodes", "request_target_confirmation", "propose_plan", "request_approval", "exec_on_nodes", "summarize_execution"}
	if len(defs) != len(want) {
		t.Fatalf("len(defs) = %d, want %d", len(defs), len(want))
	}
	for i, def := range defs {
		if def.Function.Name != want[i] {
			t.Fatalf("defs[%d].Function.Name = %q, want %q", i, def.Function.Name, want[i])
		}
	}
}

func TestListNodesTool(t *testing.T) {
	registry := NewRegistry(fakeNodeSource{nodes: sampleNodes()})

	raw := mustJSON(t, ListNodesInput{Region: "asia", Tag: "prod", Busy: boolPtr(true)})
	result, err := registry.Call(context.Background(), functionCall("list_nodes", raw))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if result.MetaText != "listed 1 nodes" {
		t.Fatalf("MetaText = %q", result.MetaText)
	}
	var output ListNodesOutput
	if err := json.Unmarshal(result.ToolMessage, &output); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(output.Nodes) != 1 || output.Nodes[0].Hostname != "jp-tokyo-01" {
		t.Fatalf("output = %#v", output)
	}
}

func TestResolveTargetNodesTool(t *testing.T) {
	registry := NewRegistry(fakeNodeSource{nodes: sampleNodes()})

	result, err := registry.Call(context.Background(), functionCall("resolve_target_nodes", mustJSON(t, ResolveTargetNodesInput{
		Query: "tokyo",
	})))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	var output ResolveTargetNodesOutput
	if err := json.Unmarshal(result.ToolMessage, &output); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if output.TargetContext.Status != domain.TargetStatusPendingConfirmation {
		t.Fatalf("status = %q", output.TargetContext.Status)
	}
	if len(output.TargetContext.NodeIDs) != 1 || output.TargetContext.NodeIDs[0] != "node-1" {
		t.Fatalf("target context = %#v", output.TargetContext)
	}
	if len(output.Candidates) != 1 || output.Candidates[0].MatchedBy != "hostname" {
		t.Fatalf("candidates = %#v", output.Candidates)
	}
}

func TestRequestTargetConfirmationTool(t *testing.T) {
	tool := NewRequestTargetConfirmationTool()
	ctxValue := domain.ActiveTargetContext{
		Status:       domain.TargetStatusPendingConfirmation,
		Scope:        domain.TargetScopeMulti,
		NodeIDs:      []string{"node-1", "node-2"},
		DisplayLabel: "two nodes",
		Source:       domain.TargetSourceAssistantResolved,
	}

	result, err := tool.Call(context.Background(), functionCall("request_target_confirmation", mustJSON(t, RequestTargetConfirmationInput{
		TargetContext: ctxValue,
		Message:       "confirm the selected nodes",
	})))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if !result.WaitForUser {
		t.Fatal("WaitForUser = false, want true")
	}
	if result.PendingActionType != domain.PendingActionTypeTargetConfirmation {
		t.Fatalf("PendingActionType = %q", result.PendingActionType)
	}
	if len(result.PendingActionPayload) == 0 {
		t.Fatal("expected pending action payload")
	}
}

func TestProposePlanTool(t *testing.T) {
	registry := NewRegistry(fakeNodeSource{nodes: sampleNodes()})

	result, err := registry.Call(context.Background(), functionCall("propose_plan", mustJSON(t, ProposePlanInput{
		InputText: "restart nginx on jp-tokyo-01",
		TargetContext: domain.ActiveTargetContext{
			Status:       domain.TargetStatusPendingConfirmation,
			Scope:        domain.TargetScopeSingle,
			NodeIDs:      []string{"node-1"},
			DisplayLabel: "jp-tokyo-01",
			Source:       domain.TargetSourceAssistantResolved,
		},
	})))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if !result.AppendPlanRow {
		t.Fatal("AppendPlanRow = false, want true")
	}
	var output ProposedPlan
	if err := json.Unmarshal(result.ToolMessage, &output); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if output.RiskLevel != domain.RiskLevelHigh {
		t.Fatalf("RiskLevel = %q, want high", output.RiskLevel)
	}
	if !output.RequiresApproval {
		t.Fatal("RequiresApproval = false, want true for high-risk plan")
	}
}

func TestRequestApprovalTool(t *testing.T) {
	registry := NewRegistry(fakeNodeSource{nodes: sampleNodes()})

	result, err := registry.Call(context.Background(), functionCall("request_approval", mustJSON(t, RequestApprovalInput{
		TaskID:    "task-7",
		RiskLevel: domain.RiskLevelHigh,
		Reason:    "restart production nginx",
	})))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if !result.WaitForUser {
		t.Fatal("WaitForUser = false, want true")
	}
	if result.PendingActionType != domain.PendingActionTypeApproval {
		t.Fatalf("PendingActionType = %q, want approval", result.PendingActionType)
	}
	if !result.AppendApprovalRow {
		t.Fatal("AppendApprovalRow = false, want true")
	}
	if result.TaskID != "task-7" {
		t.Fatalf("TaskID = %q, want task-7", result.TaskID)
	}

	var output RequestApprovalOutput
	if err := json.Unmarshal(result.ToolMessage, &output); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if output.TaskID != "task-7" || output.RiskLevel != domain.RiskLevelHigh || !output.RequiresApproval {
		t.Fatalf("output = %#v", output)
	}
}

func TestExecOnNodesTool(t *testing.T) {
	starter := &fakeExecutionStarter{
		result: appexecution.StartDispatchResult{
			TaskID:           "task-9",
			ExecutionGroupID: "group-9",
			ExecutionIDs:     []string{"exec-1", "exec-2"},
		},
	}
	registry := NewRegistry(fakeNodeSource{nodes: sampleNodes()}, WithExecutionStarter(starter))

	result, err := registry.Call(context.Background(), functionCall("exec_on_nodes", mustJSON(t, ExecOnNodesInput{
		SessionID: "sess-1",
		InputText: "run diagnostics",
		Command:   "uptime",
		TargetContext: domain.ActiveTargetContext{
			Status:       domain.TargetStatusConfirmed,
			Scope:        domain.TargetScopeMulti,
			NodeIDs:      []string{"node-1", "node-2"},
			DisplayLabel: "2 targets",
			Source:       domain.TargetSourceUserExplicit,
		},
		RiskLevel: domain.RiskLevelLow,
	})))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if !result.AsyncExecutionStarted || !result.AppendExecutionRow || result.TaskID != "task-9" || result.ExecutionGroupID != "group-9" {
		t.Fatalf("result = %#v", result)
	}
	if starter.input.SessionID != "sess-1" || len(starter.input.TargetContext.NodeIDs) != 2 {
		t.Fatalf("starter input = %#v", starter.input)
	}
	if starter.input.Command != "uptime" || len(starter.input.CommandArgs) != 0 {
		t.Fatalf("starter command = %#v, want explicit command propagated", starter.input)
	}
}

func TestInferRiskForbidden(t *testing.T) {
	if risk := inferRisk("rm -rf /var/lib/app"); risk != domain.RiskLevelForbidden {
		t.Fatalf("inferRisk() = %q, want forbidden", risk)
	}
}

func TestSummarizeExecutionTool(t *testing.T) {
	registry := NewRegistry(fakeNodeSource{nodes: sampleNodes()})

	result, err := registry.Call(context.Background(), functionCall("summarize_execution", mustJSON(t, SummarizeExecutionInput{
		TaskID:      "task-10",
		Status:      domain.TaskStatusPartialFailed,
		TargetLabel: "tokyo batch",
		Aggregate: domain.ExecutionAggregate{
			Total:   3,
			Success: 2,
			Failed:  1,
		},
	})))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if !result.AppendSummaryRow || result.TaskID != "task-10" {
		t.Fatalf("result = %#v", result)
	}
	var output SummarizeExecutionOutput
	if err := json.Unmarshal(result.ToolMessage, &output); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if output.TaskID != "task-10" || output.Status != domain.TaskStatusPartialFailed {
		t.Fatalf("output = %#v", output)
	}
}

type fakeNodeSource struct {
	nodes []NodeSummary
}

type fakeExecutionStarter struct {
	input  appexecution.StartDispatchInput
	result appexecution.StartDispatchResult
	err    error
}

func (f *fakeExecutionStarter) StartDispatch(ctx context.Context, input appexecution.StartDispatchInput) (appexecution.StartDispatchResult, error) {
	_ = ctx
	f.input = input
	return f.result, f.err
}

func (f *fakeExecutionStarter) CancelTask(ctx context.Context, sessionID string, taskID string, idempotencyKey string) error {
	_ = ctx
	_ = sessionID
	_ = taskID
	_ = idempotencyKey
	return nil
}

func (f *fakeExecutionStarter) RecordChunk(ctx context.Context, input appexecution.RecordChunkInput) error {
	_ = ctx
	_ = input
	return nil
}

func functionCall(name string, raw json.RawMessage) agentapi.Item {
	return agentapi.Item{
		Type:      "function_call",
		Name:      name,
		Arguments: string(raw),
		CallID:    "call_" + name,
	}
}

func (f *fakeExecutionStarter) FinishExecution(ctx context.Context, input appexecution.FinishExecutionInput) error {
	_ = ctx
	_ = input
	return nil
}

func (f fakeNodeSource) ListNodes(ctx context.Context) ([]NodeSummary, error) {
	return append([]NodeSummary(nil), f.nodes...), nil
}

func sampleNodes() []NodeSummary {
	now := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	return []NodeSummary{
		{
			ID:       "node-1",
			Hostname: "jp-tokyo-01",
			Region:   "asia",
			OS:       "Debian 11",
			Version:  "1.0.0",
			Tags:     []string{"prod", "web"},
			Status:   "online",
			Busy:     true,
			LastSeen: now.Format(timeLayout),
			Metrics: Metrics{
				CPU:    0.6,
				Memory: 0.7,
				Disk:   0.4,
			},
		},
		{
			ID:       "node-2",
			Hostname: "us-east-02",
			Region:   "us",
			OS:       "Ubuntu 22.04",
			Version:  "1.0.0",
			Tags:     []string{"staging"},
			Status:   "online",
			Busy:     false,
			LastSeen: now.Format(timeLayout),
		},
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}

func boolPtr(v bool) *bool { return &v }
