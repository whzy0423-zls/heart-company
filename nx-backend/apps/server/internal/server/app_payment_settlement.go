package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/xznpay"
)

const xznReconcileMinInterval = 5 * time.Second

// appOrderSettlementInput carries the trusted result of a payment provider
// verification into the single membership-grant transaction.
type appOrderSettlementInput struct {
	OrderID        int64
	ActivationAt   time.Time
	ProviderTrade  string
	ProviderStatus string
	TransactionID  string
}

type appOrderSettlementResult struct {
	OrderID        int64
	PlanCode       string
	StartedAt      time.Time
	ExpiresAt      time.Time
	AlreadyGranted bool
}

// settleAppOrderTx locks the order and user, then atomically grants the plan.
// It is intentionally idempotent: a repeated callback returns the existing
// membership without extending it a second time.
func settleAppOrderTx(ctx context.Context, tx *sql.Tx, input appOrderSettlementInput) (appOrderSettlementResult, error) {
	if input.OrderID <= 0 {
		return appOrderSettlementResult{}, errors.New("invalid app order id")
	}
	activationAt := input.ActivationAt
	if activationAt.IsZero() {
		activationAt = time.Now()
	}
	var orderID, appUserID int64
	var productID, status string
	var currentActivation, currentMembershipExpiry sql.NullTime
	err := tx.QueryRowContext(ctx, `
		SELECT id, app_user_id, product_id, status, activation_at, membership_expires_at
		FROM app_orders WHERE id=$1 FOR UPDATE`, input.OrderID).Scan(
		&orderID, &appUserID, &productID, &status, &currentActivation, &currentMembershipExpiry)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appOrderSettlementResult{}, errXZNCallbackNotFound
		}
		return appOrderSettlementResult{}, fmt.Errorf("settlement order: %w", err)
	}
	var memberLevel string
	var currentStartedAt, currentExpiresAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		SELECT member_level, member_started_at, member_expires_at
		FROM app_users WHERE id=$1 FOR UPDATE`, appUserID).Scan(
		&memberLevel, &currentStartedAt, &currentExpiresAt); err != nil {
		return appOrderSettlementResult{}, fmt.Errorf("settlement user: %w", err)
	}
	if status == "paid" {
		started := nullableTimeValue(currentStartedAt, currentActivation)
		expires := nullableTimeValue(currentExpiresAt, currentMembershipExpiry)
		return appOrderSettlementResult{OrderID: orderID, PlanCode: firstNonEmpty(productID, memberLevel), StartedAt: started, ExpiresAt: expires, AlreadyGranted: true}, nil
	}
	if _, err := membershipDurationDays(productID); err != nil {
		return appOrderSettlementResult{}, fmt.Errorf("settlement plan: %w", err)
	}
	var currentExpiry *time.Time
	if currentExpiresAt.Valid {
		currentExpiry = &currentExpiresAt.Time
	}
	period, err := calculateMembershipPeriod(productID, activationAt, currentExpiry)
	if err != nil {
		return appOrderSettlementResult{}, err
	}
	startedAt := period.Start
	if currentExpiresAt.Valid && currentExpiresAt.Time.After(activationAt) && currentStartedAt.Valid {
		startedAt = currentStartedAt.Time
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_users
		SET member_level=$1, member_started_at=$2, member_expires_at=$3, update_time=now()
		WHERE id=$4`, productID, startedAt, period.Expires, appUserID); err != nil {
		return appOrderSettlementResult{}, fmt.Errorf("settlement user update: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_orders
		SET status='paid', paid_at=COALESCE(paid_at, now()), activation_at=$2,
		    membership_expires_at=$3,
		    provider_trade_no=CASE WHEN $4<>'' THEN $4 ELSE provider_trade_no END,
		    provider_status=CASE WHEN $5<>'' THEN $5 ELSE provider_status END,
		    transaction_id=CASE WHEN $6<>'' THEN $6 ELSE transaction_id END,
		    payment_error='', update_time=now()
		WHERE id=$1`, orderID, activationAt, period.Expires, input.ProviderTrade, input.ProviderStatus, input.TransactionID); err != nil {
		return appOrderSettlementResult{}, fmt.Errorf("settlement order update: %w", err)
	}
	return appOrderSettlementResult{OrderID: orderID, PlanCode: productID, StartedAt: startedAt, ExpiresAt: period.Expires}, nil
}

func nullableTimeValue(primary, fallback sql.NullTime) time.Time {
	if primary.Valid {
		return primary.Time
	}
	if fallback.Valid {
		return fallback.Time
	}
	return time.Time{}
}

// reconcileXZNAppOrder verifies the provider record before changing local
// state. The returned object is suitable for both App and admin callers.
func (s *Server) reconcileXZNAppOrder(ctx context.Context, outTradeNo string) (appOrderResp, error) {
	outTradeNo = strings.TrimSpace(outTradeNo)
	if outTradeNo == "" {
		return appOrderResp{}, errors.New("outTradeNo required")
	}
	var orderID, appUserID int64
	var productID, title, provider, payChannel, gatewayID, providerTradeNo, providerStatus, payURL string
	var amount int
	var status string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id, app_user_id, product_id, title, amount, status,
		       COALESCE(payment_provider,''), COALESCE(pay_channel,''), COALESCE(gateway_id,''),
		       COALESCE(provider_trade_no,''), COALESCE(provider_status,''), COALESCE(pay_url,'')
		FROM app_orders WHERE out_trade_no=$1`, outTradeNo).Scan(
		&orderID, &appUserID, &productID, &title, &amount, &status,
		&provider, &payChannel, &gatewayID, &providerTradeNo, &providerStatus, &payURL); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appOrderResp{}, errXZNCallbackNotFound
		}
		return appOrderResp{}, err
	}
	if provider != appPaymentProviderXZN {
		return appOrderResp{}, errors.New("订单不是在线支付订单")
	}
	client, cfg, err := s.newXZNClient(ctx)
	if err != nil {
		return appOrderResp{}, errors.New("在线支付尚未配置")
	}
	// Claim the query slot atomically. This timestamp is written before the
	// network call so concurrent App polls cannot fan out to the provider; a
	// failed attempt is still rate-limited and recorded below.
	claimed, claimErr := s.db.ExecContext(ctx, `
		UPDATE app_orders
		SET last_query_at=now(), update_time=now()
		WHERE out_trade_no=$1
		  AND (last_query_at IS NULL OR last_query_at <= now() - INTERVAL '5 seconds')`, outTradeNo)
	if claimErr != nil {
		return appOrderResp{}, claimErr
	}
	if rows, rowsErr := claimed.RowsAffected(); rowsErr == nil && rows == 0 {
		current, loadErr := s.loadAppOrderByOutTradeNo(ctx, appUserID, outTradeNo)
		if loadErr != nil {
			return appOrderResp{}, loadErr
		}
		return s.enrichOnlineOrder(ctx, appUserID, current)
	}
	query, err := client.Query(ctx, xznpay.QueryRequest{TradeNo: providerTradeNo, OutTradeNo: outTradeNo})
	if err != nil {
		_, _ = s.db.ExecContext(ctx, `UPDATE app_orders SET payment_error=$2, update_time=now() WHERE out_trade_no=$1`, outTradeNo, truncatePaymentError(err.Error()))
		return appOrderResp{}, fmt.Errorf("查单失败: %w", err)
	}
	if query.OutTradeNo != outTradeNo || query.TotalAmountCents != int64(amount) {
		return appOrderResp{}, errors.New("平台订单信息与本地订单不一致")
	}
	if providerTradeNo != "" && query.TradeNo != providerTradeNo {
		return appOrderResp{}, errors.New("平台交易号与本地订单不一致")
	}
	localStatus := xznLocalStatus(query.TradeStatus)
	// Keep a successful provider result pending until settleAppOrderTx grants
	// membership in the same transaction. Writing "paid" first would make the
	// idempotency guard treat a newly paid provider order as already settled.
	reconciledStatus := xznReconcileLocalStatus(status, query.TradeStatus)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return appOrderResp{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_orders SET provider_trade_no=$2, provider_status=$3, last_query_at=now(),
			status=CASE WHEN status='paid' THEN status ELSE $4 END,
			payment_error='', update_time=now()
		WHERE id=$1`, orderID, query.TradeNo, query.TradeStatus, reconciledStatus); err != nil {
		return appOrderResp{}, err
	}
	if query.TradeStatus == "TRADE_SUCCESS" {
		if _, err := settleAppOrderTx(ctx, tx, appOrderSettlementInput{
			OrderID: orderID, ActivationAt: time.Now(), ProviderTrade: query.TradeNo,
			ProviderStatus: query.TradeStatus,
		}); err != nil {
			return appOrderResp{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return appOrderResp{}, err
	}
	resp := appOrderResp{OutTradeNo: outTradeNo, ProductID: productID, Title: title, Amount: amount, Status: localStatus,
		PaymentProvider: provider, PayChannel: displayAppPayChannel(payChannel), GatewayID: gatewayID,
		ProviderTradeNo: query.TradeNo, ProviderStatus: query.TradeStatus, PayURL: payURL}
	if query.TradeStatus == "TRADE_SUCCESS" {
		resp.Status = "paid"
	}
	if cfg.ReturnURL != "" {
		resp.Payment = map[string]any{"type": "web", "mode": "h5", "channel": resp.PayChannel, "url": payURL, "payUrl": payURL, "returnUrl": xznOrderReturnURL(cfg.ReturnURL, outTradeNo)}
	}
	return s.enrichOnlineOrder(ctx, appUserID, resp)
}

func xznReconcileLocalStatus(currentStatus, providerStatus string) string {
	if currentStatus == "paid" || strings.EqualFold(strings.TrimSpace(providerStatus), "TRADE_SUCCESS") {
		return currentStatus
	}
	return xznLocalStatus(providerStatus)
}

func truncatePaymentError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 500 {
		return value[:500]
	}
	return value
}
