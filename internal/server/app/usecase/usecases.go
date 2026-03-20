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

type dependencies struct {
	Planner         plan.Planner
	SchemaValidator plan.SchemaValidator
	PolicyValidator policy.Validator
	NodeRepo        node.Repository
	SessionStore    node.SessionStore
	TaskRepo        task.Repository
	AuditRepo       audit.Repository
	IDGen           idgen.Generator
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
	TaskRepo        task.Repository
	AuditRepo       audit.Repository
	IDGen           idgen.Generator
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

	draft, err := uc.Planner.GeneratePlan(ctx, plan.Input{
		Mode:      req.Mode,
		Target:    req.Target,
		InputText: req.InputText,
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

	return types.TaskPlanResponse{
		TaskID: taskID,
		Status: finalStatus,
		Plan:   types.Plan(draft),
	}, nil
}

type taskMutation struct {
	TaskRepo   task.Repository
	AuditRepo  audit.Repository
	IDGen      idgen.Generator
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
	TaskRepo task.Repository
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
	TaskRepo task.Repository
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

	return hydrateTaskResponse(ctx, uc.TaskRepo, *model)
}

func NewServices(planner plan.Planner, schemaValidator plan.SchemaValidator, policyValidator policy.Validator, nodeRepo node.Repository, sessionStore node.SessionStore, taskRepo task.Repository, auditRepo audit.Repository, idGenerator idgen.Generator) Services {
	return Services{
		RegisterNode:      RegisterNode{NodeRepo: nodeRepo, IDGen: idGenerator},
		AuthenticateAgent: AuthenticateAgent{NodeRepo: nodeRepo},
		HeartbeatNode:     HeartbeatNode{NodeRepo: nodeRepo, SessionStore: sessionStore},
		GenerateTaskPlan: GenerateTaskPlan{
			Planner:         planner,
			SchemaValidator: schemaValidator,
			PolicyValidator: policyValidator,
			TaskRepo:        taskRepo,
			AuditRepo:       auditRepo,
			IDGen:           idGenerator,
		},
		ApproveTask: ApproveTask{taskMutation{
			TaskRepo:   taskRepo,
			AuditRepo:  auditRepo,
			IDGen:      idGenerator,
			status:     "approved",
			finalState: "approved",
			eventType:  "task_approved",
			message:    "task approved",
		}},
		RejectTask: RejectTask{taskMutation{
			TaskRepo:   taskRepo,
			AuditRepo:  auditRepo,
			IDGen:      idGenerator,
			status:     "rejected",
			finalState: "cancelled",
			eventType:  "task_rejected",
			message:    "task rejected",
		}},
		CancelTask: CancelTask{taskMutation{
			TaskRepo:   taskRepo,
			AuditRepo:  auditRepo,
			IDGen:      idGenerator,
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
		RecordTaskLog:      RecordTaskLog{TaskRepo: taskRepo},
		RecordTaskResult:   RecordTaskResult{TaskRepo: taskRepo},
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

	if aggregate.Total < aggregate.Success+aggregate.Failed+aggregate.Running {
		aggregate.Total = aggregate.Success + aggregate.Failed + aggregate.Running
	}
	if model.FinalStatus == "success" || model.FinalStatus == "failed" || model.FinalStatus == "partial_failed" || model.FinalStatus == "timeout" || model.FinalStatus == "cancelled" {
		known := aggregate.Success + aggregate.Failed + aggregate.Running
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
	if aggregate.Success+aggregate.Failed < aggregate.Total {
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
