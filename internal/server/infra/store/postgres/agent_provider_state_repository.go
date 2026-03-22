package postgres

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type agentProviderStateRepository struct {
	q Queryer
}

func NewAgentProviderStateRepository(q Queryer) domain.AgentProviderStateRepository {
	return &agentProviderStateRepository{q: q}
}

func (r *agentProviderStateRepository) Append(ctx context.Context, state domain.AgentProviderState) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO agent_provider_state (id, session_id, version, payload, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6)
`, state.ID, state.SessionID, state.Version, rawMessage(state.Payload), state.CreatedAt, state.CreatedAt)
	return err
}

func (r *agentProviderStateRepository) ListBySession(ctx context.Context, sessionID string) ([]domain.AgentProviderState, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, session_id, version, payload, created_at
FROM agent_provider_state
WHERE session_id = $1
ORDER BY version ASC, created_at ASC, id ASC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.AgentProviderState, 0)
	for rows.Next() {
		item, scanErr := scanAgentProviderState(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *agentProviderStateRepository) LatestBySession(ctx context.Context, sessionID string) (domain.AgentProviderState, error) {
	items, err := r.ListBySession(ctx, sessionID)
	if err != nil {
		return domain.AgentProviderState{}, err
	}
	if len(items) == 0 {
		return domain.AgentProviderState{}, domain.ErrNotFound
	}
	return items[len(items)-1], nil
}

func scanAgentProviderState(rows Rows) (domain.AgentProviderState, error) {
	var (
		id, sessionID string
		version       int64
		payloadRaw    []byte
		createdAt     time.Time
	)
	if err := rows.Scan(&id, &sessionID, &version, &payloadRaw, &createdAt); err != nil {
		return domain.AgentProviderState{}, err
	}
	return domain.AgentProviderState{
		ID:        id,
		SessionID: sessionID,
		Version:   version,
		Payload:   cloneBytes(payloadRaw),
		CreatedAt: createdAt.UTC(),
	}, nil
}
