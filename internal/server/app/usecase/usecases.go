package usecase

import (
	"context"
	"errors"
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
	GetTask            GetTask
	ListTaskExecutions ListTaskExecutions
	ListAuditEvents    ListAuditEvents
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
	return types.TaskResponse{Task: *model}, nil
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
		GetTask:            GetTask{TaskRepo: taskRepo},
		ListTaskExecutions: ListTaskExecutions{TaskRepo: taskRepo},
		ListAuditEvents:    ListAuditEvents{AuditRepo: auditRepo},
	}
}
