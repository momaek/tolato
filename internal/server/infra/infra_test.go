package infra

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestFixedClockNow(t *testing.T) {
	ts := time.Date(2026, 3, 22, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	clock := FixedClock{Time: ts}

	if got := clock.Now(); !got.Equal(ts.UTC()) {
		t.Fatalf("Now() = %v, want %v", got, ts.UTC())
	}
}

func TestRandomIDGeneratorNewID(t *testing.T) {
	gen := RandomIDGenerator{}

	first := gen.NewID("sess")
	second := gen.NewID("sess")

	if !strings.HasPrefix(first, "sess_") {
		t.Fatalf("first ID = %q, want sess_ prefix", first)
	}
	if first == second {
		t.Fatalf("expected different IDs, got %q and %q", first, second)
	}
}

func TestNewLoggerWritesStructuredOutput(t *testing.T) {
	var buf bytes.Buffer

	logger := NewLogger(&buf, slog.LevelInfo)
	logger.InfoContext(context.Background(), "server started", "component", "test")

	if !strings.Contains(buf.String(), "server started") {
		t.Fatalf("log output = %q, want message", buf.String())
	}
}
