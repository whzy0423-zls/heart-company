package server

import (
	"context"
	"testing"
)

func TestAppPlanLookupPreservesCanonicalSvip(t *testing.T) {
	var server Server
	plan := server.appPlan(context.Background(), "svip")
	if plan.Code != "svip" || plan.PlanLevel != "svip" || plan.CardLimit != 10 {
		t.Fatalf("appPlan(svip) = %+v, want canonical disabled SVIP plan", plan)
	}
}

func TestDefaultAppPlansCommercialPolicy(t *testing.T) {
	plans := defaultAppPlans()
	if len(plans) != 7 {
		t.Fatalf("len(defaultAppPlans()) = %d, want 7", len(plans))
	}
	wants := map[string]struct {
		price, chat, stories, cards int
	}{
		"free":         {0, 5, 1, 1},
		"vip_month":    {2900, -1, 3, 3},
		"vip_quarter":  {7900, -1, 3, 3},
		"vip_year":     {19900, -1, 3, 3},
		"svip_month":   {5900, -1, 12, 10},
		"svip_quarter": {15900, -1, 12, 10},
		"svip_year":    {49900, -1, 12, 10},
	}
	for _, plan := range plans {
		want, ok := wants[plan.Code]
		if !ok {
			t.Fatalf("unexpected plan %q", plan.Code)
		}
		if plan.PriceCents != want.price || plan.DailyChatLimit != want.chat || plan.StoryMonthlyLimit != want.stories || plan.CardLimit != want.cards {
			t.Errorf("plan %s = price %d chat %d stories %d cards %d", plan.Code, plan.PriceCents, plan.DailyChatLimit, plan.StoryMonthlyLimit, plan.CardLimit)
		}
	}
}

func TestDefaultSvipPlanIsVisibleButRequiresAdminPricingBeforeActivation(t *testing.T) {
	plan := defaultAppPlan("svip")
	if plan.PlanLevel != "svip" || plan.BillingCycle != "year" || plan.CardLimit != 10 {
		t.Fatalf("defaultAppPlan(svip) = %+v", plan)
	}
	if plan.Enabled {
		t.Fatal("unpriced SVIP fallback must not be purchasable")
	}
}

func TestNormalizeCardFeatureCopyReplacesExistingCapacityPhrase(t *testing.T) {
	got := normalizeCardFeatureCopy([]string{"深度陪伴", "最多 5 张人物卡"}, 3)
	if got[1] != "最多 3 张人物卡" {
		t.Fatalf("normalizeCardFeatureCopy() = %v, want canonical capacity copy", got)
	}
}

func TestValidateAppPlanRejectsInvalidCommercialValues(t *testing.T) {
	base := defaultAppPlans()[1]
	tests := []struct {
		name string
		edit func(*appPlanConfig)
	}{
		{"unknown code", func(p *appPlanConfig) { p.Code = "custom" }},
		{"negative price", func(p *appPlanConfig) { p.PriceCents = -1 }},
		{"zero duration", func(p *appPlanConfig) { p.DurationDays = 0 }},
		{"invalid unlimited value", func(p *appPlanConfig) { p.DailyChatLimit = -2 }},
		{"empty name", func(p *appPlanConfig) { p.Name = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := base
			tt.edit(&plan)
			if err := validateAppPlan(plan); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
