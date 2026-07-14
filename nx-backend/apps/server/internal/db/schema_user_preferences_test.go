package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesUserPreferencesAfterAppUsers(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)

	usersIndex := strings.Index(schema, "CREATE TABLE IF NOT EXISTS app_users")
	preferencesIndex := strings.Index(schema, "CREATE TABLE IF NOT EXISTS app_user_preferences")
	if usersIndex < 0 {
		t.Fatal("schema is missing app_users")
	}
	if preferencesIndex < 0 {
		t.Fatal("schema is missing app_user_preferences")
	}
	if preferencesIndex < usersIndex {
		t.Fatal("app_user_preferences must be created after app_users")
	}
	preferencesEnd := strings.Index(schema[preferencesIndex:], ");")
	if preferencesEnd < 0 {
		t.Fatal("app_user_preferences CREATE TABLE is not terminated")
	}
	preferenceTable := schema[preferencesIndex : preferencesIndex+preferencesEnd]
	normalizedTable := strings.Join(strings.Fields(preferenceTable), " ")

	for _, required := range []string{
		"app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE",
		"category TEXT NOT NULL CHECK (category IN ('addressing', 'length', 'tone', 'format', 'interaction', 'custom'))",
		"slot TEXT NOT NULL CHECK (slot IN ('addressing.preferred_name', 'addressing.avoid_dear', 'length.detail_level', 'tone.direct', 'tone.formality', 'tone.warmth', 'format.no_lists', 'format.conclusion_first', 'interaction.no_followup', 'custom.communication_style'))",
		"CHECK (split_part(slot, '.', 1) = category)",
		"instruction TEXT NOT NULL CHECK (char_length(instruction) BETWEEN 1 AND 512)",
		"source_text TEXT NOT NULL DEFAULT '' CHECK (char_length(source_text) <= 1024)",
		"create_time TIMESTAMPTZ NOT NULL DEFAULT now()",
		"update_time TIMESTAMPTZ NOT NULL DEFAULT now()",
		"UNIQUE (app_user_id, slot)",
	} {
		if !strings.Contains(normalizedTable, required) {
			t.Fatalf("schema is missing user preference contract %q", required)
		}
	}
	for _, index := range []string{"idx_app_user_preferences_user_order", "idx_app_user_preferences_category"} {
		if !strings.Contains(schema, index) {
			t.Fatalf("schema is missing user preference index %q", index)
		}
	}
}
