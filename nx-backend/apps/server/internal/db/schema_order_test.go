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
