package postgres

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type settingsRepository struct {
	q Queryer
}

func NewSettingsRepository(q Queryer) domain.SettingsRepository {
	return &settingsRepository{q: q}
}

func (r *settingsRepository) Put(ctx context.Context, record domain.SettingRecord) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO settings (user_id, key, value, updated_at)
VALUES ($1,$2,$3,$4)
ON CONFLICT (user_id, key)
DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at
`, record.UserID, string(record.Key), rawMessage(record.Value), record.UpdatedAt)
	return err
}

func (r *settingsRepository) Get(ctx context.Context, userID string, key domain.SettingKey) (domain.SettingRecord, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT user_id, key, value, updated_at
FROM settings
WHERE user_id = $1 AND key = $2
`, userID, string(key))
	if err != nil {
		return domain.SettingRecord{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return domain.SettingRecord{}, err
		}
		return domain.SettingRecord{}, domain.ErrNotFound
	}
	record, scanErr := scanSettingRecord(rows)
	if scanErr != nil {
		return domain.SettingRecord{}, scanErr
	}
	if err := rows.Err(); err != nil {
		return domain.SettingRecord{}, err
	}
	return record, nil
}

func (r *settingsRepository) ListByUser(ctx context.Context, userID string) ([]domain.SettingRecord, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT user_id, key, value, updated_at
FROM settings
WHERE user_id = $1
ORDER BY key ASC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.SettingRecord, 0)
	for rows.Next() {
		item, scanErr := scanSettingRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanSettingRecord(rows Rows) (domain.SettingRecord, error) {
	var (
		userID, key string
		valueRaw    []byte
		updatedAt   time.Time
	)
	if err := rows.Scan(&userID, &key, &valueRaw, &updatedAt); err != nil {
		return domain.SettingRecord{}, err
	}
	return domain.SettingRecord{
		UserID:    userID,
		Key:       domain.SettingKey(key),
		Value:     cloneBytes(valueRaw),
		UpdatedAt: updatedAt.UTC(),
	}, nil
}
