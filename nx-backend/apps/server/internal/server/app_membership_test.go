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

func timePtr(value time.Time) *time.Time { return &value }
