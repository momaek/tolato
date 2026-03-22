package devseed

import (
	"context"
	"encoding/json"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
	"github.com/momaek/tolato/internal/server/infra/store/memory"
)

func SeedHistoryStore(ctx context.Context, store *memory.Store, now time.Time) error {
	if store == nil {
		return domain.ErrInvalidArgument
	}

	summaryA := "execution completed successfully on jp-tokyo-01"
	summaryB := "execution failed on us-sfo-01 after approval"
	taskIDApproved := "task-401"
	taskIDFailed := "task-366"

	if err := store.Sessions.Create(ctx, domain.Session{
		ID:        "sess-1",
		Title:     "Tokyo Session",
		Status:    domain.SessionStatusCompleted,
		CreatedAt: now.Add(-20 * time.Minute),
		UpdatedAt: now.Add(-10 * time.Minute),
	}); err != nil {
		return err
	}
	if err := store.Sessions.Create(ctx, domain.Session{
		ID:        "sess-2",
		Title:     "San Francisco Session",
		Status:    domain.SessionStatusFailed,
		CreatedAt: now.Add(-2 * time.Hour),
		UpdatedAt: now.Add(-90 * time.Minute),
	}); err != nil {
		return err
	}

	if err := store.Tasks.Create(ctx, domain.Task{
		ID:        taskIDApproved,
		SessionID: "sess-1",
		InputText: "Restart nginx safely",
		OperationTargetSnapshot: domain.TargetSnapshot{
			Scope:        domain.TargetScopeSingle,
			NodeIDs:      []string{"jp-tokyo-01"},
			DisplayLabel: "jp-tokyo-01",
			Confirmed:    true,
			CapturedAt:   now.Add(-18 * time.Minute),
		},
		Status:         domain.TaskStatusSuccess,
		ApprovalStatus: domain.ApprovalStatusApproved,
		RiskLevel:      domain.RiskLevelMedium,
		Summary:        &summaryA,
		CreatedAt:      now.Add(-18 * time.Minute),
		UpdatedAt:      now.Add(-10 * time.Minute),
	}); err != nil {
		return err
	}
	if err := store.Tasks.Create(ctx, domain.Task{
		ID:        taskIDFailed,
		SessionID: "sess-2",
		InputText: "Restart api service",
		OperationTargetSnapshot: domain.TargetSnapshot{
			Scope:        domain.TargetScopeSingle,
			NodeIDs:      []string{"us-sfo-01"},
			DisplayLabel: "us-sfo-01",
			Confirmed:    true,
			CapturedAt:   now.Add(-110 * time.Minute),
		},
		Status:         domain.TaskStatusFailed,
		ApprovalStatus: domain.ApprovalStatusApproved,
		RiskLevel:      domain.RiskLevelHigh,
		Summary:        &summaryB,
		CreatedAt:      now.Add(-110 * time.Minute),
		UpdatedAt:      now.Add(-90 * time.Minute),
	}); err != nil {
		return err
	}

	for _, execution := range []domain.Execution{
		{
			ID:         "exec-401",
			TaskID:     taskIDApproved,
			SessionID:  "sess-1",
			NodeID:     "jp-tokyo-01",
			Status:     domain.ExecutionStatusSuccess,
			StdoutTail: "nginx restarted cleanly",
			CreatedAt:  now.Add(-17 * time.Minute),
			UpdatedAt:  now.Add(-10 * time.Minute),
		},
		{
			ID:           "exec-366",
			TaskID:       taskIDFailed,
			SessionID:    "sess-2",
			NodeID:       "us-sfo-01",
			Status:       domain.ExecutionStatusFailed,
			StdoutTail:   "service stop issued",
			StderrTail:   "health check failed",
			StatusReason: strPtr("restart command exited with non-zero status"),
			CreatedAt:    now.Add(-108 * time.Minute),
			UpdatedAt:    now.Add(-90 * time.Minute),
		},
	} {
		if err := store.Executions.Create(ctx, execution); err != nil {
			return err
		}
	}

	for _, audit := range []domain.AuditRecord{
		{
			ID:        "audit-401",
			SessionID: "sess-1",
			TaskID:    &taskIDApproved,
			ActorID:   "ui_user",
			EventType: "approval.approved",
			Payload:   []byte(`{"approved":true}`),
			CreatedAt: now.Add(-17 * time.Minute),
		},
		{
			ID:        "audit-366",
			SessionID: "sess-2",
			TaskID:    &taskIDFailed,
			ActorID:   "ui_user",
			EventType: "approval.approved",
			Payload:   []byte(`{"approved":true}`),
			CreatedAt: now.Add(-108 * time.Minute),
		},
	} {
		if err := store.Audits.Append(ctx, audit); err != nil {
			return err
		}
	}

	for _, result := range []domain.ToolResult{
		{
			ID:        "toolresult-401-plan",
			SessionID: "sess-1",
			TaskID:    &taskIDApproved,
			ToolName:  "propose_plan",
			Status:    domain.ToolResultStatusSucceeded,
			Text:      "safe restart plan generated",
			Payload: mustJSON(map[string]any{
				"targetNodes":      []string{"jp-tokyo-01"},
				"summary":          "Restart nginx safely on the selected node.",
				"estimatedImpact":  "A brief nginx worker reload on a single Tokyo edge node.",
				"riskLevel":        "medium",
				"requiresApproval": true,
				"steps": []map[string]any{
					{"action": "validate nginx config", "args": map[string]any{"command": "nginx -t"}, "risk": "low"},
					{"action": "reload nginx", "args": map[string]any{"command": "systemctl reload nginx"}, "risk": "medium"},
				},
			}),
			CreatedAt: now.Add(-18 * time.Minute),
		},
		{
			ID:        "toolresult-401-approval",
			SessionID: "sess-1",
			TaskID:    &taskIDApproved,
			ToolName:  "approval",
			Status:    domain.ToolResultStatusSucceeded,
			Text:      "approval recorded",
			CreatedAt: now.Add(-17 * time.Minute),
		},
		{
			ID:        "toolresult-366-plan",
			SessionID: "sess-2",
			TaskID:    &taskIDFailed,
			ToolName:  "propose_plan",
			Status:    domain.ToolResultStatusSucceeded,
			Text:      "restart plan generated",
			Payload: mustJSON(map[string]any{
				"targetNodes":      []string{"us-sfo-01"},
				"summary":          "Restart the API service on the San Francisco node.",
				"estimatedImpact":  "API traffic on a single production node may be interrupted during restart.",
				"riskLevel":        "high",
				"requiresApproval": true,
				"steps": []map[string]any{
					{"action": "stop api service", "args": map[string]any{"command": "systemctl stop api"}, "risk": "high"},
					{"action": "start api service", "args": map[string]any{"command": "systemctl start api"}, "risk": "high"},
				},
			}),
			CreatedAt: now.Add(-109 * time.Minute),
		},
	} {
		if err := store.ToolResults.Append(ctx, result); err != nil {
			return err
		}
	}

	for _, call := range []domain.ToolCall{
		{
			ID:          "toolcall-401-plan",
			SessionID:   "sess-1",
			TaskID:      &taskIDApproved,
			ToolName:    "propose_plan",
			Arguments:   mustJSON(map[string]any{"inputText": "Restart nginx safely", "target": []string{"jp-tokyo-01"}}),
			ArgsPreview: strPtr("jp-tokyo-01"),
			Source:      domain.ToolCallSourceAgentLoop,
			CreatedAt:   now.Add(-18 * time.Minute),
		},
		{
			ID:          "toolcall-401-exec",
			SessionID:   "sess-1",
			TaskID:      &taskIDApproved,
			ToolName:    "exec_on_nodes",
			Arguments:   mustJSON(map[string]any{"action": "reload nginx", "nodes": []string{"jp-tokyo-01"}}),
			ArgsPreview: strPtr("reload nginx"),
			Source:      domain.ToolCallSourceAgentLoop,
			CreatedAt:   now.Add(-17 * time.Minute),
		},
		{
			ID:          "toolcall-366-plan",
			SessionID:   "sess-2",
			TaskID:      &taskIDFailed,
			ToolName:    "propose_plan",
			Arguments:   mustJSON(map[string]any{"inputText": "Restart api service", "target": []string{"us-sfo-01"}}),
			ArgsPreview: strPtr("us-sfo-01"),
			Source:      domain.ToolCallSourceAgentLoop,
			CreatedAt:   now.Add(-109 * time.Minute),
		},
	} {
		if err := store.ToolCalls.Append(ctx, call); err != nil {
			return err
		}
	}

	for _, row := range []domain.TimelineRow{
		{
			ID:        "row-401-plan",
			SessionID: "sess-1",
			Kind:      domain.TimelineRowKindPlan,
			Text:      "Plan ready: validate nginx config, then reload nginx on jp-tokyo-01.",
			TaskID:    &taskIDApproved,
			Source:    domain.TimelineRowSourceAgentLoop,
			CreatedAt: now.Add(-18 * time.Minute),
		},
		{
			ID:        "row-401-approval",
			SessionID: "sess-1",
			Kind:      domain.TimelineRowKindApproval,
			Text:      "Approval granted for nginx restart.",
			TaskID:    &taskIDApproved,
			Source:    domain.TimelineRowSourceUserAction,
			CreatedAt: now.Add(-17 * time.Minute),
		},
		{
			ID:        "row-401-execution",
			SessionID: "sess-1",
			Kind:      domain.TimelineRowKindExecution,
			Text:      "Execution completed on jp-tokyo-01.",
			TaskID:    &taskIDApproved,
			Source:    domain.TimelineRowSourceAgentLoop,
			CreatedAt: now.Add(-10 * time.Minute),
		},
		{
			ID:        "row-401-summary",
			SessionID: "sess-1",
			Kind:      domain.TimelineRowKindSummary,
			Text:      summaryA,
			TaskID:    &taskIDApproved,
			Source:    domain.TimelineRowSourceAgentLoop,
			CreatedAt: now.Add(-10 * time.Minute),
		},
		{
			ID:        "row-366-plan",
			SessionID: "sess-2",
			Kind:      domain.TimelineRowKindPlan,
			Text:      "Plan ready: stop and start API service on us-sfo-01.",
			TaskID:    &taskIDFailed,
			Source:    domain.TimelineRowSourceAgentLoop,
			CreatedAt: now.Add(-109 * time.Minute),
		},
		{
			ID:        "row-366-approval",
			SessionID: "sess-2",
			Kind:      domain.TimelineRowKindApproval,
			Text:      "Approval granted for API restart.",
			TaskID:    &taskIDFailed,
			Source:    domain.TimelineRowSourceUserAction,
			CreatedAt: now.Add(-108 * time.Minute),
		},
		{
			ID:        "row-366-execution",
			SessionID: "sess-2",
			Kind:      domain.TimelineRowKindExecution,
			Text:      "Execution failed on us-sfo-01.",
			TaskID:    &taskIDFailed,
			Source:    domain.TimelineRowSourceAgentLoop,
			CreatedAt: now.Add(-90 * time.Minute),
		},
		{
			ID:        "row-366-summary",
			SessionID: "sess-2",
			Kind:      domain.TimelineRowKindSummary,
			Text:      summaryB,
			TaskID:    &taskIDFailed,
			Source:    domain.TimelineRowSourceAgentLoop,
			CreatedAt: now.Add(-90 * time.Minute),
		},
	} {
		if err := store.Timelines.Append(ctx, row); err != nil {
			return err
		}
	}

	return nil
}

func strPtr(v string) *string { return &v }

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
