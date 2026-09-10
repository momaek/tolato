package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/momaek/tolato/agent/internal/collector"
	"github.com/momaek/tolato/agent/internal/executor"
	"github.com/momaek/tolato/agent/internal/identity"
)

// Regression tests for momaek/tolato#15: an agent whose socket went bad had
// to be restarted by hand because nothing on the agent side ever tore the
// connection down and re-dialled.

// fakeServer is a minimal /ws/agent stand-in. For every connection it reads
// the register frame, answers register_ack, bumps the returned counter, then
// hands the socket to `after` together with the 1-based connection number.
// `after` runs on its own goroutine and owns the connection from then on.
//
// A socket that dies before registering is ignored rather than reported: the
// test's deferred Stop() can legitimately close a connection the agent has
// only just dialled.
func fakeServer(t *testing.T, after func(conn *websocket.Conn, n int)) (wsURL string, registered *int32) {
	t.Helper()
	var count int32
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		var msg WSMessage
		if err := conn.ReadJSON(&msg); err != nil || msg.Type != "register" {
			conn.Close()
			return
		}
		ack, _ := json.Marshal(RegisterAckPayload{NodeID: "node-1", Secret: "s3cret"})
		if err := conn.WriteJSON(WSMessage{Type: "register_ack", Payload: ack}); err != nil {
			conn.Close()
			return
		}
		after(conn, int(atomic.AddInt32(&count, 1)))
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), &count
}

func newTestClient(t *testing.T, wsURL string) *Client {
	t.Helper()
	store := identity.NewStore(t.TempDir())
	return NewClient(wsURL, "tok", store, nil, collector.NewCollector(), executor.NewExecutor())
}

// setTiming overrides the liveness knobs for one test and restores them after.
func setTiming(t *testing.T, hb, pong, write, bulk time.Duration) {
	t.Helper()
	oh, op, ow, ob := heartbeatInterval, pongWait, writeWait, bulkWriteWait
	heartbeatInterval, pongWait, writeWait, bulkWriteWait = hb, pong, write, bulk
	t.Cleanup(func() { heartbeatInterval, pongWait, writeWait, bulkWriteWait = oh, op, ow, ob })
}

func waitRegistered(t *testing.T, registered *int32, want int32, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(registered) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d registered connections within %s, got %d", want, within, atomic.LoadInt32(registered))
}

// The server accepts the socket and then never sends another byte and never
// answers Pings. The agent must notice within pongWait, close the socket
// itself, and dial again. Before the fix the agent's read loop blocked here
// forever.
func TestReconnectsWhenServerGoesSilent(t *testing.T) {
	setTiming(t, 50*time.Millisecond, 200*time.Millisecond, time.Second, time.Second)

	firstClosed := make(chan struct{})
	wsURL, registered := fakeServer(t, func(conn *websocket.Conn, n int) {
		// Drain the raw TCP stream without letting gorilla see the Ping
		// frames, so no Pong is ever sent. EOF here means the agent hung up.
		raw := conn.UnderlyingConn()
		buf := make([]byte, 4096)
		for {
			if _, err := raw.Read(buf); err != nil {
				if n == 1 {
					close(firstClosed)
				}
				return
			}
		}
	})

	c := newTestClient(t, wsURL)
	go c.Run()
	defer c.Stop()

	select {
	case <-firstClosed:
	case <-time.After(3 * time.Second):
		t.Fatal("agent never closed the silent connection")
	}
	// Run()'s reconnect backoff starts at initialBackoff (1s).
	waitRegistered(t, registered, 2, 4*time.Second)
}

// The exact failure from the issue: a large command_result hits the write
// deadline. gorilla makes that error permanent for the connection, so the
// only sane reaction is to drop the socket and reconnect — which must happen
// even though the read side has no reason of its own to fail (pongWait is
// set far beyond the test's horizon here).
func TestWriteTimeoutDropsConnection(t *testing.T) {
	setTiming(t, time.Hour, time.Hour, 200*time.Millisecond, 200*time.Millisecond)

	firstClosed := make(chan struct{})
	wsURL, registered := fakeServer(t, func(conn *websocket.Conn, n int) {
		if n != 1 {
			select {} // park the reconnect; the test only checks it arrived
		}
		// Read the raw stream a byte at a time — far slower than the agent
		// can push 32MB — so the socket backs up and the agent's write
		// deadline trips. Any non-timeout read error means the agent hung up.
		raw := conn.UnderlyingConn()
		var one [1]byte
		for {
			_ = raw.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			_, err := raw.Read(one[:])
			if err == nil {
				continue
			}
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				continue
			}
			close(firstClosed)
			return
		}
	})

	c := newTestClient(t, wsURL)
	go c.Run()
	defer c.Stop()

	waitRegistered(t, registered, 1, 3*time.Second)

	big := CommandResultPayload{Stdout: strings.Repeat("x", 32<<20)}
	if err := c.sendBulkMessage("command_result", "cmd-1", big); err == nil {
		t.Fatal("expected the oversized write to time out")
	}

	select {
	case <-firstClosed:
	case <-time.After(3 * time.Second):
		t.Fatal("agent kept the connection open after a fatal write error")
	}
	waitRegistered(t, registered, 2, 4*time.Second)
}
