package supervisor

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/agent/executor/runner"
	"go.uber.org/zap"
)

type Loop struct {
	Logger *zap.Logger
	Queue  <-chan runner.Job
}

func (l Loop) Run(ctx context.Context) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			l.Logger.Info("agent supervisor heartbeat", zap.Int("queue_depth", len(l.Queue)))
		}
	}
}
