package sms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSpugReporterPostsMessageToConfiguredWebhook(t *testing.T) {
	var gotPath string
	var payload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request method/content type: %s/%s", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"请求成功"}`))
	}))
	defer server.Close()

	reporter, err := NewSpugReporter(SpugReporterOptions{
		APIBase: server.URL, Token: "token-123", Path: "/xsend", Channel: "webhook", Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new reporter: %v", err)
	}
	if err := reporter.Report(context.Background(), "标题", "手机号 138****0000", "text", ""); err != nil {
		t.Fatalf("report: %v", err)
	}
	if gotPath != "/xsend/token-123" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if payload["title"] != "标题" || payload["content"] != "手机号 138****0000" || payload["type"] != "text" || payload["channel"] != "webhook" {
		t.Fatalf("unexpected payload %#v", payload)
	}
}

func TestSpugReporterTreatsBusinessFailureAsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":400,"msg":"凭证无效"}`))
	}))
	defer server.Close()
	reporter, err := NewSpugReporter(SpugReporterOptions{APIBase: server.URL, Token: "token"})
	if err != nil {
		t.Fatalf("new reporter: %v", err)
	}
	err = reporter.Report(context.Background(), "标题", "内容", "text", "")
	if err == nil || !strings.Contains(err.Error(), "code=400") {
		t.Fatalf("expected business error, got %v", err)
	}
}

func TestSpugReporterRequiresToken(t *testing.T) {
	if _, err := NewSpugReporter(SpugReporterOptions{APIBase: "https://push.spug.cc"}); err == nil {
		t.Fatal("expected missing token error")
	}
}
