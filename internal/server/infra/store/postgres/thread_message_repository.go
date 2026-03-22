package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type threadMessageRepository struct {
	q Queryer
}

func NewThreadMessageRepository(q Queryer) domain.ThreadMessageRepository {
	return &threadMessageRepository{q: q}
}

func (r *threadMessageRepository) Append(ctx context.Context, message domain.ThreadMessage) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO thread_messages (id, session_id, client_message_id, role, kind, content, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
`, message.ID, message.SessionID, nullableString(message.ClientMessageID), string(message.Role), string(message.Kind), message.Content, message.CreatedAt)
	return err
}

func (r *threadMessageRepository) ListBySession(ctx context.Context, sessionID string, page domain.CursorPage) ([]domain.ThreadMessage, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, session_id, client_message_id, role, kind, content, created_at
FROM thread_messages
WHERE session_id = $1
ORDER BY created_at ASC, id ASC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.ThreadMessage, 0)
	for rows.Next() {
		item, scanErr := scanThreadMessage(rows)
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
		items, filterErr = filterBeforeID(items, func(item domain.ThreadMessage) string { return item.ID }, page.BeforeID)
		if filterErr != nil {
			return nil, filterErr
		}
	}
	items = tail(items, page.Limit)
	return items, nil
}

func scanThreadMessage(rows Rows) (domain.ThreadMessage, error) {
	var (
		id, sessionID, role, kind, content string
		clientMessageID                    sql.NullString
		createdAt                          time.Time
	)
	if err := rows.Scan(&id, &sessionID, &clientMessageID, &role, &kind, &content, &createdAt); err != nil {
		return domain.ThreadMessage{}, err
	}
	message := domain.ThreadMessage{
		ID:        id,
		SessionID: sessionID,
		Role:      domain.MessageRole(role),
		Kind:      domain.ThreadMessageKind(kind),
		Content:   content,
		CreatedAt: createdAt.UTC(),
	}
	if clientMessageID.Valid {
		message.ClientMessageID = stringPtr(clientMessageID.String)
	}
	return message, nil
}
