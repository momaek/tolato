package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/momaek/tolato/internal/server/domain/audit"
	"github.com/momaek/tolato/internal/server/domain/node"
	"github.com/momaek/tolato/internal/server/domain/outbox"
	"github.com/momaek/tolato/internal/server/domain/task"
)

type NodeRepo struct{ pool *pgxpool.Pool }
type SessionRepo struct{ pool *pgxpool.Pool }
type TaskRepo struct{ pool *pgxpool.Pool }
type AuditRepo struct{ pool *pgxpool.Pool }
type OutboxRepo struct{ pool *pgxpool.Pool }

func NewStores(pool *pgxpool.Pool) (*NodeRepo, *SessionRepo, *TaskRepo, *AuditRepo, *OutboxRepo) {
	return &NodeRepo{pool: pool}, &SessionRepo{pool: pool}, &TaskRepo{pool: pool}, &AuditRepo{pool: pool}, &OutboxRepo{pool: pool}
}

func (r *NodeRepo) List(ctx context.Context) ([]node.Node, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, hostname, region, os, version, tags, status, COALESCE(last_seen_at, NOW()),
		       auth_secret_version, agent_secret, created_at, updated_at
		FROM nodes
		ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]node.Node, 0)
	for rows.Next() {
		item, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *NodeRepo) Get(ctx context.Context, id string) (*node.Node, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, hostname, region, os, version, tags, status, COALESCE(last_seen_at, NOW()),
		       auth_secret_version, agent_secret, created_at, updated_at
		FROM nodes
		WHERE id = $1`, id)

	item, err := scanNode(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *NodeRepo) GetByAgentCredentials(ctx context.Context, id, secret string) (*node.Node, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, hostname, region, os, version, tags, status, COALESCE(last_seen_at, NOW()),
		       auth_secret_version, agent_secret, created_at, updated_at
		FROM nodes
		WHERE id = $1 AND agent_secret = $2`, id, secret)

	item, err := scanNode(row)
	if err != nil {
		return nil, errors.New("agent authentication failed")
	}
	return &item, nil
}

func (r *NodeRepo) Upsert(ctx context.Context, n node.Node) error {
	tags, err := json.Marshal(n.Tags)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO nodes (
			id, hostname, region, os, version, tags, status, last_seen_at,
			auth_secret_version, agent_secret, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			region = EXCLUDED.region,
			os = EXCLUDED.os,
			version = EXCLUDED.version,
			tags = EXCLUDED.tags,
			status = EXCLUDED.status,
			last_seen_at = EXCLUDED.last_seen_at,
			auth_secret_version = EXCLUDED.auth_secret_version,
			agent_secret = EXCLUDED.agent_secret,
			updated_at = EXCLUDED.updated_at`,
		n.ID, n.Hostname, n.Region, n.OS, n.Version, tags, n.Status, nullableTime(n.LastSeenAt),
		n.AuthSecretVersion, n.AgentSecret, zeroToNow(n.CreatedAt), zeroToNow(n.UpdatedAt),
	)
	return err
}

func (r *NodeRepo) UpdatePresence(ctx context.Context, nodeID, version, status string, seenAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE nodes
		SET version = COALESCE(NULLIF($2, ''), version),
			status = $3,
			last_seen_at = $4,
			updated_at = $4
		WHERE id = $1`, nodeID, version, status, seenAt)
	return err
}

func (r *SessionRepo) Upsert(ctx context.Context, session node.NodeSession) error {
	capabilities, err := json.Marshal(session.Capabilities)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO node_sessions (
			session_id, node_id, connected_at, last_heartbeat_at, disconnected_at, remote_addr, status, capabilities
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (session_id) DO UPDATE SET
			last_heartbeat_at = EXCLUDED.last_heartbeat_at,
			disconnected_at = EXCLUDED.disconnected_at,
			remote_addr = EXCLUDED.remote_addr,
			status = EXCLUDED.status,
			capabilities = EXCLUDED.capabilities`,
		session.SessionID, session.NodeID, zeroToNow(session.ConnectedAt), zeroToNow(session.LastHeartbeatAt),
		nullableTime(session.DisconnectedAt), session.RemoteAddr, session.Status, capabilities,
	)
	return err
}

func (r *SessionRepo) Get(ctx context.Context, sessionID string) (*node.NodeSession, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT session_id, node_id, connected_at, last_heartbeat_at, disconnected_at, remote_addr, status, capabilities
		FROM node_sessions
		WHERE session_id = $1`, sessionID)

	item, err := scanSession(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *SessionRepo) ListByNodeID(ctx context.Context, nodeID string) ([]node.NodeSession, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT session_id, node_id, connected_at, last_heartbeat_at, disconnected_at, remote_addr, status, capabilities
		FROM node_sessions
		WHERE node_id = $1
		ORDER BY connected_at ASC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]node.NodeSession, 0)
	for rows.Next() {
		item, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SessionRepo) MarkDisconnected(ctx context.Context, sessionID string, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE node_sessions
		SET status = 'disconnected',
			disconnected_at = $2
		WHERE session_id = $1`, sessionID, at.UTC())
	return err
}

func (r *TaskRepo) Create(ctx context.Context, t task.Task) error {
	target, err := json.Marshal(t.Target)
	if err != nil {
		return err
	}
	planJSON, err := json.Marshal(t.Plan)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO tasks (
			id, parent_task_id, mode, initiator_id, target, input_text, plan_json,
			risk_level, approval_status, final_status, status_reason, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		t.ID, emptyToNil(t.ParentTaskID), t.Mode, t.InitiatorID, target, t.InputText, planJSON,
		t.RiskLevel, t.ApprovalStatus, t.FinalStatus, t.StatusReason, zeroToNow(t.CreatedAt), zeroToNow(t.UpdatedAt),
	)
	return err
}

func (r *TaskRepo) Get(ctx context.Context, id string) (*task.Task, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, COALESCE(parent_task_id, ''), mode, initiator_id, target, input_text, plan_json,
		       risk_level, approval_status, final_status, status_reason, created_at, updated_at
		FROM tasks
		WHERE id = $1`, id)

	item, err := scanTask(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *TaskRepo) List(ctx context.Context) ([]task.Task, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, COALESCE(parent_task_id, ''), mode, initiator_id, target, input_text, plan_json,
		       risk_level, approval_status, final_status, status_reason, created_at, updated_at
		FROM tasks
		ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]task.Task, 0)
	for rows.Next() {
		item, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *TaskRepo) Update(ctx context.Context, t task.Task) error {
	target, err := json.Marshal(t.Target)
	if err != nil {
		return err
	}
	planJSON, err := json.Marshal(t.Plan)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		UPDATE tasks
		SET parent_task_id = $2,
			mode = $3,
			initiator_id = $4,
			target = $5,
			input_text = $6,
			plan_json = $7,
			risk_level = $8,
			approval_status = $9,
			final_status = $10,
			status_reason = $11,
			updated_at = $12
		WHERE id = $1`,
		t.ID, emptyToNil(t.ParentTaskID), t.Mode, t.InitiatorID, target, t.InputText,
		planJSON, t.RiskLevel, t.ApprovalStatus, t.FinalStatus, t.StatusReason, zeroToNow(t.UpdatedAt),
	)
	return err
}

func (r *TaskRepo) ListExecutions(ctx context.Context, taskID string) ([]task.TaskExecution, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, task_id, node_id, status, attempt, started_at,
		       finished_at, COALESCE(exit_code, 0), stdout_tail, stderr_tail, status_reason
		FROM task_executions
		WHERE task_id = $1
		ORDER BY started_at ASC NULLS LAST, id ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]task.TaskExecution, 0)
	for rows.Next() {
		item, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *TaskRepo) UpsertExecution(ctx context.Context, execution task.TaskExecution) error {
	exitCode := any(nil)
	if execution.Status != "approved" && execution.Status != "queued" && execution.Status != "dispatched" && execution.Status != "running" {
		exitCode = execution.ExitCode
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO task_executions (
			id, task_id, node_id, status, attempt, started_at, finished_at, exit_code, stdout_tail, stderr_tail, status_reason
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			attempt = EXCLUDED.attempt,
			started_at = COALESCE(task_executions.started_at, EXCLUDED.started_at),
			finished_at = COALESCE(EXCLUDED.finished_at, task_executions.finished_at),
			exit_code = COALESCE(EXCLUDED.exit_code, task_executions.exit_code),
			stdout_tail = EXCLUDED.stdout_tail,
			stderr_tail = EXCLUDED.stderr_tail,
			status_reason = EXCLUDED.status_reason`,
		execution.ID, execution.TaskID, execution.NodeID, execution.Status, execution.Attempt,
		nullableTime(execution.StartedAt), nullableTime(execution.FinishedAt), exitCode,
		execution.StdoutTail, execution.StderrTail, execution.StatusReason,
	)
	return err
}

func (r *AuditRepo) Create(ctx context.Context, event audit.AuditEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO audit_events (id, task_id, actor_id, event_type, payload, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		event.ID, event.TaskID, event.ActorID, event.EventType, payload, zeroToNow(event.CreatedAt),
	)
	return err
}

func (r *AuditRepo) ListByTaskID(ctx context.Context, taskID string) ([]audit.AuditEvent, error) {
	var (
		rows pgxRows
		err  error
	)

	if taskID == "" {
		rows, err = r.pool.Query(ctx, `
			SELECT id, task_id, actor_id, event_type, payload, created_at
			FROM audit_events
			ORDER BY created_at ASC`)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, task_id, actor_id, event_type, payload, created_at
			FROM audit_events
			WHERE task_id = $1
			ORDER BY created_at ASC`, taskID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]audit.AuditEvent, 0)
	for rows.Next() {
		item, err := scanAudit(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *OutboxRepo) Create(ctx context.Context, message outbox.Message) error {
	payload, err := json.Marshal(message.Payload)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO task_outbox (id, topic, task_id, execution_id, node_id, payload, created_at, published_at, attempts)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		message.ID, message.Topic, message.TaskID, emptyToNil(message.ExecutionID), emptyToNil(message.NodeID),
		payload, zeroToNow(message.CreatedAt), nullableTime(message.PublishedAt), message.Attempts,
	)
	return err
}

func (r *OutboxRepo) ListPending(ctx context.Context, limit int) ([]outbox.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, topic, task_id, COALESCE(execution_id, ''), COALESCE(node_id, ''), payload, created_at, published_at, attempts
		FROM task_outbox
		WHERE published_at IS NULL
		ORDER BY created_at ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]outbox.Message, 0)
	for rows.Next() {
		item, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *OutboxRepo) MarkPublished(ctx context.Context, id string, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE task_outbox
		SET published_at = $2
		WHERE id = $1`, id, at.UTC())
	return err
}

func (r *OutboxRepo) IncrementAttempts(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE task_outbox
		SET attempts = attempts + 1
		WHERE id = $1`, id)
	return err
}

type pgxScanner interface {
	Scan(dest ...any) error
}

type pgxRows interface {
	pgxScanner
	Next() bool
	Close()
	Err() error
}

func scanNode(scanner pgxScanner) (node.Node, error) {
	var (
		item    node.Node
		tagsRaw []byte
	)
	err := scanner.Scan(
		&item.ID, &item.Hostname, &item.Region, &item.OS, &item.Version, &tagsRaw,
		&item.Status, &item.LastSeenAt, &item.AuthSecretVersion, &item.AgentSecret,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return node.Node{}, err
	}
	if len(tagsRaw) > 0 {
		_ = json.Unmarshal(tagsRaw, &item.Tags)
	}
	return item, nil
}

func scanSession(scanner pgxScanner) (node.NodeSession, error) {
	var (
		item            node.NodeSession
		disconnectedAt  *time.Time
		capabilitiesRaw []byte
	)
	err := scanner.Scan(
		&item.SessionID, &item.NodeID, &item.ConnectedAt, &item.LastHeartbeatAt, &disconnectedAt,
		&item.RemoteAddr, &item.Status, &capabilitiesRaw,
	)
	if err != nil {
		return node.NodeSession{}, err
	}
	if disconnectedAt != nil {
		item.DisconnectedAt = disconnectedAt.UTC()
	}
	if len(capabilitiesRaw) > 0 {
		_ = json.Unmarshal(capabilitiesRaw, &item.Capabilities)
	}
	return item, nil
}

func scanTask(scanner pgxScanner) (task.Task, error) {
	var (
		item    task.Task
		target  []byte
		planRaw []byte
	)
	err := scanner.Scan(
		&item.ID, &item.ParentTaskID, &item.Mode, &item.InitiatorID, &target, &item.InputText, &planRaw,
		&item.RiskLevel, &item.ApprovalStatus, &item.FinalStatus, &item.StatusReason, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return task.Task{}, err
	}
	if len(target) > 0 {
		_ = json.Unmarshal(target, &item.Target)
	}
	if len(planRaw) > 0 {
		_ = json.Unmarshal(planRaw, &item.Plan)
	}
	return item, nil
}

func scanExecution(scanner pgxScanner) (task.TaskExecution, error) {
	var (
		item       task.TaskExecution
		startedAt  *time.Time
		finishedAt *time.Time
	)
	err := scanner.Scan(
		&item.ID, &item.TaskID, &item.NodeID, &item.Status, &item.Attempt,
		&startedAt, &finishedAt, &item.ExitCode, &item.StdoutTail, &item.StderrTail, &item.StatusReason,
	)
	if startedAt != nil {
		item.StartedAt = startedAt.UTC()
	}
	if finishedAt != nil {
		item.FinishedAt = finishedAt.UTC()
	}
	return item, err
}

func scanAudit(scanner pgxScanner) (audit.AuditEvent, error) {
	var (
		item       audit.AuditEvent
		payloadRaw []byte
	)
	err := scanner.Scan(&item.ID, &item.TaskID, &item.ActorID, &item.EventType, &payloadRaw, &item.CreatedAt)
	if err != nil {
		return audit.AuditEvent{}, err
	}
	if len(payloadRaw) > 0 {
		_ = json.Unmarshal(payloadRaw, &item.Payload)
	}
	return item, nil
}

func scanOutbox(scanner pgxScanner) (outbox.Message, error) {
	var (
		item        outbox.Message
		payloadRaw  []byte
		publishedAt *time.Time
	)
	err := scanner.Scan(
		&item.ID, &item.Topic, &item.TaskID, &item.ExecutionID, &item.NodeID,
		&payloadRaw, &item.CreatedAt, &publishedAt, &item.Attempts,
	)
	if err != nil {
		return outbox.Message{}, err
	}
	if publishedAt != nil {
		item.PublishedAt = publishedAt.UTC()
	}
	if len(payloadRaw) > 0 {
		_ = json.Unmarshal(payloadRaw, &item.Payload)
	}
	return item, nil
}

func zeroToNow(v time.Time) time.Time {
	if v.IsZero() {
		return time.Now().UTC()
	}
	return v.UTC()
}

func nullableTime(v time.Time) any {
	if v.IsZero() {
		return nil
	}
	return v.UTC()
}

func emptyToNil(v string) any {
	if v == "" {
		return nil
	}
	return v
}
