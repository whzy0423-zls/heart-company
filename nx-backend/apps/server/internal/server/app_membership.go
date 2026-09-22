package server

import (
	"fmt"
	"strings"
	"time"
)

type membershipPeriod struct {
	Start   time.Time
	Expires time.Time
}

// membershipUpgradeQuoteInput contains prices captured from the plan catalog
// and the user's currently remaining membership time. Callers should obtain
// these values while holding the same order/user lock used to create the
// upgrade order.
type membershipUpgradeQuoteInput struct {
	CurrentPlan         string
	TargetPlan          string
	CurrentPriceCents   int
	TargetPriceCents    int
	CurrentDurationDays int
	RemainingDays       int
}

type membershipUpgradeQuote struct {
	CurrentPlan   string `json:"currentPlan"`
	TargetPlan    string `json:"targetPlan"`
	SameCycle     bool   `json:"sameCycle"`
	RemainingDays int    `json:"remainingDays"`
	CreditCents   int    `json:"creditCents"`
	PayableCents  int    `json:"payableCents"`
}

// calculateMembershipUpgradeQuote prices an upgrade without mutating state.
// Same-cycle changes charge only the remaining share of the price difference;
// cross-cycle changes credit the rounded remaining value of the old plan.
func calculateMembershipUpgradeQuote(input membershipUpgradeQuoteInput) (membershipUpgradeQuote, error) {
	if strings.TrimSpace(input.CurrentPlan) == "" || strings.TrimSpace(input.TargetPlan) == "" {
		return membershipUpgradeQuote{}, fmt.Errorf("membership plans are required")
	}
	if input.CurrentPriceCents < 0 || input.TargetPriceCents < 0 || input.CurrentDurationDays <= 0 {
		return membershipUpgradeQuote{}, fmt.Errorf("invalid membership quote prices or duration")
	}
	remainingDays := input.RemainingDays
	if remainingDays <= 0 {
		remainingDays = 1
	}
	if remainingDays > input.CurrentDurationDays {
		remainingDays = input.CurrentDurationDays
	}
	currentCycle := normalizeBillingCycle(normalizeMembershipLevel(input.CurrentPlan), input.CurrentPlan)
	targetCycle := normalizeBillingCycle(normalizeMembershipLevel(input.TargetPlan), input.TargetPlan)
	sameCycle := currentCycle == targetCycle
	creditCents := 0
	if sameCycle {
		delta := input.TargetPriceCents - input.CurrentPriceCents
		if delta > 0 {
			creditCents = roundDivide(delta*remainingDays, input.CurrentDurationDays)
		}
	} else if input.CurrentPriceCents > 0 {
		creditCents = roundDivide(input.CurrentPriceCents*remainingDays, input.CurrentDurationDays)
	}
	if creditCents > input.TargetPriceCents {
		creditCents = input.TargetPriceCents
	}
	return membershipUpgradeQuote{
		CurrentPlan: input.CurrentPlan, TargetPlan: input.TargetPlan,
		SameCycle: sameCycle, RemainingDays: remainingDays,
		CreditCents: creditCents, PayableCents: input.TargetPriceCents - creditCents,
	}, nil
}

func roundDivide(numerator, denominator int) int {
	if denominator <= 0 || numerator <= 0 {
		return 0
	}
	return (numerator + denominator/2) / denominator
}

// resolvedMembershipPlan is the canonical view used by new entitlement and
// resource-access code. SKU/code remains available for old clients and orders.
type resolvedMembershipPlan struct {
	PlanLevel    string
	BillingCycle string
	SKU          string
	Active       bool
	DowngradedAt *time.Time
}

func resolveMembershipPlan(memberLevel, sku string, expiresAt *time.Time, now time.Time) resolvedMembershipPlan {
	rawLevel := strings.ToLower(strings.TrimSpace(memberLevel))
	rawSKU := strings.ToLower(strings.TrimSpace(sku))
	if rawLevel == "" || rawLevel == "free" {
		if rawSKU != "" && rawSKU != "free" {
			rawLevel = rawSKU
		}
	}
	level := normalizeMembershipLevel(rawLevel)
	if level == "free" {
		return resolvedMembershipPlan{PlanLevel: "free", BillingCycle: "none", SKU: "free", Active: true}
	}
	cycle := normalizeBillingCycle(level, firstNonEmpty(rawSKU, rawLevel))
	legacyPerpetual := (rawLevel == "vip" || rawLevel == "svip") && expiresAt == nil
	active := legacyPerpetual || (expiresAt != nil && expiresAt.After(now))
	if active {
		return resolvedMembershipPlan{PlanLevel: level, BillingCycle: cycle, SKU: firstNonEmpty(rawSKU, rawLevel), Active: true}
	}
	var downgradedAt *time.Time
	if expiresAt != nil && !expiresAt.After(now) {
		value := *expiresAt
		downgradedAt = &value
	}
	return resolvedMembershipPlan{PlanLevel: "free", BillingCycle: "none", SKU: "free", Active: false, DowngradedAt: downgradedAt}
}

func membershipLevelCardLimit(level string) int {
	return defaultMembershipLevelPlan(normalizeMembershipLevel(level)).CardLimit
}

type appMembershipBenefit struct {
	PlanName          string
	CardLimit         int
	StoryMonthlyLimit int
}

func appMembershipBenefits(plan string) appMembershipBenefit {
	configured := defaultAppPlan(plan)
	if normalized := normalizeAppPlanCode(plan); strings.HasPrefix(normalized, "svip_") {
		configured.Name = appProductTitle(normalized)
	}
	return appMembershipBenefit{PlanName: configured.Name, CardLimit: configured.CardLimit, StoryMonthlyLimit: configured.StoryMonthlyLimit}
}

func appEffectivePlanCode(memberLevel string, expiresAt *time.Time, now time.Time) string {
	normalizedLevel := strings.ToLower(strings.TrimSpace(memberLevel))
	planCode := appPlanCode(normalizedLevel)
	if planCode == "free" {
		return "free"
	}
	// Unknown membership values must never reach appPlan's compatibility
	// fallback, which carries paid defaults for legacy display callers. The
	// effective entitlement path fails closed to the free plan.
	if !supportedAppPlanCode(planCode) {
		return "free"
	}
	legacyWithoutExpiry := (normalizedLevel == "vip" || normalizedLevel == "svip") && expiresAt == nil
	if legacyWithoutExpiry || (expiresAt != nil && expiresAt.After(now)) {
		return planCode
	}
	return "free"
}

func parseAppMembershipExpiry(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006/01/02 15:04:05", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed
		}
	}
	return nil
}

func membershipDurationDays(plan string) (int, error) {
	plan = normalizeAppPlanCode(plan)
	if plan == "free" || !supportedAppPlanCode(plan) {
		return 0, fmt.Errorf("unsupported membership plan %q", plan)
	}
	return defaultAppPlan(plan).DurationDays, nil
}

func calculateMembershipPeriod(plan string, activation time.Time, currentExpiry *time.Time) (membershipPeriod, error) {
	days, err := membershipDurationDays(plan)
	if err != nil {
		return membershipPeriod{}, err
	}
	return calculateMembershipPeriodDays(days, activation, currentExpiry)
}

func calculateMembershipPeriodDays(days int, activation time.Time, currentExpiry *time.Time) (membershipPeriod, error) {
	if days <= 0 || days > 3660 {
		return membershipPeriod{}, fmt.Errorf("invalid membership duration %d", days)
	}
	base := activation
	if currentExpiry != nil && currentExpiry.After(activation) {
		base = *currentExpiry
	}
	return membershipPeriod{
		Start:   activation,
		Expires: base.AddDate(0, 0, days),
	}, nil
}
