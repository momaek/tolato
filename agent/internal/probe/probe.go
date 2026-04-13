package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProbeConfig is the configuration received from the server.
type ProbeConfig struct {
	Enabled   bool           `json:"enabled"`
	ReportURL string         `json:"report_url"`
	Targets   []TargetConfig `json:"targets"`
}

// TargetConfig defines a single probe target.
type TargetConfig struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Host         string  `json:"host"`
	PingCount    int     `json:"ping_count"`
	TCPPort      int     `json:"tcp_port"`
	BandwidthURL *string `json:"bandwidth_url,omitempty"`
}

// MetricReport is the payload sent to the server.
type MetricReport struct {
	NodeID    string         `json:"node_id"`
	Timestamp string         `json:"timestamp"`
	Metrics   []TargetMetric `json:"metrics"`
}

// TargetMetric is per-target probe results.
type TargetMetric struct {
	TargetID       string   `json:"target_id"`
	LatencyMin     *float64 `json:"latency_min,omitempty"`
	LatencyAvg     *float64 `json:"latency_avg,omitempty"`
	LatencyMax     *float64 `json:"latency_max,omitempty"`
	PacketLoss     *float64 `json:"packet_loss,omitempty"`
	TCPConnectTime *float64 `json:"tcp_connect_time,omitempty"`
	BandwidthMbps  *float64 `json:"bandwidth_mbps,omitempty"`
}

// Scheduler manages periodic probe execution and reporting.
type Scheduler struct {
	nodeID    string
	secret    string // agent secret for authentication
	config    ProbeConfig
	client    *http.Client
	mu        sync.Mutex
	cancel    context.CancelFunc
	running   bool
}

// NewScheduler creates a new Scheduler.
func NewScheduler(nodeID, secret string) *Scheduler {
	return &Scheduler{
		nodeID: nodeID,
		secret: secret,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// UpdateConfig updates the probe configuration and restarts scheduling.
func (s *Scheduler) UpdateConfig(config ProbeConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop existing scheduler
	if s.cancel != nil {
		s.cancel()
	}

	s.config = config

	if !config.Enabled || len(config.Targets) == 0 {
		s.running = false
		log.Println("[probe] disabled or no targets")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.running = true

	go s.run(ctx)
	log.Printf("[probe] started with %d targets", len(config.Targets))
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
}

func (s *Scheduler) run(ctx context.Context) {
	// Immediate first probe
	s.probeAndReport()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	bandwidthTicker := time.NewTicker(5 * time.Minute)
	defer bandwidthTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.probeAndReport()
		case <-bandwidthTicker.C:
			s.probeBandwidth()
		}
	}
}

func (s *Scheduler) probeAndReport() {
	s.mu.Lock()
	config := s.config
	s.mu.Unlock()

	metrics := make([]TargetMetric, 0, len(config.Targets))
	for _, target := range config.Targets {
		m := s.probeTarget(target)
		metrics = append(metrics, m)
	}

	report := MetricReport{
		NodeID:    s.nodeID,
		Timestamp: time.Now().Format(time.RFC3339),
		Metrics:   metrics,
	}

	s.sendReport(config.ReportURL, report)
}

func (s *Scheduler) probeBandwidth() {
	s.mu.Lock()
	config := s.config
	s.mu.Unlock()

	for _, target := range config.Targets {
		if target.BandwidthURL == nil {
			continue
		}
		bw := measureBandwidth(*target.BandwidthURL)
		if bw != nil {
			report := MetricReport{
				NodeID:    s.nodeID,
				Timestamp: time.Now().Format(time.RFC3339),
				Metrics: []TargetMetric{{
					TargetID:      target.ID,
					BandwidthMbps: bw,
				}},
			}
			s.sendReport(config.ReportURL, report)
		}
	}
}

func (s *Scheduler) probeTarget(target TargetConfig) TargetMetric {
	m := TargetMetric{TargetID: target.ID}

	// Ping
	pingCount := target.PingCount
	if pingCount <= 0 {
		pingCount = 10
	}
	min, avg, max, loss := ping(target.Host, pingCount)
	if avg != nil {
		m.LatencyMin = min
		m.LatencyAvg = avg
		m.LatencyMax = max
		m.PacketLoss = loss
	}

	// TCP connect
	if target.TCPPort > 0 {
		tcpTime := tcpConnect(target.Host, target.TCPPort)
		m.TCPConnectTime = tcpTime
	}

	return m
}

func (s *Scheduler) sendReport(reportURL string, report MetricReport) {
	data, err := json.Marshal(report)
	if err != nil {
		log.Printf("[probe] marshal report failed: %v", err)
		return
	}

	req, err := http.NewRequest("POST", reportURL, bytes.NewReader(data))
	if err != nil {
		log.Printf("[probe] create request failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s:%s", s.nodeID, s.secret))

	resp, err := s.client.Do(req)
	if err != nil {
		log.Printf("[probe] report failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[probe] report returned %d", resp.StatusCode)
	}
}

// ping runs the system ping command and parses the results.
func ping(host string, count int) (min, avg, max, loss *float64) {
	cmd := exec.Command("ping", "-c", strconv.Itoa(count), "-W", "5", host)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return nil, nil, nil, nil
	}

	output := string(out)

	// Parse packet loss
	if idx := strings.Index(output, "% packet loss"); idx >= 0 {
		// Find the number before "% packet loss"
		start := strings.LastIndex(output[:idx], " ")
		if start >= 0 {
			if v, err := strconv.ParseFloat(strings.TrimSpace(output[start:idx]), 64); err == nil {
				loss = &v
			}
		}
	}

	// Parse rtt min/avg/max
	if idx := strings.Index(output, "min/avg/max"); idx >= 0 {
		line := output[idx:]
		eqIdx := strings.Index(line, "= ")
		if eqIdx >= 0 {
			parts := strings.Split(strings.Split(line[eqIdx+2:], " ")[0], "/")
			if len(parts) >= 3 {
				if v, err := strconv.ParseFloat(parts[0], 64); err == nil {
					min = &v
				}
				if v, err := strconv.ParseFloat(parts[1], 64); err == nil {
					avg = &v
				}
				if v, err := strconv.ParseFloat(parts[2], 64); err == nil {
					max = &v
				}
			}
		}
	}

	return min, avg, max, loss
}

// tcpConnect measures TCP connection time to host:port.
func tcpConnect(host string, port int) *float64 {
	addr := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil
	}
	conn.Close()
	ms := float64(time.Since(start).Microseconds()) / 1000.0
	return &ms
}

// measureBandwidth downloads from a URL and measures throughput.
func measureBandwidth(url string) *float64 {
	client := &http.Client{Timeout: 30 * time.Second}
	start := time.Now()
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := resp.Body.Read(buf)
		total += int64(n)
		if err != nil {
			break
		}
	}

	elapsed := time.Since(start).Seconds()
	if elapsed == 0 {
		return nil
	}

	mbps := float64(total) * 8 / (elapsed * 1000000)
	return &mbps
}
