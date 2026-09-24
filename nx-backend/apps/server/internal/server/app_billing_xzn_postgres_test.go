package server

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/testdb"
	"nine-xing/nx-backend/apps/server/internal/xznpay"
)

func TestAppBillingXZNModeAlipayPostgres(t *testing.T) {
	db, _ := testdb.OpenEnvIsolatedSchema(t, "billing_xzn")
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO app_users(id,phone) VALUES(9101,'xzn-agent-9101'),(9102,'xzn-buyer-9102');
		INSERT INTO distribution_agents(id,app_user_id,agent_code,level,root_agent_id,agent_path)
		VALUES(9101,9101,'A9101',1,9101,'/9101/');
		INSERT INTO app_agent_discount_rules(product_id,audience,mode,value,enabled)
		VALUES('vip_month','invited_user','amount_off',500,true);
	`); err != nil {
		t.Fatal(err)
	}
	const secret = "local-xzn-test-secret"
	requests := make(chan url.Values, 2)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/pay/create" || r.Method != http.MethodPost {
			http.Error(w, "unexpected gateway request", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil || !xznpay.VerifyMD5(r.Form, secret, r.Form.Get("sign")) {
			http.Error(w, "invalid request signature", http.StatusUnauthorized)
			return
		}
		requests <- r.Form
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1,
			"data": map[string]any{
				"trade_no": "local-provider-order", "out_trade_no": r.Form.Get("out_trade_no"),
				"pay_url": "https://openapi.alipay.com/gateway.do?local-test=1",
			},
		})
	}))
	defer gateway.Close()
	s := &Server{db: db, appUsers: appuser.NewStore(db)}
	request := func(method string, handler http.HandlerFunc, path string, payload any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r = r.WithContext(contextWithAppUser(r.Context(), auth.UserInfo{ID: 9102}))
		response := httptest.NewRecorder()
		handler(response, r)
		return response
	}
	configured := request(http.MethodPut, s.xznPayConfig, "/api/xzn-pay/config", xznPaymentConfig{
		PID: "local-merchant", Secret: secret, BaseURL: gateway.URL + "/openapi", SignType: "MD5",
		NotifyURL: "https://example.test/api/xzn-pay/notify", Enabled: true,
		AlipayEnabled: true, AlipayGatewayID: "34",
	})
	if configured.Code != http.StatusOK {
		t.Fatalf("configure payment: status=%d body=%s", configured.Code, configured.Body.String())
	}
	mode := request(http.MethodPut, s.appPaymentMode, "/api/app-payment-mode", map[string]any{"mode": "xzn"})
	if mode.Code != http.StatusOK {
		t.Fatalf("switch mode: status=%d body=%s", mode.Code, mode.Body.String())
	}
	products := request(http.MethodGet, s.appBillingProducts, "/api/app/billing/products", nil)
	var productBody struct {
		Data []appProductResp `json:"data"`
	}
	if err := json.Unmarshal(products.Body.Bytes(), &productBody); err != nil || len(productBody.Data) == 0 {
		t.Fatalf("products: %v %s", err, products.Body.String())
	}
	for _, product := range productBody.Data {
		if !product.Enabled {
			continue
		}
		if product.PurchaseMode != "xzn" || !product.PayEnabled || len(product.PaymentChannels) != 2 || product.PaymentChannels[0].Code != "alipay" || !product.PaymentChannels[0].Enabled {
			t.Fatalf("Alipay not available after mode switch: %+v", product)
		}
	}
	missingChannel := request(http.MethodPost, s.appBillingCreateOrder, "/api/app/billing/orders", map[string]any{"productId": "vip_month"})
	if missingChannel.Code != http.StatusBadRequest {
		t.Fatalf("stale customer-service client should choose channel: %d %s", missingChannel.Code, missingChannel.Body.String())
	}
	created := request(http.MethodPost, s.appBillingCreateOrder, "/api/app/billing/orders", map[string]any{
		"productId": "vip_month", "payChannel": "alipay", "agentCode": "A9101", "expectedAmountCents": 2400,
	})
	if created.Code != http.StatusOK {
		t.Fatalf("Alipay checkout: status=%d body=%s", created.Code, created.Body.String())
	}
	order := decodeAppBillingResponse(t, created).Data
	if order.PurchaseMode != "xzn" || order.PayChannel != "alipay" || order.Amount != 2400 || order.BasePriceCents != 2900 || order.DiscountCents != 500 || order.CustomerServiceQRURL != "" {
		t.Fatalf("wrong Alipay price or purchase mode: %+v", order)
	}
	if !order.PayEnabled || order.Payment["url"] != "https://openapi.alipay.com/gateway.do?local-test=1" {
		t.Fatalf("cashier response not launchable: %+v", order)
	}
	select {
	case providerRequest := <-requests:
		if providerRequest.Get("total_amount") != "24.00" || providerRequest.Get("paytype_code") != "alipay" || providerRequest.Get("channel_id") != "34" || providerRequest.Get("out_trade_no") != order.OutTradeNo {
			t.Fatalf("provider checkout differs from quote: %v", providerRequest)
		}
		returned, err := url.Parse(providerRequest.Get("return_url"))
		if err != nil || returned.Scheme != "ninexing" || returned.Host != "billing" || returned.Query().Get("outTradeNo") != order.OutTradeNo {
			t.Fatalf("invalid App return URL: %s", providerRequest.Get("return_url"))
		}
	default:
		t.Fatal("checkout did not reach the local payment gateway")
	}
	callback := url.Values{
		"pid": {"local-merchant"}, "trade_no": {"local-provider-order"}, "out_trade_no": {order.OutTradeNo},
		"total_amount": {"29.00"}, "subject": {order.Title}, "paytype_code": {"alipay"},
		"channel_id": {"34"}, "trade_status": {"TRADE_SUCCESS"}, "sign_type": {"MD5"},
	}
	notify := func() *httptest.ResponseRecorder {
		keys := make([]string, 0, len(callback))
		for key := range callback {
			if key != "sign" && key != "sign_type" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, key+"="+callback.Get(key))
		}
		digest := md5.Sum([]byte(strings.Join(parts, "&") + "&key=" + secret))
		callback.Set("sign", strings.ToUpper(hex.EncodeToString(digest[:])))
		r := httptest.NewRequest(http.MethodPost, "/api/xzn-pay/notify", strings.NewReader(callback.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		s.xznPayNotify(response, r)
		return response
	}
	if mismatched := notify(); mismatched.Code == http.StatusOK {
		t.Fatalf("undiscounted amount must not activate discounted order: %s", mismatched.Body.String())
	}
	callback.Set("total_amount", "24.00")
	var originalExpiry time.Time
	for i := 0; i < 2; i++ {
		if paid := notify(); paid.Code != http.StatusOK || strings.TrimSpace(paid.Body.String()) != "success" {
			t.Fatalf("payment callback %d: %d %s", i, paid.Code, paid.Body.String())
		}
		var status, member string
		var expiry time.Time
		if err := db.QueryRow(`SELECT o.status,u.member_level,u.member_expires_at FROM app_orders o JOIN app_users u ON u.id=o.app_user_id WHERE o.out_trade_no=$1`, order.OutTradeNo).Scan(&status, &member, &expiry); err != nil {
			t.Fatal(err)
		}
		if status != "paid" || member != "vip_month" || !expiry.After(time.Now()) {
			t.Fatalf("callback did not grant membership: %s/%s/%s", status, member, expiry)
		}
		if i == 0 {
			originalExpiry = expiry
		} else if !originalExpiry.Equal(expiry) {
			t.Fatal("duplicate callback extended membership twice")
		}
	}
	if len(requests) != 0 {
		t.Fatalf("unexpected duplicate gateway requests: %d", len(requests))
	}
}
