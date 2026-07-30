package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/bailianconfig"
	"nine-xing/nx-backend/apps/server/internal/voice"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

type memoryBailianCredentialStore struct {
	mu      sync.Mutex
	cfg     bailianconfig.Config
	found   bool
	readErr error
}

func (s *memoryBailianCredentialStore) Read(context.Context) (bailianconfig.Config, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readErr != nil {
		return bailianconfig.Config{}, false, s.readErr
	}
	return s.cfg, s.found, nil
}

func (s *memoryBailianCredentialStore) Update(_ context.Context, apiKey string, expectedVersion int64, clearAPIKey bool) (bailianconfig.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if (!s.found && expectedVersion != 0) || (s.found && s.cfg.Version != expectedVersion) {
		return bailianconfig.Config{}, bailianconfig.ErrConflict
	}
	apiKey = strings.TrimSpace(apiKey)
	if !s.found && apiKey == "" && !clearAPIKey {
		return bailianconfig.Config{}, nil
	}
	next := bailianconfig.Config{APIKey: apiKey, Version: 1}
	if s.found {
		next.Version = s.cfg.Version + 1
		if apiKey == "" && !clearAPIKey {
			next.APIKey = s.cfg.APIKey
		}
	}
	if clearAPIKey {
		next.APIKey = ""
	}
	s.cfg, s.found = next, true
	return next, nil
}

type staticXinzhiliConfigStore struct {
	cfg   xinzhili.Config
	found bool
	err   error
}

func (s staticXinzhiliConfigStore) Read(context.Context) (xinzhili.Config, bool, error) {
	return s.cfg, s.found, s.err
}

func (staticXinzhiliConfigStore) Update(context.Context, xinzhili.Config, int64) (xinzhili.Config, error) {
	return xinzhili.Config{}, errors.New("unexpected update")
}

func TestBailianCredentialsSharedRecordTakesPriorityOverLegacyConfiguration(t *testing.T) {
	s := &Server{
		bailianCredentials: &memoryBailianCredentialStore{cfg: bailianconfig.Config{Version: 3, APIKey: "sk-shared-final"}, found: true},
		xinzhiliModelConfig: staticXinzhiliConfigStore{found: true, cfg: xinzhili.Config{
			TTS:         xinzhili.TTSConfig{Provider: xinzhili.TTSProviderBailian, APIKey: "sk-legacy-tts"},
			RealtimeASR: xinzhili.RealtimeASRConfig{Provider: xinzhili.RealtimeASRProvider, APIKey: "sk-legacy-asr"},
		}},
	}

	got, err := s.resolveBailianCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "sk-shared-final" || got.Version != 3 || got.Source != bailianCredentialSourceShared {
		t.Fatalf("resolved=%#v want shared version 3", got)
	}
}

func TestBailianCredentialsEmptySharedRecordDisablesLegacyFallback(t *testing.T) {
	s := &Server{
		bailianCredentials: &memoryBailianCredentialStore{cfg: bailianconfig.Config{Version: 4}, found: true},
		xinzhiliModelConfig: staticXinzhiliConfigStore{found: true, cfg: xinzhili.Config{
			TTS:         xinzhili.TTSConfig{Provider: xinzhili.TTSProviderBailian, APIKey: "sk-legacy-tts"},
			RealtimeASR: xinzhili.RealtimeASRConfig{Provider: xinzhili.RealtimeASRProvider, APIKey: "sk-legacy-asr"},
		}},
	}

	got, err := s.resolveBailianCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "" || got.Version != 4 || got.Source != bailianCredentialSourceShared {
		t.Fatalf("resolved=%#v want explicit empty shared credential", got)
	}
}

func TestBailianCredentialsLegacyTTSFallbackOnlyAcceptsBailianOrOfficialDashScope(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		endpoint string
		wantKey  bool
	}{
		{name: "native bailian provider", provider: xinzhili.TTSProviderBailian, endpoint: "https://custom.example.invalid/api/v1", wantKey: true},
		{name: "official native endpoint", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com/api/v1", wantKey: true},
		{name: "official compatible endpoint", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1", wantKey: true},
		{name: "official generation endpoint", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation", wantKey: true},
		{name: "similar suffix domain", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com.example/api/v1"},
		{name: "similar prefix domain", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com.evil.test/compatible-mode/v1"},
		{name: "userinfo deception", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com@evil.test/api/v1"},
		{name: "http scheme", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "http://dashscope.aliyuncs.com/api/v1"},
		{name: "unexpected port", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com:444/api/v1"},
		{name: "unsupported path", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com/evil/v1"},
		{name: "native prefix is not enough", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com/api/v1/not-a-tts-endpoint"},
		{name: "compatible prefix is not enough", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1/not-a-tts-endpoint"},
		{name: "parent traversal path", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com/api/v1/../../evil"},
		{name: "current traversal path", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com/api/v1/./generation"},
		{name: "encoded parent traversal path", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://dashscope.aliyuncs.com/api/v1/%2e%2e/evil"},
		{name: "minimax", provider: xinzhili.TTSProviderMiniMax, endpoint: "https://dashscope.aliyuncs.com/api/v1"},
		{name: "siliconflow compatible", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://api.siliconflow.cn/v1"},
		{name: "other compatible", provider: xinzhili.TTSProviderOpenAICompatible, endpoint: "https://tts.example.com/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				bailianCredentials: &memoryBailianCredentialStore{},
				xinzhiliModelConfig: staticXinzhiliConfigStore{found: true, cfg: xinzhili.Config{TTS: xinzhili.TTSConfig{
					Provider: tt.provider, Endpoint: tt.endpoint, APIKey: "sk-legacy-tts",
				}}},
			}
			got, err := s.resolveBailianCredentials(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if (got.APIKey != "") != tt.wantKey {
				t.Fatalf("resolved=%#v wantKey=%v", got, tt.wantKey)
			}
			if tt.wantKey && got.Source != bailianCredentialSourceLegacyTTS {
				t.Fatalf("source=%q want legacy TTS", got.Source)
			}
		})
	}
}

func TestBailianCredentialsLegacyASRFallbackRequiresOfficialParaformerConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		endpoint string
		wantKey  bool
	}{
		{name: "official wss", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference", wantKey: true},
		{name: "official https compatibility", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "https://dashscope.aliyuncs.com/api-ws/v1/inference", wantKey: true},
		{name: "official explicit default port", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "wss://dashscope.aliyuncs.com:443/api-ws/v1/inference", wantKey: true},
		{name: "wrong provider", provider: "other-asr", model: xinzhili.RealtimeASRModel, endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference"},
		{name: "wrong model", provider: xinzhili.RealtimeASRProvider, model: "paraformer-v1", endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference"},
		{name: "similar domain", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "wss://dashscope.aliyuncs.com.example/api-ws/v1/inference"},
		{name: "userinfo deception", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "wss://dashscope.aliyuncs.com@evil.test/api-ws/v1/inference"},
		{name: "wrong path", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/other"},
		{name: "trailing slash is not exact", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference/"},
		{name: "dot segment", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/./inference"},
		{name: "encoded dot segment", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/%2e%2e/inference"},
		{name: "http scheme", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "http://dashscope.aliyuncs.com/api-ws/v1/inference"},
		{name: "unexpected port", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "wss://dashscope.aliyuncs.com:444/api-ws/v1/inference"},
		{name: "query is not exact", provider: xinzhili.RealtimeASRProvider, model: xinzhili.RealtimeASRModel, endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference?token=x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				bailianCredentials: &memoryBailianCredentialStore{},
				xinzhiliModelConfig: staticXinzhiliConfigStore{found: true, cfg: xinzhili.Config{
					TTS: xinzhili.TTSConfig{Provider: xinzhili.TTSProviderMiniMax, APIKey: "minimax-secret"},
					RealtimeASR: xinzhili.RealtimeASRConfig{
						Provider: tt.provider,
						Endpoint: tt.endpoint,
						Model:    tt.model,
						APIKey:   "sk-legacy-asr",
					},
				}},
			}
			got, err := s.resolveBailianCredentials(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if (got.APIKey != "") != tt.wantKey {
				t.Fatalf("resolved=%#v wantKey=%v", got, tt.wantKey)
			}
			if tt.wantKey && got.Source != bailianCredentialSourceLegacyASR {
				t.Fatalf("source=%q want legacy ASR", got.Source)
			}
		})
	}
}

func TestBailianCredentialsGETReturnsOnlySafeView(t *testing.T) {
	s := &Server{bailianCredentials: &memoryBailianCredentialStore{
		cfg: bailianconfig.Config{Version: 7, APIKey: "sk-secret-Q9UY"}, found: true,
	}}
	rr := performBailianCredentialRequest(s, http.MethodGet, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "sk-secret-Q9UY") || strings.Contains(rr.Body.String(), `"apiKey"`) {
		t.Fatalf("GET leaked plaintext API key or apiKey field: %s", rr.Body.String())
	}
	var envelope struct {
		Data bailianCredentialView `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Version != 7 || !envelope.Data.APIKeySet || envelope.Data.APIKeySuffix != "Q9UY" || envelope.Data.Source != bailianCredentialSourceShared {
		t.Fatalf("view=%#v", envelope.Data)
	}
}

func TestBailianCredentialsPUTRequiresExpectedVersionAndDoesNotOverwriteOnConflict(t *testing.T) {
	store := &memoryBailianCredentialStore{cfg: bailianconfig.Config{Version: 2, APIKey: "sk-current-ABCD"}, found: true}
	s := &Server{bailianCredentials: store, setBailianCopyConfig: func(voice.BailianConfig) {}}

	missing := performBailianCredentialRequest(s, http.MethodPut, `{"apiKey":"sk-next"}`)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing expectedVersion status=%d body=%s", missing.Code, missing.Body.String())
	}
	conflict := performBailianCredentialRequest(s, http.MethodPut, `{"expectedVersion":1,"apiKey":"sk-next"}`)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	stored, _, _ := store.Read(context.Background())
	if stored.APIKey != "sk-current-ABCD" || stored.Version != 2 {
		t.Fatalf("conflict overwrote stored credentials: %#v", stored)
	}
	wrongMethod := performBailianCredentialRequest(s, http.MethodPost, `{}`)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d body=%s", wrongMethod.Code, wrongMethod.Body.String())
	}
}

func TestBailianCredentialsPUTSavesAndClearsSharedKeyWithCAS(t *testing.T) {
	store := &memoryBailianCredentialStore{}
	var configured voice.BailianConfig
	s := &Server{
		bailianCredentials: store,
		setBailianCopyConfig: func(cfg voice.BailianConfig) {
			configured = cfg
		},
	}

	created := performBailianCredentialRequest(s, http.MethodPut, `{"expectedVersion":0,"apiKey":" sk-created-EFGH ","clearApiKey":false}`)
	if created.Code != http.StatusOK || strings.Contains(created.Body.String(), "sk-created-EFGH") {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	stored, found, err := store.Read(context.Background())
	if err != nil || !found || stored != (bailianconfig.Config{Version: 1, APIKey: "sk-created-EFGH"}) {
		t.Fatalf("stored=%#v found=%v err=%v", stored, found, err)
	}
	if configured.APIBase != defaultSharedBailianAPIBase || configured.APIKey != stored.APIKey || configured.TargetModel != defaultSharedBailianCloneModel {
		t.Fatalf("configured=%#v", configured)
	}

	cleared := performBailianCredentialRequest(s, http.MethodPut, `{"expectedVersion":1,"apiKey":"","clearApiKey":true}`)
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", cleared.Code, cleared.Body.String())
	}
	stored, found, err = store.Read(context.Background())
	if err != nil || !found || stored != (bailianconfig.Config{Version: 2}) {
		t.Fatalf("cleared stored=%#v found=%v err=%v", stored, found, err)
	}
	if configured.APIKey != "" || configured.APIBase != defaultSharedBailianAPIBase || configured.TargetModel != defaultSharedBailianCloneModel {
		t.Fatalf("clear configured=%#v", configured)
	}
}

func TestBailianCredentialsPUTUsesCommittedCASResultWhenPostCommitReadWouldFail(t *testing.T) {
	store := &memoryBailianCredentialStore{readErr: errors.New("post-commit read unavailable")}
	var configured voice.BailianConfig
	s := &Server{
		bailianCredentials: store,
		setBailianCopyConfig: func(cfg voice.BailianConfig) {
			configured = cfg
		},
	}

	rr := performBailianCredentialRequest(s, http.MethodPut, `{"expectedVersion":0,"apiKey":"sk-committed-IJKL","clearApiKey":false}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("database commit succeeded, so response must not depend on a second read: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if configured.APIKey != "sk-committed-IJKL" || configured.APIBase != defaultSharedBailianAPIBase || configured.TargetModel != defaultSharedBailianCloneModel {
		t.Fatalf("configured=%#v want committed CAS result", configured)
	}
	if strings.Contains(rr.Body.String(), "sk-committed-IJKL") {
		t.Fatalf("PUT leaked committed plaintext: %s", rr.Body.String())
	}
}

func TestBailianCredentialsFirstEmptyPUTIsNoOpAndKeepsLegacyFallback(t *testing.T) {
	store := &memoryBailianCredentialStore{}
	var configured voice.BailianConfig
	s := &Server{
		bailianCredentials: store,
		xinzhiliModelConfig: staticXinzhiliConfigStore{found: true, cfg: xinzhili.Config{TTS: xinzhili.TTSConfig{
			Provider: xinzhili.TTSProviderBailian, APIKey: "sk-legacy-WXYZ",
		}}},
		setBailianCopyConfig: func(cfg voice.BailianConfig) { configured = cfg },
	}

	rr := performBailianCredentialRequest(s, http.MethodPut, `{"expectedVersion":0,"apiKey":"","clearApiKey":false}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, found, _ := store.Read(context.Background()); found {
		t.Fatal("empty first save created a shared record")
	}
	if configured.APIKey != "sk-legacy-WXYZ" || configured.APIBase != defaultSharedBailianAPIBase || configured.TargetModel != defaultSharedBailianCloneModel {
		t.Fatalf("configured=%#v want legacy effective key with fixed Bailian runtime", configured)
	}
	if strings.Contains(rr.Body.String(), "sk-legacy-WXYZ") {
		t.Fatalf("PUT leaked legacy plaintext: %s", rr.Body.String())
	}
}

func TestBailianCredentialsRuntimeRefreshRejectsOlderCASResultAppliedLater(t *testing.T) {
	store := &memoryBailianCredentialStore{}
	first, err := store.Update(context.Background(), "sk-version-one", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Update(context.Background(), "sk-version-two", first.Version, false)
	if err != nil {
		t.Fatal(err)
	}

	var configured []string
	s := &Server{setBailianCopyConfig: func(cfg voice.BailianConfig) { configured = append(configured, cfg.APIKey) }}
	s.applyBailianCredentialRuntime(resolvedBailianCredential{Config: second, Source: bailianCredentialSourceShared})
	s.applyBailianCredentialRuntime(resolvedBailianCredential{Config: first, Source: bailianCredentialSourceShared})

	if len(configured) != 1 || configured[0] != "sk-version-two" {
		t.Fatalf("configured sequence=%v want only latest database version", configured)
	}
}

func performBailianCredentialRequest(s *Server, method string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/voice/bailian-credentials", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	s.bailianCredentialsHandler(response, request)
	return response
}

var _ bailianCredentialStore = (*memoryBailianCredentialStore)(nil)
