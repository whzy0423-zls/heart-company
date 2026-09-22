package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/voicebroadcastconfig"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

const (
	voiceBroadcastPermission = "App:VoiceBroadcast:Manage"
	voiceBroadcastTestText   = "你好，这是语音播报测试。"
)

type voiceBroadcastConfigStore interface {
	Read(context.Context) (voicebroadcastconfig.Config, bool, error)
	Update(context.Context, voicebroadcastconfig.UpdateInput, int64) (voicebroadcastconfig.Config, error)
	RecordHealth(context.Context, int64, voicebroadcastconfig.Health) (voicebroadcastconfig.Config, error)
}

type voiceBroadcastProbeResult struct {
	OK            bool
	LatencyMS     int64
	ErrorCategory string
	Message       string
}

type voiceBroadcastConfigUpdateRequest struct {
	ExpectedVersion *int64                       `json:"expectedVersion"`
	Enabled         *bool                        `json:"enabled"`
	Provider        string                       `json:"provider"`
	Region          string                       `json:"region"`
	WorkspaceID     string                       `json:"workspaceId"`
	Model           string                       `json:"model"`
	DefaultVoice    string                       `json:"defaultVoice"`
	CurrentVoice    string                       `json:"currentVoice"`
	Voices          []voicebroadcastconfig.Voice `json:"voices"`
	APIKey          string                       `json:"apiKey"`
	ClearAPIKey     bool                         `json:"clearApiKey"`
	Text            string                       `json:"text,omitempty"`
}

func (r voiceBroadcastConfigUpdateRequest) input() voicebroadcastconfig.UpdateInput {
	return voicebroadcastconfig.UpdateInput{
		Enabled: r.Enabled, Provider: r.Provider, Region: r.Region,
		WorkspaceID: r.WorkspaceID, Model: r.Model, DefaultVoice: r.DefaultVoice,
		CurrentVoice: r.CurrentVoice, Voices: r.Voices, APIKey: r.APIKey,
		ClearAPIKey: r.ClearAPIKey,
	}
}

func (s *Server) voiceBroadcastConfigHandler(w http.ResponseWriter, r *http.Request) {
	store := s.voiceBroadcastStore()
	if store == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "语音播报配置服务暂不可用")
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, found, err := store.Read(r.Context())
		if err != nil {
			logVoiceBroadcastError("read", err)
			httpx.Fail(w, http.StatusInternalServerError, "读取语音播报配置失败")
			return
		}
		httpx.OK(w, voicebroadcastconfig.BuildView(cfg, found))
	case http.MethodPut:
		s.voiceBroadcastConfigUpdate(w, r, store)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Server) voiceBroadcastConfigUpdate(w http.ResponseWriter, r *http.Request, store voiceBroadcastConfigStore) {
	input, err := decodeVoiceBroadcastRequest(w, r)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.ExpectedVersion == nil {
		httpx.Fail(w, http.StatusBadRequest, "expectedVersion is required")
		return
	}
	before, beforeFound, readErr := store.Read(r.Context())
	if readErr != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取语音播报配置失败")
		return
	}
	saved, err := store.Update(r.Context(), input.input(), *input.ExpectedVersion)
	if err != nil {
		s.voiceBroadcastConfigError(w, err)
		return
	}
	s.recordAdminAudit(r, auditlog.Entry{
		Action: "voice_broadcast_config.update", TargetType: "voice_broadcast_config", TargetID: "global",
		Before: voicebroadcastconfig.BuildView(before, beforeFound), After: voicebroadcastconfig.BuildView(saved, true),
		Summary: "更新会话语音播报配置",
	})
	httpx.OK(w, voicebroadcastconfig.BuildView(saved, true))
}

func (s *Server) voiceBroadcastConfigTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	store := s.voiceBroadcastStore()
	if store == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "语音播报配置服务暂不可用")
		return
	}
	input, err := decodeVoiceBroadcastRequest(w, r)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	current, found, err := store.Read(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取语音播报配置失败")
		return
	}
	candidate := mergeVoiceBroadcastProbeConfig(current, input.input())
	if strings.TrimSpace(candidate.APIKey) == "" {
		// The shared Bailian credential remains a supported runtime fallback for
		// deployments that have not yet entered a key on this page.
		if resolved, resolveErr := s.resolveBailianCredentials(r.Context()); resolveErr == nil {
			candidate.APIKey = strings.TrimSpace(resolved.APIKey)
		}
	}
	text := strings.TrimSpace(input.Text)
	if text == "" {
		text = voiceBroadcastTestText
	}
	probe := s.voiceBroadcastProbeFunc()
	probeCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	result := probe(probeCtx, candidate, text)
	cancel()
	result = sanitizeVoiceBroadcastProbeResult(result, candidate.APIKey)
	health := voicebroadcastconfig.Health{
		Status: resultStatus(result), OK: result.OK, LatencyMS: result.LatencyMS,
		ErrorCategory: result.ErrorCategory, Message: result.Message, TestedAt: time.Now().UTC(),
	}
	expectedVersion := current.Version
	if input.ExpectedVersion != nil {
		expectedVersion = *input.ExpectedVersion
	}
	updated, healthErr := store.RecordHealth(r.Context(), expectedVersion, health)
	if healthErr != nil {
		s.voiceBroadcastConfigError(w, healthErr)
		return
	}
	s.recordAdminAudit(r, auditlog.Entry{
		Action: "voice_broadcast_config.test", TargetType: "voice_broadcast_config", TargetID: "global",
		Before: voicebroadcastconfig.BuildView(current, found), After: voicebroadcastconfig.BuildView(updated, true),
		Summary: "测试会话语音播报合成",
	})
	httpx.OK(w, voicebroadcastconfig.BuildView(updated, true))
}

func decodeVoiceBroadcastRequest(w http.ResponseWriter, r *http.Request) (*voiceBroadcastConfigUpdateRequest, error) {
	var input voiceBroadcastConfigUpdateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, errors.New("Invalid JSON payload")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("Invalid JSON payload")
	}
	return &input, nil
}

func (s *Server) voiceBroadcastConfigError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, voicebroadcastconfig.ErrConflict):
		httpx.Fail(w, http.StatusConflict, "voice_broadcast_config_version_conflict")
	case errors.Is(err, voicebroadcastconfig.ErrSecretUnavailable):
		httpx.Fail(w, http.StatusServiceUnavailable, "语音播报密钥加密服务暂不可用")
	case errors.Is(err, voicebroadcastconfig.ErrDatabaseUnavailable):
		httpx.Fail(w, http.StatusServiceUnavailable, "语音播报配置数据库暂不可用")
	default:
		logVoiceBroadcastError("write", err)
		httpx.Fail(w, http.StatusBadRequest, "语音播报配置无效")
	}
}

func (s *Server) voiceBroadcastStore() voiceBroadcastConfigStore {
	if s == nil {
		return nil
	}
	return s.voiceBroadcastConfig
}

func (s *Server) voiceBroadcastProbeFunc() func(context.Context, voicebroadcastconfig.Config, string) voiceBroadcastProbeResult {
	if s != nil && s.voiceBroadcastProbe != nil {
		return s.voiceBroadcastProbe
	}
	return probeVoiceBroadcastTTS
}

func mergeVoiceBroadcastProbeConfig(current voicebroadcastconfig.Config, input voicebroadcastconfig.UpdateInput) voicebroadcastconfig.Config {
	if input.Enabled != nil {
		current.Enabled = *input.Enabled
	}
	if value := strings.TrimSpace(input.Provider); value != "" {
		current.Provider = value
	}
	if value := strings.TrimSpace(input.Region); value != "" {
		current.Region = value
	}
	if value := strings.TrimSpace(input.WorkspaceID); value != "" {
		current.WorkspaceID = value
	}
	if value := strings.TrimSpace(input.Model); value != "" {
		current.Model = value
	}
	if value := strings.TrimSpace(input.CurrentVoice); value != "" {
		current.CurrentVoice = value
	}
	if value := strings.TrimSpace(input.APIKey); value != "" {
		current.APIKey = value
	}
	if current.Provider == "" {
		current.Provider = voicebroadcastconfig.DefaultProvider
	}
	if current.Model == "" {
		current.Model = voicebroadcastconfig.DefaultModel
	}
	if current.CurrentVoice == "" {
		current.CurrentVoice = voicebroadcastconfig.DefaultFemaleVoice
	}
	return current
}

func probeVoiceBroadcastTTS(ctx context.Context, cfg voicebroadcastconfig.Config, text string) voiceBroadcastProbeResult {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return voiceBroadcastProbeResult{ErrorCategory: "not_configured", Message: "尚未配置阿里百炼 API Key"}
	}
	if strings.TrimSpace(cfg.Model) == "" || strings.TrimSpace(cfg.CurrentVoice) == "" {
		return voiceBroadcastProbeResult{ErrorCategory: "invalid_config", Message: "模型和音色不能为空"}
	}
	ttsConfig := xinzhili.TTSConfig{
		Provider: xinzhili.TTSProviderBailian, Endpoint: voicebroadcastconfig.DefaultEndpoint,
		APIKey: cfg.APIKey, Region: strings.TrimSpace(cfg.Region), GroupID: cfg.WorkspaceID, Model: cfg.Model,
		Voice: cfg.CurrentVoice, Format: "mp3",
	}
	started := time.Now()
	provider := xinzhili.TTSProviderFactory{}.Dynamic()
	_, _, err := provider.Synthesize(ctx, ttsConfig, text)
	result := voiceBroadcastProbeResult{LatencyMS: time.Since(started).Milliseconds()}
	if err != nil {
		result.ErrorCategory = "provider_error"
		result.Message = "阿里百炼语音合成测试失败"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.ErrorCategory = "timeout"
			result.Message = "语音合成测试超时"
		}
		return result
	}
	result.OK = true
	result.Message = fmt.Sprintf("合成测试成功（%d ms）", result.LatencyMS)
	return result
}

func resultStatus(result voiceBroadcastProbeResult) string {
	if result.OK {
		return "ok"
	}
	if result.ErrorCategory == "not_configured" {
		return "not_configured"
	}
	return "error"
}

func sanitizeVoiceBroadcastProbeResult(result voiceBroadcastProbeResult, secret string) voiceBroadcastProbeResult {
	result.ErrorCategory = normalizeVoiceBroadcastErrorCategory(result.ErrorCategory)
	message := strings.TrimSpace(result.Message)
	if secret = strings.TrimSpace(secret); secret != "" {
		message = strings.ReplaceAll(message, secret, "[redacted]")
	}
	result.Message = message
	return result
}

func normalizeVoiceBroadcastErrorCategory(category string) string {
	normalized := strings.ToLower(strings.TrimSpace(category))
	if normalized == "" {
		return ""
	}
	switch normalized {
	case "not_configured", "invalid_config", "timeout", "provider_error":
		return normalized
	default:
		return "provider_error"
	}
}

func logVoiceBroadcastError(operation string, err error) {
	// Keep provider details and secrets out of the HTTP response. The caller's
	// standard server logger can still correlate the operation in deployment.
	_ = operation
	_ = err
}
