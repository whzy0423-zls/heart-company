package server

import "testing"

func TestDefaultAppPlansCommercialPolicy(t *testing.T) {
	plans := defaultAppPlans()
	if len(plans) != 4 {
		t.Fatalf("len(defaultAppPlans()) = %d, want 4", len(plans))
	}
	wants := map[string]struct {
		price, chat, stories, cards int
	}{
		"free":        {0, 5, 1, 1},
		"vip_month":   {2900, -1, 3, 5},
		"vip_quarter": {7900, -1, 5, 8},
		"vip_year":    {19900, -1, 12, 20},
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
