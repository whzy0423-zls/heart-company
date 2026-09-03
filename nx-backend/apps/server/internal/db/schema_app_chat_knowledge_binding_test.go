package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesAppChatKnowledgeBindingContract(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.Join(strings.Fields(string(raw)), " ")
	definition := createTableDefinition(t, sqlText, "app_chat_knowledge_bindings")

	for _, fragment := range []string{
		"layer_kind TEXT NOT NULL CHECK (layer_kind IN ('theory','enneagram_type'))",
		"enneagram_type SMALLINT",
		"theory_library_id BIGINT NOT NULL REFERENCES theory_libraries(id) ON DELETE RESTRICT",
		"status TEXT NOT NULL DEFAULT 'disabled' CHECK (status IN ('enabled','disabled'))",
		"layer_kind = 'theory' AND enneagram_type IS NULL",
		"layer_kind = 'enneagram_type' AND enneagram_type BETWEEN 1 AND 9",
	} {
		if !strings.Contains(definition, fragment) {
			t.Errorf("app_chat_knowledge_bindings missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_app_chat_enabled_knowledge_binding",
		"ON app_chat_knowledge_bindings(layer_kind, COALESCE(enneagram_type, 0)) WHERE status = 'enabled'",
		"idx_app_chat_knowledge_bindings_library",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Errorf("knowledge binding schema missing %q", fragment)
		}
	}
}
