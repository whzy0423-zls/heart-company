package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const appFeatureConfigKey = "app_feature_config"

type appFeatureConfig struct {
	LifeStoryEnabled bool `json:"lifeStoryEnabled"`
}

func defaultAppFeatureConfig() appFeatureConfig {
	return appFeatureConfig{LifeStoryEnabled: true}
}

func (s *Server) loadAppFeatureConfig(ctx context.Context) (appFeatureConfig, error) {
	cfg := defaultAppFeatureConfig()
	if s == nil || s.db == nil {
		return cfg, nil
	}
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT config FROM site_configs WHERE key=$1`, appFeatureConfigKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (s *Server) saveAppFeatureConfig(ctx context.Context, cfg appFeatureConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO site_configs (key, config, update_time)
		VALUES ($1, $2::jsonb, now())
		ON CONFLICT (key) DO UPDATE SET config=EXCLUDED.config, update_time=now()`,
		appFeatureConfigKey, string(raw))
	return err
}

// appFeatureConfig serves the admin-only App feature switch.
func (s *Server) appFeatureConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.loadAppFeatureConfig(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "读取 App 功能开关失败")
			return
		}
		httpx.OK(w, cfg)
	case http.MethodPut:
		var cfg appFeatureConfig
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&cfg); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		if err := s.saveAppFeatureConfig(r.Context(), cfg); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "保存 App 功能开关失败")
			return
		}
		httpx.OK(w, cfg)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// appFeatures exposes the effective switches to authenticated App clients.
func (s *Server) appFeatures(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadAppFeatureConfig(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取 App 功能开关失败")
		return
	}
	httpx.OK(w, cfg)
}
