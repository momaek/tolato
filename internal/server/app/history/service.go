package history

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type Service interface {
	ListTasks(ctx context.Context, filter ListFilter) ([]TaskItem, error)
	GetTaskDetail(ctx context.Context, taskID string) (TaskDetail, error)
}

type Repositories struct {
	Sessions    domain.SessionRepository
	Tasks       domain.TaskRepository
	Timelines   domain.TimelineRepository
	ToolCalls   domain.ToolCallRepository
	Executions  domain.ExecutionRepository
	Audits      domain.AuditRepository
	ToolResults domain.ToolResultRepository
}

type service struct {
	repos Repositories
}

type ListFilter struct {
	Query            string
	Statuses         []domain.TaskStatus
	ApprovalStatuses []domain.ApprovalStatus
	Limit            int
}

type TaskItem struct {
	ID             string                `json:"id"`
	Title          string                `json:"title"`
	Summary        string                `json:"summary"`
	Status         domain.TaskStatus     `json:"status"`
	ApprovalStatus domain.ApprovalStatus `json:"approvalStatus"`
	Risk           domain.RiskLevel      `json:"risk"`
	TargetLabels   []string              `json:"targetLabels"`
	CreatedAt      string                `json:"createdAt"`
	UpdatedAt      string                `json:"updatedAt"`
}

type AuditEvent struct {
	ID          string `json:"id"`
	Actor       string `json:"actor"`
	EventType   string `json:"eventType"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
}

type ExecutionSummary struct {
	ID            string `json:"id"`
	TaskID        string `json:"taskId"`
	NodeID        string `json:"nodeId"`
	Label         string `json:"label"`
	Status        string `json:"status"`
	StartedAt     string `json:"startedAt,omitempty"`
	FinishedAt    string `json:"finishedAt,omitempty"`
	ExitCode      *int   `json:"exitCode,omitempty"`
	StdoutTail    string `json:"stdoutTail,omitempty"`
	StderrTail    string `json:"stderrTail,omitempty"`
	StreamSummary string `json:"streamSummary,omitempty"`
}

type PlanStep struct {
	ID     string         `json:"id"`
	Action string         `json:"action"`
	Args   map[string]any `json:"args,omitempty"`
	Risk   string         `json:"risk"`
}

type PlanDetail struct {
	TargetNodes        []string   `json:"targetNodes"`
	Summary            string     `json:"summary"`
	EstimatedImpact    string     `json:"estimatedImpact"`
	RiskLevel          string     `json:"riskLevel"`
	RequiresApproval   bool       `json:"requiresApproval"`
	Steps              []PlanStep `json:"steps"`
	SourceToolResultID string     `json:"sourceToolResultId,omitempty"`
}

type ApprovalDetail struct {
	Status           domain.ApprovalStatus `json:"status"`
	RiskLevel        domain.RiskLevel      `json:"riskLevel"`
	RequiresApproval bool                  `json:"requiresApproval"`
	LatestDecision   string                `json:"latestDecision,omitempty"`
	LatestReason     string                `json:"latestReason,omitempty"`
	LatestActor      string                `json:"latestActor,omitempty"`
	LatestTimestamp  string                `json:"latestTimestamp,omitempty"`
}

type TimelineMetaRow struct {
	ID          string                   `json:"id"`
	Kind        domain.TimelineRowKind   `json:"kind"`
	Text        string                   `json:"text,omitempty"`
	ToolName    string                   `json:"toolName,omitempty"`
	ToolStatus  domain.ToolResultStatus  `json:"toolStatus,omitempty"`
	Source      domain.TimelineRowSource `json:"source,omitempty"`
	ArgsPreview string                   `json:"argsPreview,omitempty"`
	CreatedAt   string                   `json:"createdAt"`
}

type ToolCallDetail struct {
	ID          string                `json:"id"`
	ToolName    string                `json:"toolName"`
	Source      domain.ToolCallSource `json:"source"`
	ArgsPreview string                `json:"argsPreview,omitempty"`
	Arguments   map[string]any        `json:"arguments,omitempty"`
	CreatedAt   string                `json:"createdAt"`
}

type ToolResultDetail struct {
	ID        string                   `json:"id"`
	ToolName  string                   `json:"toolName"`
	Status    domain.ToolResultStatus  `json:"status"`
	Text      string                   `json:"text,omitempty"`
	Source    domain.TimelineRowSource `json:"source"`
	Payload   map[string]any           `json:"payload,omitempty"`
	CreatedAt string                   `json:"createdAt"`
}

type TaskDetail struct {
	TaskItem
	Mode          string             `json:"mode"`
	InputText     string             `json:"inputText"`
	Target        []string           `json:"target"`
	Impact        string             `json:"impact"`
	Steps         []string           `json:"steps"`
	Plan          *PlanDetail        `json:"plan,omitempty"`
	Approval      *ApprovalDetail    `json:"approval,omitempty"`
	Executions    []ExecutionSummary `json:"executions"`
	AuditEvents   []AuditEvent       `json:"auditEvents"`
	ToolMeta      []string           `json:"toolMeta"`
	ToolCalls     []ToolCallDetail   `json:"toolCalls,omitempty"`
	ToolResults   []ToolResultDetail `json:"toolResults,omitempty"`
	PlanRows      []TimelineMetaRow  `json:"planRows,omitempty"`
	ApprovalRows  []TimelineMetaRow  `json:"approvalRows,omitempty"`
	ExecutionRows []TimelineMetaRow  `json:"executionRows,omitempty"`
	SummaryRows   []TimelineMetaRow  `json:"summaryRows,omitempty"`
	AISummary     string             `json:"aiSummary"`
}

func NewService(repos Repositories) Service {
	return &service{repos: repos}
}

func (s *service) ListTasks(ctx context.Context, filter ListFilter) ([]TaskItem, error) {
	if s.repos.Sessions == nil || s.repos.Tasks == nil {
		return nil, domain.ErrUnsupportedConfig
	}

	sessions, err := s.repos.Sessions.List(ctx, domain.SessionFilter{})
	if err != nil {
		return nil, err
	}

	items := make([]TaskItem, 0)
	for _, session := range sessions {
		tasks, err := s.repos.Tasks.ListBySession(ctx, session.ID)
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			if !matchesTaskFilter(task, filter) {
				continue
			}
			items = append(items, toTaskItem(task))
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

func (s *service) GetTaskDetail(ctx context.Context, taskID string) (TaskDetail, error) {
	if taskID == "" {
		return TaskDetail{}, domain.ErrInvalidArgument
	}
	if s.repos.Tasks == nil || s.repos.Executions == nil || s.repos.Audits == nil || s.repos.ToolResults == nil {
		return TaskDetail{}, domain.ErrUnsupportedConfig
	}

	task, err := s.repos.Tasks.Get(ctx, taskID)
	if err != nil {
		return TaskDetail{}, err
	}

	executions, err := s.repos.Executions.ListByTask(ctx, taskID)
	if err != nil {
		return TaskDetail{}, err
	}
	audits, err := s.repos.Audits.ListByTask(ctx, taskID)
	if err != nil {
		return TaskDetail{}, err
	}
	toolResults, err := s.repos.ToolResults.ListByTask(ctx, taskID)
	if err != nil {
		return TaskDetail{}, err
	}

	var toolCalls []domain.ToolCall
	if s.repos.ToolCalls != nil {
		sessionCalls, err := s.repos.ToolCalls.ListBySession(ctx, task.SessionID, domain.CursorPage{})
		if err != nil {
			return TaskDetail{}, err
		}
		toolCalls = filterToolCallsByTask(sessionCalls, taskID)
	}

	var timelineRows []domain.TimelineRow
	if s.repos.Timelines != nil {
		sessionRows, err := s.repos.Timelines.ListBySession(ctx, task.SessionID, domain.CursorPage{})
		if err != nil {
			return TaskDetail{}, err
		}
		timelineRows = filterTimelineRowsByTask(sessionRows, taskID)
	}

	item := toTaskItem(task)
	detail := TaskDetail{
		TaskItem:      item,
		Mode:          "ai_agent",
		InputText:     task.InputText,
		Target:        append([]string(nil), task.OperationTargetSnapshot.NodeIDs...),
		Impact:        buildImpact(task),
		Steps:         buildSteps(task, executions),
		Plan:          buildPlanDetail(task, toolResults),
		Approval:      buildApprovalDetail(task, audits),
		Executions:    buildExecutions(executions),
		AuditEvents:   buildAuditEvents(audits),
		ToolMeta:      buildToolMeta(toolResults),
		ToolCalls:     buildToolCallDetails(toolCalls),
		ToolResults:   buildToolResultDetails(toolResults),
		PlanRows:      buildTimelineMetaRows(timelineRows, domain.TimelineRowKindPlan),
		ApprovalRows:  buildTimelineMetaRows(timelineRows, domain.TimelineRowKindApproval),
		ExecutionRows: buildTimelineMetaRows(timelineRows, domain.TimelineRowKindExecution),
		SummaryRows:   buildTimelineMetaRows(timelineRows, domain.TimelineRowKindSummary),
		AISummary:     taskSummary(task),
	}
	return detail, nil
}

func toTaskItem(task domain.Task) TaskItem {
	return TaskItem{
		ID:             task.ID,
		Title:          taskTitle(task),
		Summary:        taskSummary(task),
		Status:         task.Status,
		ApprovalStatus: task.ApprovalStatus,
		Risk:           task.RiskLevel,
		TargetLabels:   targetLabels(task),
		CreatedAt:      task.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      task.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func taskTitle(task domain.Task) string {
	if text := strings.TrimSpace(task.InputText); text != "" {
		return text
	}
	if label := strings.TrimSpace(task.OperationTargetSnapshot.DisplayLabel); label != "" {
		return label
	}
	return task.ID
}

func taskSummary(task domain.Task) string {
	if task.Summary != nil && strings.TrimSpace(*task.Summary) != "" {
		return *task.Summary
	}
	targets := strings.Join(targetLabels(task), ", ")
	if targets == "" {
		targets = "unknown target"
	}
	return fmt.Sprintf("task %s on %s", task.Status, targets)
}

func targetLabels(task domain.Task) []string {
	if label := strings.TrimSpace(task.OperationTargetSnapshot.DisplayLabel); label != "" {
		return []string{label}
	}
	if len(task.OperationTargetSnapshot.NodeIDs) > 0 {
		return append([]string(nil), task.OperationTargetSnapshot.NodeIDs...)
	}
	return []string{"unknown target"}
}

func buildImpact(task domain.Task) string {
	target := strings.Join(targetLabels(task), ", ")
	switch task.RiskLevel {
	case domain.RiskLevelHigh:
		return fmt.Sprintf("High-risk operation targeting %s. Explicit approval and execution audit are required.", target)
	case domain.RiskLevelMedium:
		return fmt.Sprintf("Moderate-risk operation targeting %s. Review approval state and node execution outcome before retrying.", target)
	case domain.RiskLevelLow:
		return fmt.Sprintf("Low-risk operation targeting %s. Expected to be safe for read-only or low-impact automation.", target)
	default:
		return fmt.Sprintf("Operation targeting %s.", target)
	}
}

func buildSteps(task domain.Task, executions []domain.Execution) []string {
	steps := []string{
		fmt.Sprintf("Resolve target scope: %s", strings.Join(targetLabels(task), ", ")),
		fmt.Sprintf("Apply risk policy: %s", task.RiskLevel),
	}
	if task.ApprovalStatus == domain.ApprovalStatusPending || task.ApprovalStatus == domain.ApprovalStatusApproved {
		steps = append(steps, fmt.Sprintf("Approval state: %s", task.ApprovalStatus))
	}
	if len(executions) > 0 {
		steps = append(steps, fmt.Sprintf("Execute on %d node(s)", len(executions)))
	}
	steps = append(steps, fmt.Sprintf("Final task status: %s", task.Status))
	return steps
}

func buildExecutions(items []domain.Execution) []ExecutionSummary {
	out := make([]ExecutionSummary, 0, len(items))
	for _, execution := range items {
		sortKey := execution.UpdatedAt
		streamSummary := execution.StdoutTail
		if execution.StatusReason != nil && strings.TrimSpace(*execution.StatusReason) != "" {
			streamSummary = *execution.StatusReason
		}
		out = append(out, ExecutionSummary{
			ID:            execution.ID,
			TaskID:        execution.TaskID,
			NodeID:        execution.NodeID,
			Label:         execution.NodeID,
			Status:        mapExecutionStatus(execution.Status),
			StartedAt:     formatOptionalTime(execution.StartedAt),
			FinishedAt:    formatOptionalTime(execution.FinishedAt),
			ExitCode:      execution.ExitCode,
			StdoutTail:    execution.StdoutTail,
			StderrTail:    execution.StderrTail,
			StreamSummary: streamSummary,
		})
		_ = sortKey
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NodeID < out[j].NodeID
	})
	return out
}

func buildAuditEvents(items []domain.AuditRecord) []AuditEvent {
	out := make([]AuditEvent, 0, len(items))
	for _, item := range items {
		out = append(out, AuditEvent{
			ID:          item.ID,
			Actor:       item.ActorID,
			EventType:   item.EventType,
			Description: auditDescription(item),
			CreatedAt:   item.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}

func buildToolMeta(items []domain.ToolResult) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		label := fmt.Sprintf("%s:%s", item.ToolName, item.Status)
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func buildPlanDetail(task domain.Task, toolResults []domain.ToolResult) *PlanDetail {
	for _, item := range toolResults {
		if item.ToolName != "propose_plan" || len(item.Payload) == 0 {
			continue
		}
		var plan struct {
			TargetNodes      []string `json:"targetNodes"`
			Summary          string   `json:"summary"`
			EstimatedImpact  string   `json:"estimatedImpact"`
			RiskLevel        string   `json:"riskLevel"`
			RequiresApproval bool     `json:"requiresApproval"`
			Steps            []struct {
				Action string         `json:"action"`
				Args   map[string]any `json:"args"`
				Risk   string         `json:"risk"`
			} `json:"steps"`
		}
		if err := json.Unmarshal(item.Payload, &plan); err != nil {
			continue
		}
		steps := make([]PlanStep, 0, len(plan.Steps))
		for idx, step := range plan.Steps {
			steps = append(steps, PlanStep{
				ID:     fmt.Sprintf("step-%d", idx+1),
				Action: step.Action,
				Args:   step.Args,
				Risk:   step.Risk,
			})
		}
		return &PlanDetail{
			TargetNodes:        append([]string(nil), plan.TargetNodes...),
			Summary:            firstNonEmpty(plan.Summary, taskSummary(task)),
			EstimatedImpact:    firstNonEmpty(plan.EstimatedImpact, buildImpact(task)),
			RiskLevel:          firstNonEmpty(plan.RiskLevel, string(task.RiskLevel)),
			RequiresApproval:   plan.RequiresApproval,
			Steps:              steps,
			SourceToolResultID: item.ID,
		}
	}

	return &PlanDetail{
		TargetNodes:      append([]string(nil), task.OperationTargetSnapshot.NodeIDs...),
		Summary:          taskSummary(task),
		EstimatedImpact:  buildImpact(task),
		RiskLevel:        string(task.RiskLevel),
		RequiresApproval: task.ApprovalStatus != domain.ApprovalStatusNotRequired,
		Steps:            fallbackPlanSteps(buildSteps(task, nil), task.RiskLevel),
	}
}

func buildApprovalDetail(task domain.Task, audits []domain.AuditRecord) *ApprovalDetail {
	detail := &ApprovalDetail{
		Status:           task.ApprovalStatus,
		RiskLevel:        task.RiskLevel,
		RequiresApproval: task.ApprovalStatus != domain.ApprovalStatusNotRequired,
	}

	for i := len(audits) - 1; i >= 0; i-- {
		item := audits[i]
		if !strings.HasPrefix(item.EventType, "approval.") {
			continue
		}
		detail.LatestDecision = item.EventType
		detail.LatestActor = item.ActorID
		detail.LatestTimestamp = item.CreatedAt.UTC().Format(time.RFC3339)
		if len(item.Payload) > 0 {
			detail.LatestReason = string(item.Payload)
		}
		return detail
	}
	return detail
}

func buildToolCallDetails(items []domain.ToolCall) []ToolCallDetail {
	out := make([]ToolCallDetail, 0, len(items))
	for _, item := range items {
		out = append(out, ToolCallDetail{
			ID:          item.ID,
			ToolName:    item.ToolName,
			Source:      item.Source,
			ArgsPreview: derefString(item.ArgsPreview),
			Arguments:   decodeRawObject(item.Arguments),
			CreatedAt:   item.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}

func buildToolResultDetails(items []domain.ToolResult) []ToolResultDetail {
	out := make([]ToolResultDetail, 0, len(items))
	for _, item := range items {
		out = append(out, ToolResultDetail{
			ID:        item.ID,
			ToolName:  item.ToolName,
			Status:    item.Status,
			Text:      item.Text,
			Source:    item.Source,
			Payload:   decodeRawObject(item.Payload),
			CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}

func buildTimelineMetaRows(items []domain.TimelineRow, kind domain.TimelineRowKind) []TimelineMetaRow {
	out := make([]TimelineMetaRow, 0)
	for _, item := range items {
		if item.Kind != kind {
			continue
		}
		out = append(out, TimelineMetaRow{
			ID:          item.ID,
			Kind:        item.Kind,
			Text:        item.Text,
			ToolName:    item.ToolName,
			ToolStatus:  item.ToolStatus,
			Source:      item.Source,
			ArgsPreview: derefString(item.ArgsPreview),
			CreatedAt:   item.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}

func matchesTaskFilter(task domain.Task, filter ListFilter) bool {
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	if query != "" {
		if !strings.Contains(strings.ToLower(taskTitle(task)), query) &&
			!strings.Contains(strings.ToLower(taskSummary(task)), query) &&
			!containsAnyTarget(task, query) {
			return false
		}
	}
	if len(filter.Statuses) > 0 && !containsTaskStatus(filter.Statuses, task.Status) {
		return false
	}
	if len(filter.ApprovalStatuses) > 0 && !containsApprovalStatus(filter.ApprovalStatuses, task.ApprovalStatus) {
		return false
	}
	return true
}

func containsAnyTarget(task domain.Task, query string) bool {
	for _, label := range targetLabels(task) {
		if strings.Contains(strings.ToLower(label), query) {
			return true
		}
	}
	return false
}

func containsTaskStatus(items []domain.TaskStatus, wanted domain.TaskStatus) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}

func containsApprovalStatus(items []domain.ApprovalStatus, wanted domain.ApprovalStatus) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}

func filterToolCallsByTask(items []domain.ToolCall, taskID string) []domain.ToolCall {
	out := make([]domain.ToolCall, 0)
	for _, item := range items {
		if item.TaskID == nil || *item.TaskID != taskID {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterTimelineRowsByTask(items []domain.TimelineRow, taskID string) []domain.TimelineRow {
	out := make([]domain.TimelineRow, 0)
	for _, item := range items {
		if item.TaskID == nil || *item.TaskID != taskID {
			continue
		}
		out = append(out, item)
	}
	return out
}

func decodeRawObject(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func fallbackPlanSteps(steps []string, risk domain.RiskLevel) []PlanStep {
	out := make([]PlanStep, 0, len(steps))
	for idx, step := range steps {
		out = append(out, PlanStep{
			ID:     fmt.Sprintf("step-%d", idx+1),
			Action: step,
			Risk:   string(risk),
		})
	}
	return out
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mapExecutionStatus(status domain.ExecutionStatus) string {
	switch status {
	case domain.ExecutionStatusCancelled:
		return "skipped"
	default:
		return string(status)
	}
}

func auditDescription(item domain.AuditRecord) string {
	if len(item.Payload) == 0 {
		return item.EventType
	}
	var payload any
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return item.EventType
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return item.EventType
	}
	return fmt.Sprintf("%s · %s", item.EventType, string(encoded))
}
