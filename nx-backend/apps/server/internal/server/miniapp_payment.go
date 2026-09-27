package server

import (
	"context"
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
)

const miniappPaymentDisabledMessage = "小程序支付暂未开放"

// miniappPaymentEnabled reads the same public site configuration consumed by
// the miniapp. Reading it for each order keeps an admin switch effective
// without restarting the server; siteconfig.ReadStore uses the persisted DB
// value when available and falls back to the configured JSON file.
func (s *Server) miniappPaymentEnabled(ctx context.Context) (bool, error) {
	if s == nil {
		return true, nil
	}
	// Handler unit tests and lightweight deployments can construct a Server
	// without a site-config source. Preserve the legacy enabled behavior there;
	// normal server startup always supplies env.SiteConfig or a database.
	if s.db == nil && strings.TrimSpace(s.env.SiteConfig) == "" {
		return true, nil
	}
	config, err := siteconfig.ReadStore(ctx, s.db, s.env.SiteConfig)
	if err != nil {
		return false, err
	}
	return siteconfig.MiniappPaymentEnabled(config), nil
}

// requireMiniappPayment blocks only operations that create or finalize a new
// payment. Existing order status and entitlement reads remain available so a
// temporary checkout shutdown does not revoke or hide purchased content.
func (s *Server) requireMiniappPayment(w http.ResponseWriter, r *http.Request) bool {
	enabled, err := s.miniappPaymentEnabled(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "读取小程序支付配置失败")
		return false
	}
	if !enabled {
		httpx.Fail(w, http.StatusServiceUnavailable, miniappPaymentDisabledMessage)
		return false
	}
	return true
}
