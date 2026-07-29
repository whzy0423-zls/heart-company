package modelconfig

import (
	"encoding/json"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestApplyChatDefaultsLegacyConfigToOpenAICompatible(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"chat":{"apiBase":"https://coding-play.codes/","apiKey":"secret","groupId":"legacy-group","model":"gpt-5.6-sol"}}`), &cfg); err != nil {
		t.Fatal(err)
	}

	got := cfg.ApplyChat(config.MiniMaxConfig{TimeoutSeconds: 41})

	if got.Provider != ProviderOpenAICompatible {
		t.Fatalf("legacy chat provider = %q, want %q", got.Provider, ProviderOpenAICompatible)
	}
	if got.APIBase != "https://coding-play.codes" || got.Model != "gpt-5.6-sol" || got.APIKey != "secret" {
		t.Fatalf("unexpected legacy chat config: %+v", got)
	}
	if got.GroupID != "" {
		t.Fatalf("chat group id must not propagate to compatible protocols: %+v", got)
	}
}

func TestApplyChatKeepsAnthropicCompatibleProvider(t *testing.T) {
	cfg := Config{Chat: ChatConfig{
		Provider: ProviderAnthropicCompatible,
		APIBase:  " https://coding-play.codes/ ",
		APIKey:   " anthropic-key ",
		Model:    " claude-sonnet-4-5 ",
	}}

	got := cfg.ApplyChat(config.MiniMaxConfig{})

	if got.Provider != ProviderAnthropicCompatible || got.APIBase != "https://coding-play.codes" || got.APIKey != "anthropic-key" || got.Model != "claude-sonnet-4-5" {
		t.Fatalf("unexpected anthropic chat config: %+v", got)
	}
}

func TestApplyChatDefaultsAPIBaseForNewConfig(t *testing.T) {
	got := (Config{}).ApplyChat(config.MiniMaxConfig{})

	if got.APIBase != "https://coding-play.codes" {
		t.Fatalf("default chat api base = %q", got.APIBase)
	}
	if got.Provider != ProviderOpenAICompatible {
		t.Fatalf("default chat provider = %q", got.Provider)
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
			GroupID: "chat-group",
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

func TestMergeIncomingPreservesTTSAPIKeyAndTrimsVoiceConfig(t *testing.T) {
	current := Config{
		TTS: TTSConfig{
			APIKey: "tts-secret",
			Voice:  "old-voice",
		},
	}
	incoming := Config{
		TTS: TTSConfig{
			Provider: " MiniMax ",
			Endpoint: " https://api.minimaxi.com/ ",
			GroupID:  " group-1 ",
			Model:    " speech-02-hd ",
			Voice:    " cloned-voice-123 ",
			Format:   " mp3 ",
		},
	}

	got := current.MergeIncoming(incoming)

	if got.TTS.APIKey != "tts-secret" {
		t.Fatalf("expected empty incoming TTS key to preserve stored secret, got %q", got.TTS.APIKey)
	}
	if got.TTS.Provider != "minimax" || got.TTS.Endpoint != "https://api.minimaxi.com" || got.TTS.GroupID != "group-1" {
		t.Fatalf("expected normalized TTS provider/endpoint/group, got %+v", got.TTS)
	}
	if got.TTS.Model != "speech-02-hd" || got.TTS.Voice != "cloned-voice-123" || got.TTS.Format != "mp3" {
		t.Fatalf("expected trimmed TTS model/voice/format, got %+v", got.TTS)
	}
}

func TestTTSConfigAcceptsOfficialAndCloneDerivedVoiceIDs(t *testing.T) {
	for _, voiceID := range []string{"male-qn-qingse", "cloned-voice-123"} {
		cfg := Config{TTS: TTSConfig{Provider: "minimax", Voice: " " + voiceID + " "}}

		got := cfg.trimmed()

		if got.TTS.Voice != voiceID {
			t.Fatalf("expected TTS voice %q to be stored as final voice id, got %q", voiceID, got.TTS.Voice)
		}
	}
}

func TestApplyTTSKeepsBailianEnvironmentProvider(t *testing.T) {
	got := (Config{}).ApplyTTS(config.MiniMaxConfig{
		Provider: "bailian",
		APIBase:  "https://dashscope.aliyuncs.com/api/v1",
		APIKey:   "bailian-key",
		Model:    "MiniMax/speech-2.8-turbo",
	})
	if got.Provider != "bailian" {
		t.Fatalf("provider=%q", got.Provider)
	}
}

func TestApplyTTSUsesProviderSpecificDefaultModel(t *testing.T) {
	for _, tt := range []struct {
		provider string
		want     string
	}{
		{provider: "bailian", want: "MiniMax/speech-2.8-turbo"},
		{provider: "minimax", want: "speech-02-hd"},
	} {
		t.Run(tt.provider, func(t *testing.T) {
			got := (Config{}).ApplyTTS(config.MiniMaxConfig{Provider: tt.provider})
			if got.Model != tt.want {
				t.Fatalf("model = %q, want %q", got.Model, tt.want)
			}
		})
	}
}
