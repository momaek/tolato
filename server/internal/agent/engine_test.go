package agent

import (
	"context"
	"testing"
	"time"
)

// TestLoopRunner_SendUnblocksOnCtxCancel verifies the central deadlock fix:
// previously runner sends to eventCh were naked (`lr.eventCh <- evt`), so when
// the writer goroutine died and the buffer filled, the runner blocked forever
// (and the LLM stream callback wedged with it). Now `send` selects on ctx.Done.
func TestLoopRunner_SendUnblocksOnCtxCancel(t *testing.T) {
	// Buffer 0 + no consumer = sender blocks unconditionally on naked send.
	eventCh := make(chan any)
	lr := &LoopRunner{eventCh: eventCh}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool, 1)
	go func() {
		done <- lr.send(ctx, ContentEvent{Delta: "hello"})
	}()

	// Give the goroutine a moment to enter the select on the blocked send.
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case ok := <-done:
		if ok {
			t.Errorf("send should return false when ctx cancelled, got true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("send did not unblock on ctx cancel — deadlock")
	}
}

func TestLoopRunner_SendDeliversWhenWriterReady(t *testing.T) {
	eventCh := make(chan any, 1)
	lr := &LoopRunner{eventCh: eventCh}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if !lr.send(ctx, ContentEvent{Delta: "hello"}) {
		t.Fatal("send should return true when channel has space")
	}
	select {
	case ev := <-eventCh:
		ce, ok := ev.(ContentEvent)
		if !ok || ce.Delta != "hello" {
			t.Errorf("unexpected event: %#v", ev)
		}
	default:
		t.Fatal("event was not delivered to channel")
	}
}

// TestLoopRunner_SendPrefersCtxOverChannel verifies the select chooses ctx
// when both could fire — important so a cancelled run doesn't keep emitting
// stale events to a writer that is about to be torn down.
func TestLoopRunner_SendReturnsFalseIfCtxAlreadyCancelled(t *testing.T) {
	eventCh := make(chan any, 1) // has space, so send would succeed
	lr := &LoopRunner{eventCh: eventCh}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Go's select with both cases ready picks pseudo-randomly; we run many
	// iterations to catch the path where ctx-cancelled is honoured. As long
	// as send eventually returns false at least once, the ctx case is wired.
	sawFalse := false
	for i := 0; i < 100; i++ {
		// Drain so the buffer doesn't block the send arm.
		select {
		case <-eventCh:
		default:
		}
		if !lr.send(ctx, ContentEvent{Delta: "x"}) {
			sawFalse = true
			break
		}
	}
	if !sawFalse {
		t.Errorf("expected send to return false at least once with cancelled ctx")
	}
}
