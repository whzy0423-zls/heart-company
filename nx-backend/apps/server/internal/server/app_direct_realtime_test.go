package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDirectRealtimeTicketRouteIsRegistered(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `s.mux.HandleFunc("/api/app/direct-realtime/ticket"`) {
		t.Fatal("missing realtime ticket route")
	}
}

func TestDirectRealtimeTicketRequiresAuthentication(t *testing.T) {
	response := httptest.NewRecorder()
	(&Server{}).appDirectRealtimeTicket(response, httptest.NewRequest(http.MethodPost, "/api/app/direct-realtime/ticket", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", response.Code)
	}
}
