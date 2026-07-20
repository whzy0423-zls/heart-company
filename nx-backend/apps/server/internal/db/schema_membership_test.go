package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaContainsTimeBoundAppMembershipColumns(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, statement := range []string{
		"member_started_at TIMESTAMPTZ",
		"member_expires_at TIMESTAMPTZ",
		"activation_at TIMESTAMPTZ",
		"membership_expires_at TIMESTAMPTZ",
		"ALTER TABLE app_users ADD COLUMN IF NOT EXISTS member_started_at",
		"ALTER TABLE app_users ADD COLUMN IF NOT EXISTS member_expires_at",
		"ALTER TABLE app_orders ADD COLUMN IF NOT EXISTS activation_at",
		"ALTER TABLE app_orders ADD COLUMN IF NOT EXISTS membership_expires_at",
	} {
		if !strings.Contains(schema, statement) {
			t.Fatalf("schema missing %q", statement)
		}
	}
}
