package auth

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/momaek/tolato/internal/shared/types"
	goredis "github.com/redis/go-redis/v9"
)

type MemorySessionStore struct {
	mu    sync.RWMutex
	items map[string]sessionEntry
}

type sessionEntry struct {
	User      types.CurrentUser `json:"user"`
	ExpiresAt time.Time         `json:"expires_at"`
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		items: make(map[string]sessionEntry),
	}
}

func (s *MemorySessionStore) Store(ctx context.Context, token string, user types.CurrentUser, expiresAt time.Time) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[token] = sessionEntry{User: user, ExpiresAt: expiresAt.UTC()}
	return nil
}

func (s *MemorySessionStore) Load(ctx context.Context, token string) (types.CurrentUser, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.items[token]
	if !ok || time.Now().UTC().After(item.ExpiresAt) {
		return types.CurrentUser{}, errors.New("unauthorized")
	}
	return item.User, nil
}

type RedisSessionStore struct {
	client *goredis.Client
	prefix string
}

func NewRedisSessionStore(client *goredis.Client, prefix string) *RedisSessionStore {
	if prefix == "" {
		prefix = "tolato:auth:session:"
	}
	return &RedisSessionStore{client: client, prefix: prefix}
}

func (s *RedisSessionStore) Store(ctx context.Context, token string, user types.CurrentUser, expiresAt time.Time) error {
	if s.client == nil {
		return errors.New("redis session store is not configured")
	}
	payload, err := json.Marshal(sessionEntry{User: user, ExpiresAt: expiresAt.UTC()})
	if err != nil {
		return err
	}
	ttl := time.Until(expiresAt.UTC())
	if ttl <= 0 {
		ttl = time.Minute
	}
	return s.client.Set(ctx, s.prefix+token, payload, ttl).Err()
}

func (s *RedisSessionStore) Load(ctx context.Context, token string) (types.CurrentUser, error) {
	if s.client == nil {
		return types.CurrentUser{}, errors.New("unauthorized")
	}
	raw, err := s.client.Get(ctx, s.prefix+token).Bytes()
	if err != nil {
		return types.CurrentUser{}, errors.New("unauthorized")
	}

	var item sessionEntry
	if err := json.Unmarshal(raw, &item); err != nil {
		return types.CurrentUser{}, errors.New("unauthorized")
	}
	if time.Now().UTC().After(item.ExpiresAt) {
		return types.CurrentUser{}, errors.New("unauthorized")
	}
	return item.User, nil
}
