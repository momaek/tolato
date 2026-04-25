package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialPair spins up an httptest server that upgrades the request to a
// WebSocket and returns both ends (client side from Dial, server side from
// the handler). Closing both is the caller's responsibility.
func dialPair(t *testing.T) (server, client *websocket.Conn, cleanup func()) {
	t.Helper()

	upgrader := websocket.Upgrader{}
	gotServer := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		gotServer <- c
	}))

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}

	select {
	case s := <-gotServer:
		server = s
	case <-time.After(2 * time.Second):
		c.Close()
		srv.Close()
		t.Fatal("handler did not receive upgrade")
	}

	cleanup = func() {
		_ = c.Close()
		_ = server.Close()
		srv.Close()
	}
	return server, c, cleanup
}

// drain reads frames from conn until it errors. Used to keep the peer alive so
// gorilla writes don't fail with "broken pipe" before the test asserts.
func drain(conn *websocket.Conn) {
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func TestChatSession_ConcurrentWriteJSON(t *testing.T) {
	// gorilla/websocket forbids concurrent writes — without ChatSession's
	// mutex this test fails (intermittently) under -race or produces
	// corrupted frames.
	srv, client, cleanup := dialPair(t)
	defer cleanup()

	go drain(client)

	session := NewChatSession(srv)

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			if err := session.WriteJSON(map[string]int{"i": i}); err != nil {
				t.Errorf("WriteJSON: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestChatSession_WriteAfterCloseIsNoop(t *testing.T) {
	srv, _, cleanup := dialPair(t)
	defer cleanup()

	session := NewChatSession(srv)
	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := session.WriteJSON(map[string]string{"k": "v"}); err != nil {
		t.Errorf("WriteJSON after Close should be nil, got %v", err)
	}
	// Idempotent.
	if err := session.Close(); err != nil {
		t.Errorf("second Close should be nil, got %v", err)
	}
}

func TestSessionManager_ReplaceDoesNotRaceWithWriter(t *testing.T) {
	// The bug fixed: previously SessionManager.Replace called
	// conn.WriteJSON directly while chatWriteLoop on the same conn was also
	// writing — gorilla forbids that. Now Replace acquires the old
	// session's writeMu via WriteJSON, so concurrent writers serialize.
	// Run with `go test -race`.
	srv, client, cleanup := dialPair(t)
	defer cleanup()

	go drain(client)

	old := NewChatSession(srv)
	sm := NewSessionManager()
	sm.Replace(old)

	// Spam writes from one goroutine while Replace kicks the session from
	// another. Without the writeMu fix, the race detector flags this.
	var stop atomic.Bool
	var writerWG sync.WaitGroup
	writerWG.Add(1)
	go func() {
		defer writerWG.Done()
		for !stop.Load() {
			_ = old.WriteJSON(map[string]string{"k": "v"})
		}
	}()

	// Tiny sleep to let writer get in flight.
	time.Sleep(20 * time.Millisecond)

	srv2, client2, cleanup2 := dialPair(t)
	defer cleanup2()
	go drain(client2)
	newSess := NewChatSession(srv2)
	sm.Replace(newSess) // kicks `old`, writes session_replaced under its writeMu

	stop.Store(true)
	writerWG.Wait()

	// After Replace, old session is closed; further writes are no-ops.
	if err := old.WriteJSON(map[string]string{"k": "v"}); err != nil {
		t.Errorf("WriteJSON on kicked session should be nil, got %v", err)
	}
}

func TestSessionManager_StaleRemoveDoesNotEvictCurrent(t *testing.T) {
	// Real-world race: connection A's read loop returns and its deferred
	// SessionManager.Remove(a) fires AFTER connection B has already taken
	// over via Replace(b). The stale Remove(a) must be a no-op — otherwise
	// b would not be kicked when c connects.
	srv1, client1, c1 := dialPair(t)
	defer c1()
	srv2, client2, c2 := dialPair(t)
	defer c2()
	srv3, client3, c3 := dialPair(t)
	defer c3()
	go drain(client1)
	go drain(client3)
	// Don't drain client2 — we want to read the kick message ourselves.

	a := NewChatSession(srv1)
	b := NewChatSession(srv2)
	c := NewChatSession(srv3)

	sm := NewSessionManager()
	sm.Replace(a)
	sm.Replace(b) // a kicked
	sm.Remove(a)  // stale — must NOT clear current=b

	sm.Replace(c) // should kick b → writes "session_replaced" to client2

	// First frame on client2 should be the session_replaced message.
	_ = client2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, raw, err := client2.ReadMessage()
	if err != nil {
		t.Fatalf("expected session_replaced frame on client2, got read err: %v", err)
	}
	if !strings.Contains(string(raw), "session_replaced") {
		t.Errorf("expected session_replaced frame, got: %s", raw)
	}
}
