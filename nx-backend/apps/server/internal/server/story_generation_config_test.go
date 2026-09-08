package server

import (
	"context"
	"errors"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/modelconfig"
)

type storyProbeCompleter struct {
	raw   string
	err   error
	calls int
}

func (c *storyProbeCompleter) CompleteJSON(context.Context, string, string, int) (string, error) {
	c.calls++
	return c.raw, c.err
}

func TestStoryGenerationConfigViewNeverReturnsAPIKey(t *testing.T) {
	view := buildStoryGenerationConfigView(modelconfig.StoryGenerationConfig{
		Enabled: true, Provider: modelconfig.ProviderOpenAICompatible,
		APIBase: "https://story.example.com/v1", APIKey: "secret", Model: "story-model",
		Temperature: 0.5, MaxTokens: 3000, TimeoutSeconds: 70,
	})
	if !view.APIKeySet || view.APIBase == "" || view.Model != "story-model" {
		t.Fatalf("unexpected story config view: %+v", view)
	}
}

func TestProbeStoryGenerationModelRequiresStructuredJSON(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		err    error
		wantOK bool
	}{
		{name: "valid object", raw: `{"ok":true}`, wantOK: true},
		{name: "invalid JSON", raw: "plain text"},
		{name: "provider error", err: errors.New("upstream failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completer := &storyProbeCompleter{raw: tt.raw, err: tt.err}
			result := probeStoryGenerationModel(context.Background(), completer, "https://story.example.com/v1", "story-model")
			if result.OK != tt.wantOK || completer.calls != 1 {
				t.Fatalf("result=%+v calls=%d", result, completer.calls)
			}
		})
	}
}
