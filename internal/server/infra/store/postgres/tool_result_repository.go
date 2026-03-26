package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type toolResultRepository struct {
	q Queryer
}

func NewToolResultRepository(q Queryer) domain.ToolResultRepository {
	return &toolResultRepository{q: q}
}

func (r *toolResultRepository) Append(ctx context.Context, result domain.ToolResult) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO tool_results (id, session_id, task_id, tool_call_id, call_id, tool_name, status, text, source, payload, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
`, result.ID, result.SessionID, nullableString(result.TaskID), nullableString(result.ToolCallID), nullableString(result.CallID), result.ToolName,
		string(result.Status), result.Text, string(result.Source), rawMessage(result.Payload), result.CreatedAt)
	return err
}

func (r *toolResultRepository) ListBySession(ctx context.Context, sessionID string, page domain.CursorPage) ([]domain.ToolResult, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, session_id, task_id, tool_call_id, call_id, tool_name, status, text, source, payload, created_at
FROM tool_results
WHERE session_id = $1
ORDER BY created_at ASC, id ASC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.ToolResult, 0)
	for rows.Next() {
		item, scanErr := scanToolResult(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if page.BeforeID != "" {
		var filterErr error
		items, filterErr = filterBeforeID(items, func(item domain.ToolResult) string { return item.ID }, page.BeforeID)
		if filterErr != nil {
			return nil, filterErr
		}
	}
	items = tail(items, page.Limit)
	return items, nil
}

func (r *toolResultRepository) ListByTask(ctx context.Context, taskID string) ([]domain.ToolResult, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, session_id, task_id, tool_call_id, call_id, tool_name, status, text, source, payload, created_at
FROM tool_results
WHERE task_id = $1
ORDER BY created_at ASC, id ASC
`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.ToolResult, 0)
	for rows.Next() {
		item, scanErr := scanToolResult(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func scanToolResult(rows Rows) (domain.ToolResult, error) {
	var (
		id, sessionID, toolName, status, text, source string
		taskID                                        sql.NullString
		toolCallID                                    sql.NullString
		callID                                        sql.NullString
		payloadRaw                                    []byte
		createdAt                                     time.Time
	)
	if err := rows.Scan(&id, &sessionID, &taskID, &toolCallID, &callID, &toolName, &status, &text, &source, &payloadRaw, &createdAt); err != nil {
		return domain.ToolResult{}, err
	}
	return domain.ToolResult{
		ID:         id,
		SessionID:  sessionID,
		TaskID:     nullStringPtr(taskID),
		ToolCallID: nullStringPtr(toolCallID),
		CallID:     nullStringPtr(callID),
		ToolName:   toolName,
		Status:     domain.ToolResultStatus(status),
		Text:       text,
		Source:     domain.TimelineRowSource(source),
		Payload:    cloneBytes(payloadRaw),
		CreatedAt:  createdAt.UTC(),
	}, nil
}
