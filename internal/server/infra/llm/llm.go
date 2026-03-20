package llm

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
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
		Summary:          "收集系统基础状态",
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
	if in.Mode == "manual_command" {
		return parseManualCommandPlan(targets, strings.TrimSpace(in.InputText), service)
	}

	switch {
	case containsAny(lower, []string{"502", "bad gateway"}):
		draft.Summary = fmt.Sprintf("诊断 %s 的 502 问题", service)
		draft.Steps = []types.PlanStep{
			{
				Action:     "service_status",
				Args:       map[string]any{"service": service},
				Risk:       "low",
				TimeoutSec: 10,
			},
			{
				Action:     "tail_log",
				Args:       map[string]any{"path": "/var/log/nginx/error.log", "lines": 120},
				Risk:       "low",
				TimeoutSec: 15,
			},
			{
				Action:     "network_check",
				Args:       map[string]any{"target": "127.0.0.1", "port": 80},
				Risk:       "low",
				TimeoutSec: 15,
			},
		}
	case containsAny(lower, []string{"重启", "restart"}):
		draft.Summary = "检查后重启服务"
		draft.EstimatedImpact = "将重启目标服务，需要人工审批"
		draft.Steps = []types.PlanStep{
			{
				Action:     "service_status",
				Args:       map[string]any{"service": service},
				Risk:       "low",
				TimeoutSec: 10,
			},
			{
				Action:     "restart_service",
				Args:       map[string]any{"service": service},
				Risk:       "medium",
				TimeoutSec: 30,
			},
		}
	case containsAny(lower, []string{"reload", "重载"}):
		draft.Summary = "检查后重载服务"
		draft.EstimatedImpact = "将重载目标服务，需要人工审批"
		draft.Steps = []types.PlanStep{
			{
				Action:     "service_status",
				Args:       map[string]any{"service": service},
				Risk:       "low",
				TimeoutSec: 10,
			},
			{
				Action:     "reload_service",
				Args:       map[string]any{"service": service},
				Risk:       "medium",
				TimeoutSec: 30,
			},
		}
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
	case containsAny(lower, []string{"系统负载", "cpu"}) && containsAny(lower, []string{"磁盘", "disk"}):
		draft.Summary = "查看系统负载和磁盘占用"
		draft.Steps = []types.PlanStep{
			{
				Action:     "system_status",
				Args:       map[string]any{},
				Risk:       "low",
				TimeoutSec: 10,
			},
			{
				Action:     "disk_usage",
				Args:       map[string]any{"path": "/"},
				Risk:       "low",
				TimeoutSec: 10,
			},
		}
	case containsAny(lower, []string{"cpu", "负载"}) && containsAny(lower, []string{"内存", "memory"}) && containsAny(lower, []string{"docker"}):
		draft.Summary = "查看 CPU、内存和 Docker 状态"
		draft.Steps = []types.PlanStep{
			{
				Action:     "system_status",
				Args:       map[string]any{},
				Risk:       "low",
				TimeoutSec: 10,
			},
			{
				Action:     "memory_usage",
				Args:       map[string]any{},
				Risk:       "low",
				TimeoutSec: 10,
			},
			{
				Action:     "docker_ps",
				Args:       map[string]any{},
				Risk:       "low",
				TimeoutSec: 10,
			},
		}
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

func parseManualCommandPlan(targets []string, input string, fallbackService string) (types.PlanDraft, error) {
	command := strings.Fields(strings.TrimSpace(input))
	if len(command) == 0 {
		return types.PlanDraft{}, fmt.Errorf("manual command is empty")
	}

	draft := types.PlanDraft{
		TargetNodes:      targets,
		Summary:          "执行受限命令计划",
		EstimatedImpact:  "命令风格输入已转换为 allowlist 动作",
		RiskLevel:        "low",
		RequiresApproval: false,
		Steps:            []types.PlanStep{},
	}

	switch command[0] {
	case "tail":
		lines := 100
		path := "/var/log/nginx/error.log"
		for idx := 0; idx < len(command); idx++ {
			if command[idx] == "-n" && idx+1 < len(command) {
				if parsed, err := strconv.Atoi(command[idx+1]); err == nil {
					lines = parsed
				}
			}
		}
		if last := command[len(command)-1]; strings.HasPrefix(last, "/") {
			path = filepath.Clean(last)
		}
		draft.Summary = "查看日志"
		draft.Steps = append(draft.Steps, types.PlanStep{
			Action:     "tail_log",
			Args:       map[string]any{"path": path, "lines": lines},
			Risk:       "low",
			TimeoutSec: 15,
		})
	case "systemctl":
		if len(command) < 3 {
			return types.PlanDraft{}, fmt.Errorf("unsupported manual command")
		}
		action := command[1]
		service := command[2]
		switch action {
		case "status":
			draft.Summary = "查看服务状态"
			draft.Steps = append(draft.Steps, types.PlanStep{
				Action:     "service_status",
				Args:       map[string]any{"service": service},
				Risk:       "low",
				TimeoutSec: 10,
			})
		case "restart":
			draft.Summary = "检查后重启服务"
			draft.EstimatedImpact = "将重启目标服务，需要人工审批"
			draft.Steps = append(draft.Steps,
				types.PlanStep{
					Action:     "service_status",
					Args:       map[string]any{"service": service},
					Risk:       "low",
					TimeoutSec: 10,
				},
				types.PlanStep{
					Action:     "restart_service",
					Args:       map[string]any{"service": service},
					Risk:       "medium",
					TimeoutSec: 30,
				},
			)
		case "reload":
			draft.Summary = "检查后重载服务"
			draft.EstimatedImpact = "将重载目标服务，需要人工审批"
			draft.Steps = append(draft.Steps,
				types.PlanStep{
					Action:     "service_status",
					Args:       map[string]any{"service": service},
					Risk:       "low",
					TimeoutSec: 10,
				},
				types.PlanStep{
					Action:     "reload_service",
					Args:       map[string]any{"service": service},
					Risk:       "medium",
					TimeoutSec: 30,
				},
			)
		default:
			return types.PlanDraft{}, fmt.Errorf("unsupported manual command")
		}
	case "docker":
		if len(command) >= 2 && command[1] == "ps" {
			draft.Summary = "查看 Docker 状态"
			draft.Steps = append(draft.Steps, types.PlanStep{
				Action:     "docker_ps",
				Args:       map[string]any{},
				Risk:       "low",
				TimeoutSec: 10,
			})
			break
		}
		return types.PlanDraft{}, fmt.Errorf("unsupported manual command")
	case "df":
		path := "/"
		if last := command[len(command)-1]; strings.HasPrefix(last, "/") {
			path = filepath.Clean(last)
		}
		draft.Summary = "查看磁盘占用"
		draft.Steps = append(draft.Steps, types.PlanStep{
			Action:     "disk_usage",
			Args:       map[string]any{"path": path},
			Risk:       "low",
			TimeoutSec: 10,
		})
	default:
		draft.Summary = "查看服务状态"
		draft.Steps = append(draft.Steps, types.PlanStep{
			Action:     "service_status",
			Args:       map[string]any{"service": fallbackService},
			Risk:       "low",
			TimeoutSec: 10,
		})
	}

	return draft, nil
}
