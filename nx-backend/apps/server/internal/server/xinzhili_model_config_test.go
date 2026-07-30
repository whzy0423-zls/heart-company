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

	"nine-xing/nx-backend/apps/server/internal/bailianconfig"
	"nine-xing/nx-backend/apps/server/internal/voice"
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
	merged := xinzhili.MergeIncoming(f.config, cfg)
	normalized, err := merged.WithDefaults()
	if err != nil {
		return xinzhili.Config{}, err
	}
	if f.found {
		normalized.Version = f.config.Version + 1
	} else {
		normalized.Version = 1
	}
	f.config = normalized
	f.found = true
	return normalized, nil
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

func TestXinzhiliCredentialSaveAcceptsSharedBailianKeyOutsideModelJSON(t *testing.T) {
	cfg := validBailianXinzhiliModelConfigForHandler()
	store := &fakeXinzhiliModelConfigStore{}
	s := &Server{
		xinzhiliModelConfig: store,
		bailianCredentials: &memoryBailianCredentialStore{
			cfg: bailianconfig.Config{Version: 2, APIKey: "sk-shared-runtime"}, found: true,
		},
	}
	res := putXinzhiliModelConfig(t, s, cfg, 0)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if store.config.RealtimeASR.APIKey != "" || store.config.TTS.APIKey != "" {
		t.Fatalf("shared credential leaked into persisted Xinzhili JSON: %+v", store.config)
	}
}

func TestXinzhiliCredentialSaveRejectsEnabledConfigWithoutSharedOrLegalLegacyKey(t *testing.T) {
	cfg := validBailianXinzhiliModelConfigForHandler()
	store := &fakeXinzhiliModelConfigStore{}
	s := &Server{xinzhiliModelConfig: store, bailianCredentials: &memoryBailianCredentialStore{}}
	res := putXinzhiliModelConfig(t, s, cfg, 0)
	if res.Code != http.StatusBadRequest || store.updateCalls != 0 {
		t.Fatalf("status=%d updateCalls=%d body=%s", res.Code, store.updateCalls, res.Body.String())
	}
}

func TestXinzhiliCredentialSaveEmptySharedRecordBlocksLegacyFallback(t *testing.T) {
	legacy := validBailianXinzhiliModelConfigForHandler()
	legacy.Version = 7
	legacy.RealtimeASR.APIKey = "sk-legacy-asr"
	legacy.TTS.APIKey = "sk-legacy-tts"
	store := &fakeXinzhiliModelConfigStore{config: legacy, found: true}
	incoming := legacy
	incoming.RealtimeASR.APIKey = ""
	incoming.TTS.APIKey = ""
	s := &Server{
		xinzhiliModelConfig: store,
		bailianCredentials: &memoryBailianCredentialStore{
			cfg: bailianconfig.Config{Version: 3}, found: true,
		},
	}
	res := putXinzhiliModelConfig(t, s, incoming, 7)
	if res.Code != http.StatusBadRequest || store.updateCalls != 0 {
		t.Fatalf("status=%d updateCalls=%d body=%s", res.Code, store.updateCalls, res.Body.String())
	}
}

func TestXinzhiliCredentialSaveSharedRecordClearsLegacyBailianKeys(t *testing.T) {
	legacy := validBailianXinzhiliModelConfigForHandler()
	legacy.Version = 4
	legacy.RealtimeASR.APIKey = "sk-legacy-asr"
	legacy.TTS.APIKey = "sk-legacy-tts"
	store := &fakeXinzhiliModelConfigStore{config: legacy, found: true}
	incoming := legacy
	incoming.RealtimeASR.APIKey = ""
	incoming.TTS.APIKey = ""
	s := &Server{
		xinzhiliModelConfig: store,
		bailianCredentials: &memoryBailianCredentialStore{
			cfg: bailianconfig.Config{Version: 8, APIKey: "sk-shared"}, found: true,
		},
	}
	res := putXinzhiliModelConfig(t, s, incoming, 4)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if store.config.RealtimeASR.APIKey != "" || store.config.TTS.APIKey != "" {
		t.Fatalf("legacy Bailian keys were retained: %+v", store.config)
	}
}

func TestXinzhiliCredentialSaveWithoutSharedRecordPreservesLegalLegacyFallback(t *testing.T) {
	legacy := validBailianXinzhiliModelConfigForHandler()
	legacy.Version = 6
	legacy.RealtimeASR.APIKey = "sk-legacy-asr"
	legacy.TTS.APIKey = "sk-legacy-tts"
	store := &fakeXinzhiliModelConfigStore{config: legacy, found: true}
	incoming := legacy
	incoming.RealtimeASR.APIKey = ""
	incoming.TTS.APIKey = ""
	incoming.CommonPrompt = "only update prompt"
	s := &Server{xinzhiliModelConfig: store, bailianCredentials: &memoryBailianCredentialStore{}}
	res := putXinzhiliModelConfig(t, s, incoming, 6)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if store.config.RealtimeASR.APIKey != "sk-legacy-asr" || store.config.TTS.APIKey != "sk-legacy-tts" {
		t.Fatalf("legal legacy fallback was lost: %+v", store.config)
	}
}

func TestXinzhiliCredentialSaveWithoutSharedRecordAcceptsLegalIncomingFallback(t *testing.T) {
	cfg := validBailianXinzhiliModelConfigForHandler()
	cfg.RealtimeASR.APIKey = "sk-incoming-asr"
	cfg.TTS.APIKey = "sk-incoming-tts"
	store := &fakeXinzhiliModelConfigStore{}
	s := &Server{xinzhiliModelConfig: store, bailianCredentials: &memoryBailianCredentialStore{}}
	res := putXinzhiliModelConfig(t, s, cfg, 0)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if store.config.RealtimeASR.APIKey != "sk-incoming-asr" || store.config.TTS.APIKey != "sk-incoming-tts" {
		t.Fatalf("incoming legal fallback was not persisted: %+v", store.config)
	}
}

func TestXinzhiliCredentialSaveKeepsMiniMaxPrivateKeySeparateFromSharedKey(t *testing.T) {
	cfg := validXinzhiliModelConfigForHandler()
	cfg.Version = 5
	cfg.RealtimeASR.APIKey = "sk-old-asr"
	cfg.TTS = xinzhili.TTSConfig{
		Provider: xinzhili.TTSProviderMiniMax,
		Endpoint: "https://api.minimax.chat/v1/t2a_v2",
		APIKey:   "minimax-private",
		GroupID:  "minimax-group",
		Model:    "speech-02-hd",
		Voice:    "minimax-voice",
		Format:   "mp3",
	}
	store := &fakeXinzhiliModelConfigStore{config: cfg, found: true}
	incoming := cfg
	incoming.RealtimeASR.APIKey = ""
	incoming.TTS.APIKey = ""
	s := &Server{
		xinzhiliModelConfig: store,
		bailianCredentials: &memoryBailianCredentialStore{
			cfg: bailianconfig.Config{Version: 9, APIKey: "sk-shared"}, found: true,
		},
	}
	res := putXinzhiliModelConfig(t, s, incoming, 5)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if store.config.RealtimeASR.APIKey != "" || store.config.TTS.APIKey != "minimax-private" {
		t.Fatalf("credential separation failed: %+v", store.config)
	}
}

func TestXinzhiliCredentialSaveDoesNotReuseLegacyBailianKeyAsMiniMaxPrivateKey(t *testing.T) {
	legacy := validBailianXinzhiliModelConfigForHandler()
	legacy.Version = 3
	legacy.RealtimeASR.APIKey = "sk-legacy-asr"
	legacy.TTS.APIKey = "sk-legacy-bailian-tts"
	store := &fakeXinzhiliModelConfigStore{config: legacy, found: true}
	incoming := legacy
	incoming.RealtimeASR.APIKey = ""
	incoming.TTS = xinzhili.TTSConfig{
		Provider: xinzhili.TTSProviderMiniMax,
		Endpoint: "https://api.minimax.chat/v1/t2a_v2",
		GroupID:  "minimax-group",
		Model:    "speech-02-hd",
		Voice:    "minimax-voice",
		Format:   "mp3",
	}
	s := &Server{
		xinzhiliModelConfig: store,
		bailianCredentials: &memoryBailianCredentialStore{
			cfg: bailianconfig.Config{Version: 7, APIKey: "sk-shared"}, found: true,
		},
	}
	res := putXinzhiliModelConfig(t, s, incoming, 3)
	if res.Code != http.StatusBadRequest || store.updateCalls != 0 {
		t.Fatalf("status=%d updateCalls=%d body=%s", res.Code, store.updateCalls, res.Body.String())
	}
}

func TestXinzhiliCredentialSaveDoesNotSendPrivateKeyToChangedEndpointHost(t *testing.T) {
	for _, tt := range []struct {
		name string
		tts  xinzhili.TTSConfig
	}{
		{
			name: "openai compatible",
			tts: xinzhili.TTSConfig{
				Provider: xinzhili.TTSProviderOpenAICompatible,
				Endpoint: "https://provider-a.example/v1",
				APIKey:   "provider-a-private",
				Model:    "tts-1",
				Voice:    "voice-a",
				Format:   "mp3",
			},
		},
		{
			name: "minimax",
			tts: xinzhili.TTSConfig{
				Provider: xinzhili.TTSProviderMiniMax,
				Endpoint: "https://api-a.minimax.example/v1/t2a_v2",
				APIKey:   "minimax-private",
				GroupID:  "minimax-group",
				Model:    "speech-02-hd",
				Voice:    "voice-a",
				Format:   "mp3",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stored := validXinzhiliModelConfigForHandler()
			stored.Version = 10
			stored.RealtimeASR.APIKey = "sk-legacy-asr"
			stored.TTS = tt.tts
			store := &fakeXinzhiliModelConfigStore{config: stored, found: true}
			incoming := stored
			incoming.RealtimeASR.APIKey = ""
			incoming.TTS.Endpoint = "https://provider-b.example/v1"
			incoming.TTS.APIKey = ""
			s := &Server{xinzhiliModelConfig: store, bailianCredentials: &memoryBailianCredentialStore{}}
			res := putXinzhiliModelConfig(t, s, incoming, 10)
			if res.Code != http.StatusBadRequest || store.updateCalls != 0 {
				t.Fatalf("status=%d updateCalls=%d body=%s", res.Code, store.updateCalls, res.Body.String())
			}
		})
	}
}

func TestXinzhiliCredentialSavePreservesPrivateKeyAcrossSameOriginPathChange(t *testing.T) {
	stored := validXinzhiliModelConfigForHandler()
	stored.Version = 11
	stored.RealtimeASR.APIKey = "sk-legacy-asr"
	stored.TTS = xinzhili.TTSConfig{
		Provider: xinzhili.TTSProviderOpenAICompatible,
		Endpoint: "https://provider-a.example/v1",
		APIKey:   "provider-a-private",
		Model:    "tts-1",
		Voice:    "voice-a",
		Format:   "mp3",
	}
	store := &fakeXinzhiliModelConfigStore{config: stored, found: true}
	incoming := stored
	incoming.RealtimeASR.APIKey = ""
	incoming.TTS.Endpoint = "https://provider-a.example/v1/audio/speech/"
	incoming.TTS.APIKey = ""
	s := &Server{xinzhiliModelConfig: store, bailianCredentials: &memoryBailianCredentialStore{}}
	res := putXinzhiliModelConfig(t, s, incoming, 11)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if store.config.TTS.APIKey != "provider-a-private" {
		t.Fatalf("same-origin path change lost private key: %+v", store.config.TTS)
	}
}

func TestXinzhiliCredentialSaveBailianEndpointPathStillUsesSharedKey(t *testing.T) {
	stored := validBailianXinzhiliModelConfigForHandler()
	stored.Version = 12
	store := &fakeXinzhiliModelConfigStore{config: stored, found: true}
	incoming := stored
	incoming.TTS.Endpoint = "https://dashscope.aliyuncs.com/api/v1"
	s := &Server{
		xinzhiliModelConfig: store,
		bailianCredentials: &memoryBailianCredentialStore{
			cfg: bailianconfig.Config{Version: 5, APIKey: "sk-shared"}, found: true,
		},
	}
	res := putXinzhiliModelConfig(t, s, incoming, 12)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if store.config.RealtimeASR.APIKey != "" || store.config.TTS.APIKey != "" {
		t.Fatalf("Bailian shared key path persisted into model config: %+v", store.config)
	}
}

func TestXinzhiliCredentialSaveDisabledAllowsEmptyVoiceAndCredentials(t *testing.T) {
	cfg := validBailianXinzhiliModelConfigForHandler()
	cfg.Enabled = false
	cfg.TTS.Voice = ""
	store := &fakeXinzhiliModelConfigStore{}
	s := &Server{xinzhiliModelConfig: store, bailianCredentials: &memoryBailianCredentialStore{}}
	res := putXinzhiliModelConfig(t, s, cfg, 0)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestBailianRuntimeStartupRefreshesSharedCredentialsIndependentOfTTSProvider(t *testing.T) {
	legacy := validXinzhiliModelConfigForHandler()
	legacy.Version = 12
	legacy.TTS = xinzhili.TTSConfig{
		Provider: xinzhili.TTSProviderMiniMax,
		Endpoint: "https://api.minimax.chat/v1/t2a_v2",
		APIKey:   "minimax-private",
		GroupID:  "minimax-group",
		Model:    "speech-02-hd",
		Voice:    "minimax-voice",
		Format:   "mp3",
	}
	var got voice.BailianConfig
	s := &Server{
		xinzhiliModelConfig: staticXinzhiliConfigStore{found: true, cfg: legacy},
		bailianCredentials: &memoryBailianCredentialStore{
			cfg: bailianconfig.Config{Version: 4, APIKey: "sk-shared-clone"}, found: true,
		},
		setBailianCopyConfig: func(cfg voice.BailianConfig) { got = cfg },
	}
	if _, err := s.refreshBailianCopyCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "sk-shared-clone" || got.APIBase != defaultSharedBailianAPIBase || got.TargetModel != defaultSharedBailianCloneModel {
		t.Fatalf("startup Bailian clone config=%+v", got)
	}
	if legacy.TTS.APIKey != "minimax-private" {
		t.Fatalf("shared Bailian key overwrote MiniMax private key: %+v", legacy.TTS)
	}

	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "s.refreshBailianCopyCredentials(context.Background())") {
		t.Fatal("server startup does not refresh the shared Bailian credential runtime")
	}
}

func putXinzhiliModelConfig(t *testing.T, s *Server, cfg xinzhili.Config, expectedVersion int64) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(xinzhiliModelConfigUpdateRequest{Config: cfg, ExpectedVersion: &expectedVersion})
	if err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/xinzhili-model-config", strings.NewReader(string(body)))
	s.xinzhiliModelConfigHandler(res, req)
	return res
}

func validBailianXinzhiliModelConfigForHandler() xinzhili.Config {
	cfg := validXinzhiliModelConfigForHandler()
	cfg.RealtimeASR.APIKey = ""
	cfg.TTS = xinzhili.TTSConfig{
		Provider: xinzhili.TTSProviderOpenAICompatible,
		Endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		Model:    "qwen3-tts-vc-2026-01-22",
		Voice:    "teacher-voice",
		Format:   "mp3",
	}
	return cfg
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
