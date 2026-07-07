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

func TestSpugSenderPostsTemplateParams(t *testing.T) {
	var gotPath string
	var gotContentType string
	var gotBody map[string]any
	requests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok"}`))
	}))
	defer server.Close()

	sender, err := NewSpugSender(SpugOptions{
		APIBase:      server.URL + "/",
		TemplateCode: "tmpl123",
		TemplateName: "芯之力",
		Timeout:      time.Second,
	})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}

	if err := sender.Send(context.Background(), "13800000000", "123456"); err != nil {
		t.Fatalf("send: %v", err)
	}

	if requests != 1 {
		t.Fatalf("expected one request, got %d", requests)
	}
	if gotPath != "/send/tmpl123" {
		t.Fatalf("expected /send/tmpl123, got %q", gotPath)
	}
	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Fatalf("expected json content type, got %q", gotContentType)
	}
	if gotBody["name"] != "芯之力" || gotBody["code"] != "123456" || gotBody["targets"] != "13800000000" {
		t.Fatalf("unexpected body: %#v", gotBody)
	}
}

func TestSpugSenderReturnsErrorForProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "余额不足", http.StatusPaymentRequired)
	}))
	defer server.Close()

	sender, err := NewSpugSender(SpugOptions{
		APIBase:      server.URL,
		TemplateCode: "tmpl123",
		TemplateName: "芯之力",
		Timeout:      time.Second,
	})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}

	err = sender.Send(context.Background(), "13800000000", "123456")
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if !strings.Contains(err.Error(), "402") || !strings.Contains(err.Error(), "余额不足") {
		t.Fatalf("expected status and provider body in error, got %v", err)
	}
}

func TestNewSpugSenderRequiresTemplateCode(t *testing.T) {
	_, err := NewSpugSender(SpugOptions{
		APIBase:      "https://push.spug.cc",
		TemplateName: "芯之力",
	})
	if err == nil {
		t.Fatal("expected missing template code error")
	}
}
