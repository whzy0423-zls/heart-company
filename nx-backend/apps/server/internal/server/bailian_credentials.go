package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	pathpkg "path"
	"strings"
	"sync"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/bailianconfig"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/voice"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

const (
	defaultSharedBailianAPIBase    = "https://dashscope.aliyuncs.com"
	defaultSharedBailianCloneModel = "qwen3-tts-vc-2026-01-22"

	bailianCredentialSourceShared    = "shared"
	bailianCredentialSourceLegacyTTS = "legacy-tts"
	bailianCredentialSourceLegacyASR = "legacy-asr"
	bailianCredentialSourceNone      = "none"

	bailianCredentialRuntimeEpochNone   = 0
	bailianCredentialRuntimeEpochLegacy = 1
	bailianCredentialRuntimeEpochShared = 2

	maxSharedBailianAPIKeyRunes = 4096
)

type bailianCredentialStore interface {
	Read(context.Context) (bailianconfig.Config, bool, error)
	Update(context.Context, string, int64, bool) (bailianconfig.Config, error)
}

type databaseBailianCredentialStore struct{ db *sql.DB }

func (s databaseBailianCredentialStore) Read(ctx context.Context) (bailianconfig.Config, bool, error) {
	return bailianconfig.Read(ctx, s.db)
}

func (s databaseBailianCredentialStore) Update(ctx context.Context, apiKey string, expectedVersion int64, clearAPIKey bool) (bailianconfig.Config, error) {
	return bailianconfig.Update(ctx, s.db, apiKey, expectedVersion, clearAPIKey)
}

type resolvedBailianCredential struct {
	bailianconfig.Config
	Source         string
	runtimeEpoch   int
	runtimeVersion int64
}

type bailianCredentialView struct {
	Version      int64  `json:"version"`
	APIKeySet    bool   `json:"apiKeySet"`
	APIKeySuffix string `json:"apiKeySuffix"`
	Source       string `json:"source"`
}

type bailianCredentialUpdateRequest struct {
	ExpectedVersion *int64 `json:"expectedVersion"`
	APIKey          string `json:"apiKey"`
	ClearAPIKey     bool   `json:"clearApiKey"`
}

type bailianCredentialRuntimeState struct {
	sync.Mutex
	set     bool
	epoch   int
	version int64
}

func (s *Server) bailianCredentialsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	if s.bailianCredentials == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "百炼凭证服务暂不可用")
		return
	}

	if r.Method == http.MethodGet {
		resolved, err := s.resolveBailianCredentials(r.Context())
		if err != nil {
			log.Printf("bailian credentials read failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, "百炼凭证读取失败")
			return
		}
		httpx.OK(w, buildBailianCredentialView(resolved))
		return
	}

	var input bailianCredentialUpdateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if input.ExpectedVersion == nil {
		httpx.Fail(w, http.StatusBadRequest, "expectedVersion is required")
		return
	}
	input.APIKey = strings.TrimSpace(input.APIKey)
	if utf8.RuneCountInString(input.APIKey) > maxSharedBailianAPIKeyRunes {
		httpx.Fail(w, http.StatusBadRequest, "API Key 长度超出限制")
		return
	}
	updated, err := s.bailianCredentials.Update(r.Context(), input.APIKey, *input.ExpectedVersion, input.ClearAPIKey)
	if err != nil {
		if errors.Is(err, bailianconfig.ErrConflict) {
			httpx.Fail(w, http.StatusConflict, "bailian_credentials_version_conflict")
			return
		}
		log.Printf("bailian credentials update failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, "百炼凭证保存失败")
		return
	}

	resolved := resolvedBailianCredential{
		Config: updated, Source: bailianCredentialSourceShared,
		runtimeEpoch: bailianCredentialRuntimeEpochShared, runtimeVersion: updated.Version,
	}
	if updated.Version == 0 {
		resolved, err = s.refreshBailianCopyCredentials(r.Context())
		if err != nil {
			log.Printf("bailian credentials refresh after no-op failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, "百炼凭证读取失败")
			return
		}
	} else {
		s.applyBailianCredentialRuntime(resolved)
	}
	httpx.OK(w, buildBailianCredentialView(resolved))
}

func (s *Server) resolveBailianCredentials(ctx context.Context) (resolvedBailianCredential, error) {
	if s.bailianCredentials == nil {
		return resolvedBailianCredential{}, errors.New("bailian credential store is not initialized")
	}
	shared, found, err := s.bailianCredentials.Read(ctx)
	if err != nil {
		return resolvedBailianCredential{}, err
	}
	if found {
		shared.APIKey = strings.TrimSpace(shared.APIKey)
		return resolvedBailianCredential{
			Config: shared, Source: bailianCredentialSourceShared,
			runtimeEpoch: bailianCredentialRuntimeEpochShared, runtimeVersion: shared.Version,
		}, nil
	}

	if s.xinzhiliModelConfig == nil {
		return resolvedBailianCredential{Source: bailianCredentialSourceNone, runtimeEpoch: bailianCredentialRuntimeEpochNone}, nil
	}
	legacy, legacyFound, err := s.xinzhiliModelConfig.Read(ctx)
	if err != nil {
		return resolvedBailianCredential{}, err
	}
	if !legacyFound {
		return resolvedBailianCredential{Source: bailianCredentialSourceNone, runtimeEpoch: bailianCredentialRuntimeEpochNone}, nil
	}
	legacyRuntimeVersion := legacy.Version

	ttsKey := strings.TrimSpace(legacy.TTS.APIKey)
	ttsProvider := strings.ToLower(strings.TrimSpace(legacy.TTS.Provider))
	if ttsKey != "" && (ttsProvider == xinzhili.TTSProviderBailian ||
		(ttsProvider == xinzhili.TTSProviderOpenAICompatible && isOfficialDashScopeTTSEndpoint(legacy.TTS.Endpoint))) {
		return resolvedBailianCredential{
			Config:       bailianconfig.Config{APIKey: ttsKey},
			Source:       bailianCredentialSourceLegacyTTS,
			runtimeEpoch: bailianCredentialRuntimeEpochLegacy, runtimeVersion: legacyRuntimeVersion,
		}, nil
	}

	asrKey := strings.TrimSpace(legacy.RealtimeASR.APIKey)
	if asrKey != "" && strings.TrimSpace(legacy.RealtimeASR.Provider) == xinzhili.RealtimeASRProvider &&
		strings.TrimSpace(legacy.RealtimeASR.Model) == xinzhili.RealtimeASRModel &&
		isOfficialDashScopeRealtimeASREndpoint(legacy.RealtimeASR.Endpoint) {
		return resolvedBailianCredential{
			Config:       bailianconfig.Config{APIKey: asrKey},
			Source:       bailianCredentialSourceLegacyASR,
			runtimeEpoch: bailianCredentialRuntimeEpochLegacy, runtimeVersion: legacyRuntimeVersion,
		}, nil
	}
	return resolvedBailianCredential{
		Source:       bailianCredentialSourceNone,
		runtimeEpoch: bailianCredentialRuntimeEpochLegacy, runtimeVersion: legacyRuntimeVersion,
	}, nil
}

func isOfficialDashScopeTTSEndpoint(raw string) bool {
	parsed, ok := parseOfficialDashScopeEndpoint(raw, "https")
	if !ok {
		return false
	}
	endpointPath := strings.TrimSuffix(parsed.Path, "/")
	if pathpkg.Clean(endpointPath) != endpointPath {
		return false
	}
	switch endpointPath {
	case "/api/v1", "/compatible-mode/v1", "/api/v1/services/aigc/multimodal-generation/generation":
		return true
	default:
		return false
	}
}

func isOfficialDashScopeRealtimeASREndpoint(raw string) bool {
	parsed, ok := parseOfficialDashScopeEndpoint(raw, "wss", "https")
	if !ok || pathpkg.Clean(parsed.Path) != parsed.Path {
		return false
	}
	return parsed.Path == "/api-ws/v1/inference"
}

func parseOfficialDashScopeEndpoint(raw string, schemes ...string) (*url.URL, bool) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil {
		return nil, false
	}
	schemeAllowed := false
	for _, scheme := range schemes {
		if strings.EqualFold(parsed.Scheme, scheme) {
			schemeAllowed = true
			break
		}
	}
	if !schemeAllowed || !strings.EqualFold(parsed.Hostname(), "dashscope.aliyuncs.com") {
		return nil, false
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return nil, false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return nil, false
	}
	return parsed, true
}

func (s *Server) refreshBailianCopyCredentials(ctx context.Context) (resolvedBailianCredential, error) {
	resolved, err := s.resolveBailianCredentials(ctx)
	if err != nil {
		return resolvedBailianCredential{}, err
	}
	s.applyBailianCredentialRuntime(resolved)
	return resolved, nil
}

func (s *Server) applyBailianCredentialRuntime(resolved resolvedBailianCredential) {
	s.bailianRuntime.Lock()
	defer s.bailianRuntime.Unlock()
	if s.bailianRuntime.set && (resolved.runtimeEpoch < s.bailianRuntime.epoch ||
		(resolved.runtimeEpoch == s.bailianRuntime.epoch && resolved.runtimeVersion < s.bailianRuntime.version)) {
		return
	}
	if s.setBailianCopyConfig == nil {
		return
	}
	s.setBailianCopyConfig(voice.BailianConfig{
		APIBase:     defaultSharedBailianAPIBase,
		APIKey:      strings.TrimSpace(resolved.APIKey),
		TargetModel: defaultSharedBailianCloneModel,
	})
	s.bailianRuntime.set = true
	s.bailianRuntime.epoch = resolved.runtimeEpoch
	s.bailianRuntime.version = resolved.runtimeVersion
}

func buildBailianCredentialView(resolved resolvedBailianCredential) bailianCredentialView {
	key := []rune(strings.TrimSpace(resolved.APIKey))
	view := bailianCredentialView{
		Version:   resolved.Version,
		APIKeySet: len(key) > 0,
		Source:    resolved.Source,
	}
	if len(key) > 8 {
		view.APIKeySuffix = string(key[len(key)-4:])
	}
	return view
}
