package plan

import (
	"context"
	"errors"

	"github.com/momaek/tolato/internal/shared/types"
)

type Plan = types.Plan
type PlanDraft = types.PlanDraft
type PlanStep = types.PlanStep

type Input struct {
	Mode      string
	Target    []string
	InputText string
	Nodes     []types.Node
}

type RepairInput struct {
	Original types.PlanDraft
	Reason   string
}

type Planner interface {
	GeneratePlan(ctx context.Context, in Input) (PlanDraft, error)
	RepairPlan(ctx context.Context, in RepairInput) (PlanDraft, error)
}

type SchemaValidator interface {
	ValidatePlan(ctx context.Context, draft PlanDraft) error
}

type StaticSchemaValidator struct{}

func (StaticSchemaValidator) ValidatePlan(ctx context.Context, draft PlanDraft) error {
	_ = ctx
	if len(draft.TargetNodes) == 0 {
		return errors.New("plan must contain target nodes")
	}
	if len(draft.Steps) == 0 {
		return errors.New("plan must contain at least one step")
	}
	if draft.Summary == "" {
		return errors.New("plan summary is required")
	}
	return nil
}
