package postgres

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type authSessionRepository struct {
	q Queryer
}

func NewAuthSessionRepository(q Queryer) domain.AuthSessionRepository {
	return &authSessionRepository{q: q}
}

func (r *authSessionRepository) Put(ctx context.Context, session domain.AuthSession) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO auth_sessions (token, user_id, session_id, created_at, last_seen_at)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (token)
DO UPDATE SET
    user_id = EXCLUDED.user_id,
    session_id = EXCLUDED.session_id,
    created_at = EXCLUDED.created_at,
    last_seen_at = EXCLUDED.last_seen_at
`, session.Token, session.UserID, session.SessionID, session.CreatedAt, session.LastSeenAt)
	return err
}

func (r *authSessionRepository) GetByToken(ctx context.Context, token string) (domain.AuthSession, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT user_id, session_id, token, created_at, last_seen_at
FROM auth_sessions
WHERE token = $1
`, token)
	if err != nil {
		return domain.AuthSession{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return domain.AuthSession{}, err
		}
		return domain.AuthSession{}, domain.ErrNotFound
	}
	session, scanErr := scanAuthSession(rows)
	if scanErr != nil {
		return domain.AuthSession{}, scanErr
	}
	if err := rows.Err(); err != nil {
		return domain.AuthSession{}, err
	}
	return session, nil
}

func (r *authSessionRepository) ListByUser(ctx context.Context, userID string) ([]domain.AuthSession, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT user_id, session_id, token, created_at, last_seen_at
FROM auth_sessions
WHERE user_id = $1
ORDER BY created_at ASC, token ASC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.AuthSession, 0)
	for rows.Next() {
		item, scanErr := scanAuthSession(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *authSessionRepository) Touch(ctx context.Context, token string, lastSeenAt time.Time) error {
	result, err := r.q.ExecContext(ctx, `
UPDATE auth_sessions
SET last_seen_at = $2
WHERE token = $1
`, token, lastSeenAt)
	if err != nil {
		return err
	}
	return requireRowsAffected(result, domain.ErrNotFound)
}

func (r *authSessionRepository) DeleteByToken(ctx context.Context, token string) error {
	_, err := r.q.ExecContext(ctx, `
DELETE FROM auth_sessions
WHERE token = $1
`, token)
	return err
}

func (r *authSessionRepository) DeleteByUser(ctx context.Context, userID string) error {
	_, err := r.q.ExecContext(ctx, `
DELETE FROM auth_sessions
WHERE user_id = $1
`, userID)
	return err
}

func (r *authSessionRepository) DeleteByUserExceptSession(ctx context.Context, userID string, sessionID string) error {
	_, err := r.q.ExecContext(ctx, `
DELETE FROM auth_sessions
WHERE user_id = $1 AND session_id <> $2
`, userID, sessionID)
	return err
}

func scanAuthSession(rows Rows) (domain.AuthSession, error) {
	var (
		userID, sessionID, token string
		createdAt                time.Time
		lastSeenAt               time.Time
	)
	if err := rows.Scan(&userID, &sessionID, &token, &createdAt, &lastSeenAt); err != nil {
		return domain.AuthSession{}, err
	}
	return domain.AuthSession{
		UserID:     userID,
		SessionID:  sessionID,
		Token:      token,
		CreatedAt:  createdAt.UTC(),
		LastSeenAt: lastSeenAt.UTC(),
	}, nil
}
