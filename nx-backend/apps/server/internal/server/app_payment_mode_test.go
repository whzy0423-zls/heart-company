package server

import "testing"

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
