package codex

import (
	"context"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestSendStreamEventSendsWhenChannelReady(t *testing.T) {
	ctx := context.Background()
	ch := make(chan provider.StreamEvent, 1)
	event := provider.TokenDelta{Text: "hello"}

	if ok := sendStreamEvent(ctx, ch, event); !ok {
		t.Fatal("expected sendStreamEvent to succeed")
	}

	got := <-ch
	delta, ok := got.(provider.TokenDelta)
	if !ok {
		t.Fatalf("event type = %T, want provider.TokenDelta", got)
	}
	if delta.Text != "hello" {
		t.Fatalf("delta.Text = %q, want hello", delta.Text)
	}
}

func TestSendStreamEventReturnsWhenContextCancelledAndChannelBlocked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.TokenDelta{Text: "already full"}

	done := make(chan bool, 1)
	go func() {
		done <- sendStreamEvent(ctx, ch, provider.StreamError{Fatal: true, Message: "stream cancelled"})
	}()

	cancel()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("expected sendStreamEvent to report failure after cancellation")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("sendStreamEvent blocked after context cancellation")
	}
}
