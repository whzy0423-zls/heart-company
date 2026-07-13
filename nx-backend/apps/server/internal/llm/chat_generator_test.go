package llm

import (
	"context"
	"net/http"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

type contractChatGenerator struct{}

func (contractChatGenerator) Generate(context.Context, rag.GenerateInput) (string, error) {
	return "", nil
}

func (contractChatGenerator) GenerateStream(context.Context, rag.GenerateInput, rag.StreamEmitter) (string, error) {
	return "", nil
}

func (contractChatGenerator) SummarizeConversation(context.Context, string, []rag.Message) (string, error) {
	return "", nil
}

func (contractChatGenerator) CompleteJSON(context.Context, string, string, int) (string, error) {
	return "", nil
}

func (contractChatGenerator) Ping(context.Context) PingResult {
	return PingResult{}
}

func (contractChatGenerator) PolishPrompt(context.Context, string, string) (string, error) {
	return "", nil
}

var _ ChatGenerator = contractChatGenerator{}
var _ JSONCompleter = contractChatGenerator{}

func TestChatGeneratorContractPreservesProviderConfigurationAndInjectedClient(t *testing.T) {
	t.Parallel()

	client := &http.Client{}
	timeout := 17 * time.Second
	cfg := ChatGeneratorConfig{
		Provider:     "openai-compatible",
		APIBase:      "https://example.com/v1",
		APIKey:       "secret",
		Model:        "example-model",
		SystemPrompt: "be concise",
		Timeout:      timeout,
		Client:       client,
	}

	if cfg.Provider != "openai-compatible" {
		t.Fatalf("unexpected provider: %q", cfg.Provider)
	}
	if cfg.APIBase != "https://example.com/v1" || cfg.APIKey != "secret" {
		t.Fatalf("unexpected endpoint credentials: %+v", cfg)
	}
	if cfg.Model != "example-model" || cfg.SystemPrompt != "be concise" {
		t.Fatalf("unexpected model configuration: %+v", cfg)
	}
	if cfg.Timeout != timeout {
		t.Fatalf("unexpected timeout: %s", cfg.Timeout)
	}
	if cfg.Client != client {
		t.Fatal("injected HTTP client was not preserved")
	}
}
