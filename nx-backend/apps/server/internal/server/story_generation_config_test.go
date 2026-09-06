package server

import (
	"testing"

	"nine-xing/nx-backend/apps/server/internal/modelconfig"
)

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
