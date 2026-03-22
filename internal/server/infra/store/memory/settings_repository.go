package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/momaek/tolato/internal/server/domain"
)

type settingsRepository struct {
	mu    sync.RWMutex
	items map[string]domain.SettingRecord
	order []string
}

func NewSettingsRepository() domain.SettingsRepository {
	return &settingsRepository{
		items: make(map[string]domain.SettingRecord),
	}
}

func settingsKey(userID string, key domain.SettingKey) string {
	return fmt.Sprintf("%s:%s", userID, key)
}

func (r *settingsRepository) Put(ctx context.Context, record domain.SettingRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := settingsKey(record.UserID, record.Key)
	if _, exists := r.items[key]; !exists {
		r.order = append(r.order, key)
	}
	r.items[key] = cloneSettingRecord(record)
	return nil
}

func (r *settingsRepository) Get(ctx context.Context, userID string, key domain.SettingKey) (domain.SettingRecord, error) {
	if err := ctx.Err(); err != nil {
		return domain.SettingRecord{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	record, ok := r.items[settingsKey(userID, key)]
	if !ok {
		return domain.SettingRecord{}, domain.ErrNotFound
	}

	return cloneSettingRecord(record), nil
}

func (r *settingsRepository) ListByUser(ctx context.Context, userID string) ([]domain.SettingRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domain.SettingRecord, 0, len(r.order))
	prefix := userID + ":"
	for _, key := range r.order {
		if len(key) < len(prefix) || key[:len(prefix)] != prefix {
			continue
		}
		record, ok := r.items[key]
		if !ok {
			continue
		}
		out = append(out, cloneSettingRecord(record))
	}
	return out, nil
}
