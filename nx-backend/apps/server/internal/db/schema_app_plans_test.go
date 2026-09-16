package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaDefinesAppPlansAndDailyChatQuota(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS app_plans",
		"daily_chat_limit",
		"story_monthly_limit",
		"card_limit",
		"CREATE TABLE IF NOT EXISTS app_chat_daily_quotas",
		"CREATE TABLE IF NOT EXISTS app_chat_quota_reservations",
		"ALTER TABLE app_orders ADD COLUMN IF NOT EXISTS duration_days",
	} {
		if !strings.Contains(schema, fragment) {
			t.Errorf("schema missing %q", fragment)
		}
	}
	for _, code := range []string{"free", "vip_month", "vip_quarter", "vip_year"} {
		if !strings.Contains(schema, "'"+code+"'") {
			t.Errorf("schema missing seeded plan %q", code)
		}
	}
}
