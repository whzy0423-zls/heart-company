package db

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultMenusIncludeAppAgentDiscounts(t *testing.T) {
	for _, menu := range defaultMenus {
		if menu.Name != "AppAgentDiscounts" {
			continue
		}
		if menu.ID != 1622 || menu.PID != 1600 || menu.Path != "/app/agent-discounts" || menu.Component != "/app/agent-discounts" || menu.AuthCode != "App:PlanManagement:View" || menu.Type != "menu" || menu.Sort != 12 || menu.Title != "代理购卡优惠" {
			t.Fatalf("unexpected App agent discount menu: %+v", menu)
		}
		return
	}
	t.Fatal("expected default menu AppAgentDiscounts")
}

func TestSeedInvokesAppAgentDiscountMenuBackfill(t *testing.T) {
	raw, err := os.ReadFile("db.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "seedAppAgentDiscountMenu(ctx, database)") {
		t.Fatal("expected seed to backfill the App agent discount menu")
	}
	for _, expected := range []string{
		"AppPlanManagement",
		"App:PlanManagement:View",
		"role_menus",
	} {
		if !strings.Contains(appAgentDiscountMenuBindingSQL, expected) {
			t.Fatalf("expected role binding migration to contain %q", expected)
		}
	}
}
