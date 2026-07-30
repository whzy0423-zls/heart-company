package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
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
	shared, found, err := s.readSharedBailianCredentials(ctx)
	if err != nil {
		return resolvedBailianCredential{}, err
	}
	if found {
		return resolvedSharedBailianCredential(shared), nil
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
	return resolveLegacyBailianCredential(legacy, true), nil
}

// resolveBailianCredentialsForConfig resolves the shared credential against a
// specific Xinzhili snapshot. The shared record is still read fresh for every
// call, while the legacy fallback remains tied to the same model snapshot that
// will be validated or used for the current turn.
func (s *Server) resolveBailianCredentialsForConfig(ctx context.Context, legacy xinzhili.Config, legacyFound bool) (resolvedBailianCredential, error) {
	shared, found, err := s.readSharedBailianCredentials(ctx)
	if err != nil {
		return resolvedBailianCredential{}, err
	}
	if found {
		return resolvedSharedBailianCredential(shared), nil
	}
	return resolveLegacyBailianCredential(legacy, legacyFound), nil
}

func (s *Server) readSharedBailianCredentials(ctx context.Context) (bailianconfig.Config, bool, error) {
	if s.bailianCredentials == nil {
		return bailianconfig.Config{}, false, nil
	}
	return s.bailianCredentials.Read(ctx)
}

func resolvedSharedBailianCredential(shared bailianconfig.Config) resolvedBailianCredential {
	shared.APIKey = strings.TrimSpace(shared.APIKey)
	return resolvedBailianCredential{
		Config: shared, Source: bailianCredentialSourceShared,
		runtimeEpoch: bailianCredentialRuntimeEpochShared, runtimeVersion: shared.Version,
	}
}

func resolveLegacyBailianCredential(legacy xinzhili.Config, found bool) resolvedBailianCredential {
	if !found {
		return resolvedBailianCredential{Source: bailianCredentialSourceNone, runtimeEpoch: bailianCredentialRuntimeEpochNone}
	}
	legacyRuntimeVersion := legacy.Version

	ttsKey := strings.TrimSpace(legacy.TTS.APIKey)
	if ttsKey != "" && xinzhili.TTSUsesBailianCredentials(legacy.TTS) {
		return resolvedBailianCredential{
			Config:       bailianconfig.Config{APIKey: ttsKey},
			Source:       bailianCredentialSourceLegacyTTS,
			runtimeEpoch: bailianCredentialRuntimeEpochLegacy, runtimeVersion: legacyRuntimeVersion,
		}
	}

	asrKey := strings.TrimSpace(legacy.RealtimeASR.APIKey)
	if asrKey != "" && strings.TrimSpace(legacy.RealtimeASR.Provider) == xinzhili.RealtimeASRProvider &&
		strings.TrimSpace(legacy.RealtimeASR.Model) == xinzhili.RealtimeASRModel &&
		isOfficialDashScopeRealtimeASREndpoint(legacy.RealtimeASR.Endpoint) {
		return resolvedBailianCredential{
			Config:       bailianconfig.Config{APIKey: asrKey},
			Source:       bailianCredentialSourceLegacyASR,
			runtimeEpoch: bailianCredentialRuntimeEpochLegacy, runtimeVersion: legacyRuntimeVersion,
		}
	}
	return resolvedBailianCredential{
		Source:       bailianCredentialSourceNone,
		runtimeEpoch: bailianCredentialRuntimeEpochLegacy, runtimeVersion: legacyRuntimeVersion,
	}
}

func isOfficialDashScopeTTSEndpoint(raw string) bool {
	return xinzhili.IsOfficialDashScopeTTSEndpoint(raw)
}

func isOfficialDashScopeRealtimeASREndpoint(raw string) bool {
	return xinzhili.IsOfficialDashScopeRealtimeASREndpoint(raw)
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
