package xinzhili

import (
	"errors"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		Enabled: true,
		RealtimeASR: RealtimeASRConfig{
			Provider: "aliyun-bailian",
			Endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference",
			APIKey:   "asr-secret",
			Region:   "cn-beijing",
			Model:    "paraformer-realtime-v2",
		},
		TTS: TTSConfig{
			Provider: "openai-compatible",
			Endpoint: "https://tts.example.com/v1",
			APIKey:   "tts-secret",
			Model:    "tts-1",
			Voice:    "alloy",
			Format:   "mp3",
		},
		EnabledModes: []Mode{ModeNormal},
	}
}

func TestConfigValidateDisabledAllowsEmptyProviders(t *testing.T) {
	if err := (Config{Enabled: false}).Validate(); err != nil {
		t.Fatalf("disabled config should be valid: %v", err)
	}
}

func TestConfigValidateRequiresRealtimeASRAndNormalMode(t *testing.T) {
	cfg := validConfig()
	cfg.EnabledModes = []Mode{ModeComfort}
	err := cfg.Validate()
	if !errors.Is(err, ErrNormalModeRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestConfigValidateRequiresCompleteASRAndTTSWhenEnabled(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"asr provider", func(c *Config) { c.RealtimeASR.Provider = "" }},
		{"asr endpoint", func(c *Config) { c.RealtimeASR.Endpoint = "" }},
		{"asr region", func(c *Config) { c.RealtimeASR.Region = "" }},
		{"asr model", func(c *Config) { c.RealtimeASR.Model = "" }},
		{"tts provider", func(c *Config) { c.TTS.Provider = "" }},
		{"tts endpoint", func(c *Config) { c.TTS.Endpoint = "" }},
		{"tts api key", func(c *Config) { c.TTS.APIKey = "" }},
		{"tts model", func(c *Config) { c.TTS.Model = "" }},
		{"tts voice", func(c *Config) { c.TTS.Voice = "" }},
		{"tts format", func(c *Config) { c.TTS.Format = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestConfigValidateUsesRuntimeBailianCredentialsOutsidePersistedJSON(t *testing.T) {
	tests := []struct {
		name string
		tts  TTSConfig
	}{
		{
			name: "native bailian provider",
			tts: TTSConfig{
				Provider: TTSProviderBailian,
				Endpoint: "https://dashscope.aliyuncs.com/api/v1",
				Model:    "qwen3-tts-vc-2026-01-22",
				Voice:    "teacher-voice",
				Format:   "mp3",
			},
		},
		{
			name: "official dashscope compatible provider",
			tts: TTSConfig{
				Provider: TTSProviderOpenAICompatible,
				Endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1",
				Model:    "qwen3-tts-vc-2026-01-22",
				Voice:    "teacher-voice",
				Format:   "mp3",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.RealtimeASR.APIKey = ""
			cfg.TTS = tt.tts
			if err := cfg.Validate(); err != nil {
				t.Fatalf("shared Bailian credentials should be injected at runtime: %v", err)
			}
		})
	}
}

func TestConfigValidateRequiresPrivateCredentialForNonBailianTTS(t *testing.T) {
	tests := []struct {
		name string
		tts  TTSConfig
	}{
		{
			name: "minimax",
			tts: TTSConfig{
				Provider: TTSProviderMiniMax,
				Endpoint: "https://api.minimax.chat/v1/t2a_v2",
				GroupID:  "group",
				Model:    "speech-02-hd",
				Voice:    "teacher-voice",
				Format:   "mp3",
			},
		},
		{
			name: "generic openai compatible",
			tts: TTSConfig{
				Provider: TTSProviderOpenAICompatible,
				Endpoint: "https://tts.example.com/v1",
				Model:    "tts-1",
				Voice:    "teacher-voice",
				Format:   "mp3",
			},
		},
		{
			name: "native bailian custom proxy",
			tts: TTSConfig{
				Provider: TTSProviderBailian,
				Endpoint: "https://bailian-proxy.example/api/v1",
				Model:    "qwen3-tts-vc-2026-01-22",
				Voice:    "teacher-voice",
				Format:   "mp3",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.RealtimeASR.APIKey = ""
			cfg.TTS = tt.tts
			if err := cfg.Validate(); err == nil {
				t.Fatal("non-Bailian TTS must keep its own private API key")
			}
		})
	}
}

func TestConfigValidateDisabledStillChecksProvidedStructureAndAllowsEmptyVoice(t *testing.T) {
	cfg := validConfig()
	cfg.Enabled = false
	cfg.RealtimeASR.APIKey = ""
	cfg.TTS.APIKey = ""
	cfg.TTS.Voice = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled config should allow incomplete runtime fields: %v", err)
	}

	for _, mutate := range []func(*Config){
		func(c *Config) { c.RealtimeASR.Provider = "unsupported" },
		func(c *Config) { c.RealtimeASR.Endpoint = "ftp://asr.example.com" },
		func(c *Config) { c.RealtimeASR.Endpoint = "wss://user:pass@asr.example.com/realtime" },
		func(c *Config) { c.TTS.Provider = "unsupported" },
		func(c *Config) { c.TTS.Endpoint = "http://tts.example.com" },
		func(c *Config) { c.TTS.Endpoint = "https://tts.example.com/v1#secret" },
		func(c *Config) { c.TTS.Endpoint = "https://[::1" },
	} {
		invalid := cfg
		mutate(&invalid)
		if err := invalid.Validate(); err == nil {
			t.Fatal("disabled config accepted an invalid provided field")
		}
	}
}

func TestConfigValidateFixesRealtimeProviderAndModel(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"provider", func(c *Config) { c.RealtimeASR.Provider = "other" }},
		{"model", func(c *Config) { c.RealtimeASR.Model = "paraformer-v1" }},
		{"endpoint scheme", func(c *Config) { c.RealtimeASR.Endpoint = "ws://localhost/asr" }},
		{"non-official endpoint", func(c *Config) { c.RealtimeASR.Endpoint = "wss://asr.example.com/api-ws/v1/inference" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	cfg := validConfig()
	cfg.RealtimeASR.Endpoint = "https://dashscope.aliyuncs.com/api-ws/v1/inference"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("https ASR endpoint should be accepted: %v", err)
	}
}

func TestTTSUsesBailianCredentialsRequiresOfficialDashScopeEndpoint(t *testing.T) {
	tests := []struct {
		name string
		cfg  TTSConfig
		want bool
	}{
		{name: "native official", cfg: TTSConfig{Provider: TTSProviderBailian, Endpoint: "https://dashscope.aliyuncs.com/api/v1"}, want: true},
		{name: "compatible official", cfg: TTSConfig{Provider: TTSProviderOpenAICompatible, Endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1"}, want: true},
		{name: "native custom proxy", cfg: TTSConfig{Provider: TTSProviderBailian, Endpoint: "https://bailian-proxy.example/api/v1"}},
		{name: "native lookalike", cfg: TTSConfig{Provider: TTSProviderBailian, Endpoint: "https://dashscope.aliyuncs.com.evil.test/api/v1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TTSUsesBailianCredentials(tt.cfg); got != tt.want {
				t.Fatalf("TTSUsesBailianCredentials=%t want=%t", got, tt.want)
			}
		})
	}
}

func TestIsOfficialDashScopeRealtimeASREndpointStrict(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     bool
	}{
		{name: "official wss", endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference", want: true},
		{name: "official https", endpoint: "https://dashscope.aliyuncs.com/api-ws/v1/inference", want: true},
		{name: "numeric default port", endpoint: "wss://dashscope.aliyuncs.com:0443/api-ws/v1/inference", want: true},
		{name: "arbitrary host", endpoint: "wss://asr.example.com/api-ws/v1/inference"},
		{name: "userinfo", endpoint: "wss://dashscope.aliyuncs.com@evil.test/api-ws/v1/inference"},
		{name: "non default port", endpoint: "wss://dashscope.aliyuncs.com:8443/api-ws/v1/inference"},
		{name: "similar domain", endpoint: "wss://dashscope.aliyuncs.com.evil.test/api-ws/v1/inference"},
		{name: "wrong path", endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/other"},
		{name: "trailing slash", endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference/"},
		{name: "query", endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference?token=x"},
		{name: "fragment", endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference#secret"},
		{name: "raw path", endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/%69nference"},
		{name: "dot segment", endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/../v1/inference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOfficialDashScopeRealtimeASREndpoint(tt.endpoint); got != tt.want {
				t.Fatalf("IsOfficialDashScopeRealtimeASREndpoint=%t want=%t", got, tt.want)
			}
		})
	}
}

func TestConfigValidateTTSProviderDiscriminatedUnion(t *testing.T) {
	cfg := validConfig()
	cfg.TTS.Provider = "minimax"
	if err := cfg.Validate(); err == nil {
		t.Fatal("minimax should require groupId")
	}
	cfg.TTS.GroupID = "group"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid minimax config: %v", err)
	}

	cfg = validConfig()
	cfg.TTS.GroupID = "ignored"
	normalized, err := cfg.WithDefaults()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.TTS.GroupID != "" {
		t.Fatalf("openai-compatible groupId=%q, want empty", normalized.TTS.GroupID)
	}

	cfg = validConfig()
	cfg.TTS.Provider = "bailian"
	cfg.TTS.Endpoint = "https://dashscope.aliyuncs.com/api/v1"
	cfg.TTS.Model = "MiniMax/speech-2.8-turbo"
	cfg.TTS.GroupID = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid bailian config: %v", err)
	}

	cfg = validConfig()
	cfg.TTS.Provider = "anthropic-compatible"
	if err := cfg.Validate(); err == nil {
		t.Fatal("unsupported TTS provider should fail")
	}
	cfg = validConfig()
	cfg.TTS.Endpoint = "http://tts.example.com"
	if err := cfg.Validate(); err == nil {
		t.Fatal("TTS endpoint must use HTTPS")
	}
	cfg = validConfig()
	cfg.TTS.Format = "wav"
	if err := cfg.Validate(); err == nil {
		t.Fatal("TTS format must be mp3")
	}
}

func TestConfigWithDefaultsAppliesTimingDefaults(t *testing.T) {
	cfg, err := validConfig().WithDefaults()
	if err != nil {
		t.Fatal(err)
	}
	want := TimingConfig{
		PartialStableMs:            150,
		ArgumentCandidateSilenceMs: 350,
		NormalEndSilenceMs:         700,
		ComfortEndSilenceMs:        1200,
		DeepListeningEndSilenceMs:  1500,
		ComfortFirstPromptMs:       5000,
		ComfortSecondPromptMs:      12000,
		DeepListeningPromptMs:      12000,
		MaxProactivePrompts:        2,
	}
	if cfg.Timing != want {
		t.Fatalf("timing=%+v want=%+v", cfg.Timing, want)
	}
}

func TestConfigWithDefaultsPreservesExplicitZeroMaxProactivePrompts(t *testing.T) {
	cfg := validConfig()
	cfg.Timing = TimingConfig{
		PartialStableMs:            150,
		ArgumentCandidateSilenceMs: 350,
		NormalEndSilenceMs:         700,
		ComfortEndSilenceMs:        1200,
		DeepListeningEndSilenceMs:  1500,
		ComfortFirstPromptMs:       5000,
		ComfortSecondPromptMs:      12000,
		DeepListeningPromptMs:      12000,
		MaxProactivePrompts:        0,
	}
	normalized, err := cfg.WithDefaults()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Timing.MaxProactivePrompts != 0 {
		t.Fatalf("maxProactivePrompts=%d want explicit 0", normalized.Timing.MaxProactivePrompts)
	}
}

func TestConfigValidateRejectsTimingOutsideRangesAndInvalidOrdering(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TimingConfig)
	}{
		{"partial low", func(v *TimingConfig) { v.PartialStableMs = 99 }},
		{"candidate high", func(v *TimingConfig) { v.ArgumentCandidateSilenceMs = 601 }},
		{"normal low", func(v *TimingConfig) { v.NormalEndSilenceMs = 349 }},
		{"comfort high", func(v *TimingConfig) { v.ComfortEndSilenceMs = 3001 }},
		{"deep high", func(v *TimingConfig) { v.DeepListeningEndSilenceMs = 5001 }},
		{"first low", func(v *TimingConfig) { v.ComfortFirstPromptMs = 2999 }},
		{"second high", func(v *TimingConfig) { v.ComfortSecondPromptMs = 60001 }},
		{"deep prompt low", func(v *TimingConfig) { v.DeepListeningPromptMs = 4999 }},
		{"proactive high", func(v *TimingConfig) { v.MaxProactivePrompts = 6 }},
		{"candidate not before normal", func(v *TimingConfig) { v.ArgumentCandidateSilenceMs, v.NormalEndSilenceMs = 500, 500 }},
		{"first not before second", func(v *TimingConfig) { v.ComfortFirstPromptMs, v.ComfortSecondPromptMs = 12000, 12000 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Timing = TimingConfig{
				PartialStableMs:            150,
				ArgumentCandidateSilenceMs: 350,
				NormalEndSilenceMs:         700,
				ComfortEndSilenceMs:        1200,
				DeepListeningEndSilenceMs:  1500,
				ComfortFirstPromptMs:       5000,
				ComfortSecondPromptMs:      12000,
				DeepListeningPromptMs:      12000,
				MaxProactivePrompts:        2,
			}
			tt.mutate(&cfg.Timing)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected timing validation error")
			}
		})
	}
}

func TestConfigWithDefaultsNormalizesModesAndPrompts(t *testing.T) {
	cfg := validConfig()
	cfg.EnabledModes = []Mode{" normal ", ModeComfort, ModeNormal, ModeArgument}
	cfg.CommonPrompt = "  common  "
	cfg.ModePrompts = map[Mode]string{ModeComfort: "  comfort  "}
	normalized, err := cfg.WithDefaults()
	if err != nil {
		t.Fatal(err)
	}
	want := []Mode{ModeNormal, ModeComfort, ModeArgument}
	if len(normalized.EnabledModes) != len(want) {
		t.Fatalf("modes=%v", normalized.EnabledModes)
	}
	for i := range want {
		if normalized.EnabledModes[i] != want[i] {
			t.Fatalf("modes=%v want=%v", normalized.EnabledModes, want)
		}
	}
	if normalized.CommonPrompt != "common" || normalized.ModePrompts[ModeComfort] != "comfort" {
		t.Fatalf("prompts not trimmed: %#v", normalized)
	}
}

func TestConfigWithDefaultsRejectsDuplicateNormalizedModePromptKeys(t *testing.T) {
	cfg := validConfig()
	cfg.ModePrompts = map[Mode]string{
		ModeNormal:       "first",
		Mode(" normal "): "second",
	}
	if _, err := cfg.WithDefaults(); err == nil {
		t.Fatal("duplicate normalized mode prompt keys should fail")
	}
}

func TestConfigValidateRejectsUnknownModesAndOversizedPrompts(t *testing.T) {
	cfg := validConfig()
	cfg.EnabledModes = append(cfg.EnabledModes, Mode("unknown"))
	if err := cfg.Validate(); err == nil {
		t.Fatal("unknown enabled mode should fail")
	}
	cfg = validConfig()
	cfg.ModePrompts = map[Mode]string{Mode("unknown"): "prompt"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("unknown prompt mode should fail")
	}
	cfg = validConfig()
	cfg.CommonPrompt = strings.Repeat("界", 8001)
	if err := cfg.Validate(); err == nil {
		t.Fatal("oversized common prompt should fail")
	}
	cfg = validConfig()
	cfg.ModePrompts = map[Mode]string{ModeNormal: strings.Repeat("界", 8001)}
	if err := cfg.Validate(); err == nil {
		t.Fatal("oversized mode prompt should fail")
	}
}

func TestConfigValidateRejectsOversizedProviderFields(t *testing.T) {
	tests := []func(*Config){
		func(c *Config) { c.RealtimeASR.Endpoint = "wss://" + strings.Repeat("x", 2049) },
		func(c *Config) { c.RealtimeASR.APIKey = strings.Repeat("x", 4097) },
		func(c *Config) { c.RealtimeASR.Region = strings.Repeat("x", 257) },
		func(c *Config) { c.TTS.Model = strings.Repeat("x", 257) },
		func(c *Config) { c.TTS.Voice = strings.Repeat("x", 257) },
		func(c *Config) {
			c.TTS.Provider = TTSProviderMiniMax
			c.TTS.GroupID = strings.Repeat("x", 257)
		},
	}
	for i, mutate := range tests {
		cfg := validConfig()
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatalf("case %d should fail", i)
		}
	}
}

func TestConfigClearKeyMarkersAreExplicitAndTransient(t *testing.T) {
	current := validConfig()
	incoming := validConfig()
	incoming.RealtimeASR.APIKey = ""
	incoming.TTS.APIKey = ""
	merged := MergeIncoming(current, incoming)
	if merged.RealtimeASR.APIKey != "asr-secret" || merged.TTS.APIKey != "tts-secret" {
		t.Fatalf("empty incoming keys must preserve current: %#v", merged)
	}

	incoming.ClearASRKey = true
	incoming.ClearTTSKey = true
	merged = MergeIncoming(current, incoming)
	if merged.RealtimeASR.APIKey != "" || merged.TTS.APIKey != "" {
		t.Fatalf("clear markers must erase keys: %#v", merged)
	}
	if merged.ClearASRKey || merged.ClearTTSKey {
		t.Fatal("clear markers must not survive merge")
	}
}

func TestMergeIncomingPreservesSecretsForWhitespaceWithoutClear(t *testing.T) {
	current := validConfig()
	incoming := validConfig()
	incoming.Enabled = false
	incoming.RealtimeASR.APIKey = " \t\n "
	incoming.TTS.APIKey = "  "

	merged := MergeIncoming(current, incoming)
	if merged.RealtimeASR.APIKey != "asr-secret" {
		t.Fatalf("ASR API key=%q want preserved secret", merged.RealtimeASR.APIKey)
	}
	if merged.TTS.APIKey != "tts-secret" {
		t.Fatalf("TTS API key=%q want preserved secret", merged.TTS.APIKey)
	}
}
