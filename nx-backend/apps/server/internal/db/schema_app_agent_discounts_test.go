package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaContainsAppAgentDiscountRulesAndOrderSnapshots(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS app_agent_discount_rules",
		"PRIMARY KEY (product_id, audience)",
		"enabled BOOLEAN NOT NULL DEFAULT false",
		"ALTER TABLE app_orders ADD COLUMN IF NOT EXISTS base_price_cents",
		"ALTER TABLE app_orders ADD COLUMN IF NOT EXISTS discount_cents",
		"ALTER TABLE app_orders ADD COLUMN IF NOT EXISTS discount_snapshot",
	} {
		if !strings.Contains(schema, fragment) {
			t.Errorf("missing schema fragment %q", fragment)
		}
	}
}
