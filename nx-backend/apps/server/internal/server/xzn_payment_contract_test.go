package server

import (
	"net/url"
	"testing"
)

func TestAppPaymentChannelsExposeProviderIndependentCodes(t *testing.T) {
	cfg := xznPaymentConfig{
		PID:             "merchant-1",
		Secret:          "secret",
		NotifyURL:       "https://api.example.com/api/admin/xzn-pay/notify",
		Enabled:         true,
		AlipayEnabled:   true,
		AlipayGatewayID: "34",
		WechatEnabled:   true,
		WechatGatewayID: "99",
	}
	channels := appPaymentChannelsForConfig(cfg)
	if len(channels) != 2 {
		t.Fatalf("expected two payment channels, got %+v", channels)
	}
	if channels[0].Code != "alipay" || channels[0].Name != "支付宝" || !channels[0].Enabled {
		t.Fatalf("unexpected alipay option: %+v", channels[0])
	}
	if channels[1].Code != "wechat" || channels[1].Name != "微信支付" || !channels[1].Enabled {
		t.Fatalf("unexpected wechat option: %+v", channels[1])
	}
	if got := displayAppPayChannel("wxpay"); got != "wechat" {
		t.Fatalf("displayAppPayChannel(wxpay) = %q, want wechat", got)
	}
	if got := normalizeAppPayChannel("wechat"); got != "wxpay" {
		t.Fatalf("normalizeAppPayChannel(wechat) = %q, want wxpay", got)
	}
}

func TestXZNCreatePaytypeKeepsLegacyAdminChannels(t *testing.T) {
	tests := map[string]string{
		"alipay":      "alipay",
		"ali_pay":     "alipay",
		"wechat":      "wxpay",
		"weixin":      "wxpay",
		"douyinpay":   "douyinpay",
		"custom_rail": "custom_rail",
	}
	for input, want := range tests {
		if got := normalizeXZNCreatePaytypeCode(input); got != want {
			t.Errorf("normalizeXZNCreatePaytypeCode(%q) = %q, want %q", input, got, want)
		}
	}
	for _, input := range []string{"", "123pay", "bad channel", "pay/type", "é"} {
		if got := normalizeXZNCreatePaytypeCode(input); got != "" {
			t.Errorf("normalizeXZNCreatePaytypeCode(%q) = %q, want empty", input, got)
		}
	}
}

func TestXZNOrderReturnURLCarriesOrderIdentity(t *testing.T) {
	got := xznOrderReturnURL("ninexing://billing/result?source=xzn", "app7-vip_month-1")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("payment_return") != "1" || u.Query().Get("source") != "xzn" || u.Query().Get("outTradeNo") != "app7-vip_month-1" {
		t.Fatalf("unexpected return URL %q", got)
	}
}

func TestValidateXZNBaseURLAllowlist(t *testing.T) {
	for _, raw := range []string{
		"https://pay.xzncraft.cn/openapi",
		"https://pay.xzncraft.cn/openapi/",
		"http://127.0.0.1:55144/openapi",
	} {
		if err := validateXZNBaseURL(raw); err != nil {
			t.Errorf("validateXZNBaseURL(%q) = %v", raw, err)
		}
	}
	for _, raw := range []string{
		"https://evil.example/openapi",
		"http://pay.xzncraft.cn/openapi",
		"https://pay.xzncraft.cn/other",
		"https://pay.xzncraft.cn/openapi?next=http://evil.example",
	} {
		if err := validateXZNBaseURL(raw); err == nil {
			t.Errorf("validateXZNBaseURL(%q) unexpectedly accepted", raw)
		}
	}
}

func TestValidateXZNSignTypeIsMD5Only(t *testing.T) {
	if err := validateXZNSignType("md5"); err != nil {
		t.Fatalf("lowercase MD5 should be accepted: %v", err)
	}
	for _, value := range []string{"RSA", "MD5+RSA", "", "sha256"} {
		if err := validateXZNSignType(value); err == nil {
			t.Errorf("validateXZNSignType(%q) unexpectedly accepted", value)
		}
	}
}

func TestValidateXZNReturnURL(t *testing.T) {
	for _, raw := range []string{"", "ninexing://billing/result", "https://xinzhili.app/billing/result"} {
		if err := validateXZNReturnURL(raw); err != nil {
			t.Errorf("validateXZNReturnURL(%q) = %v", raw, err)
		}
	}
	for _, raw := range []string{"javascript:alert(1)", "https://", "https://user:pass@example.com/result", "https://example.com/%0a"} {
		if err := validateXZNReturnURL(raw); err == nil {
			t.Errorf("validateXZNReturnURL(%q) unexpectedly accepted", raw)
		}
	}
}

func TestParseXZNCallbackRequiresSubjectAndKnownStatus(t *testing.T) {
	base := url.Values{
		"pid":          {"merchant-1"},
		"trade_no":     {"trade-1"},
		"out_trade_no": {"app-1"},
		"total_amount": {"29.00"},
		"subject":      {"月卡会员"},
		"paytype_code": {"wxpay"},
		"channel_id":   {"99"},
		"trade_status": {"TRADE_SUCCESS"},
	}
	if callback, err := parseXZNCallback(base); err != nil || callback.TotalCents != 2900 || callback.PaytypeCode != "wxpay" {
		t.Fatalf("valid callback = %+v, %v", callback, err)
	}
	for _, mutate := range []func(url.Values){
		func(values url.Values) { values.Del("subject") },
		func(values url.Values) { values.Set("trade_status", "UNKNOWN") },
	} {
		values := make(url.Values, len(base))
		for key, entries := range base {
			values[key] = append([]string(nil), entries...)
		}
		mutate(values)
		if _, err := parseXZNCallback(values); err == nil {
			t.Fatalf("expected malformed callback to be rejected: %+v", values)
		}
	}
}

func TestXZNReconcileLocalStatusDefersSuccessfulSettlement(t *testing.T) {
	if got := xznReconcileLocalStatus("pending", "TRADE_SUCCESS"); got != "pending" {
		t.Fatalf("successful provider query must remain pending before settlement, got %q", got)
	}
	if got := xznReconcileLocalStatus("paid", "TRADE_SUCCESS"); got != "paid" {
		t.Fatalf("already settled order must remain paid, got %q", got)
	}
	if got := xznReconcileLocalStatus("pending", "TRADE_CLOSED"); got != "closed" {
		t.Fatalf("closed provider query should close pending order, got %q", got)
	}
}
