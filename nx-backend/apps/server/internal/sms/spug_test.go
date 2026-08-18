package sms

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSpugSenderSendsSMSCodeQuery(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotTo string
	var gotCode string
	var gotNumber string
	requests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotTo = r.URL.Query().Get("to")
		gotCode = r.URL.Query().Get("code")
		gotNumber = r.URL.Query().Get("number")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"请求成功"}`))
	}))
	defer server.Close()

	sender, err := NewSpugSender(SpugOptions{
		APIBase:        server.URL + "/",
		TemplateCode:   "tmpl123",
		TemplateName:   "芯之力",
		CodeTTLMinutes: 10,
		Timeout:        time.Second,
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
	if gotMethod != http.MethodGet {
		t.Fatalf("expected GET, got %q", gotMethod)
	}
	if gotPath != "/sms/tmpl123" {
		t.Fatalf("expected /sms/tmpl123, got %q", gotPath)
	}
	if gotTo != "13800000000" || gotCode != "123456" || gotNumber != "10" {
		t.Fatalf("unexpected query: to=%q code=%q number=%q", gotTo, gotCode, gotNumber)
	}
}

func TestSpugSenderReturnsErrorForBusinessFailureResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":404,"msg":"模板编码错误"}`))
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
		t.Fatal("expected business failure")
	}
	if !strings.Contains(err.Error(), "code=404") || !strings.Contains(err.Error(), "模板编码错误") {
		t.Fatalf("expected provider response details in error, got %v", err)
	}
}

func TestSpugSenderReturnsErrorForMalformedSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
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
		t.Fatal("expected malformed success response to fail")
	}
	if !strings.Contains(err.Error(), "invalid spug response") {
		t.Fatalf("expected invalid response error, got %v", err)
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
