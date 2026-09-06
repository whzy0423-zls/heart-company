package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
)

const appPaymentModeConfigKey = "app_payment_mode"

type appPaymentModeConfig struct {
	Mode string `json:"mode"`
}

type appPaymentModeResp struct {
	Mode                      string   `json:"mode"`
	CustomerServiceConfigured bool     `json:"customerServiceConfigured"`
	XZNConfigured             bool     `json:"xznConfigured"`
	XZNEnabled                bool     `json:"xznEnabled"`
	XZNChannels               []string `json:"xznChannels"`
}

func normalizeAppPaymentMode(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", appPurchaseModeCustomerService:
		return appPurchaseModeCustomerService, nil
	case appPurchaseModeXZN:
		return appPurchaseModeXZN, nil
	default:
		return "", errors.New("支付模式必须是 customer_service 或 xzn")
	}
}

func appPaymentModeCanActivate(mode string, cfg xznPaymentConfig) bool {
	return mode != appPurchaseModeXZN || xznCredentialsConfigured(cfg)
}

func (s *Server) loadAppPaymentMode(ctx context.Context) (string, error) {
	if s == nil || s.db == nil {
		return appPurchaseModeCustomerService, nil
	}
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT config FROM site_configs WHERE key=$1`, appPaymentModeConfigKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return appPurchaseModeCustomerService, nil
	}
	if err != nil {
		// The default preserves the pre-switch customer-service flow and avoids
		// accidentally enabling online payment when the setting is unavailable.
		return appPurchaseModeCustomerService, nil
	}
	var cfg appPaymentModeConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", err
	}
	return normalizeAppPaymentMode(cfg.Mode)
}

func (s *Server) saveAppPaymentMode(ctx context.Context, mode string) error {
	mode, err := normalizeAppPaymentMode(mode)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(appPaymentModeConfig{Mode: mode})
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO site_configs (key, config, update_time)
		VALUES ($1, $2::jsonb, now())
		ON CONFLICT (key) DO UPDATE SET config=EXCLUDED.config, update_time=now()`,
		appPaymentModeConfigKey, raw)
	return err
}

func (s *Server) appPaymentMode(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		mode, err := s.loadAppPaymentMode(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "读取支付模式失败")
			return
		}
		httpx.OK(w, s.appPaymentModeResponse(r.Context(), mode))
	case http.MethodPut:
		var input appPaymentModeConfig
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		mode, err := normalizeAppPaymentMode(input.Mode)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		xznCfg, _ := s.loadXZNConfig(r.Context())
		if !appPaymentModeCanActivate(mode, xznCfg) {
			httpx.Fail(w, http.StatusBadRequest, "星之柠商户配置不完整，请先配置商户号、密钥和异步回调地址")
			return
		}
		if err := s.saveAppPaymentMode(r.Context(), mode); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "保存支付模式失败")
			return
		}
		httpx.OK(w, s.appPaymentModeResponse(r.Context(), mode))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) appPaymentModeResponse(ctx context.Context, mode string) appPaymentModeResp {
	xznCfg, _ := s.loadXZNConfig(ctx)
	configuredQR := false
	if site, err := siteconfig.ReadStore(ctx, s.db, s.env.SiteConfig); err == nil {
		configuredQR = strings.TrimSpace(site.Site.CustomerServiceQr) != ""
	}
	channels := make([]string, 0, 2)
	for _, channel := range xznAvailableAppChannels(xznCfg) {
		channels = append(channels, displayAppPayChannel(channel))
	}
	return appPaymentModeResp{
		Mode:                      mode,
		CustomerServiceConfigured: configuredQR,
		XZNConfigured:             xznCredentialsConfigured(xznCfg),
		XZNEnabled:                xznCfg.Enabled,
		XZNChannels:               channels,
	}
}
