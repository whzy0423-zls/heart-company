package realtime

import (
	"testing"
	"time"
)

func TestDirectHubIsolatesConversationAndUnsubscribes(t *testing.T) {
	hub := NewDirectHub()
	one, unsubscribe := hub.Subscribe(10)
	other, stopOther := hub.Subscribe(11)
	defer stopOther()

	hub.Publish(10, "first")
	select {
	case got := <-one:
		if got != "first" {
			t.Fatalf("unexpected event: %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected event")
	}
	select {
	case got := <-other:
		t.Fatalf("event leaked to another conversation: %v", got)
	default:
	}

	unsubscribe()
	if _, ok := <-one; ok {
		t.Fatal("subscription should close")
	}
}
