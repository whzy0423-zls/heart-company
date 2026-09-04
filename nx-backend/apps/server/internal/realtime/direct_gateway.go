package realtime

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
)

type DirectGateway struct {
	Tickets  *TicketStore
	Upgrader websocket.Upgrader
}

func NewDirectGateway(tickets *TicketStore) *DirectGateway {
	return &DirectGateway{Tickets: tickets, Upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}}
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
	_ = conn.WriteJSON(map[string]any{"type": "ready", "userId": userID})
	for {
		var envelope map[string]any
		if err := conn.ReadJSON(&envelope); err != nil {
			return
		}
		if envelope["type"] == "subscribe" {
			conversationID, _ := strconv.ParseInt(toString(envelope["conversationId"]), 10, 64)
			_ = conn.WriteJSON(map[string]any{"type": "subscribed", "conversationId": conversationID})
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
