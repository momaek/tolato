package probe

import (
	"context"
	"log"
)

// Probe is the top-level entry point for the probe subsystem.
type Probe struct {
	Config ProbeConfig
	Logger *log.Logger
}

// Run starts the probe scheduler. It blocks until ctx is cancelled.
func (p *Probe) Run(ctx context.Context) error {
	reporter := NewReporter(p.Config.ServerURL, p.Config.AuthToken, p.Config.NodeID, p.Logger)
	scheduler := NewScheduler(p.Config, reporter, p.Logger)

	p.Logger.Printf("probe started: node=%s targets=%d server=%s",
		p.Config.NodeID, len(p.Config.Targets), p.Config.ServerURL)

	return scheduler.Run(ctx)
}
