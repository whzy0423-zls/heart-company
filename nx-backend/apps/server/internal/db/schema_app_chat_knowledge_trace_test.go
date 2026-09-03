package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesAppChatKnowledgeTraceContract(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.Join(strings.Fields(string(raw)), " ")
	definition := createTableDefinition(t, sqlText, "app_chat_knowledge_traces")

	for _, fragment := range []string{
		"session_id BIGINT NOT NULL REFERENCES app_chat_sessions(id) ON DELETE CASCADE",
		"assistant_message_id BIGINT NOT NULL UNIQUE REFERENCES app_chat_messages(id) ON DELETE CASCADE",
		"card_id BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE RESTRICT",
		"enneagram_type SMALLINT CHECK (enneagram_type IS NULL OR enneagram_type BETWEEN 1 AND 9)",
		"card_revision BIGINT NOT NULL CHECK (card_revision > 0)",
		"layer_hits JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CHECK (jsonb_typeof(layer_hits) = 'object')",
	} {
		if !strings.Contains(definition, fragment) {
			t.Errorf("app_chat_knowledge_traces missing %q", fragment)
		}
	}
	if !strings.Contains(sqlText, "idx_app_chat_knowledge_traces_session") {
		t.Fatal("app_chat_knowledge_traces needs a session index")
	}
}

func TestSchemaProtectsReleasedTheorySnapshots(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.Join(strings.Fields(string(raw)), " ")
	for _, fragment := range []string{
		"CREATE OR REPLACE FUNCTION protect_released_theory_mapping()",
		"CREATE TRIGGER trg_protect_released_theory_mapping",
		"release.status IN ('active','retired')",
		"CREATE OR REPLACE FUNCTION protect_released_theory_chunk()",
		"CREATE TRIGGER trg_protect_released_theory_chunk",
		"CREATE OR REPLACE FUNCTION protect_released_theory_release()",
		"CREATE TRIGGER trg_protect_released_theory_release",
		"released theory snapshot is immutable",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Errorf("release immutability schema missing %q", fragment)
		}
	}
}
