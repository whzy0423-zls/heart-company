package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/skillchat"
)

func TestAppChatSkipsTTSWhenVoiceBroadcastPreferenceIsOff(t *testing.T) {
	store := newFakeAppChatStreamStore()
	s := newAppChatStreamServer(store, successfulAppChatGenerator("这是文字回答。"))
	s.voiceBroadcastPreferences = newMemoryVoiceBroadcastPreferenceStore()
	s.voiceBroadcastConfigLoader = enabledVoiceBroadcastConfig
	var factoryCalls atomic.Int32
	s.voiceBroadcastSynthesizerFactory = func(context.Context) (voiceBroadcastSynthesizer, error) {
		factoryCalls.Add(1)
		return recordingVoiceBroadcastProvider{}, nil
	}

	w := newAppChatBlockingStreamWriter()
	s.appChatRouter(w, newAppChatStreamRequest(context.Background()))
	body := w.BodyString()
	if !strings.Contains(body, "event: done\n") || strings.Contains(body, "event: voice\n") || strings.Contains(body, "event: voice_error\n") {
		t.Fatalf("unexpected chat stream with broadcast off: %q", body)
	}
	if got := factoryCalls.Load(); got != 0 {
		t.Fatalf("TTS factory calls=%d, want 0", got)
	}
}

func TestSkillChatSkipsTTSWhenVoiceBroadcastPreferenceIsOff(t *testing.T) {
	store := &slowVoiceSkillRuntimeStore{}
	s := &Server{
		skillChatRuntime:           skillchat.NewRuntime(store, slowVoiceSkillSearcher{}, slowVoiceSkillGenerator{}),
		chatTimeout:                time.Second,
		voiceBroadcastPreferences:  newMemoryVoiceBroadcastPreferenceStore(),
		voiceBroadcastConfigLoader: enabledVoiceBroadcastConfig,
	}
	var factoryCalls atomic.Int32
	s.voiceBroadcastSynthesizerFactory = func(context.Context) (voiceBroadcastSynthesizer, error) {
		factoryCalls.Add(1)
		return recordingVoiceBroadcastProvider{}, nil
	}

	request := httptest.NewRequest(http.MethodPost, "/api/app/skill-sessions/42/ask/stream", strings.NewReader(`{"question":"请回答"}`))
	w := newAppChatBlockingStreamWriter()
	s.appSkillSessionAskStream(w, request, 7)
	body := w.BodyString()
	if !strings.Contains(body, "event: done\n") || strings.Contains(body, "event: voice\n") || strings.Contains(body, "event: voice_error\n") {
		t.Fatalf("unexpected skill stream with broadcast off: %q", body)
	}
	if got := factoryCalls.Load(); got != 0 {
		t.Fatalf("TTS factory calls=%d, want 0", got)
	}
}

func enabledVoiceBroadcastConfig(context.Context) (voiceBroadcastConfig, error) {
	return voiceBroadcastConfig{
		Enabled: true, Provider: voiceBroadcastProviderBailian, APIKey: "test",
		Model: voiceBroadcastDefaultModel, Voice: voiceBroadcastDefaultVoice,
	}, nil
}
