package server

import "testing"

func TestCalculateMembershipUpgradeQuoteSameCycleUsesRemainingDays(t *testing.T) {
	quote, err := calculateMembershipUpgradeQuote(membershipUpgradeQuoteInput{
		CurrentPlan:         "vip_month",
		TargetPlan:          "svip_month",
		CurrentPriceCents:   2900,
		TargetPriceCents:    5900,
		CurrentDurationDays: 30,
		RemainingDays:       15,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !quote.SameCycle || quote.RemainingDays != 15 || quote.CreditCents != 1500 || quote.PayableCents != 4400 {
		t.Fatalf("same-cycle quote = %+v, want credit=1500 payable=4400", quote)
	}
}

func TestCalculateMembershipUpgradeQuoteCrossCycleCreditsRemainingValue(t *testing.T) {
	quote, err := calculateMembershipUpgradeQuote(membershipUpgradeQuoteInput{
		CurrentPlan:         "vip_month",
		TargetPlan:          "svip_year",
		CurrentPriceCents:   2900,
		TargetPriceCents:    49900,
		CurrentDurationDays: 30,
		RemainingDays:       15,
	})
	if err != nil {
		t.Fatal(err)
	}
	if quote.SameCycle || quote.CreditCents != 1450 || quote.PayableCents != 48450 {
		t.Fatalf("cross-cycle quote = %+v, want credit=1450 payable=48450", quote)
	}
}

func TestCalculateMembershipUpgradeQuoteRoundsAndKeepsAtLeastOneDay(t *testing.T) {
	quote, err := calculateMembershipUpgradeQuote(membershipUpgradeQuoteInput{
		CurrentPlan:         "vip_month",
		TargetPlan:          "svip_quarter",
		CurrentPriceCents:   1000,
		TargetPriceCents:    10000,
		CurrentDurationDays: 30,
		RemainingDays:       0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if quote.RemainingDays != 1 || quote.CreditCents != 33 || quote.PayableCents != 9967 {
		t.Fatalf("rounded minimum-day quote = %+v, want days=1 credit=33 payable=9967", quote)
	}
}

func TestCalculateMembershipUpgradeQuoteNeverCreditsAboveTargetPrice(t *testing.T) {
	quote, err := calculateMembershipUpgradeQuote(membershipUpgradeQuoteInput{
		CurrentPlan:         "vip_year",
		TargetPlan:          "svip_month",
		CurrentPriceCents:   50000,
		TargetPriceCents:    5900,
		CurrentDurationDays: 365,
		RemainingDays:       365,
	})
	if err != nil {
		t.Fatal(err)
	}
	if quote.CreditCents != 5900 || quote.PayableCents != 0 {
		t.Fatalf("capped quote = %+v, want credit=5900 payable=0", quote)
	}
}
