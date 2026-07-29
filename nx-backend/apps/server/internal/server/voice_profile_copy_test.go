package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCopyProfileToBailianRouteRequiresProfileManagePermission(t *testing.T) {
	server := newRouteOnlyServer()
	req := httptest.NewRequest(http.MethodPost, "/api/voice/profiles/42/copy-to-bailian", nil)
	res := httptest.NewRecorder()
	server.mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("copy route must stop unauthenticated requests at the profile permission guard: status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestCopyProfileToBailianRouteOnlyAcceptsPost(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/voice/profiles/42/copy-to-bailian", nil)
	res := httptest.NewRecorder()
	server.voiceProfileByID(res, req)
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET copy route status=%d body=%s", res.Code, res.Body.String())
	}
}
