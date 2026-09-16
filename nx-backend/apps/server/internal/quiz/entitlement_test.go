package quiz

import "testing"

func TestSecondaryLimitUsesCommercialMembershipPlans(t *testing.T) {
	tests := map[string]int{
		"":            1,
		"free":        1,
		"vip":         5,
		"vip_month":   5,
		"vip_quarter": 8,
		"svip":        20,
		"vip_year":    20,
	}
	for plan, want := range tests {
		if got := SecondaryLimit(plan); got != want {
			t.Fatalf("SecondaryLimit(%q) = %d, want %d", plan, got, want)
		}
	}
}
