package quiz

import "testing"

func TestSecondaryLimitUsesCommercialMembershipPlans(t *testing.T) {
	tests := map[string]int{
		"":            1,
		"free":        1,
		"vip":         3,
		"vip_month":   3,
		"vip_quarter": 3,
		"svip":        10,
		"vip_year":    3,
		"legacy":      3,
	}
	for plan, want := range tests {
		if got := SecondaryLimit(plan); got != want {
			t.Fatalf("SecondaryLimit(%q) = %d, want %d", plan, got, want)
		}
	}
}
