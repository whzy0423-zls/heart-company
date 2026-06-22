package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestMiniMaxGeneratorSendsRAGContext(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing authorization header: %s", r.Header.Get("Authorization"))
		}
		if !strings.Contains(r.URL.RawQuery, "GroupId=test-group") {
			t.Fatalf("missing group id in query: %s", r.URL.RawQuery)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": "模型回答"},
				},
			},
		})
	}))
	defer server.Close()

	generator := NewMiniMaxGenerator(config.MiniMaxConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
		GroupID: "test-group",
	})
	answer, err := generator.Generate(context.Background(), rag.GenerateInput{
		Question: "完美型怎么成长？",
		UserProfile: rag.UserProfile{
			Nickname: "小九",
			MainType: 1,
		},
		Sources: []rag.Source{{Title: "1号 完美型", Snippet: "允许不完美"}},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if answer != "模型回答" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	messages, _ := requestBody["messages"].([]any)
	if len(messages) < 2 {
		t.Fatalf("expected messages in request: %+v", requestBody)
	}
	body, _ := json.Marshal(requestBody)
	if !strings.Contains(string(body), "允许不完美") || !strings.Contains(string(body), "完美型怎么成长") {
		t.Fatalf("request did not include rag context/question: %s", string(body))
	}
}

func TestMiniMaxGeneratorRequiresAPIKey(t *testing.T) {
	_, err := NewMiniMaxGenerator(config.MiniMaxConfig{}).Generate(context.Background(), rag.GenerateInput{Question: "hi"})
	if err == nil {
		t.Fatal("expected missing api key error")
	}
}

func TestMiniMaxGeneratorUsesConfiguredTimeout(t *testing.T) {
	generator := NewMiniMaxGenerator(config.MiniMaxConfig{TimeoutSeconds: 12})
	if generator.client.Timeout.String() != "12s" {
		t.Fatalf("expected configured timeout, got %s", generator.client.Timeout)
	}

	defaultGenerator := NewMiniMaxGenerator(config.MiniMaxConfig{})
	if defaultGenerator.client.Timeout.String() != "25s" {
		t.Fatalf("expected 25s default timeout, got %s", defaultGenerator.client.Timeout)
	}
}
