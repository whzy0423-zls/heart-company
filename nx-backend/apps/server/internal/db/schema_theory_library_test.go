package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesTheoryLibraryFoundation(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.Join(strings.Fields(string(raw)), " ")

	required := []string{
		"CREATE TABLE IF NOT EXISTS theory_libraries",
		"CREATE TABLE IF NOT EXISTS theory_library_releases",
		"CREATE TABLE IF NOT EXISTS theory_source_works",
		"CREATE TABLE IF NOT EXISTS theory_source_files",
		"CREATE TABLE IF NOT EXISTS theory_cards",
		"CREATE TABLE IF NOT EXISTS theory_practices",
		"CREATE TABLE IF NOT EXISTS theory_card_relations",
		"CREATE TABLE IF NOT EXISTS theory_card_sources",
		"CREATE TABLE IF NOT EXISTS theory_chunks",
		"CREATE TABLE IF NOT EXISTS theory_chunk_embeddings",
		"CREATE TABLE IF NOT EXISTS theory_release_cards",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_active_release ON theory_library_releases(library_id) WHERE status = 'active'",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_published_card_key ON theory_cards(library_id, canonical_key) WHERE status = 'published'",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_release_chunk ON theory_release_cards(release_id, chunk_id)",
		"ALTER TABLE theory_chunk_embeddings ADD COLUMN IF NOT EXISTS embedding vector(1536)",
		"CREATE INDEX IF NOT EXISTS idx_theory_chunk_embeddings_hnsw ON theory_chunk_embeddings USING hnsw (embedding vector_cosine_ops)",
		"CREATE OR REPLACE FUNCTION validate_theory_card_source_file_work()",
		"CREATE CONSTRAINT TRIGGER theory_card_sources_file_work_match",
		"DEFERRABLE INITIALLY DEFERRED",
		"CREATE EXTENSION IF NOT EXISTS pg_trgm",
		"CREATE INDEX IF NOT EXISTS idx_theory_chunks_lexical_trgm ON theory_chunks USING gin ((title || ' ' || content || ' ' || keywords::text || ' ' || tags::text) gin_trgm_ops)",
		"CREATE INDEX IF NOT EXISTS idx_theory_cards_lexical_trgm ON theory_cards USING gin ((canonical_name || ' ' || aliases::text) gin_trgm_ops)",
		"idx_theory_libraries_created_by",
		"idx_theory_libraries_updated_by",
		"idx_theory_releases_activated_by",
		"idx_theory_source_works_library",
		"idx_theory_source_works_canonical_work",
		"idx_theory_source_files_work",
		"idx_theory_source_files_duplicate",
		"idx_theory_cards_library",
		"idx_theory_cards_reviewed_by",
		"idx_theory_cards_created_by",
		"idx_theory_cards_updated_by",
		"idx_theory_practices_card",
		"idx_theory_relations_from_card",
		"idx_theory_relations_to_card",
		"idx_theory_relations_created_by",
		"idx_theory_relations_reviewed_by",
		"idx_theory_card_sources_card",
		"idx_theory_card_sources_work",
		"idx_theory_card_sources_file",
		"idx_theory_card_sources_verified_by",
		"idx_theory_chunks_library",
		"idx_theory_chunks_card",
		"idx_theory_chunks_practice",
		"idx_theory_embeddings_chunk",
		"idx_theory_release_cards_release",
		"idx_theory_release_cards_card",
		"idx_theory_release_cards_chunk",
		"idx_theory_libraries_status",
		"idx_theory_libraries_update_time",
		"idx_theory_releases_status",
		"idx_theory_releases_update_time",
		"idx_theory_source_works_canonical_key",
		"idx_theory_source_works_status",
		"idx_theory_source_works_update_time",
		"idx_theory_source_files_sha256",
		"idx_theory_source_files_status",
		"idx_theory_source_files_update_time",
		"idx_theory_cards_canonical_key",
		"idx_theory_cards_status",
		"idx_theory_practices_status",
		"idx_theory_relations_status",
		"idx_theory_card_sources_update_time",
		"idx_theory_chunks_key",
		"idx_theory_chunks_status",
		"idx_theory_chunks_content_hash",
		"idx_theory_embeddings_status",
		"idx_theory_embeddings_content_hash",
	}
	for _, fragment := range required {
		if !strings.Contains(sqlText, fragment) {
			t.Errorf("missing %q", fragment)
		}
	}

	tableContracts := map[string][]string{
		"theory_libraries": {
			"CHECK (status IN ('draft','enabled','disabled'))",
		},
		"theory_library_releases": {
			"CHECK (status IN ('draft','building','ready','active','retired','failed'))",
			"CHECK (retrieval_mode IN ('lexical_only','hybrid'))",
		},
		"theory_source_works": {
			"CHECK (work_type IN ('book','course','handout','article','original_text','research','other'))",
			"CHECK (authority_level BETWEEN 1 AND 5)",
			"CHECK (status IN ('registered','extracting','reviewed','archived'))",
		},
		"theory_source_files": {
			"CHECK (extraction_class IN ('text_rich','mixed','image_dominant','cover_only'))",
			"CHECK (extraction_status IN ('pending','extracted','needs_ocr','ocr_running','review_required','failed'))",
			"CHECK (extraction_quality BETWEEN 0 AND 1)",
		},
		"theory_cards": {
			"CHECK (card_kind IN ('concept','claim','axis','stage','relation','profile','practice','warning'))",
			"CHECK (evidence_level IN ('strong','moderate','limited','traditional','experiential','unknown'))",
			"CHECK (clinical_safety IN ('general','caution','restricted','escalate'))",
			"CHECK (authority_level BETWEEN 1 AND 5)",
			"CHECK (status IN ('draft','in_review','published','superseded','retired'))",
		},
		"theory_practices": {
			"practice_schema_version TEXT NOT NULL DEFAULT 'xinzhili.practice.v1'",
			"CHECK (status IN ('draft','in_review','published','superseded','retired'))",
		},
		"theory_card_relations": {
			"CHECK (from_card_id <> to_card_id)",
			"CHECK (confidence BETWEEN 0 AND 1)",
			"CHECK (status IN ('draft','published','disabled'))",
		},
		"theory_card_sources": {
			"CHECK (source_role IN ('primary','supporting','extension','counterpoint','controversy'))",
			"CHECK (extraction_quality BETWEEN 0 AND 1)",
			"CHECK (page_start IS NULL OR page_start > 0)",
			"CHECK (page_end IS NULL OR page_end > 0)",
			"CHECK (page_end IS NULL OR page_start IS NULL OR page_end >= page_start)",
		},
		"theory_chunks": {
			"CHECK (authority_level BETWEEN 1 AND 5)",
			"CHECK (evidence_level IN ('strong','moderate','limited','traditional','experiential','unknown'))",
			"CHECK (clinical_safety IN ('general','caution','restricted','escalate'))",
			"CHECK (status IN ('enabled','disabled','retired'))",
		},
		"theory_chunk_embeddings": {
			"CHECK (dimensions = 1536)",
			"CHECK (status IN ('pending','ready','failed','stale'))",
		},
	}
	for table, fragments := range tableContracts {
		definition := createTableDefinition(t, sqlText, table)
		for _, fragment := range fragments {
			if !strings.Contains(definition, fragment) {
				t.Errorf("%s missing %q", table, fragment)
			}
		}
	}
}

func TestTheoryLibrarySchemaOrdersDependenciesAndProtectsOptionalExtensions(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.Join(strings.Fields(string(raw)), " ")

	tablesInDependencyOrder := []string{
		"theory_libraries",
		"theory_library_releases",
		"theory_source_works",
		"theory_source_files",
		"theory_cards",
		"theory_practices",
		"theory_card_relations",
		"theory_card_sources",
		"theory_chunks",
		"theory_chunk_embeddings",
		"theory_release_cards",
	}
	previous := -1
	for _, table := range tablesInDependencyOrder {
		position := strings.Index(sqlText, "CREATE TABLE IF NOT EXISTS "+table+" (")
		if position < 0 {
			t.Fatalf("schema missing %s", table)
		}
		if position <= previous {
			t.Fatalf("%s is out of dependency order", table)
		}
		previous = position
	}

	trgmBlock := schemaSection(t, sqlText,
		"-- 中文词法检索优先使用 pg_trgm",
		"-- ============ pgvector 向量检索")
	for _, fragment := range []string{
		"EXCEPTION WHEN OTHERS",
		"CREATE EXTENSION IF NOT EXISTS pg_trgm",
		"idx_theory_chunks_lexical_trgm",
		"idx_theory_cards_lexical_trgm",
	} {
		if !strings.Contains(trgmBlock, fragment) {
			t.Errorf("pg_trgm block missing %q", fragment)
		}
	}

	vectorBlock := schemaSection(t, sqlText,
		"-- ============ pgvector 向量检索",
		"-- ============ 成长心语")
	for _, fragment := range []string{
		"EXCEPTION WHEN OTHERS",
		"CREATE EXTENSION IF NOT EXISTS vector",
		"ALTER TABLE theory_chunk_embeddings ADD COLUMN IF NOT EXISTS embedding vector(1536)",
		"idx_theory_chunk_embeddings_hnsw",
	} {
		if !strings.Contains(vectorBlock, fragment) {
			t.Errorf("pgvector block missing %q", fragment)
		}
	}
}

func TestTheoryChunkReleaseOwnershipIsOnlyInReleaseCards(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.Join(strings.Fields(string(raw)), " ")

	for _, table := range []string{"theory_chunks", "theory_chunk_embeddings"} {
		definition := createTableDefinition(t, sqlText, table)
		if strings.Contains(definition, "release_id") {
			t.Errorf("%s must not contain release_id", table)
		}
	}

	releaseCards := createTableDefinition(t, sqlText, "theory_release_cards")
	if !strings.Contains(releaseCards, "release_id BIGINT NOT NULL") ||
		!strings.Contains(releaseCards, "chunk_id BIGINT NOT NULL") {
		t.Fatal("theory_release_cards must map releases to chunks")
	}
	if !strings.Contains(sqlText, "CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_release_chunk ON theory_release_cards(release_id, chunk_id)") {
		t.Fatal("theory_release_cards must uniquely map (release_id, chunk_id)")
	}
}

func createTableDefinition(t *testing.T, sqlText, table string) string {
	t.Helper()
	startMarker := "CREATE TABLE IF NOT EXISTS " + table + " ("
	start := strings.Index(sqlText, startMarker)
	if start < 0 {
		t.Fatalf("schema missing %s", table)
	}
	rest := sqlText[start:]
	end := strings.Index(rest, ");")
	if end < 0 {
		t.Fatalf("schema has unterminated %s definition", table)
	}
	return rest[:end]
}

func schemaSection(t *testing.T, sqlText, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(sqlText, startMarker)
	if start < 0 {
		t.Fatalf("schema missing section %q", startMarker)
	}
	end := strings.Index(sqlText[start:], endMarker)
	if end < 0 {
		t.Fatalf("schema section %q missing end marker %q", startMarker, endMarker)
	}
	return sqlText[start : start+end]
}
