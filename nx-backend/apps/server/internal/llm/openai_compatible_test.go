package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStructuredJSONClientOpenAICompatibleSendsJSONRequest(t *testing.T) {
	var gotPath string
	var gotAuth string
	var requestBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": `{"questions":[{"body":"九型人格校准题","dimension":"motivation","options":[{"id":"A","label":"A","text":"追求正确","typeWeights":{"1":2}}]}]}`}}},
		})
	}))
	defer upstream.Close()

	client := NewStructuredJSONClient(StructuredJSONConfig{
		Provider: "openai-compatible",
		APIBase:  upstream.URL,
		APIKey:   "test-key",
		Model:    "gpt-5.5-mini",
		Timeout:  3 * time.Second,
		Client:   upstream.Client(),
	})
	result, err := client.GenerateJSON(context.Background(), StructuredJSONRequest{
		SystemPrompt: "只返回 JSON",
		UserPrompt:   "生成九型人格每日题",
		MaxTokens:    900,
		Temperature:  0.2,
	})
	if err != nil {
		t.Fatalf("GenerateJSON returned error: %v", err)
	}

	if gotPath != "/v1/chat/completions" {
		t.Fatalf("expected openai-compatible chat completions path, got %q", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("expected bearer auth, got %q", gotAuth)
	}
	body, _ := json.Marshal(requestBody)
	if !strings.Contains(string(body), "只返回 JSON") || !strings.Contains(string(body), "生成九型人格每日题") {
		t.Fatalf("expected prompts in request body, got %s", string(body))
	}
	if requestBody["model"] != "gpt-5.5-mini" {
		t.Fatalf("expected configured model, got %+v", requestBody["model"])
	}
	if result.Provider != "openai-compatible" || result.Model != "gpt-5.5-mini" || !strings.Contains(result.Content, "九型人格校准题") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestStructuredJSONClientRejectsAnthropicProviderUntilImplemented(t *testing.T) {
	client := NewStructuredJSONClient(StructuredJSONConfig{Provider: "anthropic-compatible", APIKey: "key", Model: "claude"})
	_, err := client.GenerateJSON(context.Background(), StructuredJSONRequest{UserPrompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "anthropic-compatible") {
		t.Fatalf("expected unsupported anthropic-compatible error, got %v", err)
	}
}
