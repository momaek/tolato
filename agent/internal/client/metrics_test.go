package client

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"

	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type fakeMetrics struct{ calls atomic.Int32 }

func (g *fakeMetrics) Gather(context.Context) (string, time.Time, error) {
	g.calls.Add(1)
	return "node_load1 1\n", time.Now(), nil
}

func TestMetricsNegotiationAndTransport(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		version int
		want    bool
	}{
		{"enabled new server", true, 1, true}, {"disabled", false, 1, false}, {"old server", true, 0, false}, {"unknown protocol", true, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registered := make(chan RegisterPayload, 1)
			received := make(chan metricsSnapshot, 1)
			up := websocket.Upgrader{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := up.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				var msg WSMessage
				if conn.ReadJSON(&msg) != nil {
					return
				}
				var reg RegisterPayload
				_ = json.Unmarshal(msg.Payload, &reg)
				registered <- reg
				raw, _ := json.Marshal(RegisterAckPayload{NodeID: "n", Secret: "s", MetricsVersion: tc.version})
				if conn.WriteJSON(WSMessage{Type: "register_ack", Payload: raw}) != nil {
					return
				}
				for conn.ReadJSON(&msg) == nil {
					if msg.Type == "node_metrics" {
						var s metricsSnapshot
						_ = json.Unmarshal(msg.Payload, &s)
						select {
						case received <- s:
						default:
						}
					}
				}
			}))
			defer srv.Close()
			c := newTestClient(t, "ws"+strings.TrimPrefix(srv.URL, "http"))
			gatherer := &fakeMetrics{}
			if tc.enabled {
				c.EnableNodeExporter(gatherer, 30*time.Millisecond, time.Second)
			}
			done := make(chan struct{})
			go func() { defer close(done); c.Run() }()
			defer func() { c.Stop(); <-done }()
			select {
			case reg := <-registered:
				if reg.NodeExporterEnabled != tc.enabled {
					t.Fatal("wrong registration capability")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("registration timeout")
			}
			if tc.want {
				select {
				case snapshot := <-received:
					if snapshot.Version != 1 || snapshot.Data != "node_load1 1\n" || snapshot.CollectedAt <= 0 {
						t.Fatalf("bad snapshot: %+v", snapshot)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("metrics not delivered")
				}
			} else {
				select {
				case <-received:
					t.Fatal("metrics sent without negotiation")
				case <-time.After(100 * time.Millisecond):
				}
				if gatherer.calls.Load() != 0 {
					t.Fatal("collection started without negotiation")
				}
			}
		})
	}
}

func TestCompressedMetricsRoundTrip(t *testing.T) {
	original := strings.Repeat("node_network_receive_bytes_total{device=\"eth0\"} 123456789\n", 1000)
	data, encoding := encodeMetricsData(original)
	if encoding != "gzip+base64" || len(data) >= len(original)/2 {
		t.Fatal("metrics not compacted")
	}
	compressed, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil || string(decoded) != original {
		t.Fatalf("compression changed metrics: %v", err)
	}
}
