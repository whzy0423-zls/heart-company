package modelconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestChatConfigNormalizedTrimsCompatibleFields(t *testing.T) {
	cfg := ChatConfig{
		Provider:       " OPENAI-COMPATIBLE ",
		APIBase:        " https://gateway.example.com/v1/ ",
		APIKey:         " secret ",
		Model:          " gpt-5.5 ",
		TimeoutSeconds: 45,
	}

	got := cfg.Normalized()

	if got.Provider != ProviderOpenAICompatible {
		t.Fatalf("expected normalized provider %q, got %q", ProviderOpenAICompatible, got.Provider)
	}
	if got.APIBase != "https://gateway.example.com/v1" || got.APIKey != "secret" || got.Model != "gpt-5.5" {
		t.Fatalf("expected normalized chat fields, got %+v", got)
	}
	if got.TimeoutSeconds != 45 {
		t.Fatalf("expected timeout to remain 45, got %d", got.TimeoutSeconds)
	}
}

func TestChatConfigValidateAcceptsOnlyCompatibleProviders(t *testing.T) {
	valid := ChatConfig{
		APIBase:        "https://gateway.example.com/v1",
		APIKey:         "secret",
		Model:          "model",
		TimeoutSeconds: 30,
	}

	for _, provider := range []string{ProviderOpenAICompatible, ProviderAnthropicCompatible} {
		t.Run(provider, func(t *testing.T) {
			cfg := valid
			cfg.Provider = provider
			if err := cfg.Validate(); err != nil {
				t.Fatalf("expected provider %q to validate, got %v", provider, err)
			}
		})
	}

	for _, provider := range []string{"", "minimax", "openai", "unknown"} {
		t.Run("reject_"+provider, func(t *testing.T) {
			cfg := valid
			cfg.Provider = provider
			if err := cfg.Validate(); err == nil {
				t.Fatalf("expected provider %q to be rejected", provider)
			}
		})
	}
}

func TestChatConfigValidateRequiresCompleteConnectionFields(t *testing.T) {
	valid := ChatConfig{
		Provider:       ProviderOpenAICompatible,
		APIBase:        "https://gateway.example.com/v1",
		APIKey:         "secret",
		Model:          "model",
		TimeoutSeconds: 30,
	}

	tests := []struct {
		name   string
		mutate func(*ChatConfig)
		want   string
	}{
		{name: "api base", mutate: func(cfg *ChatConfig) { cfg.APIBase = " " }, want: "apiBase"},
		{name: "api key", mutate: func(cfg *ChatConfig) { cfg.APIKey = " " }, want: "apiKey"},
		{name: "model", mutate: func(cfg *ChatConfig) { cfg.Model = " " }, want: "model"},
		{name: "timeout", mutate: func(cfg *ChatConfig) { cfg.TimeoutSeconds = 0 }, want: "timeoutSeconds"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s validation error, got %v", tc.want, err)
			}
		})
	}
}

func TestChatConfigJSONContainsCompatibleContractWithoutGroupID(t *testing.T) {
	body, err := json.Marshal(ChatConfig{
		Provider:       ProviderAnthropicCompatible,
		APIBase:        "https://api.anthropic.com/v1",
		APIKey:         "secret",
		Model:          "claude-sonnet",
		TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"provider", "apiBase", "apiKey", "model", "timeoutSeconds"} {
		if _, ok := got[field]; !ok {
			t.Fatalf("expected chat JSON field %q in %s", field, body)
		}
	}
	if _, ok := got["groupId"]; ok {
		t.Fatalf("expected chat JSON to omit legacy groupId, got %s", body)
	}
}

func TestEffectiveChatUsesOnlyStoredCompatibleConfig(t *testing.T) {
	cfg := Config{Chat: ChatConfig{
		Provider:       " anthropic-compatible ",
		APIBase:        " https://api.anthropic.com/v1/ ",
		APIKey:         " secret ",
		Model:          " claude-sonnet ",
		TimeoutSeconds: 60,
	}}

	got := cfg.EffectiveChat()

	if got.Provider != ProviderAnthropicCompatible || got.APIBase != "https://api.anthropic.com/v1" || got.APIKey != "secret" || got.Model != "claude-sonnet" || got.TimeoutSeconds != 60 {
		t.Fatalf("expected effective chat to be the normalized stored compatible config, got %+v", got)
	}
}

func TestMergeIncomingChatKeyRetentionDependsOnProviderIdentity(t *testing.T) {
	t.Parallel()

	stored := Config{Chat: ChatConfig{
		Provider:       ProviderOpenAICompatible,
		APIBase:        "https://api.openai.com/v1",
		APIKey:         "openai-secret",
		Model:          "gpt-model",
		TimeoutSeconds: 30,
	}}

	unchanged := stored.MergeIncoming(Config{Chat: ChatConfig{
		Provider:       ProviderOpenAICompatible,
		APIBase:        "https://api.openai.com/v1",
		Model:          "gpt-model",
		TimeoutSeconds: 30,
	}})
	if unchanged.Chat.APIKey != "openai-secret" {
		t.Fatalf("expected unchanged provider to retain its key, got %q", unchanged.Chat.APIKey)
	}

	changed := stored.MergeIncoming(Config{Chat: ChatConfig{
		Provider:       ProviderAnthropicCompatible,
		APIBase:        "https://api.anthropic.com/v1",
		Model:          "claude-model",
		TimeoutSeconds: 30,
	}})
	if changed.Chat.APIKey != "" {
		t.Fatalf("expected provider change to require a new key, got %q", changed.Chat.APIKey)
	}
}

func TestMergeIncomingPartialPagePreservesOmittedChatAssist(t *testing.T) {
	t.Parallel()

	enabled := true
	stored := Config{
		Chat: ChatConfig{
			Provider:       ProviderOpenAICompatible,
			APIBase:        "https://api.openai.com/v1",
			APIKey:         "secret",
			Model:          "gpt-model",
			TimeoutSeconds: 30,
		},
		Assist: AssistConfig{Enabled: &enabled, SystemPrompt: "保持简洁"},
	}

	merged := stored.MergeIncoming(Config{Video: VideoConfig{Model: "new-video-model"}})
	if merged.Chat != stored.Chat.Normalized() {
		t.Fatalf("omitted chat was changed: %+v", merged.Chat)
	}
	if merged.Assist.SystemPrompt != "保持简洁" || merged.Assist.Enabled == nil || !*merged.Assist.Enabled {
		t.Fatalf("omitted assist was changed: %+v", merged.Assist)
	}
}

func TestApplyAnalysisUsesVoiceMiniMaxCredentialsAndDefaultM3(t *testing.T) {
	voiceBase := config.MiniMaxConfig{
		APIBase:        "https://api.minimaxi.com",
		APIKey:         "voice-key",
		GroupID:        "voice-group",
		Model:          "abab6.5s-chat",
		TimeoutSeconds: 77,
	}
	cfg := Config{
		Chat: ChatConfig{
			APIBase: "https://coding-play.codes",
			APIKey:  "chat-key",
			Model:   "gpt-5.5",
		},
		Analysis: AnalysisConfig{
			APIBase: "https://old-analysis.example",
			APIKey:  "old-analysis-key",
			GroupID: "old-analysis-group",
		},
	}

	got := cfg.ApplyAnalysis(voiceBase)

	if got.APIBase != voiceBase.APIBase || got.APIKey != voiceBase.APIKey || got.GroupID != voiceBase.GroupID {
		t.Fatalf("expected analysis to reuse voice MiniMax credentials, got %+v", got)
	}
	if got.Model != DefaultAnalysisModel {
		t.Fatalf("expected default analysis model %q, got %q", DefaultAnalysisModel, got.Model)
	}
	if got.TimeoutSeconds != DefaultAnalysisTimeoutSeconds {
		t.Fatalf("expected analysis timeout %d, got %d", DefaultAnalysisTimeoutSeconds, got.TimeoutSeconds)
	}
}

func TestApplyAnalysisAllowsModelOverrideOnly(t *testing.T) {
	voiceBase := config.MiniMaxConfig{
		APIBase: "https://api.minimaxi.com",
		APIKey:  "voice-key",
		GroupID: "voice-group",
		Model:   "abab6.5s-chat",
	}
	cfg := Config{
		Analysis: AnalysisConfig{
			APIBase: "https://old-analysis.example",
			APIKey:  "old-analysis-key",
			GroupID: "old-analysis-group",
			Model:   "MiniMax-M3-Preview",
		},
	}

	got := cfg.ApplyAnalysis(voiceBase)

	if got.APIBase != voiceBase.APIBase || got.APIKey != voiceBase.APIKey || got.GroupID != voiceBase.GroupID {
		t.Fatalf("expected only analysis model to override voice credentials, got %+v", got)
	}
	if got.Model != "MiniMax-M3-Preview" {
		t.Fatalf("expected model override, got %q", got.Model)
	}
}

func TestApplyAnalysisIgnoresStaleNonMiniMaxModel(t *testing.T) {
	voiceBase := config.MiniMaxConfig{
		APIBase: "https://api.minimaxi.com",
		APIKey:  "voice-key",
		GroupID: "voice-group",
		Model:   "abab6.5s-chat",
	}
	cfg := Config{
		Analysis: AnalysisConfig{Model: "gpt-5.5"},
	}

	got := cfg.ApplyAnalysis(voiceBase)

	if got.Model != DefaultAnalysisModel {
		t.Fatalf("expected stale non-MiniMax model to fall back to %q, got %q", DefaultAnalysisModel, got.Model)
	}
}

func TestApplyDailyQuizInheritsAdminCompatibleModelConfig(t *testing.T) {
	cfg := Config{
		Admin: CompatibleModelConfig{
			Provider:       " openai-compatible ",
			APIBase:        " https://gateway.example.com/v1 ",
			APIKey:         " admin-key ",
			Model:          " gpt-5.5 ",
			TimeoutSeconds: 31,
		},
		DailyQuiz: CompatibleModelConfig{
			Model:          " gpt-5.5-mini ",
			TimeoutSeconds: 47,
		},
	}

	got := cfg.ApplyDailyQuiz()

	if got.Provider != "openai-compatible" || got.APIBase != "https://gateway.example.com/v1" || got.APIKey != "admin-key" {
		t.Fatalf("expected daily quiz to inherit admin provider/base/key, got %+v", got)
	}
	if got.Model != "gpt-5.5-mini" || got.TimeoutSeconds != 47 {
		t.Fatalf("expected daily quiz model/timeout override, got %+v", got)
	}
}

func TestMergeIncomingPreservesAdminAndDailyQuizAPIKeys(t *testing.T) {
	current := Config{
		Admin:     CompatibleModelConfig{APIKey: "admin-secret", Model: "gpt-old"},
		DailyQuiz: CompatibleModelConfig{APIKey: "quiz-secret", Model: "gpt-quiz-old"},
	}
	incoming := Config{
		Admin:     CompatibleModelConfig{Provider: "openai-compatible", APIBase: "https://admin.example.com/v1", Model: "gpt-new", TimeoutSeconds: 41},
		DailyQuiz: CompatibleModelConfig{Provider: "anthropic-compatible", APIBase: "https://quiz.example.com/v1", Model: "claude-new", TimeoutSeconds: 52},
	}

	got := current.MergeIncoming(incoming)

	if got.Admin.APIKey != "admin-secret" || got.DailyQuiz.APIKey != "quiz-secret" {
		t.Fatalf("expected empty incoming keys to preserve stored secrets, got admin=%q daily=%q", got.Admin.APIKey, got.DailyQuiz.APIKey)
	}
	if got.Admin.Model != "gpt-new" || got.DailyQuiz.Model != "claude-new" {
		t.Fatalf("expected non-secret fields to update, got %+v", got)
	}
}
