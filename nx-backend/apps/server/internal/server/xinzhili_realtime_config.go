package server

import (
	"context"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

const (
	defaultXinzhiliRealtimeASREndpoint = "wss://dashscope.aliyuncs.com/api-ws/v1/inference"
	defaultXinzhiliRealtimeASRRegion   = "cn-beijing"
)

// loadXinzhiliRealtimeConfig keeps the restored WebSocket realtime voice path
// compatible with the current branch's model_config storage. The current admin
// page persists TTS under model_config.tts; realtime ASR currently uses the
// server ASR API key and Aliyun Paraformer WebSocket defaults.
func (s *Server) loadXinzhiliRealtimeConfig(ctx context.Context) (xinzhili.Config, error) {
	stored, _, err := modelconfig.ReadStore(ctx, s.db)
	if err != nil {
		return xinzhili.Config{}, err
	}
	tts := stored.ApplyTTS(s.env.MiniMax)
	asrAPIKey := strings.TrimSpace(s.env.ASR.APIKey)
	usesBailianKey := strings.EqualFold(strings.TrimSpace(tts.Provider), xinzhili.TTSProviderBailian) ||
		(strings.EqualFold(strings.TrimSpace(tts.Provider), xinzhili.TTSProviderOpenAICompatible) &&
			strings.Contains(strings.ToLower(tts.Endpoint), "dashscope"))
	if usesBailianKey && strings.TrimSpace(tts.APIKey) != "" {
		asrAPIKey = strings.TrimSpace(tts.APIKey)
	} else if asrAPIKey == "" {
		asrAPIKey = strings.TrimSpace(tts.APIKey)
	}
	cfg := xinzhili.Config{
		Enabled: asrAPIKey != "" && strings.TrimSpace(tts.Voice) != "",
		Version: 1,
		RealtimeASR: xinzhili.RealtimeASRConfig{
			Provider: xinzhili.RealtimeASRProvider,
			Endpoint: defaultXinzhiliRealtimeASREndpoint,
			APIKey:   asrAPIKey,
			Region:   defaultXinzhiliRealtimeASRRegion,
			Model:    xinzhili.RealtimeASRModel,
		},
		TTS: xinzhili.TTSConfig{
			Provider: strings.TrimSpace(tts.Provider),
			Endpoint: strings.TrimSpace(tts.Endpoint),
			APIKey:   strings.TrimSpace(tts.APIKey),
			GroupID:  strings.TrimSpace(tts.GroupID),
			Model:    strings.TrimSpace(tts.Model),
			Voice:    strings.TrimSpace(tts.Voice),
			Format:   strings.TrimSpace(tts.Format),
		},
		EnabledModes: []xinzhili.Mode{
			xinzhili.ModeNormal,
			xinzhili.ModeArgument,
			xinzhili.ModeComfort,
			xinzhili.ModeDeepListening,
		},
		ModePrompts: map[xinzhili.Mode]string{},
	}
	if cfg.TTS.Provider == "" {
		cfg.TTS.Provider = xinzhili.TTSProviderMiniMax
	}
	if cfg.TTS.Format == "" {
		cfg.TTS.Format = "mp3"
	}
	if cfg.TTS.Model == "" {
		cfg.TTS.Model = "speech-02-hd"
	}
	return cfg, nil
}
