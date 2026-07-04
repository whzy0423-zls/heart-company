package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaDeviceTokensRegistrationIDIsUnique(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	if !strings.Contains(schema, "ON app_device_tokens(registration_id)") {
		t.Fatal("expected app_device_tokens to enforce one row per registration_id")
	}
	if strings.Contains(schema, "ON app_device_tokens(app_user_id, registration_id)") {
		t.Fatal("app_device_tokens must not allow the same registration_id for multiple users")
	}
}

func TestSchemaAddsIndexesForUserInsightsQueries(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, index := range []string{
		"idx_app_users_insights_order",
		"idx_app_users_status_member_order",
		"idx_app_memories_user_status_update",
	} {
		if !strings.Contains(schema, index) {
			t.Fatalf("expected schema to include %s for admin insight query stability", index)
		}
	}
}
