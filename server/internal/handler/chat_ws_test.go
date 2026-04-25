package handler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/agent"
	"github.com/momaek/tolato/server/internal/model"
)

// TestChatWriteLoop_ExitsCleanlyOnChannelClose covers the original goroutine
// leak: the loop ranged forever over an eventCh that was never closed. With
// the shutdown-sequence fix, close(eventCh) lets the loop exit naturally.
func TestChatWriteLoop_ExitsCleanlyOnChannelClose(t *testing.T) {
	srv, client, cleanup := dialPair(t)
	defer cleanup()
	go drain(client)

	session := NewChatSession(srv)
	eventCh := make(chan any, 4)

	done := make(chan struct{})
	go func() {
		chatWriteLoop(session, eventCh)
		close(done)
	}()

	eventCh <- agent.ContentEvent{ConversationID: "c1", Delta: "hi"}
	eventCh <- agent.DoneEvent{ConversationID: "c1"}
	close(eventCh)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("chatWriteLoop did not exit after channel close — goroutine leak")
	}
}

// TestChatWriteLoop_DrainsAfterWriteError covers the scenario where the
// connection is closed (e.g., session_replaced from another tab) while
// runners are still streaming events. The writer's first WriteJSON fails;
// it must drain remaining events instead of returning so runners don't
// block on send-to-full-channel and deadlock.
func TestChatWriteLoop_DrainsAfterWriteError(t *testing.T) {
	srv, client, cleanup := dialPair(t)
	defer cleanup()

	// Close the client side immediately so server-side WriteJSON fails on
	// the first attempt.
	_ = client.Close()

	session := NewChatSession(srv)
	eventCh := make(chan any, 2)

	done := make(chan struct{})
	go func() {
		chatWriteLoop(session, eventCh)
		close(done)
	}()

	// Push more events than the buffer holds. If the writer didn't drain
	// after its WriteJSON error, this third send would block forever.
	pushed := make(chan struct{})
	go func() {
		eventCh <- agent.ContentEvent{ConversationID: "c1", Delta: "a"}
		eventCh <- agent.ContentEvent{ConversationID: "c1", Delta: "b"}
		eventCh <- agent.ContentEvent{ConversationID: "c1", Delta: "c"}
		eventCh <- agent.ContentEvent{ConversationID: "c1", Delta: "d"}
		close(pushed)
	}()

	select {
	case <-pushed:
	case <-time.After(2 * time.Second):
		t.Fatal("sender blocked — writer is not draining after WriteJSON error")
	}

	close(eventCh)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("chatWriteLoop did not exit after channel close")
	}
}

// TestChatPingLoop_DeliversPingsAndExitsOnCancel verifies the heartbeat path:
// without a server-side pinger, dead clients (browser refresh that dropped
// TCP, NAT idle timeout) leave the server's ReadMessage blocked forever and
// SessionManager pointing at a corpse — which is what makes a refreshed page
// appear to "still use the old connection" until the next dial.
func TestChatPingLoop_DeliversPingsAndExitsOnCancel(t *testing.T) {
	srv, client, cleanup := dialPair(t)
	defer cleanup()

	var pings atomic.Int32
	client.SetPingHandler(func(string) error {
		pings.Add(1)
		// Reply with pong so the test mimics real browser behavior.
		return client.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second))
	})

	// The client only invokes its PingHandler from inside ReadMessage, so we
	// have to keep reading. ReadMessage returns on close — that's our exit.
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		for {
			if _, _, err := client.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	pingerDone := make(chan struct{})
	go func() {
		chatPingLoop(ctx, srv, 20*time.Millisecond)
		close(pingerDone)
	}()

	// Wait for at least 3 pings to confirm the ticker is firing and the
	// peer is receiving them.
	deadline := time.Now().Add(2 * time.Second)
	for pings.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := pings.Load(); got < 3 {
		t.Errorf("expected >=3 pings within 2s, got %d", got)
	}

	cancel()
	select {
	case <-pingerDone:
	case <-time.After(time.Second):
		t.Fatal("chatPingLoop did not exit promptly on ctx cancel")
	}

	_ = client.Close()
	<-clientDone
}

// TestChatPingLoop_ExitsOnWriteFailure verifies the pinger gives up when the
// peer is gone (WriteControl fails on a closed conn). Without this, the
// pinger would log forever and leak the goroutine.
func TestChatPingLoop_ExitsOnWriteFailure(t *testing.T) {
	srv, client, cleanup := dialPair(t)
	defer cleanup()

	// Kill the peer side so WriteControl on srv fails on the very first tick.
	_ = client.Close()
	// Also close the server side so WriteControl errors deterministically.
	_ = srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		chatPingLoop(ctx, srv, 10*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("chatPingLoop did not exit after WriteControl failure")
	}
}

// TestChatWriteLoop_SerializesWritesUnderConcurrentReplace verifies the
// original race: SessionManager.Replace used to call conn.WriteJSON
// concurrently with chatWriteLoop's WriteJSON on the same conn. Now both
// paths go through ChatSession.WriteJSON which is mutex-guarded. Run with
// `go test -race` for full assurance.
func TestChatWriteLoop_SerializesWritesUnderConcurrentReplace(t *testing.T) {
	srv, client, cleanup := dialPair(t)
	defer cleanup()
	go drain(client)

	session := NewChatSession(srv)
	eventCh := make(chan any, 16)

	var writerWG sync.WaitGroup
	writerWG.Add(1)
	go func() {
		defer writerWG.Done()
		chatWriteLoop(session, eventCh)
	}()

	// Pump events from one side; concurrently issue Replace-style kicks
	// (direct WriteJSON of session_replaced) from another goroutine. With
	// the writeMu in place, neither side races.
	var stop sync.WaitGroup
	stop.Add(2)
	go func() {
		defer stop.Done()
		for i := 0; i < 200; i++ {
			eventCh <- agent.ContentEvent{ConversationID: "c", Delta: "x"}
		}
	}()
	go func() {
		defer stop.Done()
		for i := 0; i < 50; i++ {
			_ = session.WriteJSON(model.WSMessage{Type: "session_replaced"})
			time.Sleep(time.Millisecond)
		}
	}()
	stop.Wait()

	close(eventCh)
	writerWG.Wait()
}
