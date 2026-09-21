package server

import (
	"testing"
	"time"
)

func TestCalculateMembershipPeriod(t *testing.T) {
	activation := time.Date(2026, 7, 20, 10, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	activeExpiry := activation.Add(12 * 24 * time.Hour)

	tests := []struct {
		name          string
		plan          string
		currentExpiry *time.Time
		wantStart     time.Time
		wantExpiry    time.Time
		wantErr       bool
	}{
		{name: "month starts at activation", plan: "vip_month", wantStart: activation, wantExpiry: activation.AddDate(0, 0, 30)},
		{name: "quarter lasts ninety days", plan: "vip_quarter", wantStart: activation, wantExpiry: activation.AddDate(0, 0, 90)},
		{name: "year lasts three hundred sixty five days", plan: "vip_year", wantStart: activation, wantExpiry: activation.AddDate(0, 0, 365)},
		{name: "active membership renews from current expiry", plan: "vip_month", currentExpiry: &activeExpiry, wantStart: activation, wantExpiry: activeExpiry.AddDate(0, 0, 30)},
		{name: "expired membership restarts at activation", plan: "vip_month", currentExpiry: timePtr(activation.Add(-time.Hour)), wantStart: activation, wantExpiry: activation.AddDate(0, 0, 30)},
		{name: "invalid plan", plan: "deep_report", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			period, err := calculateMembershipPeriod(tc.plan, activation, tc.currentExpiry)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !period.Start.Equal(tc.wantStart) || !period.Expires.Equal(tc.wantExpiry) {
				t.Fatalf("period = %+v, want start %v expiry %v", period, tc.wantStart, tc.wantExpiry)
			}
		})
	}
}

func TestAppMembershipBenefitsByPlan(t *testing.T) {
	tests := []struct {
		plan           string
		wantName       string
		wantCards      int
		wantStoryLimit int
	}{
		{plan: "free", wantName: "免费版", wantCards: 1, wantStoryLimit: 1},
		{plan: "vip_month", wantName: "月卡会员", wantCards: 3, wantStoryLimit: 3},
		{plan: "vip_quarter", wantName: "季卡会员", wantCards: 3, wantStoryLimit: 5},
		{plan: "vip_year", wantName: "年卡会员", wantCards: 3, wantStoryLimit: 12},
		{plan: "svip", wantName: "S VIP", wantCards: 10, wantStoryLimit: 12},
	}

	for _, tc := range tests {
		t.Run(tc.plan, func(t *testing.T) {
			benefits := appMembershipBenefits(tc.plan)
			if benefits.PlanName != tc.wantName || benefits.CardLimit != tc.wantCards || benefits.StoryMonthlyLimit != tc.wantStoryLimit {
				t.Fatalf("appMembershipBenefits(%q) = %+v", tc.plan, benefits)
			}
		})
	}
}

func TestAppMembershipBenefitsUseSafeDefaultsForUnknownMemberPlan(t *testing.T) {
	benefits := appMembershipBenefits("legacy_partner")
	if benefits.PlanName != "会员版" || benefits.CardLimit != 3 || benefits.StoryMonthlyLimit != 3 {
		t.Fatalf("unexpected compatibility benefits: %+v", benefits)
	}
}

func TestAppEffectivePlanCodeHonorsMembershipExpiry(t *testing.T) {
	now := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	tests := []struct {
		name   string
		level  string
		expiry *time.Time
		want   string
	}{
		{name: "free", level: "free", want: "free"},
		{name: "legacy vip without expiry", level: "vip", want: "vip_month"},
		{name: "legacy vip is case insensitive", level: "VIP", want: "vip_month"},
		{name: "svip remains canonical level product", level: "svip", want: "svip"},
		{name: "dated plan active", level: "vip_quarter", expiry: &future, want: "vip_quarter"},
		{name: "dated plan expired", level: "vip_year", expiry: &past, want: "free"},
		{name: "dated plan missing expiry", level: "vip_month", want: "free"},
		{name: "unknown level fails closed", level: "legacy_partner", expiry: &future, want: "free"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := appEffectivePlanCode(tc.level, tc.expiry, now); got != tc.want {
				t.Fatalf("appEffectivePlanCode(%q) = %q, want %q", tc.level, got, tc.want)
			}
		})
	}
}

func TestCanonicalMembershipBenefitsUseLevelCardLimits(t *testing.T) {
	for _, tc := range []struct {
		plan string
		want int
	}{
		{plan: "vip_month", want: 3},
		{plan: "vip_quarter", want: 3},
		{plan: "vip_year", want: 3},
		{plan: "svip", want: 10},
	} {
		benefits := appMembershipBenefits(tc.plan)
		if benefits.CardLimit != tc.want {
			t.Fatalf("appMembershipBenefits(%q).CardLimit = %d, want %d", tc.plan, benefits.CardLimit, tc.want)
		}
	}
}

func TestParseAppMembershipExpiryAcceptsStoredTimestamp(t *testing.T) {
	expiry := parseAppMembershipExpiry("2026/09/14 16:30:00")
	if expiry == nil || expiry.Year() != 2026 || expiry.Month() != time.September || expiry.Day() != 14 {
		t.Fatalf("unexpected parsed expiry: %v", expiry)
	}
}

func timePtr(value time.Time) *time.Time { return &value }
