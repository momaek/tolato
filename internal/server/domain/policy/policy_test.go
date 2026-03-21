package policy

import (
	"context"
	"strings"
	"testing"

	"github.com/momaek/tolato/internal/shared/types"
)

func TestStaticValidatorRejectsDisallowedService(t *testing.T) {
	validator := NewStaticValidator()
	draft := types.PlanDraft{
		TargetNodes: []string{"sg-prod-01"},
		Summary:     "检查服务状态",
		Steps: []types.PlanStep{{
			Action: "service_status",
			Args:   map[string]any{"service": "sshd"},
		}},
	}

	err := validator.ValidatePlan(context.Background(), &draft)
	if err == nil {
		t.Fatal("expected disallowed service to be rejected")
	}
	if !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStaticValidatorRejectsUnsafeLogPath(t *testing.T) {
	validator := NewStaticValidator()
	draft := types.PlanDraft{
		TargetNodes: []string{"sg-prod-01"},
		Summary:     "查看日志",
		Steps: []types.PlanStep{{
			Action: "tail_log",
			Args: map[string]any{
				"path":  "/etc/passwd",
				"lines": 100,
			},
		}},
	}

	err := validator.ValidatePlan(context.Background(), &draft)
	if err == nil {
		t.Fatal("expected unsafe log path to be rejected")
	}
	if !strings.Contains(err.Error(), "outside of the allowlist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStaticValidatorEnrichesRestartServicePlan(t *testing.T) {
	validator := NewStaticValidator()
	draft := types.PlanDraft{
		TargetNodes: []string{"sg-prod-01"},
		Summary:     "检查后重启服务",
		Steps: []types.PlanStep{{
			Action: "restart_service",
			Args:   map[string]any{"service": "nginx"},
		}},
	}

	if err := validator.ValidatePlan(context.Background(), &draft); err != nil {
		t.Fatalf("ValidatePlan returned error: %v", err)
	}

	if !draft.RequiresApproval {
		t.Fatal("expected restart_service plan to require approval")
	}
	if draft.RiskLevel != "medium" {
		t.Fatalf("expected medium risk, got %q", draft.RiskLevel)
	}
	if draft.Steps[0].TimeoutSec != 30 {
		t.Fatalf("expected timeout 30, got %d", draft.Steps[0].TimeoutSec)
	}
	if draft.Steps[0].BroadcastAllowed {
		t.Fatal("expected restart_service broadcast to remain disabled")
	}
}
