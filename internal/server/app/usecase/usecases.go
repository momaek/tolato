package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/server/domain/audit"
	"github.com/momaek/tolato/internal/server/domain/node"
	"github.com/momaek/tolato/internal/server/domain/plan"
	"github.com/momaek/tolato/internal/server/domain/policy"
	"github.com/momaek/tolato/internal/server/domain/task"
	"github.com/momaek/tolato/internal/server/infra/idgen"
	"github.com/momaek/tolato/internal/shared/types"
)

type Services struct {
	RegisterNode       RegisterNode
	AuthenticateAgent  AuthenticateAgent
	HeartbeatNode      HeartbeatNode
	GenerateTaskPlan   GenerateTaskPlan
	ApproveTask        ApproveTask
	RejectTask         RejectTask
	CancelTask         CancelTask
	ListNodes          ListNodes
	GetNode            GetNode
	ListTasks          ListTasks
	GetTask            GetTask
	ListTaskExecutions ListTaskExecutions
	ListAuditEvents    ListAuditEvents
	RecordTaskLog      RecordTaskLog
	RecordTaskResult   RecordTaskResult
}

type TaskDispatcher interface {
	DispatchTask(ctx context.Context, nodeID, taskID, executionID string, steps []types.PlanStep, timeoutSec int) error
	CancelTask(ctx context.Context, nodeID, taskID, executionID, reason string) error
}

type dependencies struct {
	Planner         plan.Planner
	SchemaValidator plan.SchemaValidator
	PolicyValidator policy.Validator
	NodeRepo        node.Repository
	SessionStore    node.SessionStore
	TaskRepo        task.Repository
	AuditRepo       audit.Repository
	IDGen           idgen.Generator
	Dispatcher      TaskDispatcher
}

type RegisterNode struct {
	NodeRepo node.Repository
	IDGen    idgen.Generator
}

func (uc RegisterNode) Execute(ctx context.Context, req types.EnrollRequest) (types.EnrollResponse, error) {
	now := time.Now().UTC()
	enrolledNode := node.Node{
		ID:                uc.IDGen.New(),
		Hostname:          req.Hostname,
		Region:            req.Region,
		OS:                req.OS,
		Version:           req.Version,
		Tags:              req.Tags,
		Status:            "registering",
		AuthSecretVersion: 1,
		AgentSecret:       uc.IDGen.New(),
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := uc.NodeRepo.Upsert(ctx, enrolledNode); err != nil {
		return types.EnrollResponse{}, err
	}

	return types.EnrollResponse{
		NodeID: enrolledNode.ID,
		Secret: enrolledNode.AgentSecret,
	}, nil
}

type AuthenticateAgent struct {
	NodeRepo node.Repository
}

func (uc AuthenticateAgent) Execute(ctx context.Context, nodeID, secret string) (*node.Node, error) {
	if nodeID == "" || secret == "" {
		return nil, errors.New("missing node credentials")
	}
	return uc.NodeRepo.GetByAgentCredentials(ctx, nodeID, secret)
}

type HeartbeatInput struct {
	NodeID       string
	SessionID    string
	AgentVersion string
	Capabilities []string
	RemoteAddr   string
}

type HeartbeatNode struct {
	NodeRepo     node.Repository
	SessionStore node.SessionStore
}

func (uc HeartbeatNode) Execute(ctx context.Context, in HeartbeatInput) error {
	now := time.Now().UTC()
	if err := uc.NodeRepo.UpdatePresence(ctx, in.NodeID, in.AgentVersion, "online", now); err != nil {
		return err
	}

	if in.SessionID == "" {
		return nil
	}

	return uc.SessionStore.Upsert(ctx, node.NodeSession{
		NodeID:          in.NodeID,
		SessionID:       in.SessionID,
		ConnectedAt:     now,
		LastHeartbeatAt: now,
		RemoteAddr:      in.RemoteAddr,
		Capabilities:    in.Capabilities,
		Status:          "active",
	})
}

type GenerateTaskPlan struct {
	Planner         plan.Planner
	SchemaValidator plan.SchemaValidator
	PolicyValidator policy.Validator
	NodeRepo        node.Repository
	TaskRepo        task.Repository
	AuditRepo       audit.Repository
	IDGen           idgen.Generator
	Dispatcher      TaskDispatcher
}

func (uc GenerateTaskPlan) Execute(ctx context.Context, req types.TaskPlanRequest) (types.TaskPlanResponse, error) {
	if req.Mode == "" {
		return types.TaskPlanResponse{}, errors.New("mode is required")
	}
	if len(req.Target) == 0 {
		return types.TaskPlanResponse{}, errors.New("target is required")
	}
	if req.InputText == "" {
		return types.TaskPlanResponse{}, errors.New("input_text is required")
	}

	resolvedTargets, err := resolveTargetNodes(ctx, uc.NodeRepo, task.Task{Target: req.Target})
	if err != nil {
		return types.TaskPlanResponse{}, err
	}

	draft, err := uc.Planner.GeneratePlan(ctx, plan.Input{
		Mode:      req.Mode,
		Target:    req.Target,
		InputText: req.InputText,
		Nodes:     resolvedTargets,
	})
	if err != nil {
		return types.TaskPlanResponse{}, err
	}

	if err := uc.SchemaValidator.ValidatePlan(ctx, draft); err != nil {
		draft, err = uc.Planner.RepairPlan(ctx, plan.RepairInput{Original: draft, Reason: err.Error()})
		if err != nil {
			return types.TaskPlanResponse{}, err
		}
		if err := uc.SchemaValidator.ValidatePlan(ctx, draft); err != nil {
			return types.TaskPlanResponse{}, err
		}
	}

	if err := uc.PolicyValidator.ValidatePlan(ctx, &draft); err != nil {
		draft, err = uc.Planner.RepairPlan(ctx, plan.RepairInput{Original: draft, Reason: err.Error()})
		if err != nil {
			return types.TaskPlanResponse{}, err
		}
		if err := uc.PolicyValidator.ValidatePlan(ctx, &draft); err != nil {
			return types.TaskPlanResponse{}, err
		}
	}

	now := time.Now().UTC()
	taskID := uc.IDGen.New()
	approvalStatus := "not_required"
	finalStatus := "approved"
	if draft.RequiresApproval {
		approvalStatus = "pending"
		finalStatus = "waiting_approval"
	}

	model := task.Task{
		ID:             taskID,
		Mode:           req.Mode,
		InitiatorID:    "u_admin",
		Target:         req.Target,
		InputText:      req.InputText,
		Plan:           types.Plan(draft),
		RiskLevel:      draft.RiskLevel,
		ApprovalStatus: approvalStatus,
		FinalStatus:    finalStatus,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := uc.TaskRepo.Create(ctx, model); err != nil {
		return types.TaskPlanResponse{}, err
	}

	if err := uc.AuditRepo.Create(ctx, audit.AuditEvent{
		ID:        uc.IDGen.New(),
		TaskID:    taskID,
		ActorID:   "u_admin",
		EventType: "task_planned",
		Payload: map[string]any{
			"mode":   req.Mode,
			"target": req.Target,
		},
		CreatedAt: now,
	}); err != nil {
		return types.TaskPlanResponse{}, err
	}

	if draft.RequiresApproval {
		if err := uc.AuditRepo.Create(ctx, audit.AuditEvent{
			ID:        uc.IDGen.New(),
			TaskID:    taskID,
			ActorID:   "system",
			EventType: "task_approval_requested",
			Payload:   map[string]any{"risk_level": draft.RiskLevel},
			CreatedAt: now,
		}); err != nil {
			return types.TaskPlanResponse{}, err
		}
	}

	if !draft.RequiresApproval {
		if err := uc.dispatchTask(ctx, &model, "auto-approved low-risk task"); err != nil {
			return types.TaskPlanResponse{}, err
		}
		finalStatus = model.FinalStatus
	}

	return types.TaskPlanResponse{
		TaskID: taskID,
		Status: finalStatus,
		Plan:   types.Plan(draft),
	}, nil
}

type taskMutation struct {
	NodeRepo   node.Repository
	TaskRepo   task.Repository
	AuditRepo  audit.Repository
	IDGen      idgen.Generator
	Dispatcher TaskDispatcher
	status     string
	finalState string
	eventType  string
	message    string
}

func (uc taskMutation) Execute(ctx context.Context, taskID string) (types.TaskMutationResponse, error) {
	if taskID == "" {
		return types.TaskMutationResponse{}, errors.New("task id is required")
	}

	current, err := uc.TaskRepo.Get(ctx, taskID)
	if err != nil {
		return types.TaskMutationResponse{}, err
	}

	switch uc.status {
	case "approved":
		if current.ApprovalStatus != "pending" {
			return types.TaskMutationResponse{}, errors.New("task is not waiting for approval")
		}
	case "rejected":
		if current.ApprovalStatus != "pending" {
			return types.TaskMutationResponse{}, errors.New("task is not waiting for rejection")
		}
	}

	current.ApprovalStatus = uc.status
	current.FinalStatus = uc.finalState
	current.StatusReason = uc.message
	current.UpdatedAt = time.Now().UTC()

	if err := uc.TaskRepo.Update(ctx, *current); err != nil {
		return types.TaskMutationResponse{}, err
	}

	if err := uc.AuditRepo.Create(ctx, audit.AuditEvent{
		ID:        uc.IDGen.New(),
		TaskID:    current.ID,
		ActorID:   "u_admin",
		EventType: uc.eventType,
		Payload:   map[string]any{"final_status": current.FinalStatus},
		CreatedAt: current.UpdatedAt,
	}); err != nil {
		return types.TaskMutationResponse{}, err
	}

	if uc.status == "approved" {
		if err := uc.dispatchTask(ctx, current, uc.message); err != nil {
			return types.TaskMutationResponse{}, err
		}
	}

	if uc.status == "cancelled" {
		if err := uc.cancelTask(ctx, current, uc.message); err != nil {
			return types.TaskMutationResponse{}, err
		}
	}

	return types.TaskMutationResponse{
		TaskID:  current.ID,
		Status:  current.FinalStatus,
		Message: uc.message,
	}, nil
}

type ApproveTask struct{ taskMutation }
type RejectTask struct{ taskMutation }
type CancelTask struct{ taskMutation }

type ListNodes struct {
	NodeRepo node.Repository
}

func (uc ListNodes) Execute(ctx context.Context) (types.ListNodesResponse, error) {
	nodes, err := uc.NodeRepo.List(ctx)
	if err != nil {
		return types.ListNodesResponse{}, err
	}
	return types.ListNodesResponse{Nodes: nodes}, nil
}

type GetNode struct {
	NodeRepo node.Repository
}

func (uc GetNode) Execute(ctx context.Context, nodeID string) (*types.Node, error) {
	return uc.NodeRepo.Get(ctx, nodeID)
}

type GetTask struct {
	TaskRepo task.Repository
}

func (uc GetTask) Execute(ctx context.Context, taskID string) (types.TaskResponse, error) {
	model, err := uc.TaskRepo.Get(ctx, taskID)
	if err != nil {
		return types.TaskResponse{}, err
	}

	executions, err := uc.TaskRepo.ListExecutions(ctx, taskID)
	if err != nil {
		return types.TaskResponse{}, err
	}

	aggregate := buildTaskAggregate(*model, executions)
	summary := buildTaskSummary(*model, aggregate)
	model.Aggregate = aggregate
	model.Summary = summary

	return types.TaskResponse{
		Task:      *model,
		Aggregate: aggregate,
		Summary:   summary,
	}, nil
}

type ListTasks struct {
	TaskRepo task.Repository
}

func (uc ListTasks) Execute(ctx context.Context) (types.ListTasksResponse, error) {
	items, err := uc.TaskRepo.List(ctx)
	if err != nil {
		return types.ListTasksResponse{}, err
	}

	resp := types.ListTasksResponse{
		Tasks: make([]types.TaskResponseItem, 0, len(items)),
	}

	for _, item := range items {
		executions, err := uc.TaskRepo.ListExecutions(ctx, item.ID)
		if err != nil {
			return types.ListTasksResponse{}, err
		}

		aggregate := buildTaskAggregate(item, executions)
		summary := buildTaskSummary(item, aggregate)
		item.Aggregate = aggregate
		item.Summary = summary
		resp.Tasks = append(resp.Tasks, types.TaskResponseItem{
			Task:      item,
			Aggregate: aggregate,
			Summary:   summary,
		})
	}

	return resp, nil
}

type ListTaskExecutions struct {
	TaskRepo task.Repository
}

func (uc ListTaskExecutions) Execute(ctx context.Context, taskID string) (types.TaskExecutionsResponse, error) {
	executions, err := uc.TaskRepo.ListExecutions(ctx, taskID)
	if err != nil {
		return types.TaskExecutionsResponse{}, err
	}
	return types.TaskExecutionsResponse{Executions: executions}, nil
}

type ListAuditEvents struct {
	AuditRepo audit.Repository
}

func (uc ListAuditEvents) Execute(ctx context.Context, taskID string) (types.AuditEventsResponse, error) {
	events, err := uc.AuditRepo.ListByTaskID(ctx, taskID)
	if err != nil {
		return types.AuditEventsResponse{}, err
	}
	return types.AuditEventsResponse{Events: events}, nil
}

type TaskLogInput struct {
	TaskID      string
	ExecutionID string
	NodeID      string
	Stream      string
	Chunk       string
	Timestamp   time.Time
}

type RecordTaskLog struct {
	TaskRepo  task.Repository
	AuditRepo audit.Repository
	IDGen     idgen.Generator
}

func (uc RecordTaskLog) Execute(ctx context.Context, in TaskLogInput) (types.TaskResponse, error) {
	model, err := uc.TaskRepo.Get(ctx, in.TaskID)
	if err != nil {
		return types.TaskResponse{}, err
	}

	executions, err := uc.TaskRepo.ListExecutions(ctx, in.TaskID)
	if err != nil {
		return types.TaskResponse{}, err
	}

	execution := findExecution(executions, in.ExecutionID)
	wasNewExecution := execution == nil
	if execution == nil {
		execution = &task.TaskExecution{
			ID:      in.ExecutionID,
			TaskID:  in.TaskID,
			NodeID:  in.NodeID,
			Attempt: 1,
		}
	}

	if execution.StartedAt.IsZero() {
		execution.StartedAt = in.Timestamp.UTC()
	}
	execution.Status = "running"
	execution.StatusReason = fmt.Sprintf("streaming %s output", in.Stream)
	if in.Stream == "stderr" {
		execution.StderrTail = appendTail(execution.StderrTail, in.Chunk)
	} else {
		execution.StdoutTail = appendTail(execution.StdoutTail, in.Chunk)
	}

	if err := uc.TaskRepo.UpsertExecution(ctx, *execution); err != nil {
		return types.TaskResponse{}, err
	}

	model.FinalStatus = "running"
	model.UpdatedAt = in.Timestamp.UTC()
	model.StatusReason = "receiving live execution output"
	if err := uc.TaskRepo.Update(ctx, *model); err != nil {
		return types.TaskResponse{}, err
	}

	if wasNewExecution && uc.AuditRepo != nil {
		_ = uc.AuditRepo.Create(ctx, audit.AuditEvent{
			ID:        uc.IDGen.New(),
			TaskID:    in.TaskID,
			ActorID:   in.NodeID,
			EventType: "task_execution_started",
			Payload: map[string]any{
				"execution_id": in.ExecutionID,
				"node_id":      in.NodeID,
			},
			CreatedAt: in.Timestamp.UTC(),
		})
	}

	return hydrateTaskResponse(ctx, uc.TaskRepo, *model)
}

type TaskResultInput struct {
	TaskID      string
	ExecutionID string
	NodeID      string
	Status      string
	ExitCode    int
	StdoutTail  string
	StderrTail  string
	Timestamp   time.Time
}

type RecordTaskResult struct {
	TaskRepo  task.Repository
	AuditRepo audit.Repository
	IDGen     idgen.Generator
}

func (uc RecordTaskResult) Execute(ctx context.Context, in TaskResultInput) (types.TaskResponse, error) {
	model, err := uc.TaskRepo.Get(ctx, in.TaskID)
	if err != nil {
		return types.TaskResponse{}, err
	}

	executions, err := uc.TaskRepo.ListExecutions(ctx, in.TaskID)
	if err != nil {
		return types.TaskResponse{}, err
	}

	execution := findExecution(executions, in.ExecutionID)
	if execution == nil {
		execution = &task.TaskExecution{
			ID:      in.ExecutionID,
			TaskID:  in.TaskID,
			NodeID:  in.NodeID,
			Attempt: 1,
		}
	}

	if execution.StartedAt.IsZero() {
		execution.StartedAt = in.Timestamp.UTC()
	}
	execution.FinishedAt = in.Timestamp.UTC()
	execution.Status = in.Status
	execution.ExitCode = in.ExitCode
	if in.StdoutTail != "" {
		execution.StdoutTail = in.StdoutTail
	}
	if in.StderrTail != "" {
		execution.StderrTail = in.StderrTail
	}
	execution.StatusReason = finalExecutionSummary(*execution)

	if err := uc.TaskRepo.UpsertExecution(ctx, *execution); err != nil {
		return types.TaskResponse{}, err
	}

	refreshed, err := uc.TaskRepo.ListExecutions(ctx, in.TaskID)
	if err != nil {
		return types.TaskResponse{}, err
	}

	aggregate := buildTaskAggregate(*model, refreshed)
	model.FinalStatus = finalTaskStatus(*model, aggregate)
	model.UpdatedAt = in.Timestamp.UTC()
	model.StatusReason = buildTaskSummary(*model, aggregate)
	if err := uc.TaskRepo.Update(ctx, *model); err != nil {
		return types.TaskResponse{}, err
	}

	if uc.AuditRepo != nil {
		eventType := "task_execution_finished"
		if in.Status == "timeout" {
			eventType = "task_execution_timeout"
		}
		if in.Status == "cancelled" {
			eventType = "task_execution_cancelled"
		}
		_ = uc.AuditRepo.Create(ctx, audit.AuditEvent{
			ID:        uc.IDGen.New(),
			TaskID:    in.TaskID,
			ActorID:   in.NodeID,
			EventType: eventType,
			Payload: map[string]any{
				"execution_id": in.ExecutionID,
				"node_id":      in.NodeID,
				"status":       in.Status,
				"exit_code":    in.ExitCode,
			},
			CreatedAt: in.Timestamp.UTC(),
		})
	}

	return hydrateTaskResponse(ctx, uc.TaskRepo, *model)
}

func NewServices(planner plan.Planner, schemaValidator plan.SchemaValidator, policyValidator policy.Validator, nodeRepo node.Repository, sessionStore node.SessionStore, taskRepo task.Repository, auditRepo audit.Repository, idGenerator idgen.Generator, dispatcher TaskDispatcher) Services {
	return Services{
		RegisterNode:      RegisterNode{NodeRepo: nodeRepo, IDGen: idGenerator},
		AuthenticateAgent: AuthenticateAgent{NodeRepo: nodeRepo},
		HeartbeatNode:     HeartbeatNode{NodeRepo: nodeRepo, SessionStore: sessionStore},
		GenerateTaskPlan: GenerateTaskPlan{
			Planner:         planner,
			SchemaValidator: schemaValidator,
			PolicyValidator: policyValidator,
			NodeRepo:        nodeRepo,
			TaskRepo:        taskRepo,
			AuditRepo:       auditRepo,
			IDGen:           idGenerator,
			Dispatcher:      dispatcher,
		},
		ApproveTask: ApproveTask{taskMutation{
			NodeRepo:   nodeRepo,
			TaskRepo:   taskRepo,
			AuditRepo:  auditRepo,
			IDGen:      idGenerator,
			Dispatcher: dispatcher,
			status:     "approved",
			finalState: "approved",
			eventType:  "task_approved",
			message:    "task approved",
		}},
		RejectTask: RejectTask{taskMutation{
			NodeRepo:   nodeRepo,
			TaskRepo:   taskRepo,
			AuditRepo:  auditRepo,
			IDGen:      idGenerator,
			Dispatcher: dispatcher,
			status:     "rejected",
			finalState: "cancelled",
			eventType:  "task_rejected",
			message:    "task rejected",
		}},
		CancelTask: CancelTask{taskMutation{
			NodeRepo:   nodeRepo,
			TaskRepo:   taskRepo,
			AuditRepo:  auditRepo,
			IDGen:      idGenerator,
			Dispatcher: dispatcher,
			status:     "cancelled",
			finalState: "cancelled",
			eventType:  "task_cancelled",
			message:    "task cancelled",
		}},
		ListNodes:          ListNodes{NodeRepo: nodeRepo},
		GetNode:            GetNode{NodeRepo: nodeRepo},
		ListTasks:          ListTasks{TaskRepo: taskRepo},
		GetTask:            GetTask{TaskRepo: taskRepo},
		ListTaskExecutions: ListTaskExecutions{TaskRepo: taskRepo},
		ListAuditEvents:    ListAuditEvents{AuditRepo: auditRepo},
		RecordTaskLog:      RecordTaskLog{TaskRepo: taskRepo, AuditRepo: auditRepo, IDGen: idGenerator},
		RecordTaskResult:   RecordTaskResult{TaskRepo: taskRepo, AuditRepo: auditRepo, IDGen: idGenerator},
	}
}

func hydrateTaskResponse(ctx context.Context, repo task.Repository, model task.Task) (types.TaskResponse, error) {
	executions, err := repo.ListExecutions(ctx, model.ID)
	if err != nil {
		return types.TaskResponse{}, err
	}

	aggregate := buildTaskAggregate(model, executions)
	summary := buildTaskSummary(model, aggregate)
	model.Aggregate = aggregate
	model.Summary = summary

	return types.TaskResponse{
		Task:      model,
		Aggregate: aggregate,
		Summary:   summary,
	}, nil
}

func buildTaskAggregate(model task.Task, executions []task.TaskExecution) types.TaskAggregate {
	runningStatuses := map[string]struct{}{
		"approved":   {},
		"queued":     {},
		"dispatched": {},
		"running":    {},
	}
	failedStatuses := map[string]struct{}{
		"failed":         {},
		"partial_failed": {},
		"timeout":        {},
		"cancelled":      {},
	}

	aggregate := types.TaskAggregate{
		Total: targetCount(model),
	}
	for _, execution := range executions {
		if isOfflineSkippedExecution(execution) {
			aggregate.OfflineSkipped++
			continue
		}

		switch execution.Status {
		case "success":
			aggregate.Success++
		default:
			if _, ok := runningStatuses[execution.Status]; ok {
				aggregate.Running++
			}
			if _, ok := failedStatuses[execution.Status]; ok {
				aggregate.Failed++
			}
		}
	}

	if aggregate.Total < aggregate.Success+aggregate.Failed+aggregate.Running+aggregate.OfflineSkipped {
		aggregate.Total = aggregate.Success + aggregate.Failed + aggregate.Running + aggregate.OfflineSkipped
	}
	if model.FinalStatus == "success" || model.FinalStatus == "failed" || model.FinalStatus == "partial_failed" || model.FinalStatus == "timeout" || model.FinalStatus == "cancelled" {
		known := aggregate.Success + aggregate.Failed + aggregate.Running + aggregate.OfflineSkipped
		if aggregate.Total > known {
			aggregate.OfflineSkipped = aggregate.Total - known
		}
	}

	return aggregate
}

func buildTaskSummary(model task.Task, aggregate types.TaskAggregate) string {
	if model.FinalStatus == "waiting_approval" {
		return "Task is waiting for approval."
	}
	if model.FinalStatus == "approved" && aggregate.Running == 0 && aggregate.Success == 0 && aggregate.Failed == 0 {
		return "Task approved and waiting for execution."
	}
	if model.FinalStatus == "cancelled" && model.StatusReason != "" {
		return model.StatusReason
	}
	if aggregate.Running > 0 {
		return fmt.Sprintf("Task is running: %d/%d running, %d succeeded, %d failed.", aggregate.Running, aggregate.Total, aggregate.Success, aggregate.Failed)
	}
	if aggregate.Success > 0 || aggregate.Failed > 0 || aggregate.OfflineSkipped > 0 {
		return fmt.Sprintf("Task finished: %d/%d succeeded, %d failed, %d offline skipped.", aggregate.Success, aggregate.Total, aggregate.Failed, aggregate.OfflineSkipped)
	}
	if model.StatusReason != "" {
		return model.StatusReason
	}
	return model.Plan.Summary
}

func finalTaskStatus(model task.Task, aggregate types.TaskAggregate) string {
	if model.FinalStatus == "cancelled" || model.FinalStatus == "timeout" {
		return model.FinalStatus
	}
	if aggregate.Running > 0 {
		return "running"
	}
	if aggregate.Success+aggregate.Failed+aggregate.OfflineSkipped < aggregate.Total {
		return "running"
	}
	if aggregate.Failed > 0 && aggregate.Success > 0 {
		return "partial_failed"
	}
	if aggregate.Failed > 0 {
		return "failed"
	}
	if aggregate.Success > 0 {
		return "success"
	}
	return model.FinalStatus
}

func targetCount(model task.Task) int {
	if len(model.Plan.TargetNodes) > len(model.Target) {
		return len(model.Plan.TargetNodes)
	}
	return len(model.Target)
}

func findExecution(executions []task.TaskExecution, executionID string) *task.TaskExecution {
	for idx := range executions {
		if executions[idx].ID == executionID {
			item := executions[idx]
			return &item
		}
	}
	return nil
}

func appendTail(existing, chunk string) string {
	const maxTailLength = 4096
	combined := strings.TrimSpace(strings.TrimSpace(existing) + "\n" + strings.TrimSpace(chunk))
	if len(combined) <= maxTailLength {
		return combined
	}
	return combined[len(combined)-maxTailLength:]
}

func finalExecutionSummary(execution task.TaskExecution) string {
	if execution.StatusReason != "" {
		return execution.StatusReason
	}
	return fmt.Sprintf("%s · exit=%d", execution.Status, execution.ExitCode)
}

func (uc GenerateTaskPlan) dispatchTask(ctx context.Context, model *task.Task, reason string) error {
	return dispatchTask(ctx, uc.NodeRepo, uc.TaskRepo, uc.AuditRepo, uc.IDGen, uc.Dispatcher, model, reason)
}

func (uc taskMutation) dispatchTask(ctx context.Context, model *task.Task, reason string) error {
	return dispatchTask(ctx, uc.NodeRepo, uc.TaskRepo, uc.AuditRepo, uc.IDGen, uc.Dispatcher, model, reason)
}

func (uc taskMutation) cancelTask(ctx context.Context, model *task.Task, reason string) error {
	return cancelTask(ctx, uc.TaskRepo, uc.AuditRepo, uc.IDGen, uc.Dispatcher, model, reason)
}

func dispatchTask(ctx context.Context, nodeRepo node.Repository, taskRepo task.Repository, auditRepo audit.Repository, idGen idgen.Generator, dispatcher TaskDispatcher, model *task.Task, reason string) error {
	if dispatcher == nil {
		return errors.New("task dispatcher is not configured")
	}

	targets, err := resolveTargetNodes(ctx, nodeRepo, *model)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	model.Target = make([]string, 0, len(targets))
	model.Plan.TargetNodes = make([]string, 0, len(targets))

	for _, target := range targets {
		model.Target = append(model.Target, target.ID)
		model.Plan.TargetNodes = append(model.Plan.TargetNodes, target.ID)

		execution := task.TaskExecution{
			ID:      idGen.New(),
			TaskID:  model.ID,
			NodeID:  target.ID,
			Attempt: 1,
		}

		if node.NormalizeStatus(target, now) != "online" {
			execution.Status = "cancelled"
			execution.ExitCode = 0
			execution.FinishedAt = now
			execution.StatusReason = "offline skipped"
			if err := taskRepo.UpsertExecution(ctx, execution); err != nil {
				return err
			}
			recordAudit(ctx, auditRepo, idGen, model.ID, "system", "task_execution_skipped", map[string]any{
				"execution_id": execution.ID,
				"node_id":      target.ID,
				"reason":       execution.StatusReason,
			}, now)
			continue
		}

		execution.Status = "queued"
		execution.StatusReason = "queued on control plane"
		if err := taskRepo.UpsertExecution(ctx, execution); err != nil {
			return err
		}
		recordAudit(ctx, auditRepo, idGen, model.ID, "system", "task_execution_queued", map[string]any{
			"execution_id": execution.ID,
			"node_id":      target.ID,
			"reason":       reason,
		}, now)

		if err := dispatcher.DispatchTask(ctx, target.ID, model.ID, execution.ID, model.Plan.Steps, maxStepTimeout(model.Plan.Steps)); err != nil {
			execution.Status = "failed"
			execution.FinishedAt = now
			execution.ExitCode = 1
			execution.StatusReason = fmt.Sprintf("dispatch failed: %v", err)
			if err := taskRepo.UpsertExecution(ctx, execution); err != nil {
				return err
			}
			recordAudit(ctx, auditRepo, idGen, model.ID, "system", "task_execution_dispatch_failed", map[string]any{
				"execution_id": execution.ID,
				"node_id":      target.ID,
				"reason":       execution.StatusReason,
			}, now)
			continue
		}

		execution.Status = "dispatched"
		execution.StatusReason = "dispatched to agent"
		if err := taskRepo.UpsertExecution(ctx, execution); err != nil {
			return err
		}
		recordAudit(ctx, auditRepo, idGen, model.ID, "system", "task_execution_dispatched", map[string]any{
			"execution_id": execution.ID,
			"node_id":      target.ID,
		}, now)
	}

	return syncTaskState(ctx, taskRepo, model, now)
}

func cancelTask(ctx context.Context, taskRepo task.Repository, auditRepo audit.Repository, idGen idgen.Generator, dispatcher TaskDispatcher, model *task.Task, reason string) error {
	executions, err := taskRepo.ListExecutions(ctx, model.ID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, execution := range executions {
		if execution.Status == "success" || execution.Status == "failed" || execution.Status == "timeout" || execution.Status == "cancelled" {
			continue
		}

		if dispatcher != nil {
			_ = dispatcher.CancelTask(ctx, execution.NodeID, model.ID, execution.ID, reason)
		}

		execution.Status = "cancelled"
		execution.FinishedAt = now
		execution.StatusReason = reason
		if err := taskRepo.UpsertExecution(ctx, execution); err != nil {
			return err
		}
		recordAudit(ctx, auditRepo, idGen, model.ID, "system", "task_execution_cancel_requested", map[string]any{
			"execution_id": execution.ID,
			"node_id":      execution.NodeID,
			"reason":       reason,
		}, now)
	}

	model.FinalStatus = "cancelled"
	model.StatusReason = reason
	model.UpdatedAt = now
	return taskRepo.Update(ctx, *model)
}

func resolveTargetNodes(ctx context.Context, repo node.Repository, model task.Task) ([]types.Node, error) {
	if repo == nil {
		return nil, errors.New("node repository is not configured")
	}

	if len(model.Target) == 1 && model.Target[0] == "all" {
		items, err := repo.List(ctx)
		if err != nil {
			return nil, err
		}

		out := make([]types.Node, 0, len(items))
		for _, item := range items {
			out = append(out, item)
		}
		return out, nil
	}

	out := make([]types.Node, 0, len(model.Target))
	for _, nodeID := range model.Target {
		item, err := repo.Get(ctx, nodeID)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, nil
}

func syncTaskState(ctx context.Context, repo task.Repository, model *task.Task, now time.Time) error {
	executions, err := repo.ListExecutions(ctx, model.ID)
	if err != nil {
		return err
	}

	aggregate := buildTaskAggregate(*model, executions)
	switch {
	case aggregate.Running > 0:
		model.FinalStatus = "dispatched"
		model.StatusReason = "task dispatched to target nodes"
	case aggregate.Failed > 0 && aggregate.Success == 0 && aggregate.OfflineSkipped == aggregate.Total:
		model.FinalStatus = "cancelled"
		model.StatusReason = "all target nodes were skipped because they are offline"
	default:
		model.FinalStatus = finalTaskStatus(*model, aggregate)
		model.StatusReason = buildTaskSummary(*model, aggregate)
	}
	model.UpdatedAt = now
	return repo.Update(ctx, *model)
}

func maxStepTimeout(steps []types.PlanStep) int {
	maxTimeout := 10
	for _, step := range steps {
		if step.TimeoutSec > maxTimeout {
			maxTimeout = step.TimeoutSec
		}
	}
	return maxTimeout
}

func recordAudit(ctx context.Context, repo audit.Repository, idGen idgen.Generator, taskID, actorID, eventType string, payload map[string]any, at time.Time) {
	if repo == nil {
		return
	}
	_ = repo.Create(ctx, audit.AuditEvent{
		ID:        idGen.New(),
		TaskID:    taskID,
		ActorID:   actorID,
		EventType: eventType,
		Payload:   payload,
		CreatedAt: at,
	})
}

func isOfflineSkippedExecution(execution task.TaskExecution) bool {
	return strings.EqualFold(execution.StatusReason, "offline skipped")
}
