package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesDirectMessageTables(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Join(strings.Fields(string(raw)), " ")
	for _, table := range []string{"direct_conversations", "direct_message_media", "direct_messages", "direct_message_read_cursors", "direct_chat_appearances"} {
		if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("schema missing %s", table)
		}
	}
	for _, fragment := range []string{
		"user_low_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE",
		"user_high_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE",
		"CHECK (user_low_id < user_high_id)",
		"event_sequence BIGINT NOT NULL DEFAULT 0",
		"conversation_id BIGINT NOT NULL REFERENCES direct_conversations(id) ON DELETE CASCADE",
		"sender_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE",
		"client_message_id TEXT NOT NULL",
		"payload_hash TEXT NOT NULL",
		"sequence_no BIGINT NOT NULL",
		"UNIQUE (conversation_id, client_message_id)",
		"UNIQUE (conversation_id, sequence_no)",
		"idx_direct_conversations_pair",
		"idx_direct_messages_sequence",
		"media_type TEXT NOT NULL CHECK (media_type IN ('image', 'voice'))",
		"media_id BIGINT CONSTRAINT fk_direct_messages_media REFERENCES direct_message_media(id) ON DELETE RESTRICT",
		"ADD CONSTRAINT fk_direct_messages_media",
		"idx_direct_message_media_conversation",
		"idx_direct_message_cursors_user",
		"background_type TEXT NOT NULL DEFAULT 'preset' CHECK (background_type IN ('preset', 'color', 'image'))",
		"PRIMARY KEY (conversation_id, user_id)",
		"idx_direct_chat_appearances_user",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema missing direct message contract %q", fragment)
		}
	}
}
