package llm

import (
	"context"
	"strings"

	"github.com/momaek/tolato/internal/server/domain/plan"
	"github.com/momaek/tolato/internal/shared/types"
)

type StubPlanner struct{}

func NewStubPlanner() StubPlanner {
	return StubPlanner{}
}

func (StubPlanner) GeneratePlan(ctx context.Context, in plan.Input) (types.PlanDraft, error) {
	_ = ctx

	targets := in.Target
	if len(targets) == 0 {
		targets = []string{"all_nodes"}
	}

	lower := strings.ToLower(in.InputText)

	draft := types.PlanDraft{
		TargetNodes:      targets,
		Summary:          "占位计划：根据输入生成受控动作",
		EstimatedImpact:  "只读诊断，不修改系统状态",
		RiskLevel:        "low",
		RequiresApproval: false,
		Steps: []types.PlanStep{
			{
				Action:     "system_status",
				Args:       map[string]any{},
				Risk:       "low",
				TimeoutSec: 10,
			},
		},
	}

	service := guessServiceName(lower)

	switch {
	case containsAny(lower, []string{"重启", "restart"}):
		draft.Summary = "重启服务"
		draft.EstimatedImpact = "将重启目标服务，需要人工审批"
		draft.Steps = []types.PlanStep{{
			Action:     "restart_service",
			Args:       map[string]any{"service": service},
			Risk:       "medium",
			TimeoutSec: 30,
		}}
	case containsAny(lower, []string{"reload", "重载"}):
		draft.Summary = "重载服务"
		draft.EstimatedImpact = "将重载目标服务，需要人工审批"
		draft.Steps = []types.PlanStep{{
			Action:     "reload_service",
			Args:       map[string]any{"service": service},
			Risk:       "medium",
			TimeoutSec: 30,
		}}
	case containsAny(lower, []string{"日志", "log", "tail"}):
		path := "/var/log/nginx/error.log"
		if containsAny(lower, []string{"docker"}) {
			path = "/var/log/docker.log"
		}
		draft.Summary = "查看日志"
		draft.Steps = []types.PlanStep{{
			Action:     "tail_log",
			Args:       map[string]any{"path": path, "lines": 100},
			Risk:       "low",
			TimeoutSec: 15,
		}}
	case containsAny(lower, []string{"docker"}):
		draft.Summary = "查看 Docker 状态"
		draft.Steps = []types.PlanStep{{
			Action:     "docker_ps",
			Args:       map[string]any{},
			Risk:       "low",
			TimeoutSec: 10,
		}}
	case containsAny(lower, []string{"磁盘", "disk"}):
		draft.Summary = "查看磁盘占用"
		draft.Steps = []types.PlanStep{{
			Action:     "disk_usage",
			Args:       map[string]any{"path": "/"},
			Risk:       "low",
			TimeoutSec: 10,
		}}
	case containsAny(lower, []string{"内存", "memory"}):
		draft.Summary = "查看内存占用"
		draft.Steps = []types.PlanStep{{
			Action:     "memory_usage",
			Args:       map[string]any{},
			Risk:       "low",
			TimeoutSec: 10,
		}}
	case containsAny(lower, []string{"nginx", "service", "状态", "status"}):
		draft.Summary = "查看服务状态"
		draft.Steps = []types.PlanStep{{
			Action:     "service_status",
			Args:       map[string]any{"service": service},
			Risk:       "low",
			TimeoutSec: 10,
		}}
	case containsAny(lower, []string{"网络", "network"}):
		draft.Summary = "检查网络连通性"
		draft.Steps = []types.PlanStep{{
			Action:     "network_check",
			Args:       map[string]any{"target": "127.0.0.1", "port": 80},
			Risk:       "low",
			TimeoutSec: 15,
		}}
	}

	return draft, nil
}

func (StubPlanner) RepairPlan(ctx context.Context, in plan.RepairInput) (types.PlanDraft, error) {
	_ = ctx
	return in.Original, nil
}

func guessServiceName(input string) string {
	switch {
	case containsAny(input, []string{"nginx"}):
		return "nginx"
	case containsAny(input, []string{"docker"}):
		return "docker"
	default:
		return "nginx"
	}
}

func containsAny(s string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(s, keyword) {
			return true
		}
	}
	return false
}
