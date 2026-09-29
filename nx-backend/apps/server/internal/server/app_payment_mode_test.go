package server

import (
	"net/http/httptest"
	"testing"
)

func TestAppBillingSurfaceUsesDedicatedRoutes(t *testing.T) {
	for _, tt := range []struct {
		path   string
		header string
		want   string
	}{
		{path: "/api/app/web/billing/products", want: "web"},
		{path: "/api/app/native/billing/products", want: "native"},
		{path: "/api/app/billing/products", header: "native", want: "legacy"},
		{path: "/api/app/billing/products", header: "web", want: "legacy"},
		{path: "/api/app/billing/products?surface=native", want: "legacy"},
		{path: "/unregistered/web/billing/products", want: "legacy"},
		{path: "/api/app/billing/products", want: "legacy"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			r := httptest.NewRequest("GET", tt.path, nil)
			if tt.header != "" {
				r.Header.Set("X-Nine-Xing-Billing-Surface", tt.header)
			}
			if got := appBillingSurface(r); got != tt.want {
				t.Fatalf("appBillingSurface(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestAppBillingModeForSurfaceKeepsAppAndWebIndependent(t *testing.T) {
	if got := appBillingModeForSurface("web", appPurchaseModeCustomerService); got != appPurchaseModeXZN {
		t.Fatalf("web billing mode = %q, want xzn", got)
	}
	if got := appBillingModeForSurface("native", appPurchaseModeXZN); got != appPurchaseModeCustomerService {
		t.Fatalf("native billing mode = %q, want customer_service", got)
	}
	if got := appBillingModeForSurface("legacy", appPurchaseModeXZN); got != appPurchaseModeXZN {
		t.Fatalf("legacy billing mode = %q, want configured mode", got)
	}
}

func TestNormalizeAppPaymentMode(t *testing.T) {
	for _, tt := range []struct{ raw, want string }{
		{"", appPurchaseModeCustomerService},
		{"customer_service", appPurchaseModeCustomerService},
		{" XZN ", appPurchaseModeXZN},
	} {
		got, err := normalizeAppPaymentMode(tt.raw)
		if err != nil || got != tt.want {
			t.Fatalf("normalize %q = %q, %v", tt.raw, got, err)
		}
	}
	if _, err := normalizeAppPaymentMode("unknown"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestAppProductForPaymentModeUsesSelectedMode(t *testing.T) {
	base := appProductResp{ID: "vip_month", Enabled: true}
	customer := appProductForPaymentMode(appPurchaseModeCustomerService, xznPaymentConfig{}, base)
	if customer.PurchaseMode != appPurchaseModeCustomerService || customer.PayEnabled || len(customer.PaymentChannels) != 0 {
		t.Fatalf("unexpected customer-service product: %+v", customer)
	}

	xzn := appProductForPaymentMode(appPurchaseModeXZN, xznPaymentConfig{
		PID: "p", Secret: "s", NotifyURL: "https://example.test/notify",
		Enabled: true, AlipayEnabled: true, AlipayGatewayID: "34",
	}, base)
	if xzn.PurchaseMode != appPurchaseModeXZN || !xzn.PayEnabled || len(xzn.PaymentChannels) != 2 {
		t.Fatalf("unexpected xzn product: %+v", xzn)
	}
}

func TestXZNModeNeverFallsBackToCustomerService(t *testing.T) {
	product := appProductForPaymentMode(appPurchaseModeXZN, xznPaymentConfig{}, appProductResp{ID: "vip_month", Enabled: true})
	if product.PurchaseMode != appPurchaseModeXZN || product.PayEnabled {
		t.Fatalf("unconfigured xzn mode must stay disabled xzn, got %+v", product)
	}
	if product.ConfigurationStatus != "payment_not_configured" {
		t.Fatalf("unexpected configuration status: %+v", product)
	}
}

func TestResolveAppOrderPurchaseModeKeepsStoredMode(t *testing.T) {
	if got := resolveAppOrderPurchaseMode(appPurchaseModeCustomerService, appPaymentProviderXZN); got != appPurchaseModeCustomerService {
		t.Fatalf("stored customer mode changed to %q", got)
	}
	if got := resolveAppOrderPurchaseMode(appPurchaseModeXZN, "manual"); got != appPurchaseModeXZN {
		t.Fatalf("stored xzn mode changed to %q", got)
	}
	if got := resolveAppOrderPurchaseMode("", appPaymentProviderXZN); got != appPurchaseModeXZN {
		t.Fatalf("legacy xzn order resolved to %q", got)
	}
	if got := resolveAppOrderPurchaseMode("", "manual"); got != appPurchaseModeCustomerService {
		t.Fatalf("legacy manual order resolved to %q", got)
	}
}

func TestAppPaymentModeXZNRequiresConfiguration(t *testing.T) {
	if appPaymentModeCanActivate(appPurchaseModeXZN, xznPaymentConfig{}) {
		t.Fatal("xzn must not activate without merchant configuration")
	}
	if !appPaymentModeCanActivate(appPurchaseModeXZN, xznPaymentConfig{PID: "p", Secret: "s", NotifyURL: "https://example.test/notify"}) {
		t.Fatal("configured xzn should activate")
	}
}
