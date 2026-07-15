package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"
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

func TestSchemaUserPreferencesPostgresConstraintsAndCascade(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run user preference schema integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := Open(ctx, dsn, "admin", "123456")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var userID int64
	if err := database.QueryRowContext(ctx,
		`INSERT INTO app_users (phone) VALUES ($1) RETURNING id`,
		fmt.Sprintf("pref-schema-%d", time.Now().UnixNano()),
	).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO app_user_preferences (app_user_id, category, slot, instruction, source_text)
		 VALUES ($1, 'length', 'length.detail_level', '回答简短一些', '以后短一点')`, userID,
	); err != nil {
		t.Fatalf("insert valid preference: %v", err)
	}

	invalidStatements := []struct {
		name  string
		query string
	}{
		{name: "category", query: `INSERT INTO app_user_preferences (app_user_id, category, slot, instruction) VALUES ($1, 'identity', 'length.detail_level', 'x')`},
		{name: "slot", query: `INSERT INTO app_user_preferences (app_user_id, category, slot, instruction) VALUES ($1, 'length', 'length.unknown', 'x')`},
		{name: "category slot mismatch", query: `INSERT INTO app_user_preferences (app_user_id, category, slot, instruction) VALUES ($1, 'tone', 'format.no_lists', 'x')`},
		{name: "empty instruction", query: `INSERT INTO app_user_preferences (app_user_id, category, slot, instruction) VALUES ($1, 'tone', 'tone.direct', '')`},
		{name: "duplicate slot", query: `INSERT INTO app_user_preferences (app_user_id, category, slot, instruction) VALUES ($1, 'length', 'length.detail_level', 'duplicate')`},
	}
	for _, tt := range invalidStatements {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := database.ExecContext(ctx, tt.query, userID); err == nil {
				t.Fatalf("expected PostgreSQL to reject %s", tt.name)
			}
		})
	}

	if _, err := database.ExecContext(ctx, `DELETE FROM app_users WHERE id = $1`, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var count int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) FROM app_user_preferences WHERE app_user_id = $1`, userID,
	).Scan(&count); err != nil {
		t.Fatalf("count preferences after user delete: %v", err)
	}
	if count != 0 {
		t.Fatalf("ON DELETE CASCADE left %d preferences", count)
	}
}
