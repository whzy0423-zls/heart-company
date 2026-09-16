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

var errAppOrderNotRefundable = errors.New("app order is not refundable")

type appOrderRefundInput struct {
	OrderID        int64
	Reason         string
	RefundedAt     time.Time
	ProviderStatus string
}

type appOrderRefundResult struct {
	OrderID             int64
	AlreadyRefunded     bool
	EntitlementReverted bool
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
	var durationDays int
	var productID, status string
	var orderAmount int64
	var currentActivation, currentMembershipExpiry sql.NullTime
	err := tx.QueryRowContext(ctx, `
		SELECT id, app_user_id, product_id, status, duration_days, activation_at, membership_expires_at, amount
		FROM app_orders WHERE id=$1 FOR UPDATE`, input.OrderID).Scan(
		&orderID, &appUserID, &productID, &status, &durationDays, &currentActivation, &currentMembershipExpiry, &orderAmount)
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
	if status == "refunded" {
		return appOrderSettlementResult{}, fmt.Errorf("settlement order: refunded orders are terminal")
	}
	if durationDays <= 0 || durationDays > 3660 {
		return appOrderSettlementResult{}, fmt.Errorf("settlement duration: invalid snapshot %d", durationDays)
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
		    member_level_before=$7, member_started_at_before=$8, member_expires_at_before=$9,
		    payment_error='', update_time=now()
		WHERE id=$1`, orderID, activationAt, period.Expires, input.ProviderTrade, input.ProviderStatus, input.TransactionID,
		memberLevel, nullableTimeArgument(currentStartedAt), nullableTimeArgument(currentExpiresAt)); err != nil {
		return appOrderSettlementResult{}, fmt.Errorf("settlement order update: %w", err)
	}
	if err := generateDistributionCommissionsTx(ctx, tx, orderID, appUserID, orderAmount); err != nil {
		return appOrderSettlementResult{}, fmt.Errorf("distribution commission: %w", err)
	}
	return appOrderSettlementResult{OrderID: orderID, PlanCode: productID, StartedAt: startedAt, ExpiresAt: period.Expires}, nil
}

func nullableTimeArgument(value sql.NullTime) any {
	if value.Valid {
		return value.Time
	}
	return nil
}

// refundAppOrderTx makes paid -> refunded a terminal, idempotent transition
// and removes exactly this order's saved duration from the user's entitlement.
func refundAppOrderTx(ctx context.Context, tx *sql.Tx, input appOrderRefundInput) (appOrderRefundResult, error) {
	if input.OrderID <= 0 {
		return appOrderRefundResult{}, errors.New("invalid app order id")
	}
	refundedAt := input.RefundedAt
	if refundedAt.IsZero() {
		refundedAt = time.Now()
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return appOrderRefundResult{}, errors.New("refund reason required")
	}

	var orderID, appUserID int64
	var durationDays int
	var status string
	var grantedExpiry, beforeStartedAt, beforeExpiresAt sql.NullTime
	var beforeLevel sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT id,app_user_id,status,duration_days,membership_expires_at,
		       member_level_before,member_started_at_before,member_expires_at_before
		FROM app_orders WHERE id=$1 FOR UPDATE`, input.OrderID).Scan(
		&orderID, &appUserID, &status, &durationDays, &grantedExpiry,
		&beforeLevel, &beforeStartedAt, &beforeExpiresAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appOrderRefundResult{}, errXZNCallbackNotFound
		}
		return appOrderRefundResult{}, fmt.Errorf("refund order: %w", err)
	}
	if status == "refunded" {
		return appOrderRefundResult{OrderID: orderID, AlreadyRefunded: true}, nil
	}
	if status != "paid" {
		return appOrderRefundResult{}, fmt.Errorf("%w: status=%s", errAppOrderNotRefundable, status)
	}
	if durationDays <= 0 || durationDays > 3660 {
		return appOrderRefundResult{}, fmt.Errorf("refund duration: invalid snapshot %d", durationDays)
	}

	var currentLevel string
	var currentStartedAt, currentExpiresAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		SELECT member_level,member_started_at,member_expires_at
		FROM app_users WHERE id=$1 FOR UPDATE`, appUserID).Scan(&currentLevel, &currentStartedAt, &currentExpiresAt); err != nil {
		return appOrderRefundResult{}, fmt.Errorf("refund user: %w", err)
	}

	nextLevel := "free"
	var nextStartedAt any
	var nextExpiresAt any
	if grantedExpiry.Valid && currentExpiresAt.Valid && currentExpiresAt.Time.Equal(grantedExpiry.Time) && beforeLevel.Valid {
		nextLevel = firstNonEmpty(strings.TrimSpace(beforeLevel.String), "free")
		nextStartedAt = nullableTimeArgument(beforeStartedAt)
		nextExpiresAt = nullableTimeArgument(beforeExpiresAt)
	} else if currentExpiresAt.Valid {
		reducedExpiry := currentExpiresAt.Time.AddDate(0, 0, -durationDays)
		if reducedExpiry.After(refundedAt) {
			nextExpiresAt = reducedExpiry
			nextStartedAt = nullableTimeArgument(currentStartedAt)
			if err := tx.QueryRowContext(ctx, `
				SELECT product_id FROM app_orders
				WHERE app_user_id=$1 AND id<>$2 AND status='paid'
				ORDER BY paid_at DESC NULLS LAST,id DESC LIMIT 1`, appUserID, orderID).Scan(&nextLevel); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return appOrderRefundResult{}, fmt.Errorf("refund remaining plan: %w", err)
			}
			if strings.TrimSpace(nextLevel) == "" {
				nextLevel = firstNonEmpty(currentLevel, "free")
			}
		}
	}
	if nextLevel == "free" {
		nextStartedAt, nextExpiresAt = nil, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_users
		SET member_level=$2,member_started_at=$3,member_expires_at=$4,update_time=now()
		WHERE id=$1`, appUserID, nextLevel, nextStartedAt, nextExpiresAt); err != nil {
		return appOrderRefundResult{}, fmt.Errorf("refund user update: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_orders
		SET status='refunded',refunded_at=$2,refund_reason=$3,
		    provider_status=CASE WHEN $4<>'' THEN $4 ELSE provider_status END,
		    payment_error='',update_time=now()
		WHERE id=$1`, orderID, refundedAt, reason, input.ProviderStatus); err != nil {
		return appOrderRefundResult{}, fmt.Errorf("refund order update: %w", err)
	}
	if err := lockDistributionLedger(ctx, tx); err != nil {
		return appOrderRefundResult{}, fmt.Errorf("distribution ledger lock: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE distribution_commission_records
		SET status='reversed',reversal_reason=$2,updated_at=now()
		WHERE order_id=$1 AND status<>'reversed'`, orderID, reason); err != nil {
		return appOrderRefundResult{}, fmt.Errorf("distribution commission reversal: %w", err)
	}
	return appOrderRefundResult{OrderID: orderID, EntitlementReverted: true}, nil
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
	client, _, err := s.newXZNClient(ctx)
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
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return appOrderResp{}, err
	}
	defer tx.Rollback()
	switch {
	case status == "refunded":
		// Refunded is terminal, including against delayed success observations.
	case query.TradeStatus == "TRADE_REFUND" && status == "paid":
		if _, err := refundAppOrderTx(ctx, tx, appOrderRefundInput{
			OrderID: orderID, Reason: "支付平台查单确认退款", ProviderStatus: query.TradeStatus,
		}); err != nil {
			return appOrderResp{}, err
		}
	case query.TradeStatus == "TRADE_REFUND":
		if _, err := tx.ExecContext(ctx, `UPDATE app_orders
			SET status='refunded',provider_trade_no=$2,provider_status=$3,last_query_at=now(),
			    refunded_at=now(),refund_reason='支付平台查单确认退款',payment_error='',update_time=now()
			WHERE id=$1`, orderID, query.TradeNo, query.TradeStatus); err != nil {
			return appOrderResp{}, err
		}
	case query.TradeStatus == "TRADE_SUCCESS":
		if _, err := settleAppOrderTx(ctx, tx, appOrderSettlementInput{
			OrderID: orderID, ActivationAt: time.Now(), ProviderTrade: query.TradeNo,
			ProviderStatus: query.TradeStatus,
		}); err != nil {
			return appOrderResp{}, err
		}
	case status != "paid":
		if _, err := tx.ExecContext(ctx, `UPDATE app_orders
			SET provider_trade_no=$2,provider_status=$3,last_query_at=now(),status=$4,
			    payment_error='',update_time=now()
			WHERE id=$1`, orderID, query.TradeNo, query.TradeStatus, xznLocalStatus(query.TradeStatus)); err != nil {
			return appOrderResp{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return appOrderResp{}, err
	}
	resp, err := s.loadAppOrderByOutTradeNo(ctx, appUserID, outTradeNo)
	if err != nil {
		return appOrderResp{}, err
	}
	return s.enrichOnlineOrder(ctx, appUserID, resp)
}

func xznReconcileLocalStatus(currentStatus, providerStatus string) string {
	if currentStatus == "refunded" || currentStatus == "paid" || strings.EqualFold(strings.TrimSpace(providerStatus), "TRADE_SUCCESS") {
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
	if err := lockDistributionLedger(ctx, tx); err != nil {
		return fmt.Errorf("distribution ledger lock: %w", err)
	}
	// One INSERT SELECT avoids issuing a second command while PostgreSQL rows
	// are still streaming on the transaction connection. Every beneficiary keeps
	// the same complete referral snapshot, not a progressively truncated path.
	_, err := tx.ExecContext(ctx, `
 WITH RECURSIVE chain AS (
   SELECT a.id,a.level,a.parent_agent_id,a.status,a.agent_path AS path,
          ARRAY[a.id] AS visited,1 AS depth
   FROM distribution_user_relations r
   JOIN distribution_agents a ON a.id=r.direct_agent_id
   WHERE r.app_user_id=$2
   UNION ALL
   SELECT p.id,p.level,p.parent_agent_id,p.status,c.path,
          c.visited||p.id,c.depth+1
   FROM chain c JOIN distribution_agents p ON p.id=c.parent_agent_id
   WHERE c.depth<3 AND NOT p.id=ANY(c.visited)
 ), active_rule AS (
   SELECT id,version FROM distribution_commission_rules
   WHERE status='active' ORDER BY version DESC LIMIT 1
 )
 INSERT INTO distribution_commission_records
 (order_id,agent_id,app_user_id,agent_level,order_amount,rate_bps,commission_amount,rule_version,chain_snapshot)
 SELECT $1,c.id,$2,c.level,$3,i.rate_bps,
        floor(($3::bigint)::numeric*i.rate_bps/10000)::bigint,ar.version,c.path
 FROM chain c CROSS JOIN active_rule ar
 JOIN distribution_commission_rule_items i ON i.rule_id=ar.id AND i.agent_level=c.level
 WHERE c.status='active'
 ON CONFLICT(order_id,agent_id) DO NOTHING`, orderID, paidUserID, amount)
	return err
}
