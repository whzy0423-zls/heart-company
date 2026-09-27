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
		"DROP INDEX IF EXISTS uq_app_chat_enabled_knowledge_binding",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_app_chat_enabled_enneagram_binding",
		"ON app_chat_knowledge_bindings(enneagram_type) WHERE status = 'enabled' AND layer_kind = 'enneagram_type'",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_app_chat_enabled_theory_library_binding",
		"ON app_chat_knowledge_bindings(theory_library_id) WHERE status = 'enabled' AND layer_kind = 'theory'",
		"idx_app_chat_knowledge_bindings_library",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Errorf("knowledge binding schema missing %q", fragment)
		}
	}
	for _, libraryKey := range []string{
		"skill-qinmi-guanxi",
		"skill-social-psychology-myers",
		"skill-sociology-of-human-emotions",
		"skill-crowd-psychology",
		"skill-brain-and-cognitive-science",
	} {
		if !strings.Contains(sqlText, libraryKey) {
			t.Errorf("knowledge binding schema does not seed %q", libraryKey)
		}
	}
}
