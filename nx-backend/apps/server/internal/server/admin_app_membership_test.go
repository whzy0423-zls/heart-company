package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminAppOrderGrantRequiresActivationTime(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/app-orders/7/grant", strings.NewReader(`{}`))
	res := httptest.NewRecorder()

	s.adminAppOrderGrant(res, req)

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "生效时间") {
		t.Fatalf("expected activation time error, got %d %s", res.Code, res.Body.String())
	}
}
