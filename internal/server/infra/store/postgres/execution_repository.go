package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type executionRepository struct {
	q Queryer
}

func NewExecutionRepository(q Queryer) domain.ExecutionRepository {
	return &executionRepository{q: q}
}

func (r *executionRepository) Create(ctx context.Context, execution domain.Execution) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO executions (
    id, task_id, session_id, node_id, status, started_at, finished_at, exit_code, stdout_tail, stderr_tail, status_reason, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
`, execution.ID, execution.TaskID, execution.SessionID, execution.NodeID, string(execution.Status), nullableTime(execution.StartedAt),
		nullableTime(execution.FinishedAt), nullableInt(execution.ExitCode), execution.StdoutTail, execution.StderrTail, execution.StatusReason,
		execution.CreatedAt, execution.UpdatedAt)
	return err
}

func (r *executionRepository) Get(ctx context.Context, executionID string) (domain.Execution, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, task_id, session_id, node_id, status, started_at, finished_at, exit_code, stdout_tail, stderr_tail, status_reason, created_at, updated_at
FROM executions
WHERE id = $1
`, executionID)
	if err != nil {
		return domain.Execution{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return domain.Execution{}, err
		}
		return domain.Execution{}, domain.ErrNotFound
	}
	execution, scanErr := scanExecution(rows)
	if scanErr != nil {
		return domain.Execution{}, scanErr
	}
	if err := rows.Err(); err != nil {
		return domain.Execution{}, err
	}
	return execution, nil
}

func (r *executionRepository) ListByTask(ctx context.Context, taskID string) ([]domain.Execution, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, task_id, session_id, node_id, status, started_at, finished_at, exit_code, stdout_tail, stderr_tail, status_reason, created_at, updated_at
FROM executions
WHERE task_id = $1
ORDER BY created_at ASC, id ASC
`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.Execution, 0)
	for rows.Next() {
		item, scanErr := scanExecution(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *executionRepository) Update(ctx context.Context, execution domain.Execution) error {
	result, err := r.q.ExecContext(ctx, `
UPDATE executions SET
    task_id = $2,
    session_id = $3,
    node_id = $4,
    status = $5,
    started_at = $6,
    finished_at = $7,
    exit_code = $8,
    stdout_tail = $9,
    stderr_tail = $10,
    status_reason = $11,
    created_at = $12,
    updated_at = $13
WHERE id = $1
`, execution.ID, execution.TaskID, execution.SessionID, execution.NodeID, string(execution.Status), nullableTime(execution.StartedAt),
		nullableTime(execution.FinishedAt), nullableInt(execution.ExitCode), execution.StdoutTail, execution.StderrTail, execution.StatusReason,
		execution.CreatedAt, execution.UpdatedAt)
	if err != nil {
		return err
	}
	return requireRowsAffected(result, domain.ErrNotFound)
}

func (r *executionRepository) AggregateByTask(ctx context.Context, taskID string) (domain.ExecutionAggregate, error) {
	items, err := r.ListByTask(ctx, taskID)
	if err != nil {
		return domain.ExecutionAggregate{}, err
	}
	var aggregate domain.ExecutionAggregate
	for _, execution := range items {
		aggregate.Total++
		switch execution.Status {
		case domain.ExecutionStatusQueued:
			aggregate.Queued++
		case domain.ExecutionStatusDispatched:
			aggregate.Dispatched++
		case domain.ExecutionStatusRunning:
			aggregate.Running++
		case domain.ExecutionStatusSuccess:
			aggregate.Success++
		case domain.ExecutionStatusFailed:
			aggregate.Failed++
		case domain.ExecutionStatusTimeout:
			aggregate.Timeout++
		case domain.ExecutionStatusCancelled:
			aggregate.Cancelled++
		}
	}
	return aggregate, nil
}

func scanExecution(rows Rows) (domain.Execution, error) {
	var (
		id, taskID, sessionID, nodeID, status, stdoutTail, stderrTail string
		startedAt                                                     sql.NullTime
		finishedAt                                                    sql.NullTime
		exitCode                                                      sql.NullInt64
		statusReason                                                  sql.NullString
		createdAt                                                     time.Time
		updatedAt                                                     time.Time
	)
	if err := rows.Scan(&id, &taskID, &sessionID, &nodeID, &status, &startedAt, &finishedAt, &exitCode, &stdoutTail, &stderrTail, &statusReason, &createdAt, &updatedAt); err != nil {
		return domain.Execution{}, err
	}
	execution := domain.Execution{
		ID:           id,
		TaskID:       taskID,
		SessionID:    sessionID,
		NodeID:       nodeID,
		Status:       domain.ExecutionStatus(status),
		StartedAt:    nullTimePtr(startedAt),
		FinishedAt:   nullTimePtr(finishedAt),
		StdoutTail:   stdoutTail,
		StderrTail:   stderrTail,
		StatusReason: nullStringPtr(statusReason),
		CreatedAt:    createdAt.UTC(),
		UpdatedAt:    updatedAt.UTC(),
	}
	if exitCode.Valid {
		execution.ExitCode = intPtr(int(exitCode.Int64))
	}
	return execution, nil
}
