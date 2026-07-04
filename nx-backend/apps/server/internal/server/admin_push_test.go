package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminPushSendRejectsOversizedBody(t *testing.T) {
	body := `{"title":"` + strings.Repeat("a", 9000) + `","content":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/push/send", strings.NewReader(body))
	res := httptest.NewRecorder()

	s := &Server{}
	s.adminPushSend(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected oversized push body to be rejected, got %d", res.Code)
	}
}

func TestAdminPushSendRejectsUnknownMemberLevel(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/push/send", strings.NewReader(`{
		"title":"会员推送",
		"content":"test",
		"targetType":"level",
		"targetValue":"gold"
	}`))
	res := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected unknown member level to return 400, panicked: %v", recovered)
		}
	}()

	s := &Server{}
	s.adminPushSend(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown member level to be rejected, got %d body=%s", res.Code, res.Body.String())
	}
}
