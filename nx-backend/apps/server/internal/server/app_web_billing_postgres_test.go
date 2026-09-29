package server

import (
	"bytes"
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/testdb"
	"nine-xing/nx-backend/apps/server/internal/xznpay"
)

func TestWebBillingXZNPostgresIsolatedFromApp(t *testing.T) {
	db, _ := testdb.OpenEnvIsolatedSchema(t, "web_billing")
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_users(id,phone) VALUES(9201,'web-test-9201'),(9202,'web-test-9202'),(9203,'web-test-9203'),(9204,'web-test-9204')`); err != nil {
		t.Fatal(err)
	}
	const secret = "isolated-web-payment-test-secret"
	requests := make(chan url.Values, 10)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || !xznpay.VerifyMD5(r.Form, secret, r.Form.Get("sign")) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/openapi/pay/create":
			requests <- r.Form
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "data": map[string]any{
				"trade_no": "web-provider-" + r.Form.Get("out_trade_no"), "out_trade_no": r.Form.Get("out_trade_no"),
				"pay_url": "https://cashier.example.test/pay/" + r.Form.Get("out_trade_no"),
			}})
		case "/openapi/pay/query":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "data": map[string]any{
				"trade_no": "web-provider-" + r.Form.Get("out_trade_no"), "out_trade_no": r.Form.Get("out_trade_no"),
				"total_amount": "29.00", "trade_status": "WAIT_BUYER_PAY",
			}})
		default:
			http.Error(w, "unexpected gateway path", http.StatusBadRequest)
		}
	}))
	t.Cleanup(gateway.Close)
	s := &Server{db: db, appUsers: appuser.NewStore(db), env: config.Env{AppEnv: "production"}}
	request := func(userID int64, method string, handler http.HandlerFunc, path string, payload any) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(payload)
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r = r.WithContext(contextWithAppUser(r.Context(), auth.UserInfo{ID: userID}))
		response := httptest.NewRecorder()
		handler(response, r)
		return response
	}
	cfg := xznPaymentConfig{
		PID: "isolated-web-merchant", Secret: secret, BaseURL: gateway.URL + "/openapi", SignType: "MD5",
		NotifyURL: "https://api.example.test/api/xzn-pay/notify", ReturnURL: "ninexing://billing/result",
		WebReturnURL: "https://web.example.test/h5/index.html#/billing?payment_return=1",
		Enabled:      true, AlipayEnabled: true, AlipayGatewayID: "34",
	}
	configured := request(9201, http.MethodPut, s.xznPayConfig, "/api/admin/xzn-pay/config", cfg)
	if configured.Code != http.StatusOK {
		t.Fatalf("configure test gateway: %d %s", configured.Code, configured.Body.String())
	}
	t.Run("legacy config preserves Web return but explicit empty clears it", func(t *testing.T) {
		encoded, _ := json.Marshal(cfg)
		var legacy map[string]any
		_ = json.Unmarshal(encoded, &legacy)
		delete(legacy, "webReturnURL")
		legacy["secret"] = ""
		if response := request(9201, http.MethodPut, s.xznPayConfig, "/api/admin/xzn-pay/config", legacy); response.Code != http.StatusOK {
			t.Fatalf("legacy config save: %d %s", response.Code, response.Body.String())
		}
		stored, err := s.loadXZNConfig(context.Background())
		if err != nil || stored.WebReturnURL != cfg.WebReturnURL || stored.ReturnURL != cfg.ReturnURL {
			t.Fatalf("legacy payload changed Web/native return: web=%q native=%q error=%v", stored.WebReturnURL, stored.ReturnURL, err)
		}
		legacy["webReturnURL"] = ""
		if response := request(9201, http.MethodPut, s.xznPayConfig, "/api/admin/xzn-pay/config", legacy); response.Code != http.StatusOK {
			t.Fatalf("clear Web return: %d %s", response.Code, response.Body.String())
		}
		stored, err = s.loadXZNConfig(context.Background())
		if err != nil || stored.WebReturnURL != "" || stored.ReturnURL != cfg.ReturnURL {
			t.Fatalf("explicit empty did not clear only Web return: web=%q native=%q error=%v", stored.WebReturnURL, stored.ReturnURL, err)
		}
		if restored := request(9201, http.MethodPut, s.xznPayConfig, "/api/admin/xzn-pay/config", cfg); restored.Code != http.StatusOK {
			t.Fatal("restore test config")
		}
	})

	t.Run("web products do not depend on malformed App mode", func(t *testing.T) {
		if _, err := db.Exec(`INSERT INTO site_configs(key,config) VALUES('app_payment_mode','{"mode":"broken"}'::jsonb)`); err != nil {
			t.Fatal(err)
		}
		response := request(9201, http.MethodGet, s.appBillingProducts, "/api/app/web/billing/products", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("Web inherited broken App mode: %d %s", response.Code, response.Body.String())
		}
		var body struct {
			Data []appProductResp `json:"data"`
		}
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		for _, product := range body.Data {
			if product.Enabled && (product.PurchaseMode != appPurchaseModeXZN || !product.PayEnabled) {
				t.Fatalf("Web XZN unavailable: %+v", product)
			}
		}
		if _, err := db.Exec(`DELETE FROM site_configs WHERE key='app_payment_mode'`); err != nil {
			t.Fatal(err)
		}
	})

	webCreated := request(9201, http.MethodPost, s.appBillingCreateOrder, "/api/app/web/billing/orders", map[string]any{
		"productId": "vip_month", "payChannel": "alipay", "expectedAmountCents": 2900, "webReturnUrl": cfg.WebReturnURL,
	})
	if webCreated.Code != http.StatusOK {
		t.Fatalf("Web checkout: %d %s", webCreated.Code, webCreated.Body.String())
	}
	order := decodeAppBillingResponse(t, webCreated).Data
	if order.PurchaseMode != appPurchaseModeXZN || !order.PayEnabled || order.CustomerServiceQRURL != "" {
		t.Fatalf("not a Web XZN order: %+v", order)
	}
	var persistedReturn string
	if err := db.QueryRow(`SELECT web_return_url FROM app_orders WHERE out_trade_no=$1`, order.OutTradeNo).Scan(&persistedReturn); err != nil {
		t.Fatal(err)
	}
	providerRequest := <-requests
	if persistedReturn != providerRequest.Get("return_url") || order.Payment["returnUrl"] != persistedReturn || strings.HasPrefix(persistedReturn, "ninexing:") {
		t.Fatalf("Web callback not persisted consistently: db=%q response=%v gateway=%q", persistedReturn, order.Payment["returnUrl"], providerRequest.Get("return_url"))
	}
	parsedReturn, _ := url.Parse(persistedReturn)
	fragment, _ := url.Parse(parsedReturn.Fragment)
	if parsedReturn.RawQuery != "" || fragment.Query().Get("outTradeNo") != order.OutTradeNo || fragment.Query().Get("payment_return") != "1" {
		t.Fatalf("Flutter hash callback params missing: %q", persistedReturn)
	}

	t.Run("native cannot start or launch across pending Web checkout", func(t *testing.T) {
		duplicate := request(9201, http.MethodPost, s.appBillingCreateOrder, "/api/app/native/billing/orders", map[string]any{"productId": "vip_month"})
		if duplicate.Code != http.StatusConflict {
			t.Fatalf("native duplicate checkout allowed: %d %s", duplicate.Code, duplicate.Body.String())
		}
		status := request(9201, http.MethodGet, s.appBillingOrderStatus, "/api/app/native/billing/orders/status?outTradeNo="+order.OutTradeNo, nil)
		other := decodeAppBillingResponse(t, status).Data
		if status.Code != http.StatusOK || other.PayEnabled || other.Payment != nil || other.PayURL != "" || other.ConfigurationStatus != "checkout_surface_mismatch" {
			t.Fatalf("native received Web checkout launcher: %d %+v", status.Code, other)
		}
	})

	t.Run("verified return survives later configuration change", func(t *testing.T) {
		if _, err := db.Exec(`UPDATE site_configs SET config=jsonb_set(config,'{webReturnURL}','"https://new.example.test/#/billing"'::jsonb) WHERE key='xzn_payment'`); err != nil {
			t.Fatal(err)
		}
		status := request(9201, http.MethodGet, s.appBillingOrderStatus, "/api/app/web/billing/orders/status?outTradeNo="+order.OutTradeNo, nil)
		if status.Code != http.StatusOK || decodeAppBillingResponse(t, status).Data.Payment["returnUrl"] != persistedReturn {
			t.Fatalf("stored callback was changed: %d %s", status.Code, status.Body.String())
		}
		if restored := request(9201, http.MethodPut, s.xznPayConfig, "/api/admin/xzn-pay/config", cfg); restored.Code != http.StatusOK {
			t.Fatal("restore test config")
		}
	})

	t.Run("signed callback grants membership exactly once", func(t *testing.T) {
		callback := url.Values{
			"pid": {cfg.PID}, "trade_no": {"web-provider-" + order.OutTradeNo}, "out_trade_no": {order.OutTradeNo},
			"total_amount": {"29.00"}, "subject": {order.Title}, "paytype_code": {"alipay"}, "channel_id": {"34"}, "trade_status": {"TRADE_SUCCESS"}, "sign_type": {"MD5"},
		}
		signWebBillingCallback(callback, secret)
		var firstExpiry time.Time
		for i := 0; i < 2; i++ {
			r := httptest.NewRequest(http.MethodPost, "/api/xzn-pay/notify", strings.NewReader(callback.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			s.xznPayNotify(response, r)
			if response.Code != http.StatusOK || response.Body.String() != "success" {
				t.Fatalf("callback failed: %d %s", response.Code, response.Body.String())
			}
			var level, status string
			var expiry time.Time
			if err := db.QueryRow(`SELECT u.member_level,u.member_expires_at,o.status FROM app_users u JOIN app_orders o ON o.app_user_id=u.id WHERE o.out_trade_no=$1`, order.OutTradeNo).Scan(&level, &expiry, &status); err != nil {
				t.Fatal(err)
			}
			if level != "vip_month" || status != "paid" || !expiry.After(time.Now()) || (i > 0 && !expiry.Equal(firstExpiry)) {
				t.Fatalf("callback grant not idempotent: %s %s %s", level, status, expiry)
			}
			firstExpiry = expiry
		}
		status := request(9201, http.MethodGet, s.appBillingOrderStatus, "/api/app/native/billing/orders/status?outTradeNo="+order.OutTradeNo, nil)
		completed := decodeAppBillingResponse(t, status).Data
		if completed.Status != "paid" || completed.Message != "支付成功，会员已开通" || completed.PayEnabled || completed.Payment != nil {
			t.Fatalf("completed Web order is not readable on App: %+v", completed)
		}
	})

	t.Run("App remains manual and pending is visible but not launchable in Web", func(t *testing.T) {
		nativeCreated := request(9202, http.MethodPost, s.appBillingCreateOrder, "/api/app/native/billing/orders", map[string]any{"productId": "vip_month"})
		manual := decodeAppBillingResponse(t, nativeCreated).Data
		if nativeCreated.Code != http.StatusOK || manual.PurchaseMode != appPurchaseModeCustomerService || manual.CustomerServiceQRURL == "" || manual.Status != appOrderPendingConfirmation {
			t.Fatalf("H5 configuration changed App checkout: %d %+v", nativeCreated.Code, manual)
		}
		webDuplicate := request(9202, http.MethodPost, s.appBillingCreateOrder, "/api/app/web/billing/orders", map[string]any{"productId": "vip_month", "payChannel": "alipay", "webReturnUrl": cfg.WebReturnURL})
		if webDuplicate.Code != http.StatusConflict {
			t.Fatalf("Web ignored manual pending order: %d %s", webDuplicate.Code, webDuplicate.Body.String())
		}
		webStatus := request(9202, http.MethodGet, s.appBillingOrderStatus, "/api/app/web/billing/orders/status?outTradeNo="+manual.OutTradeNo, nil)
		other := decodeAppBillingResponse(t, webStatus).Data
		if webStatus.Code != http.StatusOK || other.OutTradeNo != manual.OutTradeNo || other.CustomerServiceQRURL != "" || other.ConfigurationStatus != "checkout_surface_mismatch" {
			t.Fatalf("Web received manual launcher or lost pending: %d %+v", webStatus.Code, other)
		}
		canceled := request(9202, http.MethodPost, s.appBillingCancelOrder, "/api/app/web/billing/orders/cancel", map[string]any{"outTradeNo": manual.OutTradeNo})
		if canceled.Code != http.StatusOK || decodeAppBillingResponse(t, canceled).Data.Status != "closed" {
			t.Fatalf("cannot cancel cross-surface pending: %d %s", canceled.Code, canceled.Body.String())
		}
		if decodeAppBillingResponse(t, canceled).Data.Message == appCrossSurfacePendingMessage {
			t.Fatal("canceled order must not keep pending instructions")
		}
	})

	t.Run("expired pending is closed before replacement", func(t *testing.T) {
		if _, err := db.Exec(`INSERT INTO app_orders(out_trade_no,app_user_id,product_id,title,amount,duration_days,status,purchase_mode,payment_provider,create_time) VALUES('stale-manual-web',9203,'vip_month','VIP 月卡',2900,30,'pending_confirmation','customer_service','manual',now()-interval '20 minutes')`); err != nil {
			t.Fatal(err)
		}
		created := request(9203, http.MethodPost, s.appBillingCreateOrder, "/api/app/web/billing/orders", map[string]any{"productId": "vip_month", "payChannel": "alipay", "webReturnUrl": cfg.WebReturnURL})
		if created.Code != http.StatusOK || decodeAppBillingResponse(t, created).Data.PurchaseMode != appPurchaseModeXZN {
			t.Fatalf("expired cross-surface pending blocked checkout: %d %s", created.Code, created.Body.String())
		}
		var status string
		if err := db.QueryRow(`SELECT status FROM app_orders WHERE out_trade_no='stale-manual-web'`).Scan(&status); err != nil || status != "closed" {
			t.Fatalf("stale order not closed: %q %v", status, err)
		}
	})

	t.Run("concurrent native and Web reservation creates one pending", func(t *testing.T) {
		testCrossSurfaceReservation(t, s, db, 9204)
	})
}

func signWebBillingCallback(values url.Values, secret string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key != "sign" && key != "sign_type" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	digest := md5.Sum([]byte(strings.Join(parts, "&") + "&key=" + secret))
	values.Set("sign", strings.ToUpper(hex.EncodeToString(digest[:])))
}

func testCrossSurfaceReservation(t *testing.T, s *Server, db *sql.DB, userID int64) {
	t.Helper()
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, provider := range []string{"manual", "xzn"} {
		wg.Add(1)
		go func(provider string) {
			defer wg.Done()
			<-start
			status, mode := appOrderPendingConfirmation, appPurchaseModeCustomerService
			if provider == "xzn" {
				status, mode = "pending", appPurchaseModeXZN
			}
			results <- s.reserveAppBillingOrder(context.Background(), userID, `INSERT INTO app_orders(out_trade_no,app_user_id,product_id,title,amount,duration_days,status,purchase_mode,payment_provider) VALUES($1,$2,'vip_month','VIP 月卡',2900,30,$3,$4,$5)`, "concurrent-"+provider, userID, status, mode, provider)
		}(provider)
	}
	close(start)
	wg.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if errors.Is(err, errAppOrderAlreadyPending) {
			conflicted++
		} else {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app_orders WHERE app_user_id=$1`, userID).Scan(&count); err != nil || count != 1 || succeeded != 1 || conflicted != 1 {
		t.Fatalf("cross-surface race: rows=%d succeeded=%d conflicts=%d error=%v", count, succeeded, conflicted, err)
	}
}
