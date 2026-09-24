package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestAppOrderLifecycleUsesSnapshotAndRefundsIdempotently(t *testing.T) {
	database := openAppOrderLifecycleTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	phone := fmt.Sprintf("197%08d", time.Now().UnixNano()%100000000)
	var userID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone,member_level) VALUES($1,'free') RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID) })

	var orderID int64
	outTradeNo := fmt.Sprintf("lifecycle-%d", userID)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_orders
		(out_trade_no,app_user_id,product_id,title,amount,duration_days,status,purchase_mode,payment_provider,pay_channel,gateway_id,provider_trade_no,provider_status)
		VALUES($1,$2,'vip_month','月卡会员',2900,45,'pending','xzn','xzn','alipay','34','provider-lifecycle','WAIT_BUYER_PAY')
		RETURNING id`, outTradeNo, userID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: database}
	success := xznCallback{
		TradeNo: "provider-lifecycle", OutTradeNo: outTradeNo, TotalAmount: "29.00", TotalCents: 2900,
		Subject: "月卡会员", PaytypeCode: "alipay", ChannelID: "34", TradeStatus: "TRADE_SUCCESS",
	}
	if err := s.applyXZNCallback(ctx, xznPaymentConfig{}, success); err != nil {
		t.Fatalf("first callback: %v", err)
	}

	var grantedAt time.Time
	if err := database.QueryRowContext(ctx, `SELECT activation_at FROM app_orders WHERE id=$1`, orderID).Scan(&grantedAt); err != nil {
		t.Fatal(err)
	}
	wantExpiry := grantedAt.AddDate(0, 0, 45)
	assertAppOrderLifecycleState(t, database, orderID, userID, "paid", "vip_month", wantExpiry)

	if err := s.applyXZNCallback(ctx, xznPaymentConfig{}, success); err != nil {
		t.Fatalf("duplicate callback: %v", err)
	}
	assertAppOrderLifecycleState(t, database, orderID, userID, "paid", "vip_month", wantExpiry)

	refund := success
	refund.TradeStatus = "TRADE_REFUND"
	if err := s.applyXZNCallback(ctx, xznPaymentConfig{}, refund); err != nil {
		t.Fatalf("refund callback: %v", err)
	}
	assertAppOrderLifecycleState(t, database, orderID, userID, "refunded", "free", time.Time{})

	if err := s.applyXZNCallback(ctx, xznPaymentConfig{}, refund); err != nil {
		t.Fatalf("duplicate refund callback: %v", err)
	}
	if err := s.applyXZNCallback(ctx, xznPaymentConfig{}, success); err != nil {
		t.Fatalf("delayed success callback: %v", err)
	}
	assertAppOrderLifecycleState(t, database, orderID, userID, "refunded", "free", time.Time{})
}

func TestRefundAppOrderRejectsUnpaidOrder(t *testing.T) {
	database := openAppOrderLifecycleTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	phone := fmt.Sprintf("195%08d", time.Now().UnixNano()%100000000)
	var userID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone,member_level) VALUES($1,'free') RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID) })

	var orderID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_orders
		(out_trade_no,app_user_id,product_id,title,amount,duration_days,status,purchase_mode,payment_provider)
		VALUES($1,$2,'vip_month','月卡会员',2900,30,'pending_confirmation','customer_service','manual') RETURNING id`,
		fmt.Sprintf("unpaid-refund-%d", userID), userID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}

	tx, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := refundAppOrderTx(ctx, tx, appOrderRefundInput{OrderID: orderID, Reason: "用户申请退款"}); err == nil {
		t.Fatal("unpaid order refund must be rejected")
	}
}

func TestAdminAppOrderRefundIsIdempotentAndAudited(t *testing.T) {
	database := openAppOrderLifecycleTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	phone := fmt.Sprintf("194%08d", time.Now().UnixNano()%100000000)
	var userID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone,member_level,member_started_at,member_expires_at)
		VALUES($1,'vip_month',now(),now()+interval '30 days') RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID) })

	var orderID int64
	outTradeNo := fmt.Sprintf("manual-refund-%d", userID)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_orders
		(out_trade_no,app_user_id,product_id,title,amount,duration_days,status,purchase_mode,payment_provider,paid_at,activation_at,membership_expires_at,
		 member_level_before)
		VALUES($1,$2,'vip_month','月卡会员',2900,30,'paid','customer_service','manual',now(),now(),now()+interval '30 days','free') RETURNING id`,
		outTradeNo, userID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: database, auditLogs: auditlog.NewStore(database)}
	perform := func() *httptest.ResponseRecorder {
		payload, _ := json.Marshal(map[string]string{"reason": "用户取消续费"})
		request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/app-orders/%d/refund", orderID), bytes.NewReader(payload))
		response := httptest.NewRecorder()
		s.adminAppOrderRefund(response, request)
		return response
	}

	first := perform()
	if first.Code != http.StatusOK {
		t.Fatalf("first refund status=%d body=%s", first.Code, first.Body.String())
	}
	second := perform()
	if second.Code != http.StatusOK || !bytes.Contains(second.Body.Bytes(), []byte(`"alreadyRefunded":true`)) {
		t.Fatalf("repeated refund status=%d body=%s", second.Code, second.Body.String())
	}
	assertAppOrderLifecycleState(t, database, orderID, userID, "refunded", "free", time.Time{})

	var auditCount int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM admin_operation_logs
		WHERE action='app_order.refund' AND target_type='app_order' AND target_id=$1`, fmt.Sprint(orderID)).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("refund audit count=%d, want 1", auditCount)
	}
}

func TestAppBillingCancelOrderClosesOwnPendingOrderIdempotently(t *testing.T) {
	database := openAppOrderLifecycleTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	phone := fmt.Sprintf("193%08d", time.Now().UnixNano()%100000000)
	var userID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone,member_level) VALUES($1,'free') RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID) })

	outTradeNo := fmt.Sprintf("cancel-order-%d", userID)
	if _, err := database.ExecContext(ctx, `INSERT INTO app_orders
		(out_trade_no,app_user_id,product_id,title,amount,duration_days,status,purchase_mode,payment_provider,pay_channel,gateway_id)
		VALUES($1,$2,'vip_month','月卡会员',2900,30,'pending','xzn','xzn','alipay','34')`, outTradeNo, userID); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: database}
	perform := func() *httptest.ResponseRecorder {
		payload, _ := json.Marshal(map[string]string{"outTradeNo": outTradeNo})
		request := httptest.NewRequest(http.MethodPost, "/api/app/billing/orders/cancel", bytes.NewReader(payload))
		request = request.WithContext(contextWithAppUser(request.Context(), auth.UserInfo{ID: userID, Phone: phone}))
		response := httptest.NewRecorder()
		s.appBillingCancelOrder(response, request)
		return response
	}

	for attempt := 1; attempt <= 2; attempt++ {
		response := perform()
		if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"status":"closed"`)) ||
			!bytes.Contains(response.Body.Bytes(), []byte("订单已取消")) {
			t.Fatalf("cancel attempt %d status=%d body=%s", attempt, response.Code, response.Body.String())
		}
	}
}

func openAppOrderLifecycleTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, _ := testdb.OpenEnvIsolatedSchema(t, "app_order_lifecycle")
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	return database
}

func assertAppOrderLifecycleState(t *testing.T, database *sql.DB, orderID, userID int64, wantStatus, wantLevel string, wantExpiry time.Time) {
	t.Helper()
	var status, level string
	var expiry sql.NullTime
	if err := database.QueryRow(`SELECT status FROM app_orders WHERE id=$1`, orderID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT member_level,member_expires_at FROM app_users WHERE id=$1`, userID).Scan(&level, &expiry); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || level != wantLevel {
		t.Fatalf("state status=%s level=%s, want status=%s level=%s", status, level, wantStatus, wantLevel)
	}
	if wantExpiry.IsZero() {
		if expiry.Valid {
			t.Fatalf("expiry=%s, want null", expiry.Time)
		}
	} else if !expiry.Valid || !expiry.Time.Equal(wantExpiry) {
		t.Fatalf("expiry=%v, want %s", expiry, wantExpiry)
	}
}
