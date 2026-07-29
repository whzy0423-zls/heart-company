package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAppXinzhiliRealtimeCapabilitiesReturnsContract(t *testing.T) {
	s := newRouteOnlyServer()
	req := httptest.NewRequest(http.MethodGet, "/api/app/xinzhili/realtime/capabilities", nil)
	rr := httptest.NewRecorder()
	s.appXinzhiliRealtimeCapabilities(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() == 0 {
		t.Fatal("capabilities response is empty")
	}
}

func TestServerRegistersXinzhiliRealtimeCapabilitiesRoute(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `s.mux.HandleFunc("/api/app/xinzhili/realtime/capabilities"`) {
		t.Fatal("capabilities route is not registered")
	}
}
