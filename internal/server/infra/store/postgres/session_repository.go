package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type sessionRepository struct {
	q Queryer
}

func NewSessionRepository(q Queryer) domain.SessionRepository {
	return &sessionRepository{q: q}
}

func (r *sessionRepository) Create(ctx context.Context, session domain.Session) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO sessions (
    id, title, status, active_target_context, pending_action_type, pending_action_payload,
    current_operation_id, current_task_id, current_execution_group_id, last_agent_state,
    provider_state_blob, revision, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
`, session.ID, session.Title, string(session.Status), rawMessage(session.ActiveTargetContext),
		pendingActionType(session.PendingAction), pendingActionPayload(session.PendingAction),
		nullableString(session.CurrentOperationID), nullableString(session.CurrentTaskID), nullableString(session.CurrentExecutionGroupID),
		rawMessage(session.LastAgentState), rawMessage(session.ProviderStateBlob),
		session.Revision, session.CreatedAt, session.UpdatedAt)
	return err
}

func (r *sessionRepository) Delete(ctx context.Context, sessionID string) error {
	result, err := r.q.ExecContext(ctx, `
DELETE FROM sessions
WHERE id = $1
`, sessionID)
	if err != nil {
		return err
	}
	return requireRowsAffected(result, domain.ErrNotFound)
}

func (r *sessionRepository) Get(ctx context.Context, sessionID string) (domain.Session, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, title, status, active_target_context, pending_action_type, pending_action_payload,
       current_operation_id, current_task_id, current_execution_group_id, last_agent_state,
       provider_state_blob, revision, created_at, updated_at
FROM sessions
WHERE id = $1
`, sessionID)
	if err != nil {
		return domain.Session{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return domain.Session{}, err
		}
		return domain.Session{}, domain.ErrNotFound
	}
	session, scanErr := scanSession(rows)
	if scanErr != nil {
		return domain.Session{}, scanErr
	}
	if err := rows.Err(); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (r *sessionRepository) List(ctx context.Context, filter domain.SessionFilter) ([]domain.Session, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, title, status, active_target_context, pending_action_type, pending_action_payload,
       current_operation_id, current_task_id, current_execution_group_id, last_agent_state,
       provider_state_blob, revision, created_at, updated_at
FROM sessions
ORDER BY updated_at DESC, id DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.Session, 0)
	for rows.Next() {
		item, scanErr := scanSession(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	items = filterStatuses(items, filter.Statuses)
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

func (r *sessionRepository) Update(ctx context.Context, session domain.Session) error {
	result, err := r.q.ExecContext(ctx, `
UPDATE sessions SET
    title = $2,
    status = $3,
    active_target_context = $4,
    pending_action_type = $5,
    pending_action_payload = $6,
    current_operation_id = $7,
    current_task_id = $8,
    current_execution_group_id = $9,
    last_agent_state = $10,
    provider_state_blob = $11,
    revision = $12,
    created_at = $13,
    updated_at = $14
WHERE id = $1
`, session.ID, session.Title, string(session.Status), rawMessage(session.ActiveTargetContext),
		pendingActionType(session.PendingAction), pendingActionPayload(session.PendingAction),
		nullableString(session.CurrentOperationID), nullableString(session.CurrentTaskID), nullableString(session.CurrentExecutionGroupID),
		rawMessage(session.LastAgentState), rawMessage(session.ProviderStateBlob),
		session.Revision, session.CreatedAt, session.UpdatedAt)
	if err != nil {
		return err
	}
	return requireRowsAffected(result, domain.ErrNotFound)
}

func pendingActionType(action *domain.PendingAction) any {
	if action == nil {
		return nil
	}
	return string(action.Type)
}

func pendingActionPayload(action *domain.PendingAction) any {
	if action == nil {
		return nil
	}
	return rawMessage(action.Payload)
}

func scanSession(rows Rows) (domain.Session, error) {
	var (
		id, title, status                                             string
		activeTargetRaw, pendingPayloadRaw, lastAgentRaw, providerRaw []byte
		pendingType                                                   sql.NullString
		currentOperationID                                            sql.NullString
		currentTaskID                                                 sql.NullString
		currentExecutionGroupID                                       sql.NullString
		revision                                                      int64
		createdAt                                                     time.Time
		updatedAt                                                     time.Time
	)
	if err := rows.Scan(
		&id, &title, &status, &activeTargetRaw, &pendingType, &pendingPayloadRaw,
		&currentOperationID, &currentTaskID, &currentExecutionGroupID,
		&lastAgentRaw, &providerRaw, &revision, &createdAt, &updatedAt,
	); err != nil {
		return domain.Session{}, err
	}

	var activeTarget domain.ActiveTargetContext
	if len(activeTargetRaw) > 0 {
		if err := json.Unmarshal(activeTargetRaw, &activeTarget); err != nil {
			return domain.Session{}, fmt.Errorf("decode active_target_context: %w", err)
		}
	}
	session := domain.Session{
		ID:                  id,
		Title:               title,
		Status:              domain.SessionStatus(status),
		ActiveTargetContext: activeTarget,
		LastAgentState:      cloneBytes(lastAgentRaw),
		ProviderStateBlob:   cloneBytes(providerRaw),
		Revision:            revision,
		CreatedAt:           createdAt.UTC(),
		UpdatedAt:           updatedAt.UTC(),
	}
	if pendingType.Valid {
		session.PendingAction = &domain.PendingAction{
			Type:    domain.PendingActionType(pendingType.String),
			Payload: cloneBytes(pendingPayloadRaw),
		}
	}
	if currentOperationID.Valid {
		session.CurrentOperationID = stringPtr(currentOperationID.String)
	}
	if currentTaskID.Valid {
		session.CurrentTaskID = stringPtr(currentTaskID.String)
	}
	if currentExecutionGroupID.Valid {
		session.CurrentExecutionGroupID = stringPtr(currentExecutionGroupID.String)
	}
	return session, nil
}
