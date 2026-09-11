package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

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

func TestCompatibleChatExplicitOutputBudgetUsesProviderPayloadForSyncAndStream(t *testing.T) {
	var budgets []float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		budget, _ := body["max_completion_tokens"].(float64)
		budgets = append(budgets, budget)
		if body["stream"] == true {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()

	generator := NewCompatibleChatGenerator(config.MiniMaxConfig{
		Provider: "openai-compatible",
		APIBase:  server.URL,
		APIKey:   "test-key",
		Model:    "gpt-5.6-sol",
	})
	generator.client = server.Client()
	explicit := rag.GenerateInput{Question: "1 2 3 4 这些型号的反馈", MaxOutputTokens: 1880}
	ordinary := rag.GenerateInput{Question: "你好", Tier: "basic"}
	allTypes := rag.GenerateInput{Question: "介绍1到9型号的分别解释"}
	if _, err := generator.Generate(context.Background(), explicit); err != nil {
		t.Fatalf("explicit sync: %v", err)
	}
	if _, err := generator.GenerateStream(context.Background(), explicit, nil); err != nil {
		t.Fatalf("explicit stream: %v", err)
	}
	if _, err := generator.Generate(context.Background(), ordinary); err != nil {
		t.Fatalf("ordinary sync: %v", err)
	}
	if _, err := generator.GenerateStream(context.Background(), allTypes, nil); err != nil {
		t.Fatalf("adaptive stream: %v", err)
	}
	if want := []float64{1880, 1880, 220, 1200}; !slices.Equal(budgets, want) {
		t.Fatalf("provider budgets = %v, want %v", budgets, want)
	}
}

func TestCompatibleChatExplicitCompletionTimeoutAppliesToSyncAndStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		time.Sleep(120 * time.Millisecond)
		if body["stream"] == true {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"late\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"late"}}]}`)
	}))
	defer server.Close()

	generator := NewCompatibleChatGenerator(config.MiniMaxConfig{
		Provider: "openai-compatible",
		APIBase:  server.URL,
		APIKey:   "test-key",
		Model:    "gpt-4o-mini",
	})
	generator.client = server.Client()
	input := rag.GenerateInput{Question: "test", CompletionTimeout: 20 * time.Millisecond}
	assertExplicitCompletionTimeout(t, "sync", func() error {
		_, err := generator.Generate(context.Background(), input)
		return err
	})
	assertExplicitCompletionTimeout(t, "stream", func() error {
		_, err := generator.GenerateStream(context.Background(), input, nil)
		return err
	})

	ordinaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] == true {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"last\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		time.Sleep(60 * time.Millisecond)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"late"}}]}`)
	}))
	defer ordinaryServer.Close()
	ordinaryGenerator := NewCompatibleChatGenerator(config.MiniMaxConfig{Provider: "openai-compatible", APIBase: ordinaryServer.URL, APIKey: "test-key", Model: "gpt-4o-mini"})
	ordinaryGenerator.client = ordinaryServer.Client()
	ordinaryGenerator.client.Timeout = 20 * time.Millisecond
	assertConfiguredCompletionTimeout(t, "sync", func() error {
		_, err := ordinaryGenerator.Generate(context.Background(), rag.GenerateInput{Question: "test"})
		return err
	})
	answer, err := ordinaryGenerator.GenerateStream(context.Background(), rag.GenerateInput{Question: "test"}, nil)
	if err != nil || answer != "firstlast" {
		t.Fatalf("zero-timeout stream answer/error = %q/%v", answer, err)
	}
	if ordinaryGenerator.client.Timeout != 20*time.Millisecond {
		t.Fatalf("configured client timeout mutated to %v", ordinaryGenerator.client.Timeout)
	}
}

func TestChatRequestClientExplicitCompletionTimeoutClonesAndPreservesConfiguration(t *testing.T) {
	transport := http.DefaultTransport
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	cookieURL, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(cookieURL, []*http.Cookie{{Name: "session", Value: "kept"}})
	redirectErr := errors.New("redirect blocked")
	configured := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return redirectErr
		},
		Jar:     jar,
		Timeout: 9 * time.Second,
	}

	if syncClient := chatRequestClient(configured, 0, false); syncClient != configured {
		t.Fatal("zero-timeout sync must use the configured client")
	}
	streamClient := chatRequestClient(configured, 0, true)
	if streamClient == configured || streamClient.Timeout != 0 {
		t.Fatalf("zero-timeout stream client = %#v, want a clone with no total timeout", streamClient)
	}
	assertClonedChatClientConfiguration(t, configured, streamClient, cookieURL, redirectErr)

	for _, stream := range []bool{false, true} {
		client := chatRequestClient(configured, 70*time.Second, stream)
		if client == configured || client.Timeout != 70*time.Second {
			t.Fatalf("explicit client(stream=%v) = %#v, want clone with 70s timeout", stream, client)
		}
		assertClonedChatClientConfiguration(t, configured, client, cookieURL, redirectErr)
	}
	if configured.Timeout != 9*time.Second {
		t.Fatalf("configured client timeout mutated to %v", configured.Timeout)
	}
}

func assertClonedChatClientConfiguration(t *testing.T, configured, clone *http.Client, cookieURL *url.URL, redirectErr error) {
	t.Helper()
	if clone.Transport != configured.Transport {
		t.Fatal("request client clone did not preserve transport")
	}
	if clone.CheckRedirect == nil || !errors.Is(clone.CheckRedirect(&http.Request{}, nil), redirectErr) {
		t.Fatal("request client clone did not preserve redirect behavior")
	}
	if configured.Jar == nil || clone.Jar != configured.Jar {
		t.Fatal("request client clone did not preserve cookie jar")
	}
	if cookies := clone.Jar.Cookies(cookieURL); len(cookies) != 1 || cookies[0].Name != "session" || cookies[0].Value != "kept" {
		t.Fatalf("request client clone cookie state = %#v", cookies)
	}
}

func assertExplicitCompletionTimeout(t *testing.T, mode string, call func() error) {
	t.Helper()
	started := time.Now()
	err := call()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%s error = %v, want deadline exceeded", mode, err)
	}
	if elapsed := time.Since(started); elapsed >= 100*time.Millisecond {
		t.Fatalf("%s elapsed = %v, explicit timeout was not applied", mode, elapsed)
	}
}

func assertConfiguredCompletionTimeout(t *testing.T, mode string, call func() error) {
	t.Helper()
	if err := call(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("zero-override %s error = %v, want configured client deadline", mode, err)
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
