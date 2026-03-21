package queue

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/momaek/tolato/internal/shared/types"
	goredis "github.com/redis/go-redis/v9"
)

type Stream struct {
	client *goredis.Client
	key    string

	mu     sync.Mutex
	buffer []types.OutboxMessage
}

func NewStream(client *goredis.Client, key string) *Stream {
	if key == "" {
		key = "tolato:task-queue"
	}
	return &Stream{client: client, key: key, buffer: make([]types.OutboxMessage, 0)}
}

func (s *Stream) Publish(ctx context.Context, message types.OutboxMessage) error {
	if s.client == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.buffer = append(s.buffer, message)
		return nil
	}

	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return s.client.XAdd(ctx, &goredis.XAddArgs{
		Stream: s.key,
		Values: map[string]any{"payload": string(raw)},
	}).Err()
}

func (s *Stream) Consume(ctx context.Context, lastID string, count int, block time.Duration) ([]types.OutboxMessage, string, error) {
	if s.client == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.buffer) == 0 {
			select {
			case <-ctx.Done():
				return nil, lastID, ctx.Err()
			case <-time.After(block):
				return nil, lastID, nil
			}
		}
		item := s.buffer[0]
		s.buffer = s.buffer[1:]
		return []types.OutboxMessage{item}, lastID, nil
	}

	if lastID == "" {
		lastID = "$"
	}
	streams, err := s.client.XRead(ctx, &goredis.XReadArgs{
		Streams: []string{s.key, lastID},
		Block:   block,
		Count:   int64(count),
	}).Result()
	if errors.Is(err, goredis.Nil) {
		return nil, lastID, nil
	}
	if err != nil {
		return nil, lastID, err
	}

	items := make([]types.OutboxMessage, 0)
	nextID := lastID
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			payload, _ := msg.Values["payload"].(string)
			if payload == "" {
				continue
			}
			var item types.OutboxMessage
			if err := json.Unmarshal([]byte(payload), &item); err != nil {
				continue
			}
			items = append(items, item)
			nextID = msg.ID
		}
	}
	return items, nextID, nil
}
