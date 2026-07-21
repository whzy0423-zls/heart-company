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

func TestApplyXinzhiliVoiceNormalizesDefaultsAndConfiguration(t *testing.T) {
	cfg := Config{XinzhiliVoice: XinzhiliVoiceConfig{
		Enabled: true,
		ASR: SpeechModelConfig{
			APIBase: " https://speech.example.com/v1/ ",
			APIKey:  " asr-secret ",
			Model:   " whisper-1 ",
		},
		TTS: SpeechModelConfig{
			APIBase: " https://speech.example.com/v1/ ",
			APIKey:  " tts-secret ",
			Model:   " tts-1 ",
			Voice:   " alloy ",
		},
	}}

	got := cfg.ApplyXinzhiliVoice()

	if !got.Enabled || got.ASR.APIBase != "https://speech.example.com/v1" || got.TTS.APIBase != "https://speech.example.com/v1" {
		t.Fatalf("unexpected normalized config: %+v", got)
	}
	if got.ASR.Language != "zh" || got.ASR.TimeoutSeconds != 30 {
		t.Fatalf("unexpected ASR defaults: %+v", got.ASR)
	}
	if got.TTS.ResponseFormat != "mp3" || got.TTS.Speed != 1 || got.TTS.TimeoutSeconds != 45 {
		t.Fatalf("unexpected TTS defaults: %+v", got.TTS)
	}
	if got.Interaction.EndSilenceMs != 700 || got.Interaction.MinSpeechMs != 300 || got.Interaction.MaxTurnSeconds != 60 || !got.Interaction.AutoRelisten || !got.Interaction.TapToInterrupt {
		t.Fatalf("unexpected interaction defaults: %+v", got.Interaction)
	}
	if err := got.ValidateReady(); err != nil {
		t.Fatalf("configured voice models should be ready: %v", err)
	}
}

func TestXinzhiliVoiceValidateReadyRequiresEnabledASRAndTTS(t *testing.T) {
	cases := []XinzhiliVoiceConfig{
		{},
		{Enabled: true},
		{Enabled: true, ASR: SpeechModelConfig{APIBase: "https://speech.example.com/v1", APIKey: "asr", Model: "whisper-1"}},
	}
	for _, cfg := range cases {
		if err := cfg.ValidateReady(); err == nil {
			t.Fatalf("ValidateReady(%+v) unexpectedly succeeded", cfg)
		}
	}
}

func TestMergeIncomingPreservesXinzhiliSpeechKeys(t *testing.T) {
	current := Config{XinzhiliVoice: XinzhiliVoiceConfig{
		ASR: SpeechModelConfig{APIKey: "asr-secret"},
		TTS: SpeechModelConfig{APIKey: "tts-secret"},
	}}
	incoming := Config{XinzhiliVoice: XinzhiliVoiceConfig{
		Enabled: true,
		ASR:     SpeechModelConfig{APIBase: "https://new.example/v1", Model: "whisper-new"},
		TTS:     SpeechModelConfig{APIBase: "https://new.example/v1", Model: "tts-new", Voice: "nova"},
	}}

	got := current.MergeIncoming(incoming)

	if got.XinzhiliVoice.ASR.APIKey != "asr-secret" || got.XinzhiliVoice.TTS.APIKey != "tts-secret" {
		t.Fatalf("expected xinzhili keys to be preserved: %+v", got.XinzhiliVoice)
	}
}
