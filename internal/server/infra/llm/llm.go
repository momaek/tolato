package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/server/domain/plan"
	"github.com/momaek/tolato/internal/shared/action"
	"github.com/momaek/tolato/internal/shared/types"
)

type Service struct {
	config   types.LLMConfig
	client   *http.Client
	fallback StubPlanner
}

type StubPlanner struct{}

func NewPlanner(config types.LLMConfig) Service {
	return Service{
		config:   config,
		client:   &http.Client{Timeout: 20 * time.Second},
		fallback: StubPlanner{},
	}
}

func NewStubPlanner() StubPlanner {
	return StubPlanner{}
}

func (s Service) GeneratePlan(ctx context.Context, in plan.Input) (types.PlanDraft, error) {
	if in.Mode == "manual_command" {
		targets := in.Target
		if len(targets) == 0 {
			targets = []string{"all_nodes"}
		}
		return parseManualCommandPlan(targets, strings.TrimSpace(in.InputText), guessServiceName(strings.ToLower(in.InputText)))
	}
	if !s.isConfigured() {
		return s.fallback.GeneratePlan(ctx, in)
	}

	targets := in.Target
	if len(targets) == 0 {
		targets = []string{"all_nodes"}
	}

	userPrompt := buildPlannerPrompt(in, targets)
	content, err := s.completeJSON(ctx, "You convert operator intent into a safe JSON task plan.", userPrompt)
	if err != nil {
		return s.fallback.GeneratePlan(ctx, in)
	}

	var draft types.PlanDraft
	if err := json.Unmarshal([]byte(content), &draft); err != nil {
		return s.fallback.GeneratePlan(ctx, in)
	}
	if len(draft.TargetNodes) == 0 {
		draft.TargetNodes = targets
	}
	return draft, nil
}

func (s Service) RepairPlan(ctx context.Context, in plan.RepairInput) (types.PlanDraft, error) {
	if !s.isConfigured() {
		return s.fallback.RepairPlan(ctx, in)
	}

	payload, _ := json.Marshal(in.Original)
	content, err := s.completeJSON(ctx, "You repair JSON task plans without changing intent.", fmt.Sprintf(`Return only fixed JSON.
Validation error: %s
Original plan:
%s`, in.Reason, string(payload)))
	if err != nil {
		return s.fallback.RepairPlan(ctx, in)
	}

	var repaired types.PlanDraft
	if err := json.Unmarshal([]byte(content), &repaired); err != nil {
		return s.fallback.RepairPlan(ctx, in)
	}
	return repaired, nil
}

func (s Service) isConfigured() bool {
	return strings.TrimSpace(s.config.Model) != "" && strings.TrimSpace(s.config.APIKey) != ""
}

func (s Service) completeJSON(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(s.config.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	body := map[string]any{
		"model": s.config.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.2,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("llm request failed: %s", resp.Status)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

func buildPlannerPrompt(in plan.Input, targets []string) string {
	nodeIDs := make([]string, 0, len(in.Nodes))
	for _, item := range in.Nodes {
		nodeIDs = append(nodeIDs, item.ID)
	}
	return fmt.Sprintf(`Return JSON matching this Go shape:
{
  "target_nodes": ["node_id"],
  "summary": "string",
  "estimated_impact": "string",
  "risk_level": "low|medium|high|forbidden",
  "requires_approval": true,
  "required_approval_role": "operator|admin",
  "steps": [{"action":"string","args":{},"risk":"low|medium|high|forbidden","timeout_sec":10,"broadcast_allowed":false}],
  "metadata": {"planner":"llm"}
}

Allowed actions: system_status,disk_usage,memory_usage,docker_ps,service_status,tail_log,restart_service,reload_service,network_check
Allowed services: %s
User input: %s
Mode: %s
Requested targets: %s
Resolved nodes: %s
Use only allowlisted actions.`, strings.Join(action.AllowedServices(), ","), in.InputText, in.Mode, strings.Join(targets, ","), strings.Join(nodeIDs, ","))
}

func (StubPlanner) GeneratePlan(ctx context.Context, in plan.Input) (types.PlanDraft, error) {
	_ = ctx

	targets := in.Target
	if len(targets) == 0 {
		targets = []string{"all_nodes"}
	}

	lower := strings.ToLower(in.InputText)

	draft := types.PlanDraft{
		TargetNodes:          targets,
		Summary:              "收集系统基础状态",
		EstimatedImpact:      "只读诊断，不修改系统状态",
		RiskLevel:            "low",
		RequiresApproval:     false,
		RequiredApprovalRole: "",
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
			{Action: "service_status", Args: map[string]any{"service": service}, Risk: "low", TimeoutSec: 10},
			{Action: "tail_log", Args: map[string]any{"path": "/var/log/nginx/error.log", "lines": 120}, Risk: "low", TimeoutSec: 15},
			{Action: "network_check", Args: map[string]any{"target": "127.0.0.1", "port": 80}, Risk: "low", TimeoutSec: 15},
		}
	case containsAny(lower, []string{"重启", "restart"}):
		draft.Summary = "检查后重启服务"
		draft.EstimatedImpact = "将重启目标服务，需要人工审批"
		draft.RequiresApproval = true
		draft.RequiredApprovalRole = "operator"
		draft.Steps = []types.PlanStep{
			{Action: "service_status", Args: map[string]any{"service": service}, Risk: "low", TimeoutSec: 10},
			{Action: "restart_service", Args: map[string]any{"service": service}, Risk: "medium", TimeoutSec: 30},
		}
	case containsAny(lower, []string{"reload", "重载"}):
		draft.Summary = "检查后重载服务"
		draft.EstimatedImpact = "将重载目标服务，需要人工审批"
		draft.RequiresApproval = true
		draft.RequiredApprovalRole = "operator"
		draft.Steps = []types.PlanStep{
			{Action: "service_status", Args: map[string]any{"service": service}, Risk: "low", TimeoutSec: 10},
			{Action: "reload_service", Args: map[string]any{"service": service}, Risk: "medium", TimeoutSec: 30},
		}
	case containsAny(lower, []string{"日志", "log", "tail"}):
		path := "/var/log/nginx/error.log"
		if containsAny(lower, []string{"docker"}) {
			path = "/var/log/docker.log"
		}
		draft.Summary = "查看日志"
		draft.Steps = []types.PlanStep{{Action: "tail_log", Args: map[string]any{"path": path, "lines": 100}, Risk: "low", TimeoutSec: 15}}
	case containsAny(lower, []string{"docker"}):
		draft.Summary = "查看 Docker 状态"
		draft.Steps = []types.PlanStep{{Action: "docker_ps", Args: map[string]any{}, Risk: "low", TimeoutSec: 10}}
	case containsAny(lower, []string{"系统负载", "cpu"}) && containsAny(lower, []string{"磁盘", "disk"}):
		draft.Summary = "查看系统负载和磁盘占用"
		draft.Steps = []types.PlanStep{
			{Action: "system_status", Args: map[string]any{}, Risk: "low", TimeoutSec: 10},
			{Action: "disk_usage", Args: map[string]any{"path": "/"}, Risk: "low", TimeoutSec: 10},
		}
	case containsAny(lower, []string{"cpu", "负载"}) && containsAny(lower, []string{"内存", "memory"}) && containsAny(lower, []string{"docker"}):
		draft.Summary = "查看 CPU、内存和 Docker 状态"
		draft.Steps = []types.PlanStep{
			{Action: "system_status", Args: map[string]any{}, Risk: "low", TimeoutSec: 10},
			{Action: "memory_usage", Args: map[string]any{}, Risk: "low", TimeoutSec: 10},
			{Action: "docker_ps", Args: map[string]any{}, Risk: "low", TimeoutSec: 10},
		}
	case containsAny(lower, []string{"磁盘", "disk"}):
		draft.Summary = "查看磁盘占用"
		draft.Steps = []types.PlanStep{{Action: "disk_usage", Args: map[string]any{"path": "/"}, Risk: "low", TimeoutSec: 10}}
	case containsAny(lower, []string{"内存", "memory"}):
		draft.Summary = "查看内存占用"
		draft.Steps = []types.PlanStep{{Action: "memory_usage", Args: map[string]any{}, Risk: "low", TimeoutSec: 10}}
	case containsAny(lower, []string{"nginx", "service", "状态", "status"}):
		draft.Summary = "查看服务状态"
		draft.Steps = []types.PlanStep{{Action: "service_status", Args: map[string]any{"service": service}, Risk: "low", TimeoutSec: 10}}
	case containsAny(lower, []string{"网络", "network"}):
		draft.Summary = "检查网络连通性"
		draft.Steps = []types.PlanStep{{Action: "network_check", Args: map[string]any{"target": "127.0.0.1", "port": 80}, Risk: "low", TimeoutSec: 15}}
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
	normalized := strings.TrimSpace(input)
	if containsForbiddenManualSyntax(normalized) {
		return types.PlanDraft{}, fmt.Errorf("manual command is forbidden by policy")
	}

	command := strings.Fields(normalized)
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
		if err := validateTailCommand(command); err != nil {
			return types.PlanDraft{}, err
		}
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
		if len(command) != 3 {
			return types.PlanDraft{}, fmt.Errorf("unsupported manual command")
		}
		operation := command[1]
		service := command[2]
		if !action.IsAllowedService(service) {
			return types.PlanDraft{}, fmt.Errorf("service %q is not allowlisted", service)
		}
		switch operation {
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
			draft.RequiresApproval = true
			draft.RequiredApprovalRole = "operator"
			draft.Steps = append(draft.Steps,
				types.PlanStep{Action: "service_status", Args: map[string]any{"service": service}, Risk: "low", TimeoutSec: 10},
				types.PlanStep{Action: "restart_service", Args: map[string]any{"service": service}, Risk: "medium", TimeoutSec: 30},
			)
		case "reload":
			draft.Summary = "检查后重载服务"
			draft.EstimatedImpact = "将重载目标服务，需要人工审批"
			draft.RequiresApproval = true
			draft.RequiredApprovalRole = "operator"
			draft.Steps = append(draft.Steps,
				types.PlanStep{Action: "service_status", Args: map[string]any{"service": service}, Risk: "low", TimeoutSec: 10},
				types.PlanStep{Action: "reload_service", Args: map[string]any{"service": service}, Risk: "medium", TimeoutSec: 30},
			)
		default:
			return types.PlanDraft{}, fmt.Errorf("unsupported manual command")
		}
	case "docker":
		if len(command) == 2 && command[1] == "ps" {
			draft.Summary = "查看 Docker 状态"
			draft.Steps = append(draft.Steps, types.PlanStep{Action: "docker_ps", Args: map[string]any{}, Risk: "low", TimeoutSec: 10})
			break
		}
		return types.PlanDraft{}, fmt.Errorf("unsupported manual command")
	case "df":
		if err := validateDFCommand(command); err != nil {
			return types.PlanDraft{}, err
		}
		path := "/"
		if last := command[len(command)-1]; strings.HasPrefix(last, "/") {
			path = filepath.Clean(last)
		}
		draft.Summary = "查看磁盘占用"
		draft.Steps = append(draft.Steps, types.PlanStep{Action: "disk_usage", Args: map[string]any{"path": path}, Risk: "low", TimeoutSec: 10})
	case "free":
		if len(command) != 2 || command[1] != "-m" {
			return types.PlanDraft{}, fmt.Errorf("unsupported manual command")
		}
		draft.Summary = "查看内存占用"
		draft.Steps = append(draft.Steps, types.PlanStep{Action: "memory_usage", Args: map[string]any{}, Risk: "low", TimeoutSec: 10})
	case "uptime", "uname":
		if err := validateSystemStatusCommand(command); err != nil {
			return types.PlanDraft{}, err
		}
		draft.Summary = "查看系统状态"
		draft.Steps = append(draft.Steps, types.PlanStep{Action: "system_status", Args: map[string]any{}, Risk: "low", TimeoutSec: 10})
	default:
		if fallbackService != "" && action.IsAllowedService(fallbackService) && len(command) == 1 {
			draft.Summary = "查看服务状态"
			draft.Steps = append(draft.Steps, types.PlanStep{Action: "service_status", Args: map[string]any{"service": fallbackService}, Risk: "low", TimeoutSec: 10})
			break
		}
		return types.PlanDraft{}, fmt.Errorf("unsupported manual command")
	}

	return draft, nil
}

func containsForbiddenManualSyntax(input string) bool {
	lower := strings.ToLower(strings.TrimSpace(input))
	forbiddenFragments := []string{"&&", "||", ";", "|", ">", "<", "`", "$(", "sudo ", " rm ", "mkfs", "dd ", "chmod ", "chown ", "curl ", "wget ", "bash ", "sh "}
	for _, fragment := range forbiddenFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func validateTailCommand(command []string) error {
	if len(command) == 1 {
		return nil
	}
	if len(command) == 2 && strings.HasPrefix(command[1], "/") {
		return nil
	}
	if len(command) == 4 && command[1] == "-n" {
		return nil
	}
	return fmt.Errorf("unsupported manual command")
}

func validateDFCommand(command []string) error {
	if len(command) == 1 {
		return nil
	}
	if len(command) == 2 && (command[1] == "-h" || strings.HasPrefix(command[1], "/")) {
		return nil
	}
	if len(command) == 3 && command[1] == "-h" && strings.HasPrefix(command[2], "/") {
		return nil
	}
	return fmt.Errorf("unsupported manual command")
}

func validateSystemStatusCommand(command []string) error {
	if len(command) == 1 && command[0] == "uptime" {
		return nil
	}
	if len(command) == 2 && command[0] == "uname" && command[1] == "-a" {
		return nil
	}
	return fmt.Errorf("unsupported manual command")
}
