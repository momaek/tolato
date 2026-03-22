package postgres

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (Rows, error)
}

type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

type SQLDB struct {
	DB *sql.DB
}

func Open(dsn string) (*sql.DB, error) {
	return sql.Open("pgx", dsn)
}

func (d SQLDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.DB.ExecContext(ctx, query, args...)
}

func (d SQLDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := d.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{Rows: rows}, nil
}

type sqlRows struct {
	Rows *sql.Rows
}

func (r *sqlRows) Next() bool             { return r.Rows.Next() }
func (r *sqlRows) Scan(dest ...any) error { return r.Rows.Scan(dest...) }
func (r *sqlRows) Close() error           { return r.Rows.Close() }
func (r *sqlRows) Err() error             { return r.Rows.Err() }
