package server

import (
	"context"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestLoadXinzhiliRealtimeConfigReusesTTSKeyForAliyunASRWhenASRKeyEmpty(t *testing.T) {
	s := &Server{env: config.Env{MiniMax: config.MiniMaxConfig{APIBase: "https://dashscope.aliyuncs.com/compatible-mode/v1", APIKey: "bailian-key", GroupID: "group", Model: "qwen-audio-tts", Provider: "openai-compatible"}}}

	cfg, err := s.loadXinzhiliRealtimeConfig(context.Background())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.RealtimeASR.APIKey != "bailian-key" {
		t.Fatalf("realtime ASR should reuse TTS/Bailian API key when ASR_API_KEY is empty, got %q", cfg.RealtimeASR.APIKey)
	}
}

func TestLoadXinzhiliRealtimeConfigPrefersBailianTTSKeyOverLegacyASRKey(t *testing.T) {
	s := &Server{env: config.Env{
		ASR: config.ASRConfig{APIKey: "legacy-asr-key"},
		MiniMax: config.MiniMaxConfig{
			APIBase:  "https://dashscope.aliyuncs.com/api/v1",
			APIKey:   "bailian-key",
			Model:    "MiniMax/speech-2.8-turbo",
			Provider: "bailian",
		},
	}}

	cfg, err := s.loadXinzhiliRealtimeConfig(context.Background())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.RealtimeASR.APIKey != "bailian-key" {
		t.Fatalf("bailian realtime ASR should use the saved bailian key, got %q", cfg.RealtimeASR.APIKey)
	}
}
