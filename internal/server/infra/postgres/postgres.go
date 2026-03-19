package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, nil
	}
	return pgxpool.New(ctx, dsn)
}

func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	return pool.Ping(ctx)
}
