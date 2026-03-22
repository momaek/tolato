package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type auditRepository struct {
	q Queryer
}

func NewAuditRepository(q Queryer) domain.AuditRepository {
	return &auditRepository{q: q}
}

func (r *auditRepository) Append(ctx context.Context, record domain.AuditRecord) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO audits (id, session_id, task_id, actor_id, event_type, payload, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
`, record.ID, record.SessionID, nullableString(record.TaskID), record.ActorID, record.EventType, rawMessage(record.Payload), record.CreatedAt)
	return err
}

func (r *auditRepository) ListByTask(ctx context.Context, taskID string) ([]domain.AuditRecord, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, session_id, task_id, actor_id, event_type, payload, created_at
FROM audits
WHERE task_id = $1
ORDER BY created_at ASC, id ASC
`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.AuditRecord, 0)
	for rows.Next() {
		item, scanErr := scanAuditRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanAuditRecord(rows Rows) (domain.AuditRecord, error) {
	var (
		id, sessionID, actorID, eventType string
		taskID                            sql.NullString
		payloadRaw                        []byte
		createdAt                         time.Time
	)
	if err := rows.Scan(&id, &sessionID, &taskID, &actorID, &eventType, &payloadRaw, &createdAt); err != nil {
		return domain.AuditRecord{}, err
	}
	return domain.AuditRecord{
		ID:        id,
		SessionID: sessionID,
		TaskID:    nullStringPtr(taskID),
		ActorID:   actorID,
		EventType: eventType,
		Payload:   cloneBytes(payloadRaw),
		CreatedAt: createdAt.UTC(),
	}, nil
}
