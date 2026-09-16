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

func TestDirectHubIsolatesUserInboxFromConversationChannels(t *testing.T) {
	hub := NewDirectHub()
	inbox, stopInbox := hub.SubscribeUser(7)
	defer stopInbox()
	otherInbox, stopOther := hub.SubscribeUser(8)
	defer stopOther()
	conversation, stopConversation := hub.Subscribe(7)
	defer stopConversation()

	hub.PublishUser(7, "incoming")
	select {
	case got := <-inbox:
		if got != "incoming" {
			t.Fatalf("unexpected inbox event: %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected user inbox event")
	}
	select {
	case got := <-otherInbox:
		t.Fatalf("event leaked to another user inbox: %v", got)
	default:
	}
	select {
	case got := <-conversation:
		t.Fatalf("user inbox event leaked to conversation channel: %v", got)
	default:
	}
}
