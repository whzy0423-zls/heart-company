package knowledgeclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRetrieveSerializesContractAndAuthentication(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/internal/v1/retrieve" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"requestId":"req-1","documents":[],"trace":{"retrievalMethod":"fixture","candidateCount":0,"returnedCount":0}}`)
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, Token: "TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Retrieve(context.Background(), RetrievalRequest{
		RequestID: "req-1",
		Query:     "用户问题",
		Scene:     "app_chat",
		Scope:     Scope{Public: true, TheoryReleaseIDs: []int64{101}},
		Retrieval: RetrievalOptions{TopK: 8},
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotAuth != "Bearer TOKEN" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["requestId"] != "req-1" || gotBody["query"] != "用户问题" {
		t.Fatalf("unexpected body: %#v", gotBody)
	}
	if response.RequestID != "req-1" || response.Trace.RetrievalMethod != "fixture" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestRetrieveRejectsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad scope", http.StatusBadRequest)
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL, Token: "TOKEN"})

	_, err := client.Retrieve(context.Background(), RetrievalRequest{})
	var responseErr *ResponseError
	if !errors.As(err, &responseErr) || responseErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 ResponseError, got %v", err)
	}
}

func TestRetrieveEnforcesResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, strings.Repeat("x", 65))
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL, Token: "TOKEN", MaxResponseBytes: 64})

	_, err := client.Retrieve(context.Background(), RetrievalRequest{})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
}

func TestRetrieveHonorsHTTPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	client, _ := New(Config{
		BaseURL:    server.URL,
		Token:      "TOKEN",
		HTTPClient: &http.Client{Timeout: 10 * time.Millisecond},
	})

	_, err := client.Retrieve(context.Background(), RetrievalRequest{})
	if err == nil {
		t.Fatal("expected timeout")
	}
}

func TestHealthAndGenerateUseInternalEndpoints(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health/ready":
			fmt.Fprint(w, `{"status":"ready"}`)
		case "/internal/v1/answer":
			fmt.Fprint(w, `{"requestId":"req-1","answer":"你好","citations":[],"traceId":"trace-1"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL, Token: "TOKEN"})

	if err := client.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	answer, err := client.Generate(context.Background(), AnswerRequest{RequestID: "req-1"})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Answer != "你好" || answer.TraceID != "trace-1" {
		t.Fatalf("unexpected answer: %#v", answer)
	}
	if strings.Join(paths, ",") != "/health/ready,/internal/v1/answer" {
		t.Fatalf("unexpected paths %v", paths)
	}
}

func TestStreamAnswerParsesAllEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: retrieval_started\ndata: {\"requestId\":\"req-1\"}\n\n")
		fmt.Fprint(w, "event: retrieval_done\ndata: {\"documentCount\":2}\n\n")
		fmt.Fprint(w, "event: token\ndata: {\"text\":\"你好\"}\n\n")
		fmt.Fprint(w, "event: citations\ndata: {\"citations\":[]}\n\n")
		fmt.Fprint(w, "event: done\ndata: {\"traceId\":\"trace-1\"}\n\n")
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL, Token: "TOKEN"})

	events, errs := client.StreamAnswer(context.Background(), AnswerRequest{RequestID: "req-1"})
	var names []string
	for event := range events {
		names = append(names, event.Type)
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "retrieval_started,retrieval_done,token,citations,done" {
		t.Fatalf("unexpected events %v", names)
	}
}

func TestStreamAnswerReturnsErrorEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: error\ndata: {\"code\":\"upstream_error\",\"message\":\"failed\"}\n\n")
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL, Token: "TOKEN"})

	events, errs := client.StreamAnswer(context.Background(), AnswerRequest{})
	for range events {
	}
	var streamErr *StreamError
	if err := <-errs; !errors.As(err, &streamErr) || streamErr.Code != "upstream_error" {
		t.Fatalf("expected StreamError, got %v", err)
	}
}

func TestStreamAnswerReportsTruncatedStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: token\ndata: {\"text\":\"partial\"}\n\n")
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL, Token: "TOKEN"})

	events, errs := client.StreamAnswer(context.Background(), AnswerRequest{})
	for range events {
	}
	if err := <-errs; !errors.Is(err, ErrStreamInterrupted) {
		t.Fatalf("expected ErrStreamInterrupted, got %v", err)
	}
}

func TestStreamAnswerEnforcesIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: retrieval_started\ndata: {}\n\n")
		flusher.Flush()
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL, Token: "TOKEN", StreamIdleTimeout: 20 * time.Millisecond})

	events, errs := client.StreamAnswer(context.Background(), AnswerRequest{})
	for range events {
	}
	if err := <-errs; !errors.Is(err, ErrStreamIdleTimeout) {
		t.Fatalf("expected ErrStreamIdleTimeout, got %v", err)
	}
}
