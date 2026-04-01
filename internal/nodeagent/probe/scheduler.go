package probe

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/momaek/tolato/internal/nodeprobe/model"
)

const (
	fastInterval = 30 * time.Second
	slowInterval = 5 * time.Minute
)

// Scheduler runs probe tasks on intervals.
type Scheduler struct {
	config   ProbeConfig
	reporter *Reporter
	logger   *log.Logger
}

// NewScheduler creates a Scheduler.
func NewScheduler(cfg ProbeConfig, reporter *Reporter, logger *log.Logger) *Scheduler {
	return &Scheduler{
		config:   cfg,
		reporter: reporter,
		logger:   logger,
	}
}

// Run blocks until ctx is cancelled, executing probes on schedule.
func (s *Scheduler) Run(ctx context.Context) error {
	// Run probes immediately on start
	s.runFastProbes(ctx)
	s.runSlowProbes(ctx)

	fastTicker := time.NewTicker(fastInterval)
	slowTicker := time.NewTicker(slowInterval)
	defer fastTicker.Stop()
	defer slowTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-fastTicker.C:
			s.runFastProbes(ctx)
		case <-slowTicker.C:
			s.runSlowProbes(ctx)
		}
	}
}

// runFastProbes executes ping + TCP connect for all targets concurrently.
func (s *Scheduler) runFastProbes(ctx context.Context) {
	var (
		mu      sync.Mutex
		metrics []model.TargetMetric
		wg      sync.WaitGroup
	)

	for _, target := range s.config.Targets {
		wg.Add(1)
		go func(t TargetConfig) {
			defer wg.Done()

			pingRes := Ping(ctx, t.Host, t.PingCount)
			if pingRes.Err != nil {
				s.logger.Printf("ping %s (%s) error: %v", t.Name, t.Host, pingRes.Err)
			}

			tcpRes := TCPConnect(ctx, t.Host, t.TCPPort)
			if tcpRes.Err != nil {
				s.logger.Printf("tcp %s (%s:%d) error: %v", t.Name, t.Host, t.TCPPort, tcpRes.Err)
			}

			m := model.TargetMetric{
				TargetID:       t.ID,
				LatencyMin:     pingRes.Min,
				LatencyAvg:     pingRes.Avg,
				LatencyMax:     pingRes.Max,
				PacketLoss:     pingRes.PacketLoss,
				TCPConnectTime: tcpRes.ConnectTime,
			}

			mu.Lock()
			metrics = append(metrics, m)
			mu.Unlock()
		}(target)
	}

	wg.Wait()

	if len(metrics) > 0 {
		if err := s.reporter.Report(ctx, metrics); err != nil {
			s.logger.Printf("report fast metrics error: %v", err)
		}
	}
}

// runSlowProbes executes bandwidth tests for targets that have a bandwidth URL.
func (s *Scheduler) runSlowProbes(ctx context.Context) {
	var (
		mu      sync.Mutex
		metrics []model.TargetMetric
		wg      sync.WaitGroup
	)

	for _, target := range s.config.Targets {
		if target.BandwidthURL == "" {
			continue
		}
		wg.Add(1)
		go func(t TargetConfig) {
			defer wg.Done()

			bwRes := MeasureBandwidth(ctx, t.BandwidthURL)
			if bwRes.Err != nil {
				s.logger.Printf("bandwidth %s (%s) error: %v", t.Name, t.BandwidthURL, bwRes.Err)
				return
			}

			m := model.TargetMetric{
				TargetID:      t.ID,
				BandwidthMbps: &bwRes.Mbps,
			}

			mu.Lock()
			metrics = append(metrics, m)
			mu.Unlock()
		}(target)
	}

	wg.Wait()

	if len(metrics) > 0 {
		if err := s.reporter.Report(ctx, metrics); err != nil {
			s.logger.Printf("report bandwidth metrics error: %v", err)
		}
	}
}
