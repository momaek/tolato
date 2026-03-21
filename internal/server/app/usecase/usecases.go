package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/server/domain/audit"
	"github.com/momaek/tolato/internal/server/domain/node"
	"github.com/momaek/tolato/internal/server/domain/outbox"
	"github.com/momaek/tolato/internal/server/domain/plan"
	"github.com/momaek/tolato/internal/server/domain/policy"
	domainsummary "github.com/momaek/tolato/internal/server/domain/summary"
	"github.com/momaek/tolato/internal/server/domain/task"
	"github.com/momaek/tolato/internal/server/infra/idgen"
	"github.com/momaek/tolato/internal/shared/types"
)

type Services struct {
	RegisterNode       RegisterNode
	AuthenticateAgent  AuthenticateAgent
	HeartbeatNode      HeartbeatNode
	DisconnectNode     DisconnectNode
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
	TimeoutTasks       TimeoutTasks
}

type TaskDispatcher interface {
	DispatchTask(ctx context.Context, nodeID, taskID, executionID string, steps []types.PlanStep, timeoutSec int) error
	CancelTask(ctx context.Context, nodeID, taskID, executionID, reason string) error
}

type dependencies struct {
	Planner         plan.Planner
	SchemaValidator plan.SchemaValidator
	PolicyValidator policy.Validator
	SummaryService  domainsummary.Service
	NodeRepo        node.Repository
	SessionStore    node.SessionStore
	TaskRepo        task.Repository
	AuditRepo       audit.Repository
	OutboxRepo      outbox.Repository
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
		ConnectedAt:     connectedAt(uc.SessionStore, ctx, in.SessionID, now),
		LastHeartbeatAt: now,
		RemoteAddr:      in.RemoteAddr,
		Capabilities:    in.Capabilities,
		Status:          "active",
	})
}

type DisconnectNodeInput struct {
	NodeID    string
	SessionID string
	Reason    string
}

type DisconnectNode struct {
	NodeRepo     node.Repository
	SessionStore node.SessionStore
	TaskRepo     task.Repository
	AuditRepo    audit.Repository
	OutboxRepo   outbox.Repository
	IDGen        idgen.Generator
}

func (uc DisconnectNode) Execute(ctx context.Context, in DisconnectNodeInput) error {
	if in.NodeID == "" || in.SessionID == "" {
		return nil
	}

	now := time.Now().UTC()
	if uc.SessionStore != nil {
		_ = uc.SessionStore.MarkDisconnected(ctx, in.SessionID, now)
	}
	if uc.NodeRepo != nil {
		_ = uc.NodeRepo.UpdatePresence(ctx, in.NodeID, "", "stale", now)
	}

	if uc.TaskRepo == nil {
		return nil
	}

	tasks, err := uc.TaskRepo.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range tasks {
		executions, err := uc.TaskRepo.ListExecutions(ctx, item.ID)
		if err != nil {
			return err
		}
		for _, execution := range executions {
			if execution.NodeID != in.NodeID {
				continue
			}
			switch execution.Status {
			case "queued", "dispatched":
				execution.Status = "queued"
				execution.StatusReason = "agent disconnected before execution"
				if err := uc.TaskRepo.UpsertExecution(ctx, execution); err != nil {
					return err
				}
				enqueueOutbox(ctx, uc.OutboxRepo, uc.IDGen, types.OutboxMessage{
					ID:          uc.IDGen.New(),
					Topic:       "task.dispatch",
					TaskID:      item.ID,
					ExecutionID: execution.ID,
					NodeID:      execution.NodeID,
					Payload: map[string]any{
						"steps":       item.Plan.Steps,
						"timeout_sec": maxStepTimeout(item.Plan.Steps),
					},
					CreatedAt: now,
				})
			case "running":
				recordAudit(ctx, uc.AuditRepo, uc.IDGen, item.ID, "system", "task_execution_agent_disconnected", map[string]any{
					"execution_id": execution.ID,
					"node_id":      execution.NodeID,
					"reason":       in.Reason,
				}, now)
			}
		}
	}
	return nil
}

type GenerateTaskPlan struct {
	Planner         plan.Planner
	SchemaValidator plan.SchemaValidator
	PolicyValidator policy.Validator
	SummaryService  domainsummary.Service
	NodeRepo        node.Repository
	TaskRepo        task.Repository
	AuditRepo       audit.Repository
	OutboxRepo      outbox.Repository
	IDGen           idgen.Generator
}

func (uc GenerateTaskPlan) Execute(ctx context.Context, user types.CurrentUser, req types.TaskPlanRequest) (types.TaskPlanResponse, error) {
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
		if req.Mode == "manual_command" {
			return uc.createRejectedManualCommand(ctx, user, req, err)
		}
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
			if req.Mode == "manual_command" {
				return uc.createRejectedManualCommand(ctx, user, req, err)
			}
			return types.TaskPlanResponse{}, err
		}
		if err := uc.PolicyValidator.ValidatePlan(ctx, &draft); err != nil {
			if req.Mode == "manual_command" {
				return uc.createRejectedManualCommand(ctx, user, req, err)
			}
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
		ID:                   taskID,
		Mode:                 req.Mode,
		InitiatorID:          user.ID,
		InitiatorRole:        user.Role,
		Target:               req.Target,
		InputText:            req.InputText,
		Plan:                 types.Plan(draft),
		RiskLevel:            draft.RiskLevel,
		ApprovalStatus:       approvalStatus,
		RequiredApprovalRole: draft.RequiredApprovalRole,
		FinalStatus:          finalStatus,
		SummarySource:        "planner",
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := uc.TaskRepo.Create(ctx, model); err != nil {
		return types.TaskPlanResponse{}, err
	}

	if err := uc.AuditRepo.Create(ctx, audit.AuditEvent{
		ID:        uc.IDGen.New(),
		TaskID:    taskID,
		ActorID:   user.ID,
		EventType: "task_planned",
		Payload: map[string]any{
			"mode":            req.Mode,
			"target":          req.Target,
			"target_nodes":    draft.TargetNodes,
			"input_text":      req.InputText,
			"approval_status": approvalStatus,
			"plan_json":       draft,
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
			Payload:   map[string]any{"risk_level": draft.RiskLevel, "required_approval_role": draft.RequiredApprovalRole},
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

func (uc GenerateTaskPlan) createRejectedManualCommand(ctx context.Context, user types.CurrentUser, req types.TaskPlanRequest, reason error) (types.TaskPlanResponse, error) {
	now := time.Now().UTC()
	taskID := uc.IDGen.New()
	model := task.Task{
		ID:             taskID,
		Mode:           req.Mode,
		InitiatorID:    user.ID,
		InitiatorRole:  user.Role,
		Target:         req.Target,
		InputText:      req.InputText,
		Plan:           types.Plan{TargetNodes: req.Target, Summary: "manual command rejected", EstimatedImpact: "策略拒绝该命令", RiskLevel: "forbidden"},
		RiskLevel:      "forbidden",
		ApprovalStatus: "rejected",
		FinalStatus:    "cancelled",
		StatusReason:   reason.Error(),
		ResultSummary:  reason.Error(),
		Summary:        reason.Error(),
		SummarySource:  "policy",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := uc.TaskRepo.Create(ctx, model); err != nil {
		return types.TaskPlanResponse{}, err
	}
	recordAudit(ctx, uc.AuditRepo, uc.IDGen, taskID, user.ID, "manual_command_rejected", map[string]any{
		"input_text":      req.InputText,
		"target_nodes":    req.Target,
		"approval_status": model.ApprovalStatus,
		"result_summary":  reason.Error(),
		"reason":          reason.Error(),
	}, now)
	return types.TaskPlanResponse{
		TaskID: taskID,
		Status: model.FinalStatus,
		Plan:   model.Plan,
	}, nil
}

type taskMutation struct {
	NodeRepo       node.Repository
	TaskRepo       task.Repository
	AuditRepo      audit.Repository
	OutboxRepo     outbox.Repository
	SummaryService domainsummary.Service
	IDGen          idgen.Generator
	status         string
	finalState     string
	eventType      string
	message        string
}

func (uc taskMutation) Execute(ctx context.Context, user types.CurrentUser, taskID string) (types.TaskMutationResponse, error) {
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
		if current.RequiredApprovalRole == "admin" && user.Role != "admin" {
			return types.TaskMutationResponse{}, errors.New("task requires admin approval")
		}
	case "rejected":
		if current.ApprovalStatus != "pending" {
			return types.TaskMutationResponse{}, errors.New("task is not waiting for rejection")
		}
	}

	current.ApprovalStatus = uc.status
	current.FinalStatus = uc.finalState
	current.StatusReason = uc.message
	current.ApproverID = user.ID
	current.UpdatedAt = time.Now().UTC()

	if err := uc.TaskRepo.Update(ctx, *current); err != nil {
		return types.TaskMutationResponse{}, err
	}

	if err := uc.AuditRepo.Create(ctx, audit.AuditEvent{
		ID:        uc.IDGen.New(),
		TaskID:    current.ID,
		ActorID:   user.ID,
		EventType: uc.eventType,
		Payload:   map[string]any{"final_status": current.FinalStatus, "approval_status": current.ApprovalStatus, "approver_id": current.ApproverID},
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
	TaskRepo       task.Repository
	AuditRepo      audit.Repository
	SummaryService domainsummary.Service
	IDGen          idgen.Generator
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
	model.FinalStatus = finalTaskStatus(*model, refreshed, aggregate)
	model.UpdatedAt = in.Timestamp.UTC()
	applyTaskSummary(ctx, uc.SummaryService, model, refreshed, aggregate)
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

func NewServices(planner plan.Planner, schemaValidator plan.SchemaValidator, policyValidator policy.Validator, summaryService domainsummary.Service, nodeRepo node.Repository, sessionStore node.SessionStore, taskRepo task.Repository, auditRepo audit.Repository, outboxRepo outbox.Repository, idGenerator idgen.Generator) Services {
	return Services{
		RegisterNode:      RegisterNode{NodeRepo: nodeRepo, IDGen: idGenerator},
		AuthenticateAgent: AuthenticateAgent{NodeRepo: nodeRepo},
		HeartbeatNode:     HeartbeatNode{NodeRepo: nodeRepo, SessionStore: sessionStore},
		DisconnectNode:    DisconnectNode{NodeRepo: nodeRepo, SessionStore: sessionStore, TaskRepo: taskRepo, AuditRepo: auditRepo, OutboxRepo: outboxRepo, IDGen: idGenerator},
		GenerateTaskPlan: GenerateTaskPlan{
			Planner:         planner,
			SchemaValidator: schemaValidator,
			PolicyValidator: policyValidator,
			SummaryService:  summaryService,
			NodeRepo:        nodeRepo,
			TaskRepo:        taskRepo,
			AuditRepo:       auditRepo,
			OutboxRepo:      outboxRepo,
			IDGen:           idGenerator,
		},
		ApproveTask: ApproveTask{taskMutation{
			NodeRepo:       nodeRepo,
			TaskRepo:       taskRepo,
			AuditRepo:      auditRepo,
			OutboxRepo:     outboxRepo,
			SummaryService: summaryService,
			IDGen:          idGenerator,
			status:         "approved",
			finalState:     "approved",
			eventType:      "task_approved",
			message:        "task approved",
		}},
		RejectTask: RejectTask{taskMutation{
			NodeRepo:       nodeRepo,
			TaskRepo:       taskRepo,
			AuditRepo:      auditRepo,
			OutboxRepo:     outboxRepo,
			SummaryService: summaryService,
			IDGen:          idGenerator,
			status:         "rejected",
			finalState:     "cancelled",
			eventType:      "task_rejected",
			message:        "task rejected",
		}},
		CancelTask: CancelTask{taskMutation{
			NodeRepo:       nodeRepo,
			TaskRepo:       taskRepo,
			AuditRepo:      auditRepo,
			OutboxRepo:     outboxRepo,
			SummaryService: summaryService,
			IDGen:          idGenerator,
			status:         "cancelled",
			finalState:     "cancelled",
			eventType:      "task_cancelled",
			message:        "task cancelled",
		}},
		ListNodes:          ListNodes{NodeRepo: nodeRepo},
		GetNode:            GetNode{NodeRepo: nodeRepo},
		ListTasks:          ListTasks{TaskRepo: taskRepo},
		GetTask:            GetTask{TaskRepo: taskRepo},
		ListTaskExecutions: ListTaskExecutions{TaskRepo: taskRepo},
		ListAuditEvents:    ListAuditEvents{AuditRepo: auditRepo},
		RecordTaskLog:      RecordTaskLog{TaskRepo: taskRepo, AuditRepo: auditRepo, IDGen: idGenerator},
		RecordTaskResult:   RecordTaskResult{TaskRepo: taskRepo, AuditRepo: auditRepo, SummaryService: summaryService, IDGen: idGenerator},
		TimeoutTasks:       TimeoutTasks{TaskRepo: taskRepo, AuditRepo: auditRepo, IDGen: idGenerator},
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
	if model.ResultSummary == "" {
		model.ResultSummary = summary
	}

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
	if model.ResultSummary != "" && aggregate.Running == 0 && model.FinalStatus != "waiting_approval" {
		return model.ResultSummary
	}
	if model.FinalStatus == "waiting_approval" {
		return "Task is waiting for approval."
	}
	if (model.FinalStatus == "approved" || model.FinalStatus == "queued") && aggregate.Running == 0 && aggregate.Success == 0 && aggregate.Failed == 0 {
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

func finalTaskStatus(model task.Task, executions []task.TaskExecution, aggregate types.TaskAggregate) string {
	if model.FinalStatus == "cancelled" || model.FinalStatus == "timeout" {
		return model.FinalStatus
	}
	if aggregate.Running > 0 {
		return "running"
	}
	if aggregate.Success+aggregate.Failed+aggregate.OfflineSkipped < aggregate.Total {
		return "running"
	}

	timeoutCount := 0
	hardFailureCount := 0
	for _, execution := range executions {
		switch execution.Status {
		case "timeout":
			timeoutCount++
		case "failed", "partial_failed":
			hardFailureCount++
		}
	}

	if timeoutCount > 0 && aggregate.Success == 0 && hardFailureCount == 0 {
		return "timeout"
	}
	if aggregate.Failed > 0 && (aggregate.Success > 0 || timeoutCount > 0) {
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
	return dispatchTask(ctx, uc.NodeRepo, uc.TaskRepo, uc.AuditRepo, uc.OutboxRepo, uc.IDGen, model, reason)
}

func (uc taskMutation) dispatchTask(ctx context.Context, model *task.Task, reason string) error {
	return dispatchTask(ctx, uc.NodeRepo, uc.TaskRepo, uc.AuditRepo, uc.OutboxRepo, uc.IDGen, model, reason)
}

func (uc taskMutation) cancelTask(ctx context.Context, model *task.Task, reason string) error {
	return cancelTask(ctx, uc.TaskRepo, uc.AuditRepo, uc.OutboxRepo, uc.IDGen, model, reason)
}

func dispatchTask(ctx context.Context, nodeRepo node.Repository, taskRepo task.Repository, auditRepo audit.Repository, outboxRepo outbox.Repository, idGen idgen.Generator, model *task.Task, reason string) error {
	if outboxRepo == nil {
		return errors.New("outbox repository is not configured")
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
		enqueueOutbox(ctx, outboxRepo, idGen, types.OutboxMessage{
			ID:          idGen.New(),
			Topic:       "task.dispatch",
			TaskID:      model.ID,
			ExecutionID: execution.ID,
			NodeID:      target.ID,
			Payload: map[string]any{
				"steps":       model.Plan.Steps,
				"timeout_sec": maxStepTimeout(model.Plan.Steps),
			},
			CreatedAt: now,
		})
	}

	return syncTaskState(ctx, taskRepo, model, now)
}

func cancelTask(ctx context.Context, taskRepo task.Repository, auditRepo audit.Repository, outboxRepo outbox.Repository, idGen idgen.Generator, model *task.Task, reason string) error {
	executions, err := taskRepo.ListExecutions(ctx, model.ID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, execution := range executions {
		if execution.Status == "success" || execution.Status == "failed" || execution.Status == "timeout" || execution.Status == "cancelled" {
			continue
		}

		execution.Status = "cancelled"
		execution.FinishedAt = now
		execution.StatusReason = reason
		if err := taskRepo.UpsertExecution(ctx, execution); err != nil {
			return err
		}
		enqueueOutbox(ctx, outboxRepo, idGen, types.OutboxMessage{
			ID:          idGen.New(),
			Topic:       "task.cancel",
			TaskID:      model.ID,
			ExecutionID: execution.ID,
			NodeID:      execution.NodeID,
			Payload:     map[string]any{"reason": reason},
			CreatedAt:   now,
		})
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
	case hasQueuedExecution(executions):
		model.FinalStatus = "queued"
		model.StatusReason = "task queued for dispatch workers"
	case aggregate.Running > 0:
		model.FinalStatus = "dispatched"
		model.StatusReason = "task dispatched to target nodes"
	case aggregate.Failed > 0 && aggregate.Success == 0 && aggregate.OfflineSkipped == aggregate.Total:
		model.FinalStatus = "cancelled"
		model.StatusReason = "all target nodes were skipped because they are offline"
	default:
		model.FinalStatus = finalTaskStatus(*model, executions, aggregate)
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

func connectedAt(store node.SessionStore, ctx context.Context, sessionID string, fallback time.Time) time.Time {
	if store == nil || sessionID == "" {
		return fallback
	}
	item, err := store.Get(ctx, sessionID)
	if err != nil || item == nil || item.ConnectedAt.IsZero() {
		return fallback
	}
	return item.ConnectedAt.UTC()
}

func enqueueOutbox(ctx context.Context, repo outbox.Repository, idGen idgen.Generator, message types.OutboxMessage) {
	if repo == nil {
		return
	}
	if message.ID == "" {
		message.ID = idGen.New()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	message.Attempts = 0
	_ = repo.Create(ctx, message)
}

func applyTaskSummary(ctx context.Context, service domainsummary.Service, model *task.Task, executions []task.TaskExecution, aggregate types.TaskAggregate) {
	if service == nil {
		model.StatusReason = buildTaskSummary(*model, aggregate)
		model.ResultSummary = model.StatusReason
		model.Summary = model.StatusReason
		model.SummarySource = "rule_fallback"
		model.FailureNodeIDs = failureNodeIDs(executions)
		return
	}

	result, err := service.SummarizeTask(ctx, *model, executions, aggregate)
	if err != nil || strings.TrimSpace(result.Summary) == "" {
		model.StatusReason = buildTaskSummary(*model, aggregate)
		model.ResultSummary = model.StatusReason
		model.Summary = model.StatusReason
		model.SummarySource = "rule_fallback"
		model.FailureNodeIDs = failureNodeIDs(executions)
		return
	}

	model.StatusReason = result.Summary
	model.ResultSummary = result.ResultSummary
	if model.ResultSummary == "" {
		model.ResultSummary = result.Summary
	}
	model.Summary = result.Summary
	model.SummarySource = result.Source
	model.FailureNodeIDs = result.FailureNodeIDs
}

func failureNodeIDs(executions []task.TaskExecution) []string {
	items := make([]string, 0)
	for _, execution := range executions {
		switch execution.Status {
		case "failed", "partial_failed", "timeout", "cancelled":
			items = append(items, execution.NodeID)
		}
	}
	return items
}

func hasQueuedExecution(executions []task.TaskExecution) bool {
	for _, execution := range executions {
		if execution.Status == "queued" {
			return true
		}
	}
	return false
}
