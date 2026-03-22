package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type taskRepository struct {
	q Queryer
}

func NewTaskRepository(q Queryer) domain.TaskRepository {
	return &taskRepository{q: q}
}

func (r *taskRepository) Create(ctx context.Context, task domain.Task) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO tasks (
    id, session_id, input_text, operation_target_snapshot, status, approval_status, risk_level, summary, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
`, task.ID, task.SessionID, task.InputText, rawMessage(task.OperationTargetSnapshot), string(task.Status),
		string(task.ApprovalStatus), string(task.RiskLevel), nullableString(task.Summary), task.CreatedAt, task.UpdatedAt)
	return err
}

func (r *taskRepository) Get(ctx context.Context, taskID string) (domain.Task, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, session_id, input_text, operation_target_snapshot, status, approval_status, risk_level, summary, created_at, updated_at
FROM tasks
WHERE id = $1
`, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return domain.Task{}, err
		}
		return domain.Task{}, domain.ErrNotFound
	}
	task, scanErr := scanTask(rows)
	if scanErr != nil {
		return domain.Task{}, scanErr
	}
	if err := rows.Err(); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (r *taskRepository) ListBySession(ctx context.Context, sessionID string) ([]domain.Task, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, session_id, input_text, operation_target_snapshot, status, approval_status, risk_level, summary, created_at, updated_at
FROM tasks
WHERE session_id = $1
ORDER BY created_at DESC, id DESC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.Task, 0)
	for rows.Next() {
		item, scanErr := scanTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *taskRepository) Update(ctx context.Context, task domain.Task) error {
	result, err := r.q.ExecContext(ctx, `
UPDATE tasks SET
    session_id = $2,
    input_text = $3,
    operation_target_snapshot = $4,
    status = $5,
    approval_status = $6,
    risk_level = $7,
    summary = $8,
    created_at = $9,
    updated_at = $10
WHERE id = $1
`, task.ID, task.SessionID, task.InputText, rawMessage(task.OperationTargetSnapshot), string(task.Status),
		string(task.ApprovalStatus), string(task.RiskLevel), nullableString(task.Summary), task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return err
	}
	return requireRowsAffected(result, domain.ErrNotFound)
}

func scanTask(rows Rows) (domain.Task, error) {
	var (
		id, sessionID, inputText, status, approvalStatus, riskLevel string
		summary                                                     sql.NullString
		snapshotRaw                                                 []byte
		createdAt                                                   time.Time
		updatedAt                                                   time.Time
	)
	if err := rows.Scan(&id, &sessionID, &inputText, &snapshotRaw, &status, &approvalStatus, &riskLevel, &summary, &createdAt, &updatedAt); err != nil {
		return domain.Task{}, err
	}
	var snapshot domain.TargetSnapshot
	if len(snapshotRaw) > 0 {
		if err := json.Unmarshal(snapshotRaw, &snapshot); err != nil {
			return domain.Task{}, fmt.Errorf("decode operation_target_snapshot: %w", err)
		}
	}
	return domain.Task{
		ID:                      id,
		SessionID:               sessionID,
		InputText:               inputText,
		OperationTargetSnapshot: snapshot,
		Status:                  domain.TaskStatus(status),
		ApprovalStatus:          domain.ApprovalStatus(approvalStatus),
		RiskLevel:               domain.RiskLevel(riskLevel),
		Summary:                 nullStringPtr(summary),
		CreatedAt:               createdAt.UTC(),
		UpdatedAt:               updatedAt.UTC(),
	}, nil
}
