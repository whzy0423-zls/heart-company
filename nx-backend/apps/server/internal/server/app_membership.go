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
