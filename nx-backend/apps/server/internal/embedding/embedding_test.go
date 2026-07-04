package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbedBatchRejectsLocalAPIBaseBeforeDial(t *testing.T) {
	sawRequest := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sawRequest = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{
				map[string]any{"embedding": []float32{0.1, 0.2}},
			},
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		Provider:  "openai-compatible",
		APIBase:   server.URL,
		APIKey:    "test-key",
		Model:     "text-embedding",
		Dimension: 2,
	})

	_, err := client.EmbedBatch(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected local embedding API base to be rejected")
	}
	if !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("expected private/local address error, got %v", err)
	}
	if sawRequest {
		t.Fatal("local embedding API base must be rejected before sending the request")
	}
}
