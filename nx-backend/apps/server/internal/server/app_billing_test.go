package server

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
)

func TestAppPlanCodeNormalizesLegacyMemberLevels(t *testing.T) {
	tests := []struct {
		name        string
		memberLevel string
		want        string
	}{
		{name: "empty is free", memberLevel: "", want: "free"},
		{name: "free stays free", memberLevel: "free", want: "free"},
		{name: "legacy vip becomes month", memberLevel: "vip", want: "vip_month"},
		{name: "canonical svip stays separate", memberLevel: "svip", want: "svip"},
		{name: "month stays month", memberLevel: "vip_month", want: "vip_month"},
		{name: "quarter stays quarter", memberLevel: "vip_quarter", want: "vip_quarter"},
		{name: "year stays year", memberLevel: "vip_year", want: "vip_year"},
		{name: "unknown member remains member safe", memberLevel: "legacy_partner", want: "legacy_partner"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := appPlanCode(tt.memberLevel); got != tt.want {
				t.Fatalf("appPlanCode(%q) = %q, want %q", tt.memberLevel, got, tt.want)
			}
		})
	}
}

func TestAppPlanNameUsesNormalizedPlanCodes(t *testing.T) {
	tests := []struct {
		planCode string
		want     string
	}{
		{planCode: "free", want: "免费版"},
		{planCode: "vip_month", want: "VIP 月卡"},
		{planCode: "vip_quarter", want: "VIP 季卡"},
		{planCode: "vip_year", want: "VIP 年卡"},
		{planCode: "svip", want: "SVIP"},
		{planCode: "legacy_partner", want: "会员版"},
	}

	for _, tt := range tests {
		t.Run(tt.planCode, func(t *testing.T) {
			if got := appPlanName(tt.planCode); got != tt.want {
				t.Fatalf("appPlanName(%q) = %q, want %q", tt.planCode, got, tt.want)
			}
		})
	}
}

func TestAppBillingEntitlementsUsesNormalizedPlan(t *testing.T) {
	s := newAppBillingEntitlementTestServer(t, "vip")

	response := performAppBillingRequest(t, s.appBillingEntitlements, http.MethodGet, "/api/app/billing/entitlements", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Code int                `json:"code"`
		Data appEntitlementResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.PlanCode != "vip_month" || body.Data.PlanName != "VIP 月卡" {
		t.Fatalf("expected normalized monthly plan, got %+v", body.Data)
	}
	if !body.Data.IsMember {
		t.Fatalf("expected legacy vip to remain a member, got %+v", body.Data)
	}
	if body.Data.ChatLimit != -1 || body.Data.ChatRemaining != -1 || body.Data.DeepReportRemaining != 0 {
		t.Fatalf("expected unlimited member chat quota, got %+v", body.Data)
	}
}

func TestAppBillingEntitlementsExposesCanonicalSvipCapabilities(t *testing.T) {
	s := newAppBillingEntitlementTestServer(t, "active:svip")
	response := performAppBillingRequest(t, s.appBillingEntitlements, http.MethodGet, "/api/app/billing/entitlements", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data appEntitlementResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.PlanCode != "svip" || body.Data.PlanLevel != "svip" || body.Data.BillingCycle != "year" || !body.Data.IsMember {
		t.Fatalf("expected canonical SVIP identity, got %+v", body.Data)
	}
	if body.Data.CardLimit != 10 || body.Data.ChatLimit != -1 {
		t.Fatalf("expected canonical SVIP quotas, got %+v", body.Data)
	}
	cardLimit, cardOK := body.Data.Quotas["cardLimit"].(float64)
	dailyChatLimit, chatOK := body.Data.Quotas["dailyChatLimit"].(float64)
	storyMonthlyLimit, storyOK := body.Data.Quotas["storyMonthlyLimit"].(float64)
	if !cardOK || !chatOK || !storyOK || cardLimit != 10 || dailyChatLimit != -1 || storyMonthlyLimit != 12 {
		t.Fatalf("expected canonical quota map, got %+v", body.Data.Quotas)
	}
	if body.Data.FeatureFlags["deepChat"] != true || body.Data.FeatureFlags["companion"] != true || body.Data.FeatureFlags["memberPoster"] != true {
		t.Fatalf("expected canonical feature flags, got %+v", body.Data.FeatureFlags)
	}
}

func TestAppBillingEntitlementsUnknownMemberFailsClosedToFree(t *testing.T) {
	s := newAppBillingEntitlementTestServer(t, "active:legacy_partner")
	response := performAppBillingRequest(t, s.appBillingEntitlements, http.MethodGet, "/api/app/billing/entitlements", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data appEntitlementResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.PlanCode != "free" || body.Data.PlanLevel != "free" || body.Data.IsMember {
		t.Fatalf("unknown member should resolve to free, got %+v", body.Data)
	}
	if body.Data.ChatLimit != 5 || body.Data.CardLimit != 1 {
		t.Fatalf("unknown member received paid quotas: %+v", body.Data)
	}
	if storyLimit, ok := body.Data.Quotas["storyMonthlyLimit"].(float64); !ok || storyLimit != 1 {
		t.Fatalf("unknown member received paid story quota: %+v", body.Data.Quotas)
	}
}

func TestAppBillingEntitlementsIncludesExistingPendingOrder(t *testing.T) {
	s := newAppBillingEntitlementTestServer(t, "pending|active:vip_month")
	response := performAppBillingRequest(t, s.appBillingEntitlements, http.MethodGet, "/api/app/billing/entitlements", nil)
	var body struct {
		Data appEntitlementResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.PendingOrder == nil || body.Data.PendingOrder.OutTradeNo != "app7-vip_month-existing" {
		t.Fatalf("expected existing pending order, got %+v", body.Data.PendingOrder)
	}
}

func TestAppBillingEntitlementsHidesPendingOrderOlderThanPaidGrant(t *testing.T) {
	s := newAppBillingEntitlementTestServer(t, "stale_pending|active:vip_year")
	response := performAppBillingRequest(t, s.appBillingEntitlements, http.MethodGet, "/api/app/billing/entitlements", nil)
	var body struct {
		Data appEntitlementResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Data.IsMember || body.Data.PlanCode != "vip_year" {
		t.Fatalf("expected active yearly membership, got %+v", body.Data)
	}
	if body.Data.PendingOrder != nil {
		t.Fatalf("older pending order must not override a later paid grant: %+v", body.Data.PendingOrder)
	}
}

func TestAppBillingEntitlementsReturnsActiveMembershipDates(t *testing.T) {
	s := newAppBillingEntitlementTestServer(t, "active:vip_quarter")
	response := performAppBillingRequest(t, s.appBillingEntitlements, http.MethodGet, "/api/app/billing/entitlements", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data appEntitlementResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Data.IsMember || body.Data.PlanCode != "vip_quarter" || body.Data.StartedAt == "" || body.Data.ExpiresAt == "" {
		t.Fatalf("expected active dated quarter membership, got %+v", body.Data)
	}
}

func TestAppBillingEntitlementsTreatsExpiredMembershipAsFree(t *testing.T) {
	s := newAppBillingEntitlementTestServer(t, "expired:vip_year")
	response := performAppBillingRequest(t, s.appBillingEntitlements, http.MethodGet, "/api/app/billing/entitlements", nil)
	var body struct {
		Data appEntitlementResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.IsMember || body.Data.PlanCode != "free" || body.Data.ExpiresAt != "" {
		t.Fatalf("expected expired membership to be free, got %+v", body.Data)
	}
}

func TestAppBillingEntitlementsIncludesTrialChatBalance(t *testing.T) {
	s := newAppBillingEntitlementTestServer(t, "free")
	s.appChatQuota = &recordingAppChatQuotaManager{
		trial:       12,
		trialExpiry: "2026-09-17T10:00:00+08:00",
	}
	response := performAppBillingRequest(t, s.appBillingEntitlements, http.MethodGet, "/api/app/billing/entitlements", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data appEntitlementResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.TrialChatRemaining != 12 || body.Data.TrialChatNearestExpiresAt != "2026-09-17T10:00:00+08:00" {
		t.Fatalf("unexpected trial entitlement: %+v", body.Data)
	}
}

func TestAppBillingCreateOrderUsesDefaultCustomerServiceMode(t *testing.T) {
	appBillingInsertCount.Store(0)
	s := newAppBillingTestServer(t)

	response := performAppBillingRequest(t, s.appBillingCreateOrder, http.MethodPost, "/api/app/billing/orders", map[string]any{
		"productId": "vip_month",
	})

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	body := decodeAppBillingResponse(t, response)
	if body.Data.PurchaseMode != appPurchaseModeCustomerService || body.Data.Status != appOrderPendingConfirmation {
		t.Fatalf("expected customer-service order, got %+v", body.Data)
	}
	if got := appBillingInsertCount.Load(); got != 1 {
		t.Fatalf("customer-service mode must create one manual order, got %d inserts", got)
	}
}

func TestAppBillingCreateOrderReusesExistingPendingOrder(t *testing.T) {
	appBillingInsertCount.Store(0)
	s := newAppBillingEntitlementTestServer(t, "pending|active:vip_month")
	response := performAppBillingRequest(t, s.appBillingCreateOrder, http.MethodPost, "/api/app/billing/orders", map[string]any{
		"productId": "vip_year",
	})
	body := decodeAppBillingResponse(t, response)
	if body.Data.OutTradeNo != "app7-vip_month-existing" || body.Data.ProductID != "vip_month" {
		t.Fatalf("expected existing pending order to be reused, got %+v", body.Data)
	}
	if got := appBillingInsertCount.Load(); got != 0 {
		t.Fatalf("expected no insert for duplicate pending order, got %d", got)
	}
}

func TestAppBillingCreateOrderExpiresOldPendingOrderBeforeCreating(t *testing.T) {
	appBillingInsertCount.Store(0)
	s := newAppBillingEntitlementTestServer(t, "expired_pending|free")

	response := performAppBillingRequest(t, s.appBillingCreateOrder, http.MethodPost, "/api/app/billing/orders", map[string]any{
		"productId": "vip_year",
	})

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	body := decodeAppBillingResponse(t, response)
	if body.Data.ProductID != "vip_year" || body.Data.Status != appOrderPendingConfirmation {
		t.Fatalf("expected a fresh yearly order after expiration, got %+v", body.Data)
	}
	if got := appBillingInsertCount.Load(); got != 1 {
		t.Fatalf("expected one fresh order insert, got %d", got)
	}
}

func TestAppBillingProductsExposeSixMembershipPlans(t *testing.T) {
	s := newAppBillingTestServer(t)

	response := performAppBillingRequest(t, s.appBillingProducts, http.MethodGet, "/api/app/billing/products", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Code int              `json:"code"`
		Data []appProductResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 6 {
		t.Fatalf("expected six membership products, got %+v", body.Data)
	}
	for _, product := range body.Data {
		if product.ID == "deep_report" {
			t.Fatalf("deep report must not be offered: %+v", product)
		}
		if !product.Enabled || product.PayEnabled || product.PurchaseMode != "customer_service" {
			t.Fatalf("expected enabled manual product without SDK payment, got %+v", product)
		}
		wantLevel := "vip"
		if strings.HasPrefix(product.ID, "svip_") {
			wantLevel = "svip"
		}
		if product.PlanLevel != wantLevel {
			t.Fatalf("product must expose normalized level: %+v", product)
		}
		wantDuration := map[string]int{"vip_month": 30, "vip_quarter": 90, "vip_year": 365, "svip_month": 30, "svip_quarter": 90, "svip_year": 365}[product.ID]
		if product.DurationDays != wantDuration {
			t.Fatalf("unexpected duration for product %+v", product)
		}
		storyBenefit := map[string]string{
			"vip_month": "每月 3 篇人生故事", "vip_quarter": "每月 3 篇人生故事", "vip_year": "每月 3 篇人生故事",
			"svip_month": "每月 12 篇人生故事", "svip_quarter": "每月 12 篇人生故事", "svip_year": "每月 12 篇人生故事",
		}[product.ID]
		if !strings.Contains(strings.Join(product.Features, "|"), storyBenefit) {
			t.Fatalf("product %s is missing story benefit %q: %+v", product.ID, storyBenefit, product.Features)
		}
	}
}

func TestAppBillingOrderStatusKeepsPendingCustomerConfirmation(t *testing.T) {
	s := newAppBillingTestServer(t)

	response := performAppBillingRequest(t, s.appBillingOrderStatus, http.MethodGet, "/api/app/billing/orders/status?outTradeNo=app7-vip_month-1", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	body := decodeAppBillingResponse(t, response)
	if body.Data.Status != "pending_confirmation" {
		t.Fatalf("expected stored pending confirmation status, got %+v", body.Data)
	}
	if body.Data.PayStatus != "pending_confirmation" {
		t.Fatalf("expected payStatus pending_confirmation, got %+v", body.Data)
	}
	if body.Data.PayEnabled {
		t.Fatalf("expected payment disabled until configured, got %+v", body.Data)
	}
	if body.Data.PurchaseMode != "customer_service" {
		t.Fatalf("expected customer service purchase mode, got %+v", body.Data)
	}
	if !strings.Contains(body.Data.Message, "客服") {
		t.Fatalf("expected customer confirmation message, got %q", body.Data.Message)
	}
}

func TestAppBillingOrderStatusClosesOrderOlderThan15Minutes(t *testing.T) {
	s := newAppBillingEntitlementTestServer(t, "expired_pending|free")

	response := performAppBillingRequest(t, s.appBillingOrderStatus, http.MethodGet, "/api/app/billing/orders/status?outTradeNo=app7-vip_month-1", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	body := decodeAppBillingResponse(t, response)
	if body.Data.Status != "closed" || body.Data.PayStatus != "closed" {
		t.Fatalf("expected expired order to be closed, got %+v", body.Data)
	}
	if !strings.Contains(body.Data.Message, "超过15分钟") {
		t.Fatalf("expected expiration message, got %q", body.Data.Message)
	}
}

func TestAppBillingOrderStatusRequiresOutTradeNo(t *testing.T) {
	s := newAppBillingTestServer(t)

	response := performAppBillingRequest(t, s.appBillingOrderStatus, http.MethodGet, "/api/app/billing/orders/status", nil)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", response.Code, response.Body.String())
	}
}

func newAppBillingTestServer(t *testing.T) *Server {
	t.Helper()
	registerAppBillingTestDriver()
	db, err := sql.Open(appBillingTestDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Server{
		db:       db,
		appUsers: appuser.NewStore(db),
	}
}

func newAppBillingEntitlementTestServer(t *testing.T, memberLevel string) *Server {
	t.Helper()
	registerAppBillingTestDriver()
	db, err := sql.Open(appBillingTestDriverName, memberLevel)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Server{
		db:       db,
		appUsers: appuser.NewStore(db),
	}
}

func performAppBillingRequest(t *testing.T, handler http.HandlerFunc, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &body)
	request = request.WithContext(contextWithAppUser(request.Context(), auth.UserInfo{ID: 7, Phone: "13800000000"}))
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func decodeAppBillingResponse(t *testing.T, response *httptest.ResponseRecorder) struct {
	Code    int          `json:"code"`
	Data    appOrderResp `json:"data"`
	Message string       `json:"message"`
} {
	t.Helper()
	var body struct {
		Code    int          `json:"code"`
		Data    appOrderResp `json:"data"`
		Message string       `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

const appBillingTestDriverName = "app_billing_test"

var registerAppBillingTestDriverOnce sync.Once
var appBillingInsertCount atomic.Int64

func registerAppBillingTestDriver() {
	registerAppBillingTestDriverOnce.Do(func() {
		sql.Register(appBillingTestDriverName, appBillingTestDriver{})
	})
}

type appBillingTestDriver struct{}

func (appBillingTestDriver) Open(memberLevel string) (driver.Conn, error) {
	return &appBillingTestConn{memberLevel: memberLevel}, nil
}

type appBillingTestConn struct {
	memberLevel string
	expired     bool
}

func (c *appBillingTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *appBillingTestConn) Close() error                        { return nil }
func (c *appBillingTestConn) Begin() (driver.Tx, error)           { return appBillingTestTx{}, nil }

func (c *appBillingTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return appBillingTestTx{}, nil
}

func (c *appBillingTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "UPDATE app_orders") && strings.Contains(query, "INTERVAL '15 minutes'") {
		if strings.HasPrefix(c.memberLevel, "expired_pending|") {
			c.expired = true
			return driver.RowsAffected(1), nil
		}
		return driver.RowsAffected(0), nil
	}
	if strings.Contains(query, "INSERT INTO app_orders") && strings.Contains(query, "pending_confirmation") {
		appBillingInsertCount.Add(1)
		return driver.RowsAffected(1), nil
	}
	return nil, driver.ErrSkip
}

func (c *appBillingTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "FROM site_configs") {
		return &appBillingTestRows{columns: []string{"config"}}, nil
	}
	if strings.Contains(query, "SELECT member_expires_at FROM app_users") {
		level := strings.TrimPrefix(c.memberLevel, "pending|")
		level = strings.TrimPrefix(level, "expired_pending|")
		var expiresAt driver.Value
		if strings.HasPrefix(level, "active:") {
			expiresAt = time.Now().Add(30 * 24 * time.Hour)
		} else if strings.HasPrefix(level, "expired:") {
			expiresAt = time.Now().Add(-time.Hour)
		}
		return &appBillingTestRows{
			columns: []string{"member_expires_at"},
			values:  [][]driver.Value{{expiresAt}},
		}, nil
	}
	if strings.Contains(query, "FROM app_users") {
		level := c.memberLevel
		level = strings.TrimPrefix(level, "pending|")
		level = strings.TrimPrefix(level, "expired_pending|")
		level = strings.TrimPrefix(level, "stale_pending|")
		var startedAt driver.Value
		var expiresAt driver.Value
		now := time.Now()
		if strings.HasPrefix(level, "active:") {
			level = strings.TrimPrefix(level, "active:")
			startedAt = now.Add(-24 * time.Hour)
			expiresAt = now.Add(30 * 24 * time.Hour)
		} else if strings.HasPrefix(level, "expired:") {
			level = strings.TrimPrefix(level, "expired:")
			startedAt = now.Add(-31 * 24 * time.Hour)
			expiresAt = now.Add(-time.Hour)
		}
		return &appBillingTestRows{
			columns: []string{"member_level", "member_started_at", "member_expires_at"},
			values:  [][]driver.Value{{level, startedAt, expiresAt}},
		}, nil
	}
	if strings.Contains(query, "FROM app_user_cards") {
		return &appBillingTestRows{
			columns: []string{"count"},
			values:  [][]driver.Value{{int64(0)}},
		}, nil
	}
	if strings.Contains(query, "FROM app_orders") {
		if strings.Contains(query, "status='pending_confirmation'") && c.expired {
			return &appBillingTestRows{columns: []string{"out_trade_no", "product_id", "title", "amount", "status"}}, nil
		}
		if strings.Contains(query, "status='pending_confirmation'") &&
			strings.HasPrefix(c.memberLevel, "stale_pending|") &&
			strings.Contains(query, "NOT EXISTS") {
			return &appBillingTestRows{columns: []string{"out_trade_no", "product_id", "title", "amount", "status"}}, nil
		}
		if strings.Contains(query, "status='pending_confirmation'") &&
			!strings.HasPrefix(c.memberLevel, "pending|") &&
			!strings.HasPrefix(c.memberLevel, "expired_pending|") &&
			!strings.HasPrefix(c.memberLevel, "stale_pending|") {
			return &appBillingTestRows{columns: []string{"out_trade_no", "product_id", "title", "amount", "status"}}, nil
		}
		outTradeNo := "app7-vip_month-1"
		if strings.Contains(query, "status='pending_confirmation'") {
			outTradeNo = "app7-vip_month-existing"
		}
		if len(args) >= 2 {
			outTradeNo, _ = args[1].Value.(string)
		}
		status := "pending_confirmation"
		if c.expired {
			status = "closed"
		}
		return &appBillingTestRows{
			columns: []string{"out_trade_no", "product_id", "title", "amount", "status"},
			values:  [][]driver.Value{{outTradeNo, "vip_month", "月卡会员", int64(2900), status}},
		}, nil
	}
	return nil, driver.ErrSkip
}

type appBillingTestTx struct{}

func (appBillingTestTx) Commit() error   { return nil }
func (appBillingTestTx) Rollback() error { return nil }

type appBillingTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *appBillingTestRows) Columns() []string {
	return r.columns
}

func (r *appBillingTestRows) Close() error {
	return nil
}

func (r *appBillingTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
