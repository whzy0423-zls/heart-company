package server

import (
	"encoding/json"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/modelconfig"
)

func TestModelConfigSavedChatNeverEmitsLegacyGroupID(t *testing.T) {
	cfg := modelconfig.Config{Chat: modelconfig.ChatConfig{
		Provider:       modelconfig.ProviderOpenAICompatible,
		APIBase:        "https://api.openai.com/v1",
		APIKey:         "secret",
		GroupID:        "must-not-leak",
		Model:          "gpt-5.5",
		TimeoutSeconds: 30,
	}}

	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]map[string]any
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatal(err)
	}
	if _, ok := saved["chat"]["groupId"]; ok {
		t.Fatalf("expected saved chat config to omit legacy groupId, got %s", body)
	}
}
