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
	var orderAmount int64
	var currentActivation, currentMembershipExpiry sql.NullTime
	err := tx.QueryRowContext(ctx, `
		SELECT id, app_user_id, product_id, status, activation_at, membership_expires_at, amount
		FROM app_orders WHERE id=$1 FOR UPDATE`, input.OrderID).Scan(
		&orderID, &appUserID, &productID, &status, &currentActivation, &currentMembershipExpiry, &orderAmount)
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
	durationDays := 0
	_ = tx.QueryRowContext(ctx, `SELECT duration_days FROM app_orders WHERE id=$1`, orderID).Scan(&durationDays)
	if durationDays <= 0 {
		durationDays, err = membershipDurationDays(productID)
		if err != nil {
			return appOrderSettlementResult{}, fmt.Errorf("settlement plan: %w", err)
		}
	}
	var currentExpiry *time.Time
	if currentExpiresAt.Valid {
		currentExpiry = &currentExpiresAt.Time
	}
	period, err := calculateMembershipPeriodDays(durationDays, activationAt, currentExpiry)
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
	if err := generateDistributionCommissionsTx(ctx, tx, orderID, appUserID, orderAmount); err != nil {
		return appOrderSettlementResult{}, fmt.Errorf("distribution commission: %w", err)
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

// generateDistributionCommissionsTx snapshots the active rule and current
// agent chain while the paid order transaction is still locked.
func generateDistributionCommissionsTx(ctx context.Context, tx *sql.Tx, orderID, paidUserID int64, amount int64) error {
	if amount <= 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
		WITH RECURSIVE chain AS (
			SELECT a.id, a.app_user_id, a.level, a.parent_agent_id, a.root_agent_id,
			       ('/'||a.id||'/')::text AS path
			FROM distribution_user_relations r JOIN distribution_agents a ON a.id=r.direct_agent_id
			WHERE r.app_user_id=$1
			UNION ALL
			SELECT p.id, p.app_user_id, p.level, p.parent_agent_id, p.root_agent_id,
			       ('/'||p.id||'/'||c.path)::text
			FROM chain c JOIN distribution_agents p ON p.id=c.parent_agent_id
		), active_rule AS (
			SELECT id, version FROM distribution_commission_rules WHERE status='active' ORDER BY version DESC LIMIT 1
		)
		SELECT c.id,c.app_user_id,c.level,c.root_agent_id,c.path,
		       COALESCE(i.rate_bps,0),COALESCE(ar.id,0),COALESCE(ar.version,0)
		FROM chain c CROSS JOIN active_rule ar
		LEFT JOIN distribution_commission_rule_items i ON i.rule_id=ar.id AND i.agent_level=c.level
		ORDER BY c.level`, paidUserID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var agentID, agentUserID, rootID, ruleID, version int64
		var level int
		var path string
		var rate int64
		if err := rows.Scan(&agentID, &agentUserID, &level, &rootID, &path, &rate, &ruleID, &version); err != nil {
			return err
		}
		commission := amount * rate / 10000
		if _, err := tx.ExecContext(ctx, `INSERT INTO distribution_commission_records(order_id,agent_id,app_user_id,agent_level,order_amount,rate_bps,commission_amount,rule_version,chain_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(order_id,agent_id) DO NOTHING`, orderID, agentID, paidUserID, level, amount, rate, commission, version, path); err != nil {
			return err
		}
	}
	return rows.Err()
}
