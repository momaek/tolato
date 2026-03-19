package policy

import (
	"context"
	"errors"

	"github.com/momaek/tolato/internal/shared/action"
	"github.com/momaek/tolato/internal/shared/types"
)

type Validator interface {
	ValidatePlan(ctx context.Context, draft *types.PlanDraft) error
}

type StaticValidator struct{}

func NewStaticValidator() StaticValidator {
	return StaticValidator{}
}

func (StaticValidator) ValidatePlan(ctx context.Context, draft *types.PlanDraft) error {
	_ = ctx

	requiresApproval := false
	overallRisk := "low"

	for idx, step := range draft.Steps {
		spec, ok := action.Get(step.Action)
		if !ok {
			return errors.New("action is not allowlisted")
		}

		if len(draft.TargetNodes) > 1 && !spec.BroadcastAllowed {
			return errors.New("broadcast write action is not allowed")
		}

		draft.Steps[idx].Risk = spec.RiskLevel
		draft.Steps[idx].TimeoutSec = spec.TimeoutSec
		draft.Steps[idx].BroadcastAllowed = spec.BroadcastAllowed

		if spec.ApprovalRequired {
			requiresApproval = true
		}

		if spec.RiskLevel == "medium" || spec.RiskLevel == "high" {
			overallRisk = spec.RiskLevel
		}
	}

	draft.RequiresApproval = requiresApproval
	draft.RiskLevel = overallRisk
	if draft.EstimatedImpact == "" {
		if requiresApproval {
			draft.EstimatedImpact = "该计划包含需要审批的写操作"
		} else {
			draft.EstimatedImpact = "只读诊断，不修改系统状态"
		}
	}

	return nil
}
