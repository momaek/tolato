package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type timelineRepository struct {
	q Queryer
}

func NewTimelineRepository(q Queryer) domain.TimelineRepository {
	return &timelineRepository{q: q}
}

func (r *timelineRepository) Append(ctx context.Context, row domain.TimelineRow) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO timeline_rows (
    id, session_id, kind, created_at, text, tool_name, tool_status, source, args_preview, task_id, target_context
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
`, row.ID, row.SessionID, string(row.Kind), row.CreatedAt, row.Text, row.ToolName, string(row.ToolStatus),
		string(row.Source), row.ArgsPreview, nullableString(row.TaskID), rawMessage(row.TargetContext))
	return err
}

func (r *timelineRepository) ListBySession(ctx context.Context, sessionID string, page domain.CursorPage) ([]domain.TimelineRow, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, session_id, kind, created_at, text, tool_name, tool_status, source, args_preview, task_id, target_context
FROM timeline_rows
WHERE session_id = $1
ORDER BY created_at ASC, id ASC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.TimelineRow, 0)
	for rows.Next() {
		item, scanErr := scanTimelineRow(rows)
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
		items, filterErr = filterBeforeID(items, func(item domain.TimelineRow) string { return item.ID }, page.BeforeID)
		if filterErr != nil {
			return nil, filterErr
		}
	}
	items = tail(items, page.Limit)
	return items, nil
}

func scanTimelineRow(rows Rows) (domain.TimelineRow, error) {
	var (
		id, sessionID, kind, text, toolName, toolStatus, source string
		argsPreview                                             sql.NullString
		taskID                                                  sql.NullString
		targetContextRaw                                        []byte
		createdAt                                               time.Time
	)
	if err := rows.Scan(&id, &sessionID, &kind, &createdAt, &text, &toolName, &toolStatus, &source, &argsPreview, &taskID, &targetContextRaw); err != nil {
		return domain.TimelineRow{}, err
	}

	row := domain.TimelineRow{
		ID:          id,
		SessionID:   sessionID,
		Kind:        domain.TimelineRowKind(kind),
		CreatedAt:   createdAt.UTC(),
		Text:        text,
		ToolName:    toolName,
		ToolStatus:  domain.ToolResultStatus(toolStatus),
		Source:      domain.TimelineRowSource(source),
		ArgsPreview: nullStringPtr(argsPreview),
		TaskID:      nullStringPtr(taskID),
	}
	if len(targetContextRaw) > 0 {
		var ctxValue domain.ActiveTargetContext
		if err := json.Unmarshal(targetContextRaw, &ctxValue); err != nil {
			return domain.TimelineRow{}, fmt.Errorf("decode target_context: %w", err)
		}
		row.TargetContext = &ctxValue
	}
	return row, nil
}
