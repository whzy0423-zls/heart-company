package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesPersistentVoiceMessageColumns(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS message_type TEXT NOT NULL DEFAULT 'text'",
		"ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS audio_asset_id BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL",
		"ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS audio_duration_ms INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS transcript TEXT NOT NULL DEFAULT ''",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("schema missing voice message contract %q", required)
		}
	}
}
