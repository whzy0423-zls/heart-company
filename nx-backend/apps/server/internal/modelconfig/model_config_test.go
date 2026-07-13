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
		{name: "timeout too large", mutate: func(cfg *ChatConfig) { cfg.TimeoutSeconds = MaxChatTimeoutSeconds + 1 }, want: "timeoutSeconds"},
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

func TestMergeIncomingPreservesEveryOmittedSectionField(t *testing.T) {
	t.Parallel()

	enabled := true
	stored := Config{
		Chat:      ChatConfig{Provider: ProviderOpenAICompatible, APIBase: "https://chat.example/v1", APIKey: "chat-key", Model: "chat-old", TimeoutSeconds: 31},
		Video:     VideoConfig{APIBase: "https://video.example/v1", APIKey: "video-key", Model: "video-old"},
		Image:     ImageConfig{APIBase: "https://image.example/v1", APIKey: "image-key", Model: "image-old"},
		Analysis:  AnalysisConfig{APIBase: "https://analysis.example/v1", APIKey: "analysis-key", GroupID: "analysis-group", Model: "MiniMax-M3"},
		Admin:     CompatibleModelConfig{Provider: ProviderOpenAICompatible, APIBase: "https://admin.example/v1", APIKey: "admin-key", GroupID: "admin-group", Model: "admin-old", TimeoutSeconds: 41},
		DailyQuiz: CompatibleModelConfig{Provider: ProviderAnthropicCompatible, APIBase: "https://quiz.example/v1", APIKey: "quiz-key", GroupID: "quiz-group", Model: "quiz-old", TimeoutSeconds: 51},
		Assist:    AssistConfig{Enabled: &enabled, SystemPrompt: "old prompt"},
	}
	incoming := Config{
		Chat:      ChatConfig{Model: "chat-new"},
		Video:     VideoConfig{Model: "video-new"},
		Image:     ImageConfig{Model: "image-new"},
		Analysis:  AnalysisConfig{Model: "MiniMax-M3-Preview"},
		Admin:     CompatibleModelConfig{Model: "admin-new"},
		DailyQuiz: CompatibleModelConfig{Model: "quiz-new"},
	}

	got := stored.MergeIncoming(incoming)
	if got.Chat.Provider != stored.Chat.Provider || got.Chat.APIBase != stored.Chat.APIBase || got.Chat.APIKey != stored.Chat.APIKey || got.Chat.TimeoutSeconds != stored.Chat.TimeoutSeconds || got.Chat.Model != "chat-new" {
		t.Fatalf("chat omitted fields were not preserved: %+v", got.Chat)
	}
	if got.Video.APIBase != stored.Video.APIBase || got.Video.APIKey != stored.Video.APIKey || got.Video.Model != "video-new" {
		t.Fatalf("video omitted fields were not preserved: %+v", got.Video)
	}
	if got.Image.APIBase != stored.Image.APIBase || got.Image.APIKey != stored.Image.APIKey || got.Image.Model != "image-new" {
		t.Fatalf("image omitted fields were not preserved: %+v", got.Image)
	}
	if got.Analysis.APIBase != stored.Analysis.APIBase || got.Analysis.APIKey != stored.Analysis.APIKey || got.Analysis.GroupID != stored.Analysis.GroupID || got.Analysis.Model != "MiniMax-M3-Preview" {
		t.Fatalf("analysis omitted fields were not preserved: %+v", got.Analysis)
	}
	if got.Admin.Provider != stored.Admin.Provider || got.Admin.APIBase != stored.Admin.APIBase || got.Admin.APIKey != stored.Admin.APIKey || got.Admin.GroupID != stored.Admin.GroupID || got.Admin.TimeoutSeconds != stored.Admin.TimeoutSeconds || got.Admin.Model != "admin-new" {
		t.Fatalf("admin omitted fields were not preserved: %+v", got.Admin)
	}
	if got.DailyQuiz.Provider != stored.DailyQuiz.Provider || got.DailyQuiz.APIBase != stored.DailyQuiz.APIBase || got.DailyQuiz.APIKey != stored.DailyQuiz.APIKey || got.DailyQuiz.GroupID != stored.DailyQuiz.GroupID || got.DailyQuiz.TimeoutSeconds != stored.DailyQuiz.TimeoutSeconds || got.DailyQuiz.Model != "quiz-new" {
		t.Fatalf("daily quiz omitted fields were not preserved: %+v", got.DailyQuiz)
	}
	if got.Assist.SystemPrompt != stored.Assist.SystemPrompt || got.Assist.Enabled == nil || !*got.Assist.Enabled {
		t.Fatalf("assist omitted fields were not preserved: %+v", got.Assist)
	}
}

func TestMergeIncomingAssistTogglePreservesStoredSystemPrompt(t *testing.T) {
	t.Parallel()

	enabled := true
	disabled := false
	stored := Config{Assist: AssistConfig{Enabled: &enabled, SystemPrompt: "保持简洁自然"}}

	got := stored.MergeIncoming(Config{Assist: AssistConfig{Enabled: &disabled}})

	if got.Assist.Enabled == nil || *got.Assist.Enabled {
		t.Fatalf("expected assist to be disabled, got %+v", got.Assist)
	}
	if got.Assist.SystemPrompt != "保持简洁自然" {
		t.Fatalf("toggle-only update cleared stored prompt: %+v", got.Assist)
	}
}

func TestMergeIncomingJSONPatchPreservesOmittedFields(t *testing.T) {
	t.Parallel()

	stored := Config{
		Video: VideoConfig{APIBase: "https://video.example/v1", APIKey: "video-key", Model: "video-old"},
		Admin: CompatibleModelConfig{Provider: ProviderOpenAICompatible, APIBase: "https://admin.example/v1", APIKey: "admin-key", GroupID: "admin-group", Model: "admin-old", TimeoutSeconds: 41},
	}
	var incoming Config
	if err := json.Unmarshal([]byte(`{"video":{"model":"video-new"}}`), &incoming); err != nil {
		t.Fatal(err)
	}

	got := stored.MergeIncoming(incoming)

	if got.Video.APIBase != stored.Video.APIBase || got.Video.APIKey != stored.Video.APIKey || got.Video.Model != "video-new" {
		t.Fatalf("JSON patch lost omitted video fields: %+v", got.Video)
	}
	if got.Admin != stored.Admin.normalized() {
		t.Fatalf("JSON patch changed omitted admin section: %+v", got.Admin)
	}
}

func TestMergeIncomingExplicitBlankClearsOverridesButPreservesKeys(t *testing.T) {
	t.Parallel()

	enabled := true
	stored := Config{
		Video:     VideoConfig{APIBase: "https://video.example/v1", APIKey: "video-key", Model: "video-old"},
		Image:     ImageConfig{APIBase: "https://image.example/v1", APIKey: "image-key", Model: "image-old"},
		Analysis:  AnalysisConfig{APIBase: "https://analysis.example/v1", APIKey: "analysis-key", GroupID: "analysis-group", Model: "MiniMax-M3"},
		Admin:     CompatibleModelConfig{Provider: ProviderOpenAICompatible, APIBase: "https://admin.example/v1", APIKey: "admin-key", GroupID: "admin-group", Model: "admin-old", TimeoutSeconds: 41},
		DailyQuiz: CompatibleModelConfig{Provider: ProviderAnthropicCompatible, APIBase: "https://quiz.example/v1", APIKey: "quiz-key", GroupID: "quiz-group", Model: "quiz-old", TimeoutSeconds: 51},
		Assist:    AssistConfig{Enabled: &enabled, SystemPrompt: "old prompt"},
	}
	var incoming Config
	raw := `{
		"video":{"apiBase":"","apiKey":"","model":""},
		"image":{"apiBase":"","apiKey":"","model":""},
		"analysis":{"apiBase":"","apiKey":"","groupId":"","model":""},
		"admin":{"provider":"","apiBase":"","apiKey":"","groupId":"","model":"","timeoutSeconds":0},
		"dailyQuiz":{"provider":"","apiBase":"","apiKey":"","groupId":"","model":"","timeoutSeconds":0},
		"assist":{"systemPrompt":""}
	}`
	if err := json.Unmarshal([]byte(raw), &incoming); err != nil {
		t.Fatal(err)
	}

	got := stored.MergeIncoming(incoming)

	if got.Video.APIBase != "" || got.Video.Model != "" || got.Video.APIKey != "video-key" {
		t.Fatalf("unexpected explicit video clear: %+v", got.Video)
	}
	if got.Image.APIBase != "" || got.Image.Model != "" || got.Image.APIKey != "image-key" {
		t.Fatalf("unexpected explicit image clear: %+v", got.Image)
	}
	if got.Analysis.APIBase != "" || got.Analysis.GroupID != "" || got.Analysis.Model != "" || got.Analysis.APIKey != "analysis-key" {
		t.Fatalf("unexpected explicit analysis clear: %+v", got.Analysis)
	}
	for name, gotCfg := range map[string]CompatibleModelConfig{"admin": got.Admin, "dailyQuiz": got.DailyQuiz} {
		if gotCfg.Provider != "" || gotCfg.APIBase != "" || gotCfg.GroupID != "" || gotCfg.Model != "" || gotCfg.TimeoutSeconds != 0 {
			t.Fatalf("%s explicit clear was not preserved: %+v", name, gotCfg)
		}
	}
	if got.Admin.APIKey != "admin-key" || got.DailyQuiz.APIKey != "quiz-key" {
		t.Fatalf("explicit blank keys must retain stored secrets: admin=%q daily=%q", got.Admin.APIKey, got.DailyQuiz.APIKey)
	}
	if got.Assist.Enabled == nil || !*got.Assist.Enabled || got.Assist.SystemPrompt != "" {
		t.Fatalf("explicit prompt clear or omitted enabled was not respected: %+v", got.Assist)
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
