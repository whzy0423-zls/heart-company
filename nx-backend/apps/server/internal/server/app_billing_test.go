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
		{name: "legacy svip becomes year", memberLevel: "svip", want: "vip_year"},
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
		{planCode: "vip_month", want: "月卡会员"},
		{planCode: "vip_quarter", want: "季卡会员"},
		{planCode: "vip_year", want: "年卡会员"},
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
	if body.Data.PlanCode != "vip_month" || body.Data.PlanName != "月卡会员" {
		t.Fatalf("expected normalized monthly plan, got %+v", body.Data)
	}
	if !body.Data.IsMember {
		t.Fatalf("expected legacy vip to remain a member, got %+v", body.Data)
	}
	if body.Data.ChatRemaining != 0 || body.Data.DeepReportRemaining != 0 {
		t.Fatalf("expected numeric quota placeholders to remain zero, got %+v", body.Data)
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

func TestAppBillingCreateOrderReturnsPendingCustomerConfirmation(t *testing.T) {
	s := newAppBillingTestServer(t)

	response := performAppBillingRequest(t, s.appBillingCreateOrder, http.MethodPost, "/api/app/billing/orders", map[string]any{
		"productId": "vip_month",
	})

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	body := decodeAppBillingResponse(t, response)
	if body.Data.PayStatus != "pending_confirmation" {
		t.Fatalf("expected payStatus pending_confirmation, got %+v", body.Data)
	}
	if body.Data.Status != "pending_confirmation" {
		t.Fatalf("expected order status pending_confirmation, got %+v", body.Data)
	}
	if body.Data.PayEnabled {
		t.Fatalf("expected SDK payment to stay disabled, got %+v", body.Data)
	}
	if body.Data.PurchaseMode != "customer_service" {
		t.Fatalf("expected customer service purchase mode, got %+v", body.Data)
	}
	if body.Data.CustomerServiceQRURL != "/api/public/customer-service-qr" {
		t.Fatalf("expected public customer QR endpoint, got %+v", body.Data)
	}
	if !strings.Contains(body.Data.Message, "客服") {
		t.Fatalf("expected customer confirmation message, got %q", body.Data.Message)
	}
}

func TestAppBillingProductsExposeThreeCustomerServicePlans(t *testing.T) {
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
	if len(body.Data) != 3 {
		t.Fatalf("expected three membership products, got %+v", body.Data)
	}
	for _, product := range body.Data {
		if !product.Enabled || product.PayEnabled {
			t.Fatalf("expected enabled manual product without SDK payment, got %+v", product)
		}
		if product.PurchaseMode != "customer_service" {
			t.Fatalf("expected customer service mode, got %+v", product)
		}
		if product.ID == "deep_report" {
			t.Fatalf("deep report must not be offered: %+v", product)
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
}

func (c *appBillingTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *appBillingTestConn) Close() error                        { return nil }
func (c *appBillingTestConn) Begin() (driver.Tx, error)           { return appBillingTestTx{}, nil }

func (c *appBillingTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return appBillingTestTx{}, nil
}

func (c *appBillingTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO app_orders") && strings.Contains(query, "pending_confirmation") {
		return driver.RowsAffected(1), nil
	}
	return nil, driver.ErrSkip
}

func (c *appBillingTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "FROM app_users") {
		level := c.memberLevel
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
		outTradeNo := "app7-vip_month-1"
		if len(args) >= 2 {
			outTradeNo, _ = args[1].Value.(string)
		}
		return &appBillingTestRows{
			columns: []string{"out_trade_no", "product_id", "title", "amount", "status"},
			values:  [][]driver.Value{{outTradeNo, "vip_month", "月卡会员", int64(2900), "pending_confirmation"}},
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
