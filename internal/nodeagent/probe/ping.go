package probe

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// PingResult holds the result of a ping probe.
type PingResult struct {
	Min        float64 // ms
	Avg        float64 // ms
	Max        float64 // ms
	PacketLoss float64 // percentage 0-100
	Err        error
}

var (
	// Matches: rtt min/avg/max/mdev = 1.234/5.678/9.012/0.345 ms
	rttRegex  = regexp.MustCompile(`= ([\d.]+)/([\d.]+)/([\d.]+)/`)
	// Matches: 10% packet loss  OR  10.0% packet loss
	lossRegex = regexp.MustCompile(`([\d.]+)% packet loss`)
)

// Ping executes a ping command and parses the results.
func Ping(ctx context.Context, host string, count int) PingResult {
	cmd := exec.CommandContext(ctx, "ping",
		"-c", strconv.Itoa(count),
		"-W", "2",
		host,
	)

	out, err := cmd.CombinedOutput()
	output := string(out)

	result := PingResult{}

	// Parse packet loss even if ping exits non-zero (partial loss)
	if m := lossRegex.FindStringSubmatch(output); len(m) >= 2 {
		result.PacketLoss, _ = strconv.ParseFloat(m[1], 64)
	}

	// Parse RTT summary
	if m := rttRegex.FindStringSubmatch(output); len(m) >= 4 {
		result.Min, _ = strconv.ParseFloat(m[1], 64)
		result.Avg, _ = strconv.ParseFloat(m[2], 64)
		result.Max, _ = strconv.ParseFloat(m[3], 64)
	} else if err != nil {
		// If no RTT line and command failed, it's a complete failure
		if result.PacketLoss == 0 {
			result.PacketLoss = 100
		}
		if !strings.Contains(output, "packet loss") {
			result.Err = fmt.Errorf("ping failed: %w: %s", err, firstLine(output))
		}
	}

	return result
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
