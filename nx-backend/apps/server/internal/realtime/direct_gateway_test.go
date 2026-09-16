package realtime

import (
	"context"
	"testing"
	"time"
)

func TestDirectGatewayRequiresProtocolTicket(t *testing.T) {
	if NewDirectGateway(nil, NewDirectHub(), nil) == nil {
		t.Fatal("gateway should be constructible")
	}
}

func TestDirectGatewayPublishesTypingForActiveSubscription(t *testing.T) {
	hub := NewDirectHub()
	gateway := NewDirectGateway(nil, hub, func(context.Context, int64, int64) error { return nil })
	events, stop := hub.Subscribe(12)
	defer stop()

	code := gateway.handleClientEvent(context.Background(), 7, 12, map[string]any{
		"type":           "typing",
		"conversationId": 12,
		"isTyping":       true,
	})
	if code != "" {
		t.Fatalf("unexpected error code %q", code)
	}
	select {
	case raw := <-events:
		event, ok := raw.(map[string]any)
		if !ok || event["type"] != "typing" {
			t.Fatalf("unexpected event %#v", raw)
		}
		data, _ := event["data"].(map[string]any)
		if data["userId"] != int64(7) || data["conversationId"] != int64(12) || data["isTyping"] != true {
			t.Fatalf("unexpected typing data %#v", data)
		}
	case <-time.After(time.Second):
		t.Fatal("typing event was not published")
	}
}

func TestDirectGatewayRejectsTypingOutsideActiveSubscription(t *testing.T) {
	gateway := NewDirectGateway(nil, NewDirectHub(), func(context.Context, int64, int64) error { return nil })
	code := gateway.handleClientEvent(context.Background(), 7, 12, map[string]any{
		"type":           "typing",
		"conversationId": 99,
		"isTyping":       true,
	})
	if code != "direct_message.not_participant" {
		t.Fatalf("expected participant error, got %q", code)
	}
}

func TestDirectGatewayOpensInboxSubscriptionForAuthenticatedUser(t *testing.T) {
	hub := NewDirectHub()
	gateway := NewDirectGateway(nil, hub, func(context.Context, int64, int64) error { return nil })

	subscription, code := gateway.openSubscription(context.Background(), 7, map[string]any{
		"type":   "subscribeInbox",
		"userId": 99,
	})
	if code != "" {
		t.Fatalf("unexpected error code %q", code)
	}
	defer subscription.stop()
	if subscription.conversationID != 0 || subscription.ack["type"] != "inboxSubscribed" {
		t.Fatalf("unexpected inbox subscription: %#v", subscription)
	}

	hub.PublishUser(99, "wrong-user")
	select {
	case event := <-subscription.events:
		t.Fatalf("subscription trusted client user id: %#v", event)
	default:
	}
	hub.PublishUser(7, "current-user")
	select {
	case event := <-subscription.events:
		if event != "current-user" {
			t.Fatalf("unexpected inbox event %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("authenticated user inbox was not subscribed")
	}
}
