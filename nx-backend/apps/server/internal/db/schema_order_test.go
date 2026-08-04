package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaDoesNotAlterAppChatMessagesBeforeCreate(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)

	createIndex := strings.Index(sql, "CREATE TABLE IF NOT EXISTS app_chat_messages")
	if createIndex < 0 {
		t.Fatal("schema is missing app_chat_messages CREATE TABLE")
	}

	for _, statement := range []string{
		"ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS favorite",
		"ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS feedback",
	} {
		alterIndex := strings.Index(sql, statement)
		if alterIndex < 0 {
			continue
		}
		if alterIndex < createIndex {
			t.Fatalf("%q appears before app_chat_messages is created", statement)
		}
	}
}

func TestSchemaMigratesExistingQuizSubmissionWingType(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	statement := "ALTER TABLE app_quiz_submissions ADD COLUMN IF NOT EXISTS wing_type"
	if !strings.Contains(sql, statement) {
		t.Fatalf("schema is missing old-database migration %q", statement)
	}
}

func TestSchemaMigratesChatSessionContextSummary(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, statement := range []string{
		"ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS context_summary",
		"ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS context_summary_through_message_id",
	} {
		if !strings.Contains(sql, statement) {
			t.Fatalf("schema is missing old-database migration %q", statement)
		}
	}
}

func TestSchemaMigratesChatSessionScene(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, statement := range []string{
		"ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS scene",
		"idx_app_chat_sessions_scene",
	} {
		if !strings.Contains(sql, statement) {
			t.Fatalf("schema is missing chat scene migration %q", statement)
		}
	}
}

func TestMigrateSchemaPreparesLegacyVideoColumnsBeforeFullSchema(t *testing.T) {
	raw, err := os.ReadFile("db.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	compatibilityExec := strings.Index(source, "preSchemaCompatibilitySQL")
	fullSchemaExec := strings.Index(source, "tx.ExecContext(ctx, schemaSQL)")
	if compatibilityExec < 0 || fullSchemaExec < 0 || compatibilityExec > fullSchemaExec {
		t.Fatal("legacy video compatibility SQL must run before the full embedded schema")
	}
	for _, table := range []string{
		"ALTER TABLE IF EXISTS video_project_characters",
		"ALTER TABLE IF EXISTS video_project_scenes",
		"ALTER TABLE IF EXISTS video_project_assets",
	} {
		if !strings.Contains(source, table) {
			t.Fatalf("missing pre-schema compatibility migration %q", table)
		}
	}
	if strings.Count(source, "ADD COLUMN IF NOT EXISTS breakdown_item_key") < 3 {
		t.Fatal("all legacy video project tables must add breakdown_item_key before the full schema")
	}
}
