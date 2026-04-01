package probe

import (
	"context"
	"fmt"
	"net"
	"time"
)

// TCPResult holds the result of a TCP connect probe.
type TCPResult struct {
	ConnectTime float64 // ms
	Err         error
}

// TCPConnect measures the TCP handshake time to host:port.
func TCPConnect(ctx context.Context, host string, port int) TCPResult {
	addr := fmt.Sprintf("%s:%d", host, port)
	timeout := 5 * time.Second

	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	elapsed := time.Since(start)

	if err != nil {
		return TCPResult{
			ConnectTime: float64(elapsed.Milliseconds()),
			Err:         fmt.Errorf("tcp connect %s: %w", addr, err),
		}
	}
	conn.Close()

	_ = ctx // context used for cancellation at caller level
	return TCPResult{
		ConnectTime: float64(elapsed.Microseconds()) / 1000.0, // sub-ms precision
	}
}
