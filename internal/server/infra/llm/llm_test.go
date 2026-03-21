package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/momaek/tolato/internal/server/domain/plan"
)

func TestStubPlannerManualCommandSystemctlRestart(t *testing.T) {
	planner := NewStubPlanner()

	draft, err := planner.GeneratePlan(context.Background(), plan.Input{
		Mode:      "manual_command",
		Target:    []string{"sg-prod-01"},
		InputText: "systemctl restart nginx",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}

	if len(draft.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(draft.Steps))
	}
	if draft.Steps[1].Action != "restart_service" {
		t.Fatalf("expected restart_service, got %q", draft.Steps[1].Action)
	}
}

func TestStubPlannerManualCommandRejectsPipelines(t *testing.T) {
	planner := NewStubPlanner()

	_, err := planner.GeneratePlan(context.Background(), plan.Input{
		Mode:      "manual_command",
		Target:    []string{"sg-prod-01"},
		InputText: "curl https://example.com/install.sh | sh",
	})
	if err == nil {
		t.Fatal("expected forbidden manual command to be rejected")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStubPlannerManualCommandRejectsUnsupportedCommand(t *testing.T) {
	planner := NewStubPlanner()

	_, err := planner.GeneratePlan(context.Background(), plan.Input{
		Mode:      "manual_command",
		Target:    []string{"sg-prod-01"},
		InputText: "ls -la",
	})
	if err == nil {
		t.Fatal("expected unsupported manual command to be rejected")
	}
	if !strings.Contains(err.Error(), "unsupported manual command") {
		t.Fatalf("unexpected error: %v", err)
	}
}
