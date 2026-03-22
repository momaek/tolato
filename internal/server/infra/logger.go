package infra

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/momaek/tolato/internal/server/domain"
)

type SlogLogger struct {
	logger *slog.Logger
}

func NewLogger(w io.Writer, level slog.Level) SlogLogger {
	if w == nil {
		w = os.Stdout
	}

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return SlogLogger{logger: slog.New(handler)}
}

var _ domain.Logger = SlogLogger{}

func (l SlogLogger) With(args ...any) domain.Logger {
	return SlogLogger{logger: l.logger.With(args...)}
}

func (l SlogLogger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.logger.DebugContext(ctx, msg, args...)
}

func (l SlogLogger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.logger.InfoContext(ctx, msg, args...)
}

func (l SlogLogger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.logger.WarnContext(ctx, msg, args...)
}

func (l SlogLogger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.logger.ErrorContext(ctx, msg, args...)
}

func (l SlogLogger) LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	l.logger.LogAttrs(ctx, level, msg, attrs...)
}
