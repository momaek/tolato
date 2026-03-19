package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

func NewClient(addr string, db int) *goredis.Client {
	if addr == "" {
		return nil
	}
	return goredis.NewClient(&goredis.Options{
		Addr: addr,
		DB:   db,
	})
}

func Ping(ctx context.Context, client *goredis.Client) error {
	if client == nil {
		return nil
	}
	return client.Ping(ctx).Err()
}
