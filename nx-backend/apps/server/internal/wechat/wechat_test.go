package wechat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCode2SessionReturnsHTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient("appid", "secret", false)
	client.apiBase = server.URL

	_, err := client.Code2Session(context.Background(), "code")
	if err == nil {
		t.Fatal("expected status error")
	}
	if !strings.Contains(err.Error(), "微信登录请求失败(502)") {
		t.Fatalf("unexpected error: %v", err)
	}
}
