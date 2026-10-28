package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const metricsProtocolVersion = 1

// metricsGatherer makes the transport independently testable from host collectors.
type metricsGatherer interface {
	Gather(context.Context) (string, time.Time, error)
}

type metricsConfig struct {
	exporter metricsGatherer
	interval time.Duration
	timeout  time.Duration
}

type metricsSnapshot struct {
	Version     int     `json:"version"`
	CollectedAt float64 `json:"collected_at"`
	Encoding    string  `json:"encoding,omitempty"`
	Data        string  `json:"data,omitempty"`
	Error       string  `json:"error,omitempty"`
}

// EnableNodeExporter is called before Run. An absent capability in register_ack
// leaves this feature dormant on older servers.
func (c *Client) EnableNodeExporter(e metricsGatherer, interval, timeout time.Duration) {
	c.nodeExporter = &metricsConfig{exporter: e, interval: interval, timeout: timeout}
}

func (c *Client) metricsLoop(ctx context.Context, conn *websocket.Conn, ready <-chan int) {
	if c.nodeExporter == nil {
		return
	}
	select {
	case version := <-ready:
		if version != metricsProtocolVersion {
			log.Printf("[node_exporter] server does not support metrics protocol %d; collection dormant", metricsProtocolVersion)
			return
		}
	case <-ctx.Done():
		return
	}
	cfg := c.nodeExporter
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	for {
		collectCtx, cancel := context.WithTimeout(ctx, cfg.timeout)
		data, at, err := cfg.exporter.Gather(collectCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		snapshot := metricsSnapshot{Version: metricsProtocolVersion, Data: data}
		if err != nil {
			log.Printf("[node_exporter] collection failed: %v", err)
			snapshot.Data = ""
			snapshot.Error = err.Error()
			if len(snapshot.Error) > 1024 {
				snapshot.Error = snapshot.Error[:1024]
			}
		} else {
			snapshot.CollectedAt = float64(at.UnixMilli()) / 1000
			snapshot.Data, snapshot.Encoding = encodeMetricsData(data)
		}
		raw, _ := json.Marshal(snapshot)
		msg := WSMessage{Type: "node_metrics", Payload: raw}
		// Metrics are lower priority than interactive traffic. No transport queue:
		// if the writer is busy, discard this snapshot and try the next interval.
		if c.connMu.TryLock() {
			if ctx.Err() == nil && c.conn == conn {
				_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
				if err := conn.WriteJSON(msg); err != nil {
					log.Printf("[node_exporter] send failed: %v", err)
					_ = conn.Close()
				}
			}
			c.connMu.Unlock()
		} else {
			log.Printf("[node_exporter] snapshot dropped: WebSocket writer busy")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Reuse the compression workspace; it is allocated only when metrics are enabled.
var metricsCompressors = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
	return w
}}

func encodeMetricsData(data string) (string, string) {
	var buffer bytes.Buffer
	w := metricsCompressors.Get().(*gzip.Writer)
	w.Reset(&buffer)
	_, err := io.WriteString(w, data)
	closeErr := w.Close()
	w.Reset(io.Discard)
	metricsCompressors.Put(w)
	if err != nil || closeErr != nil {
		return data, ""
	}
	encoded := base64.StdEncoding.EncodeToString(buffer.Bytes())
	if len(encoded) >= len(data) {
		return data, ""
	}
	return encoded, "gzip+base64"
}
