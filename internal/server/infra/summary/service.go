package summary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	domainsummary "github.com/momaek/tolato/internal/server/domain/summary"
	"github.com/momaek/tolato/internal/shared/types"
)

type Service struct {
	config types.LLMConfig
	client *http.Client
}

func NewService(config types.LLMConfig) Service {
	return Service{
		config: config,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (s Service) SummarizeTask(ctx context.Context, task types.Task, executions []types.TaskExecution, aggregate types.TaskAggregate) (domainsummary.Result, error) {
	if !s.isConfigured() {
		return fallbackResult(task, executions, aggregate), nil
	}

	payload, _ := json.Marshal(struct {
		Task       types.Task            `json:"task"`
		Executions []types.TaskExecution `json:"executions"`
		Aggregate  types.TaskAggregate   `json:"aggregate"`
	}{
		Task:       task,
		Executions: executions,
		Aggregate:  aggregate,
	})

	content, err := s.completeJSON(ctx, "You summarize distributed task execution results as concise operator-facing JSON.", fmt.Sprintf(`Return JSON:
{"summary":"string","result_summary":"string","failure_node_ids":["node_id"],"source":"llm"}

Task execution data:
%s`, string(payload)))
	if err != nil {
		return fallbackResult(task, executions, aggregate), nil
	}

	var result domainsummary.Result
	if err := json.Unmarshal([]byte(content), &result); err != nil || strings.TrimSpace(result.Summary) == "" {
		return fallbackResult(task, executions, aggregate), nil
	}
	if result.ResultSummary == "" {
		result.ResultSummary = result.Summary
	}
	if result.Source == "" {
		result.Source = "llm"
	}
	return result, nil
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

func fallbackResult(task types.Task, executions []types.TaskExecution, aggregate types.TaskAggregate) domainsummary.Result {
	failures := failureNodeIDs(executions)
	summary := fallbackSummary(task, aggregate, failures)
	return domainsummary.Result{
		Summary:        summary,
		ResultSummary:  summary,
		FailureNodeIDs: failures,
		Source:         "rule_fallback",
	}
}

func fallbackSummary(task types.Task, aggregate types.TaskAggregate, failures []string) string {
	if task.FinalStatus == "waiting_approval" {
		return "Task is waiting for approval."
	}
	if aggregate.Running > 0 {
		return fmt.Sprintf("Task is running: %d/%d running, %d succeeded, %d failed.", aggregate.Running, aggregate.Total, aggregate.Success, aggregate.Failed)
	}
	if len(failures) > 0 {
		return fmt.Sprintf("Task finished with failures on %s. %d/%d succeeded.", strings.Join(failures, ", "), aggregate.Success, aggregate.Total)
	}
	if aggregate.Success > 0 || aggregate.OfflineSkipped > 0 {
		return fmt.Sprintf("Task finished: %d/%d succeeded, %d failed, %d offline skipped.", aggregate.Success, aggregate.Total, aggregate.Failed, aggregate.OfflineSkipped)
	}
	if task.StatusReason != "" {
		return task.StatusReason
	}
	return task.Plan.Summary
}

func failureNodeIDs(executions []types.TaskExecution) []string {
	items := make([]string, 0)
	for _, execution := range executions {
		switch execution.Status {
		case "failed", "partial_failed", "timeout", "cancelled":
			items = append(items, execution.NodeID)
		}
	}
	return items
}
