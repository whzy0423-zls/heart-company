package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

type xinzhiliModelConfigStore interface {
	Read(context.Context) (xinzhili.Config, bool, error)
	Update(context.Context, xinzhili.Config, int64) (xinzhili.Config, error)
}

type databaseXinzhiliModelConfigStore struct{ db *sql.DB }

func (s databaseXinzhiliModelConfigStore) Read(ctx context.Context) (xinzhili.Config, bool, error) {
	return xinzhili.ReadConfig(ctx, s.db)
}

func (s databaseXinzhiliModelConfigStore) Update(ctx context.Context, cfg xinzhili.Config, expectedVersion int64) (xinzhili.Config, error) {
	return xinzhili.UpdateConfig(ctx, s.db, cfg, expectedVersion)
}

type xinzhiliSecretView struct {
	APIKey       string `json:"apiKey"`
	APIKeySet    bool   `json:"apiKeySet"`
	APIKeySuffix string `json:"apiKeySuffix"`
}

type xinzhiliASRConfigView struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	Region   string `json:"region"`
	Model    string `json:"model"`
	xinzhiliSecretView
}

type xinzhiliTTSConfigView struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	GroupID  string `json:"groupId,omitempty"`
	Model    string `json:"model"`
	Voice    string `json:"voice"`
	Format   string `json:"format"`
	xinzhiliSecretView
}

type xinzhiliModelConfigView struct {
	Enabled      bool                     `json:"enabled"`
	Version      int64                    `json:"version"`
	RealtimeASR  xinzhiliASRConfigView    `json:"realtimeAsr"`
	TTS          xinzhiliTTSConfigView    `json:"tts"`
	EnabledModes []xinzhili.Mode          `json:"enabledModes"`
	Timing       xinzhili.TimingConfig    `json:"timing"`
	CommonPrompt string                   `json:"commonPrompt"`
	ModePrompts  map[xinzhili.Mode]string `json:"modePrompts"`
}

type xinzhiliModelConfigUpdateRequest struct {
	xinzhili.Config
	ExpectedVersion *int64 `json:"expectedVersion"`
}

func (s *Server) xinzhiliModelConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	if s.xinzhiliModelConfig == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "芯之力配置服务暂不可用")
		return
	}

	if r.Method == http.MethodGet {
		cfg, found, err := s.xinzhiliModelConfig.Read(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !found {
			cfg = xinzhili.DefaultConfig()
		}
		httpx.OK(w, buildXinzhiliModelConfigView(cfg))
		return
	}

	var input xinzhiliModelConfigUpdateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if input.ExpectedVersion == nil {
		httpx.Fail(w, http.StatusBadRequest, "expectedVersion is required")
		return
	}
	before, _, err := s.xinzhiliModelConfig.Read(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	candidate := xinzhili.MergeIncoming(before, input.Config)
	clearChangedTTSSecret := shouldClearChangedTTSSecret(before.TTS, candidate.TTS, input.Config)
	if clearChangedTTSSecret {
		candidate.TTS.APIKey = ""
	}
	candidate.Version = before.Version
	normalized, err := candidate.WithDefaults()
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	resolved, err := s.resolveBailianCredentialsForConfig(r.Context(), normalized, true)
	if err != nil {
		log.Printf("xinzhili shared Bailian credential resolution failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, "百炼凭证读取失败")
		return
	}
	if normalized.Enabled && strings.TrimSpace(resolved.APIKey) == "" {
		httpx.Fail(w, http.StatusBadRequest, "请先配置百炼公共 API Key")
		return
	}

	persisted := input.Config
	if clearChangedTTSSecret {
		persisted.ClearTTSKey = true
	}
	if resolved.Source == bailianCredentialSourceShared {
		persisted.ClearASRKey = true
		if xinzhili.TTSUsesBailianCredentials(normalized.TTS) {
			persisted.ClearTTSKey = true
		}
	}

	saved, err := s.xinzhiliModelConfig.Update(r.Context(), persisted, *input.ExpectedVersion)
	if errors.Is(err, xinzhili.ErrConfigConflict) {
		httpx.Fail(w, http.StatusConflict, "config_version_conflict")
		return
	}
	if err != nil {
		if errors.Is(err, xinzhili.ErrNormalModeRequired) {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resolved.Source != bailianCredentialSourceShared {
		resolved = resolveLegacyBailianCredential(saved, true)
	}
	s.applyBailianCredentialRuntime(resolved)
	s.broadcastXinzhiliConfigChanged(r.Context(), saved)
	s.recordAdminAudit(r, auditlog.Entry{
		Action:     "xinzhili_model_config.update",
		TargetType: "xinzhili_model_config",
		TargetID:   "global",
		Before:     buildXinzhiliModelConfigAuditView(before),
		After:      buildXinzhiliModelConfigAuditView(saved),
		Summary:    "更新芯之力模型配置",
	})
	httpx.OK(w, buildXinzhiliModelConfigView(saved))
}

func shouldClearChangedTTSSecret(before, after xinzhili.TTSConfig, incoming xinzhili.Config) bool {
	if incoming.ClearTTSKey || strings.TrimSpace(incoming.TTS.APIKey) != "" || strings.TrimSpace(before.APIKey) == "" {
		return false
	}
	return ttsCredentialScope(before) != ttsCredentialScope(after)
}

func ttsCredentialScope(cfg xinzhili.TTSConfig) string {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	endpoint := strings.TrimSpace(cfg.Endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return provider + "|" + strings.ToLower(endpoint)
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	rawPort := parsed.Port()
	port := rawPort
	if rawPort != "" {
		if numericPort, parseErr := strconv.Atoi(rawPort); parseErr == nil {
			if scheme == "https" && numericPort == 443 {
				port = ""
			} else {
				port = strconv.Itoa(numericPort)
			}
		}
	}
	return provider + "|" + scheme + "|" + hostname + "|" + port
}

func buildXinzhiliModelConfigView(cfg xinzhili.Config) xinzhiliModelConfigView {
	return xinzhiliModelConfigView{
		Enabled: cfg.Enabled, Version: cfg.Version,
		RealtimeASR: xinzhiliASRConfigView{
			Provider: cfg.RealtimeASR.Provider, Endpoint: cfg.RealtimeASR.Endpoint,
			Region: cfg.RealtimeASR.Region, Model: cfg.RealtimeASR.Model,
			xinzhiliSecretView: secretView(cfg.RealtimeASR.APIKey),
		},
		TTS: xinzhiliTTSConfigView{
			Provider: cfg.TTS.Provider, Endpoint: cfg.TTS.Endpoint, GroupID: cfg.TTS.GroupID,
			Model: cfg.TTS.Model, Voice: cfg.TTS.Voice, Format: cfg.TTS.Format,
			xinzhiliSecretView: secretView(cfg.TTS.APIKey),
		},
		EnabledModes: cfg.EnabledModes, Timing: cfg.Timing,
		CommonPrompt: cfg.CommonPrompt, ModePrompts: cfg.ModePrompts,
	}
}

func secretView(secret string) xinzhiliSecretView {
	runes := []rune(secret)
	view := xinzhiliSecretView{APIKeySet: len(runes) > 0}
	if len(runes) > 8 {
		view.APIKeySuffix = string(runes[len(runes)-4:])
	}
	return view
}

func buildXinzhiliModelConfigAuditView(cfg xinzhili.Config) map[string]any {
	return map[string]any{
		"enabled":      cfg.Enabled,
		"version":      cfg.Version,
		"asrProvider":  cfg.RealtimeASR.Provider,
		"asrKeySet":    utf8.RuneCountInString(cfg.RealtimeASR.APIKey) > 0,
		"ttsProvider":  cfg.TTS.Provider,
		"ttsKeySet":    utf8.RuneCountInString(cfg.TTS.APIKey) > 0,
		"enabledModes": cfg.EnabledModes,
	}
}
