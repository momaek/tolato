package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/agent/infra/oscmd"
	"github.com/momaek/tolato/internal/shared/types"
)

type ActionRunner struct{}

func NewActionRunner() ActionRunner {
	return ActionRunner{}
}

func (ActionRunner) Run(ctx context.Context, job Job) (Result, error) {
	start := time.Now()

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	for idx, step := range job.Steps {
		if err := ctx.Err(); err != nil {
			return cancelledResult(start, stdout.String(), stderr.String(), err), err
		}

		stepStdout, stepStderr, err := runStep(ctx, step)
		if stepStdout != "" {
			stdout.WriteString(formatStepOutput(idx, step.Action, stepStdout))
		}
		if stepStderr != "" {
			stderr.WriteString(formatStepOutput(idx, step.Action, stepStderr))
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return Result{
					Status:     "timeout",
					ExitCode:   124,
					StdoutTail: truncateTail(stdout.String()),
					StderrTail: truncateTail(joinNonEmpty(stderr.String(), err.Error())),
					Duration:   time.Since(start),
				}, err
			}
			if errors.Is(err, context.Canceled) {
				return cancelledResult(start, stdout.String(), stderr.String(), err), err
			}
			return Result{
				Status:     "failed",
				ExitCode:   exitCode(err),
				StdoutTail: truncateTail(stdout.String()),
				StderrTail: truncateTail(joinNonEmpty(stderr.String(), err.Error())),
				Duration:   time.Since(start),
			}, err
		}
	}

	return Result{
		Status:     "success",
		ExitCode:   0,
		StdoutTail: truncateTail(stdout.String()),
		StderrTail: truncateTail(stderr.String()),
		Duration:   time.Since(start),
	}, nil
}

func runStep(ctx context.Context, step types.PlanStep) (string, string, error) {
	switch step.Action {
	case "system_status":
		return runCommands(ctx,
			commandSpec{name: "uname", args: []string{"-a"}},
			commandSpec{name: "uptime"},
			commandSpec{name: "df", args: []string{"-h", "/"}},
		)
	case "disk_usage":
		path := sanitizePath(argString(step.Args, "path", "/"))
		return runCommands(ctx, commandSpec{name: "df", args: []string{"-h", path}})
	case "memory_usage":
		return runCommands(ctx, commandSpec{name: "free", args: []string{"-m"}})
	case "docker_ps":
		return runCommands(ctx, commandSpec{name: "docker", args: []string{"ps", "--format", "table {{.Names}}\t{{.Status}}\t{{.Image}}"}})
	case "service_status":
		service := sanitizeService(argString(step.Args, "service", "nginx"))
		return runCommands(ctx, commandSpec{name: "systemctl", args: []string{"status", service, "--no-pager", "--lines", "20"}})
	case "tail_log":
		path, err := sanitizeLogPath(argString(step.Args, "path", "/var/log/syslog"))
		if err != nil {
			return "", "", err
		}
		lines := clamp(argInt(step.Args, "lines", 100), 1, 500)
		return runCommands(ctx, commandSpec{name: "tail", args: []string{"-n", strconv.Itoa(lines), path}})
	case "restart_service":
		service := sanitizeService(argString(step.Args, "service", "nginx"))
		return runCommands(ctx, commandSpec{name: "systemctl", args: []string{"restart", service}})
	case "reload_service":
		service := sanitizeService(argString(step.Args, "service", "nginx"))
		return runCommands(ctx, commandSpec{name: "systemctl", args: []string{"reload", service}})
	case "network_check":
		target := sanitizeTarget(argString(step.Args, "target", "127.0.0.1"))
		port := clamp(argInt(step.Args, "port", 80), 1, 65535)
		return dialTarget(ctx, target, port)
	default:
		return "", "", fmt.Errorf("unsupported action %q", step.Action)
	}
}

type commandSpec struct {
	name string
	args []string
}

func runCommands(ctx context.Context, specs ...commandSpec) (string, string, error) {
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	for _, spec := range specs {
		cmd := oscmd.CommandContext(ctx, spec.name, spec.args...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return stdout.String(), stderr.String(), err
		}
		if stdout.Len() > 0 && !strings.HasSuffix(stdout.String(), "\n") {
			stdout.WriteByte('\n')
		}
		if stderr.Len() > 0 && !strings.HasSuffix(stderr.String(), "\n") {
			stderr.WriteByte('\n')
		}
	}

	return stdout.String(), stderr.String(), nil
}

func dialTarget(ctx context.Context, target string, port int) (string, string, error) {
	address := net.JoinHostPort(target, strconv.Itoa(port))
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return "", "", err
	}
	_ = conn.Close()
	return fmt.Sprintf("tcp connectivity ok: %s", address), "", nil
}

func argString(args map[string]any, key, fallback string) string {
	if raw, ok := args[key]; ok {
		switch value := raw.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return value
			}
		case fmt.Stringer:
			return value.String()
		}
	}
	return fallback
}

func argInt(args map[string]any, key string, fallback int) int {
	if raw, ok := args[key]; ok {
		switch value := raw.(type) {
		case int:
			return value
		case int64:
			return int(value)
		case float64:
			return int(value)
		case string:
			parsed, err := strconv.Atoi(value)
			if err == nil {
				return parsed
			}
		}
	}
	return fallback
}

func sanitizeService(raw string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.', r == '@':
			return r
		default:
			return -1
		}
	}, raw)
	if clean == "" {
		return "nginx"
	}
	return clean
}

func sanitizePath(raw string) string {
	clean := filepath.Clean(raw)
	if !strings.HasPrefix(clean, "/") {
		return "/"
	}
	return clean
}

func sanitizeLogPath(raw string) (string, error) {
	clean := sanitizePath(raw)
	if !strings.HasPrefix(clean, "/var/log/") {
		return "", errors.New("log path is outside of the allowlist")
	}
	return clean, nil
}

func sanitizeTarget(raw string) string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "127.0.0.1"
	}
	return clean
}

func truncateTail(raw string) string {
	const maxTailLength = 4096
	if len(raw) <= maxTailLength {
		return strings.TrimSpace(raw)
	}
	return strings.TrimSpace(raw[len(raw)-maxTailLength:])
}

func joinNonEmpty(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			filtered = append(filtered, strings.TrimSpace(part))
		}
	}
	return strings.Join(filtered, "\n")
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func formatStepOutput(idx int, action, output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	return fmt.Sprintf("[step %d] %s\n%s\n", idx+1, action, trimmed)
}

func cancelledResult(start time.Time, stdout, stderr string, err error) Result {
	return Result{
		Status:     "cancelled",
		ExitCode:   130,
		StdoutTail: truncateTail(stdout),
		StderrTail: truncateTail(joinNonEmpty(stderr, err.Error())),
		Duration:   time.Since(start),
	}
}
