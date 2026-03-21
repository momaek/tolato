package policy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"

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
	requiredApprovalRole := ""

	for idx, step := range draft.Steps {
		spec, ok := action.Get(step.Action)
		if !ok {
			return errors.New("action is not allowlisted")
		}
		if err := validateStepArgs(step); err != nil {
			return err
		}

		if len(draft.TargetNodes) > 1 && !spec.BroadcastAllowed {
			return errors.New("broadcast write action is not allowed")
		}

		draft.Steps[idx].Risk = spec.RiskLevel
		draft.Steps[idx].TimeoutSec = spec.TimeoutSec
		draft.Steps[idx].BroadcastAllowed = spec.BroadcastAllowed

		if spec.ApprovalRequired {
			requiresApproval = true
			if requiredApprovalRole == "" {
				requiredApprovalRole = "operator"
			}
		}

		if spec.RiskLevel == "medium" || spec.RiskLevel == "high" {
			overallRisk = spec.RiskLevel
		}
	}

	if len(draft.TargetNodes) > 1 {
		for _, step := range draft.Steps {
			spec, _ := action.Get(step.Action)
			if spec.ApprovalRequired {
				overallRisk = "high"
				requiresApproval = true
				requiredApprovalRole = "admin"
				break
			}
		}
	}

	if overallRisk == "high" && requiredApprovalRole == "" {
		requiredApprovalRole = "admin"
	}

	draft.RequiresApproval = requiresApproval
	draft.RiskLevel = overallRisk
	draft.RequiredApprovalRole = requiredApprovalRole
	if draft.EstimatedImpact == "" {
		if requiresApproval {
			draft.EstimatedImpact = "该计划包含需要审批的写操作"
		} else {
			draft.EstimatedImpact = "只读诊断，不修改系统状态"
		}
	}

	return nil
}

func validateStepArgs(step types.PlanStep) error {
	switch step.Action {
	case "disk_usage":
		path, err := requiredStringArg(step.Args, "path")
		if err != nil {
			return err
		}
		if !isAbsolutePath(path) {
			return errors.New("disk_usage path must be an absolute path")
		}
	case "service_status", "restart_service", "reload_service":
		service, err := requiredStringArg(step.Args, "service")
		if err != nil {
			return err
		}
		if !action.IsAllowedService(service) {
			return fmt.Errorf("service %q is not allowlisted", service)
		}
	case "tail_log":
		path, err := requiredStringArg(step.Args, "path")
		if err != nil {
			return err
		}
		if !isAllowedLogPath(path) {
			return fmt.Errorf("log path %q is outside of the allowlist", path)
		}

		lines, err := requiredIntArg(step.Args, "lines")
		if err != nil {
			return err
		}
		if lines < 1 || lines > 500 {
			return errors.New("tail_log lines must be between 1 and 500")
		}
	case "network_check":
		target, err := requiredStringArg(step.Args, "target")
		if err != nil {
			return err
		}
		if !isAllowedNetworkTarget(target) {
			return fmt.Errorf("network target %q is invalid", target)
		}

		port, err := requiredIntArg(step.Args, "port")
		if err != nil {
			return err
		}
		if port < 1 || port > 65535 {
			return errors.New("network_check port must be between 1 and 65535")
		}
	}

	return nil
}

func requiredStringArg(args map[string]any, key string) (string, error) {
	raw, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}

	value, ok := raw.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}

	return strings.TrimSpace(value), nil
}

func requiredIntArg(args map[string]any, key string) (int, error) {
	raw, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}

	switch value := raw.(type) {
	case int:
		return value, nil
	case int32:
		return int(value), nil
	case int64:
		return int(value), nil
	case float64:
		return int(value), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
}

func isAbsolutePath(value string) bool {
	clean := filepath.Clean(strings.TrimSpace(value))
	return strings.HasPrefix(clean, "/")
}

func isAllowedLogPath(value string) bool {
	clean := filepath.Clean(strings.TrimSpace(value))
	return strings.HasPrefix(clean, action.LogPathPrefix)
}

func isAllowedNetworkTarget(value string) bool {
	target := strings.TrimSpace(value)
	if target == "" {
		return false
	}
	if net.ParseIP(target) != nil {
		return true
	}
	if strings.EqualFold(target, "localhost") {
		return true
	}
	for _, r := range target {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}
