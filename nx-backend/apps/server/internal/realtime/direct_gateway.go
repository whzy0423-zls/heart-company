package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
)

type DirectGateway struct {
	Tickets   *TicketStore
	Hub       *DirectHub
	Authorize func(context.Context, int64, int64) error
	Upgrader  websocket.Upgrader
}

func NewDirectGateway(tickets *TicketStore, hub *DirectHub, authorize func(context.Context, int64, int64) error) *DirectGateway {
	return &DirectGateway{Tickets: tickets, Hub: hub, Authorize: authorize, Upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}}
}

func (g *DirectGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	protocols := websocket.Subprotocols(r)
	if len(protocols) == 0 || g.Tickets == nil {
		http.Error(w, "ticket required", http.StatusUnauthorized)
		return
	}
	userID, err := g.Tickets.Consume(r.Context(), protocols[0])
	if err != nil {
		http.Error(w, "ticket invalid", http.StatusUnauthorized)
		return
	}
	conn, err := g.Upgrader.Upgrade(w, r, http.Header{"Sec-WebSocket-Protocol": []string{protocols[0]}})
	if err != nil {
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	outbound := make(chan any, 32)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-outbound:
				if err := conn.WriteJSON(event); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	send := func(event any) bool {
		select {
		case outbound <- event:
			return true
		case <-ctx.Done():
			return false
		}
	}
	if !send(map[string]any{"type": "ready", "userId": userID}) {
		return
	}
	var unsubscribe func()
	defer func() {
		if unsubscribe != nil {
			unsubscribe()
		}
		cancel()
		_ = conn.Close()
		<-writerDone
	}()
	for {
		var envelope map[string]any
		if err := conn.ReadJSON(&envelope); err != nil {
			return
		}
		if envelope["type"] == "subscribe" {
			conversationID, _ := strconv.ParseInt(toString(envelope["conversationId"]), 10, 64)
			if conversationID <= 0 || g.Hub == nil || g.Authorize == nil || g.Authorize(ctx, userID, conversationID) != nil {
				send(map[string]any{"type": "error", "code": "direct_message.not_participant"})
				continue
			}
			if unsubscribe != nil {
				unsubscribe()
			}
			messages, stop := g.Hub.Subscribe(conversationID)
			unsubscribe = stop
			go func(subscription <-chan any) {
				for {
					select {
					case <-ctx.Done():
						return
					case event, ok := <-subscription:
						if !ok || !send(event) {
							return
						}
					}
				}
			}(messages)
			send(map[string]any{"type": "subscribed", "conversationId": conversationID})
		}
	}
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	raw, _ := json.Marshal(value)
	return string(raw)
}
