package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type toolCallRepository struct {
	q Queryer
}

func NewToolCallRepository(q Queryer) domain.ToolCallRepository {
	return &toolCallRepository{q: q}
}

func (r *toolCallRepository) Append(ctx context.Context, call domain.ToolCall) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO tool_calls (id, session_id, task_id, message_id, call_id, tool_name, arguments, args_preview, source, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
`, call.ID, call.SessionID, nullableString(call.TaskID), nullableString(call.MessageID), nullableString(call.CallID), call.ToolName,
		rawMessage(call.Arguments), nullableString(call.ArgsPreview), string(call.Source), call.CreatedAt)
	return err
}

func (r *toolCallRepository) ListBySession(ctx context.Context, sessionID string, page domain.CursorPage) ([]domain.ToolCall, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, session_id, task_id, message_id, call_id, tool_name, arguments, args_preview, source, created_at
FROM tool_calls
WHERE session_id = $1
ORDER BY created_at ASC, id ASC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.ToolCall, 0)
	for rows.Next() {
		item, scanErr := scanToolCall(rows)
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
		items, filterErr = filterBeforeID(items, func(item domain.ToolCall) string { return item.ID }, page.BeforeID)
		if filterErr != nil {
			return nil, filterErr
		}
	}
	items = tail(items, page.Limit)
	return items, nil
}

func scanToolCall(rows Rows) (domain.ToolCall, error) {
	var (
		id, sessionID, toolName, source string
		taskID                          sql.NullString
		messageID                       sql.NullString
		callID                          sql.NullString
		argsPreview                     sql.NullString
		argumentsRaw                    []byte
		createdAt                       time.Time
	)
	if err := rows.Scan(&id, &sessionID, &taskID, &messageID, &callID, &toolName, &argumentsRaw, &argsPreview, &source, &createdAt); err != nil {
		return domain.ToolCall{}, err
	}
	call := domain.ToolCall{
		ID:          id,
		SessionID:   sessionID,
		TaskID:      nullStringPtr(taskID),
		MessageID:   nullStringPtr(messageID),
		CallID:      nullStringPtr(callID),
		ToolName:    toolName,
		Arguments:   cloneBytes(argumentsRaw),
		ArgsPreview: nullStringPtr(argsPreview),
		Source:      domain.ToolCallSource(source),
		CreatedAt:   createdAt.UTC(),
	}
	return call, nil
}
