package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestCompatibleChatGenerateUsesOpenAIProtocol(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer openai-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "OpenAI 回答"}}},
		})
	}))
	defer server.Close()

	generator := NewCompatibleChatGenerator(config.MiniMaxConfig{
		Provider: "openai-compatible",
		APIBase:  server.URL,
		APIKey:   "openai-key",
		Model:    "gpt-5.6-sol",
	})
	generator.client = server.Client()
	answer, err := generator.Generate(context.Background(), rag.GenerateInput{Question: "你在干嘛", Tier: "basic"})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "OpenAI 回答" {
		t.Fatalf("answer = %q", answer)
	}
	if body["model"] != "gpt-5.6-sol" || body["max_completion_tokens"] != float64(220) {
		t.Fatalf("unexpected request body: %+v", body)
	}
	if _, exists := body["tokens_to_generate"]; exists {
		t.Fatalf("OpenAI request contains MiniMax token field: %+v", body)
	}
}

func TestCompatibleChatGenerateUsesAnthropicProtocol(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "anthropic-key" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("unexpected anthropic headers: %+v", r.Header)
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("anthropic request must not send bearer auth: %+v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Anthropic 回答"}},
		})
	}))
	defer server.Close()

	generator := NewCompatibleChatGenerator(config.MiniMaxConfig{
		Provider: "anthropic-compatible",
		APIBase:  server.URL + "/",
		APIKey:   "anthropic-key",
		Model:    "claude-sonnet-4-5",
	})
	generator.client = server.Client()
	answer, err := generator.Generate(context.Background(), rag.GenerateInput{Question: "请深入分析", Tier: "deep"})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Anthropic 回答" {
		t.Fatalf("answer = %q", answer)
	}
	if body["model"] != "claude-sonnet-4-5" || body["max_tokens"] != float64(700) {
		t.Fatalf("unexpected anthropic body: %+v", body)
	}
	if strings.TrimSpace(body["system"].(string)) == "" {
		t.Fatalf("anthropic request missing system prompt: %+v", body)
	}
}

func TestCompatibleChatGenerateStreamUsesOpenAIProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"呀\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	generator := NewCompatibleChatGenerator(config.MiniMaxConfig{Provider: "openai-compatible", APIBase: server.URL, APIKey: "key", Model: "gpt-4o-mini"})
	generator.client = server.Client()
	var chunks []string
	answer, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "你好"}, func(delta string) error {
		chunks = append(chunks, delta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "你好呀" || strings.Join(chunks, "") != answer {
		t.Fatalf("answer=%q chunks=%q", answer, strings.Join(chunks, ""))
	}
}

func TestCompatibleChatGenerateStreamUsesAnthropicProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"你好\"}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"呀\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	generator := NewCompatibleChatGenerator(config.MiniMaxConfig{Provider: "anthropic-compatible", APIBase: server.URL, APIKey: "key", Model: "claude-sonnet-4-5"})
	generator.client = server.Client()
	var chunks []string
	answer, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "你好"}, func(delta string) error {
		chunks = append(chunks, delta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "你好呀" || strings.Join(chunks, "") != answer {
		t.Fatalf("answer=%q chunks=%q", answer, strings.Join(chunks, ""))
	}
}

func TestCompatibleChatPingUsesSelectedProtocol(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "pong"}},
		})
	}))
	defer server.Close()

	generator := NewCompatibleChatGenerator(config.MiniMaxConfig{Provider: "anthropic-compatible", APIBase: server.URL, APIKey: "key", Model: "claude"})
	generator.client = server.Client()
	result := generator.Ping(context.Background())
	if !result.OK || gotPath != "/v1/messages" || !strings.Contains(result.Message, "Anthropic") {
		t.Fatalf("unexpected ping result: path=%q result=%+v", gotPath, result)
	}
}
