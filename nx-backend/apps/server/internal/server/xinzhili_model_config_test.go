package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

type fakeXinzhiliModelConfigStore struct {
	config          xinzhili.Config
	found           bool
	readErr         error
	updateErr       error
	updated         xinzhili.Config
	expectedVersion int64
	updateCalls     int
}

func (f *fakeXinzhiliModelConfigStore) Read(context.Context) (xinzhili.Config, bool, error) {
	return f.config, f.found, f.readErr
}

func (f *fakeXinzhiliModelConfigStore) Update(_ context.Context, cfg xinzhili.Config, expectedVersion int64) (xinzhili.Config, error) {
	f.updateCalls++
	f.updated = cfg
	f.expectedVersion = expectedVersion
	if f.updateErr != nil {
		return xinzhili.Config{}, f.updateErr
	}
	return f.config, nil
}

func TestXinzhiliModelConfigGETRedactsSecrets(t *testing.T) {
	store := &fakeXinzhiliModelConfigStore{found: true, config: xinzhili.Config{
		Enabled:     true,
		Version:     7,
		RealtimeASR: xinzhili.RealtimeASRConfig{APIKey: "asr-long-secret"},
		TTS:         xinzhili.TTSConfig{APIKey: "shortkey"},
	}}
	s := &Server{xinzhiliModelConfig: store}
	res := httptest.NewRecorder()
	s.xinzhiliModelConfigHandler(res, httptest.NewRequest(http.MethodGet, "/api/xinzhili-model-config", nil))

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "asr-long-secret") || strings.Contains(res.Body.String(), "shortkey") {
		t.Fatalf("response leaked a secret: %s", res.Body.String())
	}
	var body struct {
		Data struct {
			Version     int64 `json:"version"`
			RealtimeASR struct {
				APIKey       string `json:"apiKey"`
				APIKeySet    bool   `json:"apiKeySet"`
				APIKeySuffix string `json:"apiKeySuffix"`
			} `json:"realtimeAsr"`
			TTS struct {
				APIKey       string `json:"apiKey"`
				APIKeySet    bool   `json:"apiKeySet"`
				APIKeySuffix string `json:"apiKeySuffix"`
			} `json:"tts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Version != 7 || body.Data.RealtimeASR.APIKey != "" || !body.Data.RealtimeASR.APIKeySet || body.Data.RealtimeASR.APIKeySuffix != "cret" {
		t.Fatalf("unexpected ASR view: %+v", body.Data.RealtimeASR)
	}
	if body.Data.TTS.APIKey != "" || !body.Data.TTS.APIKeySet || body.Data.TTS.APIKeySuffix != "" {
		t.Fatalf("unexpected TTS view: %+v", body.Data.TTS)
	}
}

func TestXinzhiliModelConfigGETReturnsDefaultsWhenConfigDoesNotExist(t *testing.T) {
	store := &fakeXinzhiliModelConfigStore{found: false}
	s := &Server{xinzhiliModelConfig: store}
	res := httptest.NewRecorder()
	s.xinzhiliModelConfigHandler(res, httptest.NewRequest(http.MethodGet, "/api/xinzhili-model-config", nil))

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var body struct {
		Data xinzhiliModelConfigView `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	view := body.Data
	if view.Version != 0 || view.Enabled {
		t.Fatalf("unexpected initial state: version=%d enabled=%t", view.Version, view.Enabled)
	}
	if len(view.EnabledModes) != 1 || view.EnabledModes[0] != xinzhili.ModeNormal {
		t.Fatalf("enabledModes=%v want=[normal]", view.EnabledModes)
	}
	if view.ModePrompts == nil || len(view.ModePrompts) != 0 {
		t.Fatalf("modePrompts=%v want non-nil empty map", view.ModePrompts)
	}
	if view.Timing.PartialStableMs != 150 || view.Timing.MaxProactivePrompts != 2 {
		t.Fatalf("unexpected timing defaults: %+v", view.Timing)
	}
	if view.RealtimeASR.Provider != xinzhili.RealtimeASRProvider ||
		view.RealtimeASR.Model != xinzhili.RealtimeASRModel ||
		view.RealtimeASR.Endpoint != "wss://dashscope.aliyuncs.com/api-ws/v1/inference" ||
		view.RealtimeASR.Region != "cn-beijing" {
		t.Fatalf("unexpected ASR defaults: %+v", view.RealtimeASR)
	}
	if view.TTS.Provider != xinzhili.TTSProviderOpenAICompatible || view.TTS.Format != "mp3" || view.TTS.APIKeySet {
		t.Fatalf("unexpected TTS defaults: %+v", view.TTS)
	}
	if store.updateCalls != 0 {
		t.Fatalf("GET persisted a default configuration: updateCalls=%d", store.updateCalls)
	}
}

func TestXinzhiliModelConfigPUTPassesExpectedVersionAndEmptyKeys(t *testing.T) {
	stored := validXinzhiliModelConfigForHandler()
	stored.Version = 10
	store := &fakeXinzhiliModelConfigStore{config: stored, found: true}
	s := &Server{xinzhiliModelConfig: store}
	res := httptest.NewRecorder()
	body, err := json.Marshal(map[string]any{
		"expectedVersion": 9, "enabled": true,
		"realtimeAsr":  map[string]any{"provider": "aliyun-bailian", "endpoint": stored.RealtimeASR.Endpoint, "apiKey": "", "region": stored.RealtimeASR.Region, "model": "paraformer-realtime-v2"},
		"tts":          map[string]any{"provider": "openai-compatible", "endpoint": stored.TTS.Endpoint, "apiKey": "", "model": stored.TTS.Model, "voice": stored.TTS.Voice, "format": "mp3"},
		"enabledModes": []string{"normal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/xinzhili-model-config", strings.NewReader(string(body)))
	s.xinzhiliModelConfigHandler(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if store.expectedVersion != 9 {
		t.Fatalf("expectedVersion=%d want=9", store.expectedVersion)
	}
	if store.updated.RealtimeASR.APIKey != "" || store.updated.TTS.APIKey != "" {
		t.Fatalf("handler must preserve empty-key semantics: %+v", store.updated)
	}
}

func validXinzhiliModelConfigForHandler() xinzhili.Config {
	return xinzhili.Config{
		Enabled:      true,
		RealtimeASR:  xinzhili.RealtimeASRConfig{Provider: xinzhili.RealtimeASRProvider, Endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference", APIKey: "asr-secret", Region: "cn-beijing", Model: xinzhili.RealtimeASRModel},
		TTS:          xinzhili.TTSConfig{Provider: xinzhili.TTSProviderOpenAICompatible, Endpoint: "https://tts.example.com/v1", APIKey: "tts-secret", Model: "tts-1", Voice: "alloy", Format: "mp3"},
		EnabledModes: []xinzhili.Mode{xinzhili.ModeNormal},
	}
}

func TestXinzhiliModelConfigPUTMapsValidationAndConflict(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "normal required", err: xinzhili.ErrNormalModeRequired, status: http.StatusBadRequest},
		{name: "version conflict", err: xinzhili.ErrConfigConflict, status: http.StatusConflict, code: "config_version_conflict"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{xinzhiliModelConfig: &fakeXinzhiliModelConfigStore{updateErr: tt.err}}
			res := httptest.NewRecorder()
			s.xinzhiliModelConfigHandler(res, httptest.NewRequest(http.MethodPut, "/api/xinzhili-model-config", strings.NewReader(`{"expectedVersion":1}`)))
			if res.Code != tt.status || (tt.code != "" && !strings.Contains(res.Body.String(), tt.code)) {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
		})
	}
}

func TestXinzhiliModelConfigPUTRequiresExpectedVersion(t *testing.T) {
	store := &fakeXinzhiliModelConfigStore{}
	s := &Server{xinzhiliModelConfig: store}
	res := httptest.NewRecorder()
	s.xinzhiliModelConfigHandler(res, httptest.NewRequest(http.MethodPut, "/api/xinzhili-model-config", strings.NewReader(`{"enabled":false}`)))
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "expectedVersion") {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestXinzhiliModelConfigPUTMapsStoreFailureToServerError(t *testing.T) {
	store := &fakeXinzhiliModelConfigStore{config: xinzhili.Config{}, found: true, updateErr: errors.New("db down")}
	s := &Server{xinzhiliModelConfig: store}
	res := httptest.NewRecorder()
	s.xinzhiliModelConfigHandler(res, httptest.NewRequest(http.MethodPut, "/api/xinzhili-model-config", strings.NewReader(`{"expectedVersion":1,"enabled":false}`)))
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestXinzhiliModelConfigPUTRejectsInvalidConfigBeforeStore(t *testing.T) {
	store := &fakeXinzhiliModelConfigStore{}
	s := &Server{xinzhiliModelConfig: store}
	res := httptest.NewRecorder()
	s.xinzhiliModelConfigHandler(res, httptest.NewRequest(http.MethodPut, "/api/xinzhili-model-config", strings.NewReader(`{"expectedVersion":0,"enabled":true,"enabledModes":["comfort"]}`)))
	if res.Code != http.StatusBadRequest || store.expectedVersion != 0 {
		t.Fatalf("status=%d body=%s updateVersion=%d", res.Code, res.Body.String(), store.expectedVersion)
	}
}

func TestXinzhiliModelConfigRouteUsesDedicatedPermission(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	want := `s.mux.HandleFunc("/api/xinzhili-model-config", s.requirePermission("System:XinzhiliModel:Config", s.xinzhiliModelConfigHandler))`
	if !strings.Contains(string(raw), want) {
		t.Fatalf("missing protected route %s", want)
	}
}

var _ xinzhiliModelConfigStore = (*fakeXinzhiliModelConfigStore)(nil)
