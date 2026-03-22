package domain

import (
	"context"
	"log/slog"
	"time"
)

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID(prefix string) string
}

type UnlockFunc func()

type LockManager interface {
	LockSession(ctx context.Context, sessionID string) (UnlockFunc, error)
}

type Logger interface {
	With(args ...any) Logger
	DebugContext(ctx context.Context, msg string, args ...any)
	InfoContext(ctx context.Context, msg string, args ...any)
	WarnContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
	LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr)
}
