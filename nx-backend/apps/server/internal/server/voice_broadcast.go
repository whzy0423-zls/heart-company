package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

const (
	voiceBroadcastPreferencePath  = "/api/app/voice-broadcast"
	voiceBroadcastConfigKey       = "voice_broadcast_config"
	voiceBroadcastProviderBailian = "bailian"
	voiceBroadcastDefaultModel    = "qwen3-tts-instruct-flash"
	voiceBroadcastDefaultVoice    = "Cherry"
	voiceBroadcastDefaultEndpoint = "https://dashscope.aliyuncs.com"
)

func normalizeVoiceBroadcastProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", voiceBroadcastProviderBailian, "aliyun-bailian", "aliyun_bailian", "dashscope":
		return xinzhili.TTSProviderBailian
	default:
		return strings.TrimSpace(raw)
	}
}

// voiceBroadcastPreferenceStore intentionally has a smaller contract than the
// natural-language preference store. The switch is a boolean product setting,
// not prompt text.
type voiceBroadcastPreferenceStore interface {
	Get(context.Context, int64) (bool, error)
	Set(context.Context, int64, bool) error
}

type databaseVoiceBroadcastPreferenceStore struct{ db *sql.DB }

func (s databaseVoiceBroadcastPreferenceStore) Get(ctx context.Context, userID int64) (bool, error) {
	if s.db == nil || userID <= 0 {
		return false, nil
	}
	var enabled bool
	err := s.db.QueryRowContext(ctx,
		`SELECT enabled FROM app_voice_broadcast_preferences WHERE app_user_id=$1`, userID,
	).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}

func (s databaseVoiceBroadcastPreferenceStore) Set(ctx context.Context, userID int64, enabled bool) error {
	if s.db == nil || userID <= 0 {
		return errors.New("voice broadcast preference store unavailable")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_voice_broadcast_preferences (app_user_id, enabled, update_time)
		VALUES ($1, $2, now())
		ON CONFLICT (app_user_id) DO UPDATE SET enabled=EXCLUDED.enabled, update_time=now()`,
		userID, enabled,
	)
	return err
}

// voiceBroadcastConfig is deliberately provider-neutral. API credentials are
// never serialized into the app capability response.
type voiceBroadcastConfig struct {
	Enabled     bool
	Provider    string
	Endpoint    string
	APIKey      string
	Region      string
	WorkspaceID string
	GroupID     string
	Model       string
	Voice       string
	Format      string
	Instruction string
}

func defaultVoiceBroadcastConfig() voiceBroadcastConfig {
	return voiceBroadcastConfig{
		Provider: voiceBroadcastProviderBailian,
		Endpoint: voiceBroadcastDefaultEndpoint,
		Model:    voiceBroadcastDefaultModel,
		Voice:    voiceBroadcastDefaultVoice,
		Format:   "mp3",
	}
}

type voiceBroadcastCapability struct {
	Enabled           bool   `json:"enabled"`
	UserEnabled       bool   `json:"userEnabled"`
	Provider          string `json:"provider"`
	ProviderAvailable bool   `json:"providerAvailable"`
	Model             string `json:"model"`
	Voice             string `json:"voice"`
}

type voiceBroadcastConfigDocument struct {
	Enabled     bool   `json:"enabled"`
	Provider    string `json:"provider"`
	Endpoint    string `json:"endpoint"`
	Region      string `json:"region"`
	WorkspaceID string `json:"workspaceId"`
	GroupID     string `json:"groupId"`
	Model       string `json:"model"`
	Voice       string `json:"voice"`
	Format      string `json:"format"`
	Instruction string `json:"instruction"`
}

// loadVoiceBroadcastConfig is the seam used by the admin configuration
// implementation. Until an explicit admin row is saved, it uses stable
// Bailian defaults and the shared encrypted credential record.
func (s *Server) loadVoiceBroadcastConfig(ctx context.Context) (voiceBroadcastConfig, error) {
	if s != nil && s.voiceBroadcastConfigLoader != nil {
		return s.voiceBroadcastConfigLoader(ctx)
	}
	cfg := defaultVoiceBroadcastConfig()
	if s != nil && s.db != nil {
		var raw []byte
		err := s.db.QueryRowContext(ctx, `SELECT config FROM site_configs WHERE key=$1`, voiceBroadcastConfigKey).Scan(&raw)
		if err == nil {
			var stored voiceBroadcastConfigDocument
			if decodeErr := json.Unmarshal(raw, &stored); decodeErr != nil {
				return cfg, decodeErr
			}
			cfg.Enabled = stored.Enabled
			if strings.TrimSpace(stored.Provider) != "" {
				cfg.Provider = strings.TrimSpace(stored.Provider)
			}
			if strings.TrimSpace(stored.Endpoint) != "" {
				cfg.Endpoint = strings.TrimSpace(stored.Endpoint)
			}
			cfg.Region = strings.TrimSpace(stored.Region)
			cfg.WorkspaceID = strings.TrimSpace(stored.WorkspaceID)
			cfg.GroupID = strings.TrimSpace(stored.GroupID)
			if strings.TrimSpace(stored.Model) != "" {
				cfg.Model = strings.TrimSpace(stored.Model)
			}
			if strings.TrimSpace(stored.Voice) != "" {
				cfg.Voice = strings.TrimSpace(stored.Voice)
			}
			if strings.TrimSpace(stored.Format) != "" {
				cfg.Format = strings.TrimSpace(stored.Format)
			}
			cfg.Instruction = strings.TrimSpace(stored.Instruction)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return cfg, err
		}
	}
	if s != nil {
		if resolved, err := s.resolveBailianCredentials(ctx); err == nil {
			cfg.APIKey = strings.TrimSpace(resolved.APIKey)
		} else if !errors.Is(err, sql.ErrNoRows) {
			// A missing credential store means the capability is simply
			// unavailable. Do not make the text chat endpoint fail.
			cfg.APIKey = ""
		}
	}
	return cfg, nil
}

func (s *Server) voiceBroadcastPreferenceStore() voiceBroadcastPreferenceStore {
	if s == nil {
		return nil
	}
	if s.voiceBroadcastPreferences != nil {
		return s.voiceBroadcastPreferences
	}
	if s.db == nil {
		return nil
	}
	store := databaseVoiceBroadcastPreferenceStore{db: s.db}
	s.voiceBroadcastPreferences = store
	return store
}

func (s *Server) appVoiceBroadcast(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok || user.ID <= 0 {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	store := s.voiceBroadcastPreferenceStore()
	if store == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "语音播报偏好服务不可用")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || body.Enabled == nil {
			httpx.Fail(w, http.StatusBadRequest, "enabled is required")
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			httpx.Fail(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := store.Set(r.Context(), user.ID, *body.Enabled); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "语音播报偏好保存失败")
			return
		}
	}
	enabled, err := store.Get(r.Context(), user.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "语音播报偏好读取失败")
		return
	}
	capability := s.voiceBroadcastCapabilityForUser(r.Context(), user.ID)
	// Preserve the just-written/read value even if the config provider is
	// temporarily unavailable.
	capability.UserEnabled = enabled
	httpx.OK(w, capability)
}

func (s *Server) voiceBroadcastEnabledForUser(ctx context.Context, userID int64) bool {
	capability := s.voiceBroadcastCapabilityForUser(ctx, userID)
	return capability.Enabled && capability.UserEnabled && capability.ProviderAvailable
}

func (s *Server) voiceBroadcastCapabilityForUser(ctx context.Context, userID int64) voiceBroadcastCapability {
	capability := voiceBroadcastCapability{}
	if s == nil || userID <= 0 {
		return capability
	}
	store := s.voiceBroadcastPreferenceStore()
	if store == nil {
		return capability
	}
	enabled, err := store.Get(ctx, userID)
	if err != nil {
		return capability
	}
	cfg, err := s.loadVoiceBroadcastConfig(ctx)
	capability = voiceBroadcastCapability{
		Enabled:     err == nil && cfg.Enabled,
		UserEnabled: enabled,
		Provider:    strings.TrimSpace(cfg.Provider),
		Model:       strings.TrimSpace(cfg.Model),
		Voice:       strings.TrimSpace(cfg.Voice),
	}
	capability.ProviderAvailable = capability.Enabled && strings.TrimSpace(cfg.APIKey) != "" && capability.Provider != "" && capability.Model != "" && capability.Voice != ""
	return capability
}

type voiceBroadcastSynthesizer interface {
	Synthesize(context.Context, string) ([]byte, string, error)
}

type voiceBroadcastProviderAdapter struct {
	provider xinzhili.TTSProvider
	cfg      xinzhili.TTSConfig
}

func (p voiceBroadcastProviderAdapter) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	if p.provider == nil {
		return nil, "", errors.New("voice broadcast provider unavailable")
	}
	return p.provider.Synthesize(ctx, p.cfg, text)
}

func (s *Server) newVoiceBroadcastSynthesizer(ctx context.Context) (voiceBroadcastSynthesizer, error) {
	if s != nil && s.voiceBroadcastSynthesizerFactory != nil {
		return s.voiceBroadcastSynthesizerFactory(ctx)
	}
	cfg, err := s.loadVoiceBroadcastConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("voice broadcast provider unavailable")
	}
	ttsCfg := xinzhili.TTSConfig{
		Provider: normalizeVoiceBroadcastProvider(cfg.Provider), Endpoint: cfg.Endpoint, APIKey: cfg.APIKey,
		GroupID: cfg.GroupID, Model: cfg.Model, Voice: cfg.Voice,
		Format: cfg.Format, Instruction: cfg.Instruction,
	}
	provider, err := (xinzhili.TTSProviderFactory{Slots: s.globalTTSSlots(), Metrics: s.metrics}).New(ttsCfg)
	if err != nil {
		return nil, err
	}
	return voiceBroadcastProviderAdapter{provider: provider, cfg: ttsCfg}, nil
}
