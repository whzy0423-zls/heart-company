package server

import (
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

func TestMiniappMembershipValidity(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-time.Second)
	tests := []struct {
		name    string
		level   int
		expires *time.Time
		want    bool
	}{
		{name: "free", level: 0, want: false},
		{name: "legacy lifetime member", level: 1, want: true},
		{name: "dated active member", level: 1, expires: &future, want: true},
		{name: "expired member", level: 1, expires: &past, want: false},
		{name: "refund or revoke wins over remaining date", level: 0, expires: &future, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := miniapp.IsMembershipActive(tt.level, tt.expires, now); got != tt.want {
				t.Fatalf("IsMembershipActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWxPayCallbackAcceptsMemberOrder(t *testing.T) {
	env := config.Env{}
	env.WxPay.MchID = "merchant"
	env.WxPay.AppID = "miniapp"
	err := validateWxPayCallbackAgainstOrder(env, wxpay.CallbackResult{
		OutTradeNo: "member-order", MchID: "merchant", AppID: "miniapp", AmountTotal: 9900,
	}, paymentOrderSnapshot{Amount: 9900, Product: "member"})
	if err != nil {
		t.Fatalf("member payment callback should reach miniapp.MarkOrderPaid: %v", err)
	}
}
