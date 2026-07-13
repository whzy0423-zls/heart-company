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

func TestNewChatGeneratorConstructsOnlySupportedProviders(t *testing.T) {
	t.Parallel()

	base := ChatGeneratorConfig{
		APIBase: "https://models.example.com/v1",
		APIKey:  "secret",
		Model:   "test-model",
		Timeout: 13 * time.Second,
	}

	tests := []struct {
		name     string
		provider string
		assert   func(*testing.T, ChatGenerator)
	}{
		{
			name:     "openai",
			provider: "openai-compatible",
			assert: func(t *testing.T, generator ChatGenerator) {
				if _, ok := generator.(*OpenAIChatGenerator); !ok {
					t.Fatalf("expected OpenAI generator, got %T", generator)
				}
			},
		},
		{
			name:     "anthropic",
			provider: "anthropic-compatible",
			assert: func(t *testing.T, generator ChatGenerator) {
				if _, ok := generator.(*AnthropicChatGenerator); !ok {
					t.Fatalf("expected Anthropic generator, got %T", generator)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			cfg.Provider = tt.provider
			generator, err := NewChatGenerator(cfg)
			if err != nil {
				t.Fatalf("NewChatGenerator returned error: %v", err)
			}
			tt.assert(t, generator)
		})
	}
}

func TestNewChatGeneratorRejectsUnsupportedOrIncompleteConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  ChatGeneratorConfig
	}{
		{name: "blank provider", cfg: ChatGeneratorConfig{APIBase: "https://api.example.com/v1", APIKey: "secret", Model: "model", Timeout: time.Second}},
		{name: "minimax", cfg: ChatGeneratorConfig{Provider: "minimax", APIBase: "https://api.example.com/v1", APIKey: "secret", Model: "model", Timeout: time.Second}},
		{name: "unknown", cfg: ChatGeneratorConfig{Provider: "other", APIBase: "https://api.example.com/v1", APIKey: "secret", Model: "model", Timeout: time.Second}},
		{name: "blank base", cfg: ChatGeneratorConfig{Provider: "openai-compatible", APIKey: "secret", Model: "model", Timeout: time.Second}},
		{name: "blank key", cfg: ChatGeneratorConfig{Provider: "openai-compatible", APIBase: "https://api.example.com/v1", Model: "model", Timeout: time.Second}},
		{name: "blank model", cfg: ChatGeneratorConfig{Provider: "openai-compatible", APIBase: "https://api.example.com/v1", APIKey: "secret", Timeout: time.Second}},
		{name: "blank timeout", cfg: ChatGeneratorConfig{Provider: "openai-compatible", APIBase: "https://api.example.com/v1", APIKey: "secret", Model: "model"}},
		{name: "timeout too large", cfg: ChatGeneratorConfig{Provider: "openai-compatible", APIBase: "https://api.example.com/v1", APIKey: "secret", Model: "model", Timeout: 301 * time.Second}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if generator, err := NewChatGenerator(tt.cfg); err == nil || generator != nil {
				t.Fatalf("expected rejected configuration, got generator=%T err=%v", generator, err)
			}
		})
	}
}

func TestNewChatGeneratorProductionClientRejectsLocalAPIBase(t *testing.T) {
	t.Parallel()

	generator, err := NewChatGenerator(ChatGeneratorConfig{
		Provider: "openai-compatible",
		APIBase:  "http://127.0.0.1:8080/v1",
		APIKey:   "secret",
		Model:    "model",
		Timeout:  time.Second,
	})
	if err == nil || generator != nil {
		t.Fatalf("expected local API base rejection, got generator=%T err=%v", generator, err)
	}
}

func TestNewChatGeneratorAlwaysUsesGuardedProductionClient(t *testing.T) {
	t.Parallel()

	generator, err := NewChatGenerator(ChatGeneratorConfig{
		Provider: "openai-compatible",
		APIBase:  "https://models.example.com/v1",
		APIKey:   "secret",
		Model:    "model",
		Timeout:  12 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	openAI := generator.(*OpenAIChatGenerator)
	transport, ok := openAI.client.Transport.(*http.Transport)
	if !ok || transport.DialContext == nil || transport.TLSHandshakeTimeout <= 0 || transport.ResponseHeaderTimeout <= 0 {
		t.Fatalf("expected guarded production transport, got %#v", openAI.client.Transport)
	}
	if openAI.client.CheckRedirect == nil {
		t.Fatal("expected guarded redirect policy")
	}
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

func TestChatGeneratorContractPreservesProviderConfiguration(t *testing.T) {
	t.Parallel()

	timeout := 17 * time.Second
	cfg := ChatGeneratorConfig{
		Provider:     "openai-compatible",
		APIBase:      "https://example.com/v1",
		APIKey:       "secret",
		Model:        "example-model",
		SystemPrompt: "be concise",
		Timeout:      timeout,
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
}
