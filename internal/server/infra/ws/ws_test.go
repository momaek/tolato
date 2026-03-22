package ws

import (
	"bytes"
	"testing"
	"time"
)

func TestBoundedSendQueueClosesOnOverflow(t *testing.T) {
	client := NewMemoryClient("client-1", ClientKindUI, 1)

	if !client.Send([]byte("first")) {
		t.Fatalf("expected first send to succeed")
	}
	if client.Send([]byte("second")) {
		t.Fatalf("expected second send to fail when queue is full")
	}
	if !client.Closed() {
		t.Fatalf("expected client to be closed after overflow")
	}
	if client.CloseCode() != 1001 {
		t.Fatalf("expected close code 1001, got %d", client.CloseCode())
	}
	if client.CloseReason() != "send queue full" {
		t.Fatalf("expected close reason to record overflow, got %q", client.CloseReason())
	}

	got := drainMessages(client.Messages())
	if len(got) != 1 || string(got[0]) != "first" {
		t.Fatalf("unexpected drained messages: %#v", got)
	}
}

func TestClientCloseIsIdempotent(t *testing.T) {
	client := NewMemoryClient("client-2", ClientKindUI, 2)

	client.Close(1000, "first")
	client.Close(1006, "second")

	if !client.Closed() {
		t.Fatalf("expected client to be closed")
	}
	if client.CloseCode() != 1000 {
		t.Fatalf("expected first close code to win, got %d", client.CloseCode())
	}
	if client.CloseReason() != "first" {
		t.Fatalf("expected first close reason to win, got %q", client.CloseReason())
	}
	if client.Send([]byte("after-close")) {
		t.Fatalf("expected send after close to fail")
	}
}

func TestSessionRegistryPublishesActiveAndSummarySessions(t *testing.T) {
	hub := NewMemoryHub()
	activeClient := NewMemoryClient("client-active", ClientKindUI, 4)
	watchClient := NewMemoryClient("client-watch", ClientKindUI, 4)
	mixedClient := NewMemoryClient("client-mixed", ClientKindUI, 4)

	hub.Register(activeClient)
	hub.Register(watchClient)
	hub.Register(mixedClient)

	registry := NewMemorySessionRegistry(hub)
	registry.SetActive("client-active", "sess-1")
	registry.SetWatchSessions("client-watch", []string{"sess-1"})
	registry.SetActive("client-mixed", "sess-2")
	registry.SetWatchSessions("client-mixed", []string{"sess-1", "sess-2", "sess-1"})

	registry.PublishToSession("sess-1", []byte("timeline"))
	if got := drainMessages(activeClient.Messages()); len(got) != 1 || string(got[0]) != "timeline" {
		t.Fatalf("active client did not receive timeline event: %#v", got)
	}
	if got := drainMessages(watchClient.Messages()); len(got) != 0 {
		t.Fatalf("watch client should not receive timeline event: %#v", got)
	}
	if got := drainMessages(mixedClient.Messages()); len(got) != 0 {
		t.Fatalf("non-active watcher should not receive timeline event: %#v", got)
	}

	registry.PublishSummary("sess-1", []byte("summary"))
	if got := drainMessages(activeClient.Messages()); len(got) != 1 || string(got[0]) != "summary" {
		t.Fatalf("active client did not receive summary event: %#v", got)
	}
	if got := drainMessages(watchClient.Messages()); len(got) != 1 || string(got[0]) != "summary" {
		t.Fatalf("watch client did not receive summary event: %#v", got)
	}
	if got := drainMessages(mixedClient.Messages()); len(got) != 1 || string(got[0]) != "summary" {
		t.Fatalf("mixed client should receive summary event once: %#v", got)
	}
}

func TestSessionRegistryForgetClientRemovesTransientSubscriptions(t *testing.T) {
	hub := NewMemoryHub()
	client := NewMemoryClient("client-reconnect", ClientKindUI, 4)
	hub.Register(client)

	registry := NewMemorySessionRegistry(hub)
	registry.SetActive("client-reconnect", "sess-1")
	registry.SetWatchSessions("client-reconnect", []string{"sess-2"})

	registry.ForgetClient("client-reconnect")

	if _, ok := registry.activeByClient["client-reconnect"]; ok {
		t.Fatalf("active subscription should be removed: %#v", registry.activeByClient)
	}
	if _, ok := registry.watchByClient["client-reconnect"]; ok {
		t.Fatalf("watch subscription should be removed: %#v", registry.watchByClient)
	}
	if len(registry.activeBySession) != 0 || len(registry.watchBySession) != 0 {
		t.Fatalf("session indexes should be empty: active=%#v watch=%#v", registry.activeBySession, registry.watchBySession)
	}
}

func TestHubRegisterReplacesClient(t *testing.T) {
	hub := NewMemoryHub()
	first := NewMemoryClient("client-1", ClientKindUI, 1)
	second := NewMemoryClient("client-1", ClientKindUI, 1)

	hub.Register(first)
	hub.Register(second)

	if !first.Closed() {
		t.Fatalf("expected previous client to be closed when replaced")
	}
	got, ok := hub.Client("client-1")
	if !ok || got != second {
		t.Fatalf("expected hub to keep the latest client")
	}

	hub.Unregister("client-1")
	if !second.Closed() {
		t.Fatalf("expected current client to be closed on unregister")
	}
	if _, ok := hub.Client("client-1"); ok {
		t.Fatalf("expected client to be removed from hub")
	}
}

func TestAgentRegistryDispatchesToBoundClient(t *testing.T) {
	hub := NewMemoryHub()
	client := NewMemoryClient("agent-1", ClientKindAgent, 2)
	hub.Register(client)

	registry := NewMemoryAgentRegistry(hub)
	registry.BindNode("node-1", "agent-1")

	if err := registry.PublishDispatch("node-1", []byte("dispatch")); err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}
	if got := drainMessages(client.Messages()); len(got) != 1 || !bytes.Equal(got[0], []byte("dispatch")) {
		t.Fatalf("expected dispatch to reach bound client: %#v", got)
	}

	registry.UnbindNode("node-1", "agent-1")
	if err := registry.PublishDispatch("node-1", []byte("dispatch")); err != ErrNodeNotBound {
		t.Fatalf("expected ErrNodeNotBound after unbind, got %v", err)
	}
}

func TestAgentRegistryRecordsHeartbeat(t *testing.T) {
	hub := NewMemoryHub()
	client := NewMemoryClient("agent-2", ClientKindAgent, 2)
	hub.Register(client)

	registry := NewMemoryAgentRegistry(hub)
	registry.BindNode("node-2", "agent-2")

	at := time.Date(2026, 3, 22, 20, 0, 0, 0, time.UTC)
	if err := registry.Heartbeat("node-2", "agent-2", at); err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	got, ok := registry.LastHeartbeat("node-2")
	if !ok || !got.Equal(at) {
		t.Fatalf("LastHeartbeat() = %v, %v want %v, true", got, ok, at)
	}
}

func drainMessages(ch <-chan []byte) [][]byte {
	out := make([][]byte, 0)
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, append([]byte(nil), msg...))
		default:
			return out
		}
	}
}
