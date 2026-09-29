package server

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/config"
)

var errAppOrderAlreadyPending = errors.New("an active checkout already exists")

const appCrossSurfacePendingMessage = "已有其他端发起的待处理订单，请先完成或取消该订单后再重新下单"

// parseXZNWebReturnURL accepts only a Flutter billing hash route. An outer
// query can contain auth or unrelated navigation data, so it is never copied.
func parseXZNWebReturnURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || urlComponentHasControl(u) {
		return nil, errors.New("invalid H5 payment return URL")
	}
	fragment, err := url.Parse(u.Fragment)
	if err != nil || fragment.Path != "/billing" || fragment.Host != "" || fragment.Scheme != "" || fragment.Fragment != "" {
		return nil, errors.New("H5 payment return must use the billing hash route")
	}
	values, err := url.ParseQuery(fragment.RawQuery)
	if err != nil {
		return nil, errors.New("invalid H5 payment return parameters")
	}
	for key, values := range values {
		if key != "payment_return" || len(values) != 1 || values[0] != "1" {
			return nil, errors.New("unexpected H5 payment return parameter")
		}
	}
	return u, nil
}

func billingURLOrigin(u *url.URL) string {
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func isWebBillingLoopback(u *url.URL) bool {
	return u != nil && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}

func (s *Server) webBillingReturnAllowed(cfg xznPaymentConfig, u *url.URL) bool {
	if u == nil {
		return false
	}
	if isWebBillingLoopback(u) && !config.IsProduction(s.env.AppEnv) {
		return true
	}
	// Production always returns to HTTPS. Wildcard CORS is deliberately not an
	// allowlist, and the request's Origin/Host/forwarded headers are not trusted.
	if u.Scheme != "https" {
		return false
	}
	if configured, err := parseXZNWebReturnURL(cfg.WebReturnURL); err == nil && configured.Scheme == "https" && billingURLOrigin(configured) == billingURLOrigin(u) && configured.EscapedPath() == u.EscapedPath() {
		return true
	}
	for _, origin := range s.env.CORSAllowedOrigins {
		allowed, err := url.Parse(strings.TrimSpace(origin))
		if err == nil && allowed.Scheme == "https" && allowed.Host != "" && allowed.User == nil && (allowed.Path == "" || allowed.Path == "/") && allowed.RawQuery == "" && allowed.Fragment == "" && !urlComponentHasControl(allowed) && billingURLOrigin(allowed) == billingURLOrigin(u) {
			return true
		}
	}
	return false
}

func (s *Server) webBillingReturnConfigured(cfg xznPaymentConfig) bool {
	if !config.IsProduction(s.env.AppEnv) {
		return true // Local development permits only loopback HTTP overrides.
	}
	if u, err := parseXZNWebReturnURL(cfg.WebReturnURL); err == nil && s.webBillingReturnAllowed(cfg, u) {
		return true
	}
	for _, origin := range s.env.CORSAllowedOrigins {
		if u, err := parseXZNWebReturnURL(strings.TrimRight(origin, "/") + "/#/billing"); err == nil && s.webBillingReturnAllowed(cfg, u) {
			return true
		}
	}
	return false
}

func (s *Server) webBillingOrderReturnURL(cfg xznPaymentConfig, requested, outTradeNo string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		requested = cfg.WebReturnURL
	}
	u, err := parseXZNWebReturnURL(requested)
	if err != nil || !s.webBillingReturnAllowed(cfg, u) || strings.TrimSpace(outTradeNo) == "" {
		return "", errors.New("H5 支付返回地址尚未配置或不在允许范围内，请联系管理员")
	}
	fragment := &url.URL{Path: "/billing", RawQuery: url.Values{"payment_return": {"1"}, "outTradeNo": {outTradeNo}}.Encode()}
	u.Fragment, u.RawFragment = fragment.String(), ""
	return u.String(), nil
}

func appOrderCheckoutCompatible(surface, mode string, order appOrderResp) bool {
	if resolveAppOrderPurchaseMode(order.PurchaseMode, order.PaymentProvider) != mode {
		return false
	}
	if mode != appPurchaseModeXZN {
		return true
	}
	return (surface == appBillingSurfaceWeb) == (order.WebReturnURL != "")
}

// reserveAppBillingOrder serializes native and Web checkout for one account.
// The existing partial DB index protects only online XZN orders; a user-row
// lock also prevents a concurrent manual order and an online order. It leaves
// historical orders intact and does not require a destructive index migration.
func (s *Server) reserveAppBillingOrder(ctx context.Context, appUserID int64, insert string, args ...any) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lockedID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM app_users WHERE id=$1 AND status='active' FOR UPDATE`, appUserID).Scan(&lockedID); err != nil {
		return err
	}
	var pending appOrderResp
	err = tx.QueryRowContext(ctx, appPendingOrderQuery, appUserID).Scan(&pending.OutTradeNo, &pending.ProductID, &pending.Title, &pending.Amount, &pending.Status)
	if err == nil {
		return errAppOrderAlreadyPending
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.ExecContext(ctx, insert, args...); err != nil {
		return err
	}
	return tx.Commit()
}
