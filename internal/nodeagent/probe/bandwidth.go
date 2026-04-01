package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BandwidthResult holds the result of a bandwidth test.
type BandwidthResult struct {
	Mbps float64
	Err  error
}

// MeasureBandwidth downloads from the given URL and calculates throughput.
func MeasureBandwidth(ctx context.Context, url string) BandwidthResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return BandwidthResult{Err: fmt.Errorf("bandwidth request: %w", err)}
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return BandwidthResult{Err: fmt.Errorf("bandwidth download: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return BandwidthResult{Err: fmt.Errorf("bandwidth: HTTP %d", resp.StatusCode)}
	}

	n, err := io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(start)

	if err != nil {
		return BandwidthResult{Err: fmt.Errorf("bandwidth read: %w", err)}
	}
	if elapsed.Seconds() == 0 {
		return BandwidthResult{Err: fmt.Errorf("bandwidth: zero elapsed time")}
	}

	mbps := float64(n) * 8 / elapsed.Seconds() / 1e6
	return BandwidthResult{Mbps: mbps}
}
