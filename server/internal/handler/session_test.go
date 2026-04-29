package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

