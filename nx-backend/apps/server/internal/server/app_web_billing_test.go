package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestWebBillingReturnURLIsIndependentAndUsesHashQuery(t *testing.T) {
	s := &Server{env: config.Env{AppEnv: "production"}}
	cfg := xznPaymentConfig{ReturnURL: "ninexing://billing/result", WebReturnURL: "https://h5.example.test/app/index.html#/billing?payment_return=1"}
	got, err := s.webBillingOrderReturnURL(cfg, "https://h5.example.test/app/index.html#/billing?payment_return=1", "app7-vip_month-1")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(got)
	fragment, _ := url.Parse(u.Fragment)
	if u.RawQuery != "" || fragment.Path != "/billing" || fragment.Query().Get("payment_return") != "1" || fragment.Query().Get("outTradeNo") != "app7-vip_month-1" {
		t.Fatalf("not a Flutter hash-route return: %q", got)
	}
	if got, err := s.webBillingOrderReturnURL(cfg, "", "app-2"); err != nil || !strings.Contains(got, "outTradeNo=app-2") {
		t.Fatalf("configured Web return must work without client override: %q %v", got, err)
	}
	if got := appXZNOrderReturnURL(cfg.ReturnURL, "app-3"); !strings.HasPrefix(got, "ninexing://billing/result?") {
		t.Fatalf("native return changed: %q", got)
	}
}

func TestWebBillingReturnURLRejectsUntrustedDestinations(t *testing.T) {
	s := &Server{env: config.Env{AppEnv: "production"}}
	cfg := xznPaymentConfig{WebReturnURL: "https://h5.example.test/app/index.html#/billing"}
	for _, raw := range []string{
		"https://evil.example.test/app/index.html#/billing",
		"https://h5.example.test:8443/app/index.html#/billing",
		"http://h5.example.test/app/index.html#/billing",
		"https://user:password@h5.example.test/app/index.html#/billing",
		"https://h5.example.test/app/index.html?token=secret#/billing",
		"https://h5.example.test/app/index.html#/other",
		"https://h5.example.test/app/index.html#/billing?next=https://evil.test",
		"https://h5.example.test/app/index.html#/billing?payment_return=0",
		"https://h5.example.test/app/index.html#/billing?outTradeNo=forged",
		"https://h5.example.test/app/%0aindex.html#/billing",
		"https://h5.example.test/app/index.html#/billing%3fnext=x",
		"ninexing://billing/result",
		"javascript:alert(1)",
		"//h5.example.test/app/index.html#/billing",
		"http://127.0.0.1:62654/index.html#/billing",
	} {
		t.Run(raw, func(t *testing.T) {
			if got, err := s.webBillingOrderReturnURL(cfg, raw, "app-1"); err == nil {
				t.Fatalf("untrusted return accepted: %q", got)
			}
		})
	}
}

func TestWebBillingReturnURLUsesExplicitOriginsAndLocalOnlyOutsideProduction(t *testing.T) {
	s := &Server{env: config.Env{AppEnv: "production", CORSAllowedOrigins: []string{"https://web.example.test"}}}
	if _, err := s.webBillingOrderReturnURL(xznPaymentConfig{}, "https://web.example.test/h5/#/billing", "app-1"); err != nil {
		t.Fatal(err)
	}
	s.env.CORSAllowedOrigins = []string{"*"}
	if _, err := s.webBillingOrderReturnURL(xznPaymentConfig{}, "https://evil.example.test/#/billing", "app-1"); err == nil {
		t.Fatal("wildcard CORS must not allow arbitrary payment returns")
	}
	s.env.AppEnv = "dev"
	if _, err := s.webBillingOrderReturnURL(xznPaymentConfig{}, "http://127.0.0.1:62654/index.html#/billing", "app-1"); err != nil {
		t.Fatal(err)
	}
}

func TestWebBillingRequiresReturnConfigurationInProducts(t *testing.T) {
	s := newAppBillingTestServer(t)
	s.env.AppEnv = "production"
	response := performAppBillingRequest(t, s.appBillingProducts, http.MethodGet, "/api/app/web/billing/products", nil)
	var body struct {
		Data []appProductResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(body.Data) == 0 {
		t.Fatalf("unexpected products response: %d %s", response.Code, response.Body.String())
	}
	for _, product := range body.Data {
		if product.Enabled && (product.PurchaseMode != appPurchaseModeXZN || product.PayEnabled || product.DisabledReason == "") {
			t.Fatalf("Web products must fail closed with explicit reason: %+v", product)
		}
	}
}

func TestWebBillingModeDoesNotReadLegacySetting(t *testing.T) {
	s := &Server{}
	for _, surface := range []string{appBillingSurfaceWeb, appBillingSurfaceNative} {
		mode, err := s.appBillingMode(context.Background(), surface)
		if err != nil || mode != appBillingModeForSurface(surface, "") {
			t.Fatalf("dedicated mode = %q %v", mode, err)
		}
	}
}

func TestAppOrderCheckoutCompatibilityDoesNotReuseAnotherSurface(t *testing.T) {
	manual := appOrderResp{PurchaseMode: appPurchaseModeCustomerService, PaymentProvider: "manual"}
	web := appOrderResp{PurchaseMode: appPurchaseModeXZN, PaymentProvider: appPaymentProviderXZN, WebReturnURL: "https://web.example.test/#/billing?outTradeNo=web1&payment_return=1"}
	native := appOrderResp{PurchaseMode: appPurchaseModeXZN, PaymentProvider: appPaymentProviderXZN}
	for _, tt := range []struct {
		surface, mode string
		order         appOrderResp
		want          bool
	}{
		{appBillingSurfaceLegacy, appPurchaseModeCustomerService, manual, true},
		{appBillingSurfaceWeb, appPurchaseModeXZN, manual, false},
		{appBillingSurfaceWeb, appPurchaseModeXZN, web, true},
		{appBillingSurfaceWeb, appPurchaseModeXZN, native, false},
		{appBillingSurfaceLegacy, appPurchaseModeCustomerService, web, false},
		{appBillingSurfaceLegacy, appPurchaseModeXZN, web, false},
	} {
		if got := appOrderCheckoutCompatible(tt.surface, tt.mode, tt.order); got != tt.want {
			t.Errorf("surface=%s mode=%s order=%+v compatibility=%v want=%v", tt.surface, tt.mode, tt.order, got, tt.want)
		}
	}
}

func TestWebBillingDoesNotReusePendingCustomerServiceOrder(t *testing.T) {
	appBillingInsertCount.Store(0)
	s := newAppBillingEntitlementTestServer(t, "pending|active:vip_month")
	response := performAppBillingRequest(t, s.appBillingCreateOrder, http.MethodPost, "/api/app/web/billing/orders", map[string]any{"productId": "vip_month", "payChannel": "alipay"})
	if response.Code != http.StatusConflict || appBillingInsertCount.Load() != 0 {
		t.Fatalf("cross-surface pending must block duplicate checkout: %d %s", response.Code, response.Body.String())
	}
}
