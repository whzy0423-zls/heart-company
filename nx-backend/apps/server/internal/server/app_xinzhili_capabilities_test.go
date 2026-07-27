package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

func TestAppXinzhiliRealtimeCapabilitiesReturnsStableContract(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/app/xinzhili/realtime/capabilities", nil)

	s.appXinzhiliRealtimeCapabilities(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Code int                           `json:"code"`
		Data xinzhili.RealtimeCapabilities `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 || body.Data.PreferredVersion != xinzhili.ProtocolVersion {
		t.Fatalf("body=%s", w.Body.String())
	}
}

func TestAppXinzhiliRealtimeCapabilitiesRejectsNonGet(t *testing.T) {
	s := &Server{}
	h := s.method(http.MethodGet, s.appXinzhiliRealtimeCapabilities)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/app/xinzhili/realtime/capabilities", nil)

	h(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
