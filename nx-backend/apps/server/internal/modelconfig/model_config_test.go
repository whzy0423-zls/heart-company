package modelconfig

import (
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

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

func TestApplyVideoPreservesServerGenerationMode(t *testing.T) {
	base := config.VideoConfig{APIBase: "https://base.example", APIKey: "base-key", Mode: "demo", Model: "base-model"}
	cfg := Config{Video: VideoConfig{APIBase: "https://stored.example", APIKey: "stored-key", Model: "stored-model"}}

	got := cfg.ApplyVideo(base)

	if got.Mode != "demo" {
		t.Fatalf("stored model config changed generation mode to %q", got.Mode)
	}
	if got.APIBase != "https://stored.example" || got.APIKey != "stored-key" || got.Model != "stored-model" {
		t.Fatalf("expected provider fields to remain configurable, got %+v", got)
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
