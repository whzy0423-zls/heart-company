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

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
)

func TestAppBillingCreateOrderReturnsNotConfiguredPayment(t *testing.T) {
	s := newAppBillingTestServer(t)

	response := performAppBillingRequest(t, s.appBillingCreateOrder, http.MethodPost, "/api/app/billing/orders", map[string]any{
		"productId": "vip_month",
	})

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	body := decodeAppBillingResponse(t, response)
	if body.Data.PayStatus != "not_configured" {
		t.Fatalf("expected payStatus not_configured, got %+v", body.Data)
	}
	if body.Data.Status != "not_configured" {
		t.Fatalf("expected placeholder order status not_configured, got %+v", body.Data)
	}
	if body.Data.PayEnabled {
		t.Fatalf("expected payment to be disabled until configured, got %+v", body.Data)
	}
	if body.Data.ConfigurationStatus != "payment_not_configured" {
		t.Fatalf("expected payment_not_configured configuration status, got %+v", body.Data)
	}
	if body.Data.DisabledReason == "" {
		t.Fatalf("expected disabled reason, got %+v", body.Data)
	}
	if !strings.Contains(body.Data.Message, "不会扣款") {
		t.Fatalf("expected explicit no-charge message, got %q", body.Data.Message)
	}
}

func TestAppBillingProductsExposePaymentPlaceholder(t *testing.T) {
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
	if len(body.Data) == 0 {
		t.Fatal("expected products")
	}
	for _, product := range body.Data {
		if product.Enabled || product.PayEnabled {
			t.Fatalf("expected product disabled while payment is not configured, got %+v", product)
		}
		if product.ConfigurationStatus != "payment_not_configured" {
			t.Fatalf("expected payment_not_configured configuration status, got %+v", product)
		}
		if product.DisabledReason == "" {
			t.Fatalf("expected disabled reason, got %+v", product)
		}
	}
}

func TestAppBillingOrderStatusKeepsPaymentNotConfigured(t *testing.T) {
	s := newAppBillingTestServer(t)

	response := performAppBillingRequest(t, s.appBillingOrderStatus, http.MethodGet, "/api/app/billing/orders/status?outTradeNo=app7-vip_month-1", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	body := decodeAppBillingResponse(t, response)
	if body.Data.Status != "pending" {
		t.Fatalf("expected stored order status pending from fixture, got %+v", body.Data)
	}
	if body.Data.PayStatus != "not_configured" {
		t.Fatalf("expected payStatus not_configured until payment is configured, got %+v", body.Data)
	}
	if body.Data.PayEnabled {
		t.Fatalf("expected payment disabled until configured, got %+v", body.Data)
	}
	if body.Data.ConfigurationStatus != "payment_not_configured" {
		t.Fatalf("expected payment_not_configured configuration status, got %+v", body.Data)
	}
	if !strings.Contains(body.Data.Message, "不会扣款") {
		t.Fatalf("expected explicit no-charge message, got %q", body.Data.Message)
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

func (appBillingTestDriver) Open(string) (driver.Conn, error) {
	return &appBillingTestConn{}, nil
}

type appBillingTestConn struct{}

func (c *appBillingTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *appBillingTestConn) Close() error                        { return nil }
func (c *appBillingTestConn) Begin() (driver.Tx, error)           { return appBillingTestTx{}, nil }

func (c *appBillingTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return appBillingTestTx{}, nil
}

func (c *appBillingTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO app_orders") && strings.Contains(query, "not_configured") {
		return driver.RowsAffected(1), nil
	}
	return nil, driver.ErrSkip
}

func (c *appBillingTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "FROM app_orders") {
		outTradeNo := "app7-vip_month-1"
		if len(args) >= 2 {
			outTradeNo, _ = args[1].Value.(string)
		}
		return &appBillingTestRows{
			columns: []string{"out_trade_no", "product_id", "title", "amount", "status"},
			values:  [][]driver.Value{{outTradeNo, "vip_month", "月卡会员", int64(2900), "pending"}},
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
