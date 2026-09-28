package db

import (
	"strings"
	"testing"
)

func TestSchemaPublicKnowledgeCatalogAndManagedIndexes(t *testing.T) {
	schema := strings.Join(strings.Fields(schemaSQL), " ")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS public_knowledge_sources",
		"CREATE TABLE IF NOT EXISTS public_knowledge_import_batches",
		"file_hash TEXT NOT NULL",
		"CHECK (status IN ('completed','rolled_back'))",
		"source_kind TEXT NOT NULL CHECK (source_kind IN ('book','story'))",
		"quality_status TEXT NOT NULL DEFAULT 'pending'",
		"ADD COLUMN IF NOT EXISTS public_source_id TEXT REFERENCES public_knowledge_sources(id)",
		"ADD COLUMN IF NOT EXISTS import_batch_id TEXT",
		"ADD COLUMN IF NOT EXISTS search_text TEXT NOT NULL DEFAULT ''",
		"WHERE public_source_id IS NULL",
		"ON knowledge_documents(public_source_id, content_hash, index_version) WHERE public_source_id IS NOT NULL",
		"USING gin (COALESCE(public_search_vector, to_tsvector('simple', search_text)))",
		"CREATE INDEX IF NOT EXISTS idx_knowledge_documents_legacy_title_substring",
		"CREATE INDEX IF NOT EXISTS idx_knowledge_documents_legacy_content_substring",
		"USING gin (content %I.gin_trgm_ops) WHERE public_source_id IS NULL",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema missing %q", fragment)
		}
	}
}
