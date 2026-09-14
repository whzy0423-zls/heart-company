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
	planCode := appPlanCode(memberLevel)
	if planCode == "free" {
		return "free"
	}
	legacyWithoutExpiry := (memberLevel == "vip" || memberLevel == "svip") && expiresAt == nil
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
