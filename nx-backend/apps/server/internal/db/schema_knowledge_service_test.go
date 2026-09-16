package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaDefinesScopedKnowledgeDocumentsAndOptionalVectorIndex(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Join(strings.Fields(string(raw)), " ")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS knowledge_documents",
		"library_kind TEXT NOT NULL",
		"release_id BIGINT",
		"enneagram_type INT",
		"safety_level INT",
		"content_hash TEXT NOT NULL",
		"index_version TEXT NOT NULL",
		"ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS embedding vector(1024)",
		"idx_knowledge_documents_embedding_hnsw",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema missing %q", fragment)
		}
	}
}
