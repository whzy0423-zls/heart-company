package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"

	"github.com/jackc/pgx/v5/pgconn"
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
		"CREATE TABLE IF NOT EXISTS theory_package_imports",
		"CREATE TABLE IF NOT EXISTS theory_package_reviews",
		"CREATE TABLE IF NOT EXISTS theory_package_promotions",
		"UNIQUE (package_id)",
		"UNIQUE (import_id, review_type, content_digest)",
		"UNIQUE (import_id, content_digest)",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_active_release ON theory_library_releases(library_id) WHERE status = 'active'",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_published_card_key ON theory_cards(library_id, canonical_key) WHERE status = 'published'",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_release_chunk ON theory_release_cards(release_id, chunk_id)",
		"ALTER TABLE theory_chunk_embeddings ADD COLUMN IF NOT EXISTS embedding vector(1536)",
		"CREATE INDEX IF NOT EXISTS idx_theory_chunk_embeddings_hnsw ON theory_chunk_embeddings USING hnsw (embedding vector_cosine_ops)",
		"CREATE OR REPLACE FUNCTION validate_theory_library_ownership()",
		"DROP TRIGGER IF EXISTS theory_card_sources_file_work_match ON theory_card_sources",
		"CREATE CONSTRAINT TRIGGER theory_card_sources_file_work_match",
		"DEFERRABLE INITIALLY DEFERRED",
		"CREATE CONSTRAINT TRIGGER theory_card_relations_ownership",
		"CREATE CONSTRAINT TRIGGER theory_chunks_ownership",
		"CREATE CONSTRAINT TRIGGER theory_release_cards_ownership",
		"CREATE CONSTRAINT TRIGGER theory_cards_ownership_dependents",
		"CREATE CONSTRAINT TRIGGER theory_source_works_ownership_dependents",
		"CREATE CONSTRAINT TRIGGER theory_source_files_card_source_work_match",
		"CREATE CONSTRAINT TRIGGER theory_practices_ownership_dependents",
		"CREATE CONSTRAINT TRIGGER theory_library_releases_ownership_dependents",
		"theory ownership constraint: relation cards must belong to the same library",
		"theory ownership constraint: card source card, work, and file must share ownership",
		"theory ownership constraint: chunk library, card, and practice ownership mismatch",
		"theory ownership constraint: release card mapping ownership mismatch",
		"theory ownership constraint: canonical work must belong to the same library",
		"theory ownership constraint: duplicate source file must belong to the same library",
		"pg_advisory_xact_lock",
		"lock_theory_ownership_scope",
		"Replacement transaction order: supersede the old published card before publishing the new version.",
		"ADD CONSTRAINT ck_theory_source_works_authors_array",
		"ADD CONSTRAINT ck_theory_cards_aliases_array",
		"ADD CONSTRAINT ck_theory_practices_steps_array",
		"ADD CONSTRAINT ck_theory_chunks_keywords_array",
		"CREATE EXTENSION IF NOT EXISTS pg_trgm",
		"CREATE INDEX IF NOT EXISTS idx_theory_chunks_lexical_trgm",
		"CREATE INDEX IF NOT EXISTS idx_theory_cards_lexical_trgm",
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
		"idx_theory_package_imports_library",
		"idx_theory_package_reviews_import",
		"idx_theory_package_promotions_release",
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
			"CHECK (jsonb_typeof(authors) = 'array')",
			"CHECK (jsonb_typeof(editors) = 'array')",
			"CHECK (jsonb_typeof(translators) = 'array')",
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
			"CHECK (jsonb_typeof(aliases) = 'array')",
			"CHECK (jsonb_typeof(observable_signals) = 'array')",
			"CHECK (jsonb_typeof(common_triggers) = 'array')",
		},
		"theory_practices": {
			"practice_schema_version TEXT NOT NULL DEFAULT 'xinzhili.practice.v1'",
			"CHECK (status IN ('draft','in_review','published','superseded','retired'))",
			"CHECK (jsonb_typeof(steps) = 'array')",
			"CHECK (jsonb_typeof(reflection_prompts) = 'array')",
			"CHECK (jsonb_typeof(expected_feedback) = 'array')",
			"CHECK (jsonb_typeof(stop_conditions) = 'array')",
			"CHECK (jsonb_typeof(professional_escalation) = 'array')",
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
			"CHECK (jsonb_typeof(keywords) = 'array')",
			"CHECK (jsonb_typeof(tags) = 'array')",
		},
		"theory_chunk_embeddings": {
			"CHECK (dimensions = 1536)",
			"CHECK (status IN ('pending','ready','failed','stale'))",
		},
		"theory_package_imports": {
			"CHECK (state IN ('staged','promoted'))",
			"CHECK (jsonb_typeof(payload) = 'object')",
			"CHECK (jsonb_typeof(object_fingerprints) = 'object')",
		},
		"theory_package_reviews": {
			"CHECK (review_type IN ('source-verification','theory-review','safety-review'))",
			"CHECK (decision IN ('approved','rejected'))",
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
		"theory_package_imports",
		"theory_package_reviews",
		"theory_package_promotions",
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
		"JOIN pg_namespace",
		"gin_trgm_ops",
		"idx_theory_chunks_lexical_trgm",
		"idx_theory_cards_lexical_trgm",
	} {
		if !strings.Contains(trgmBlock, fragment) {
			t.Errorf("pg_trgm block missing %q", fragment)
		}
	}
	if strings.Count(trgmBlock, "EXCEPTION WHEN OTHERS") < 3 {
		t.Fatal("pg_trgm extension and both GIN indexes need independent exception handling")
	}
	if strings.Contains(sqlText, "LOCK TABLE theory_card_relations") {
		t.Fatal("ownership validation must not lock all theory tables")
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
	if strings.Count(vectorBlock, "EXCEPTION WHEN OTHERS") < 4 {
		t.Fatal("pgvector extension, theory vector column, HNSW index, and legacy rag index each need independent exception handling")
	}
	columnBlock := schemaSection(t, vectorBlock,
		"-- theory embedding column is independently optional",
		"-- theory HNSW index is independently optional")
	if !strings.Contains(columnBlock, "EXCEPTION WHEN OTHERS") ||
		!strings.Contains(columnBlock, "ALTER TABLE theory_chunk_embeddings ADD COLUMN IF NOT EXISTS embedding vector(1536)") {
		t.Fatal("theory embedding column DDL must be independently exception-safe")
	}
	hnswBlock := schemaSection(t, vectorBlock,
		"-- theory HNSW index is independently optional",
		"END $$;")
	if !strings.Contains(hnswBlock, "EXCEPTION WHEN OTHERS") ||
		!strings.Contains(hnswBlock, "CREATE INDEX IF NOT EXISTS idx_theory_chunk_embeddings_hnsw") {
		t.Fatal("theory HNSW DDL must be independently exception-safe")
	}
}

func TestTheoryLibraryOwnershipConstraints(t *testing.T) {
	database := openTheorySchemaTestDB(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var libraryA, libraryB int64
	for _, item := range []struct {
		key string
		id  *int64
	}{{"ownership-a-" + suffix, &libraryA}, {"ownership-b-" + suffix, &libraryB}} {
		if err := database.QueryRowContext(ctx,
			`INSERT INTO theory_libraries (key, name, status) VALUES ($1, $1, 'enabled') RETURNING id`, item.key,
		).Scan(item.id); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM theory_libraries WHERE id IN ($1,$2)`, libraryA, libraryB)
	})

	workA := insertTheoryWork(t, database, libraryA, "work-a-"+suffix)
	workB := insertTheoryWork(t, database, libraryB, "work-b-"+suffix)
	fileA := insertTheoryFile(t, database, workA, "file-a-"+suffix)
	fileB := insertTheoryFile(t, database, workB, "file-b-"+suffix)
	cardA := insertTheoryCard(t, database, libraryA, "card-a-"+suffix)
	cardB := insertTheoryCard(t, database, libraryB, "card-b-"+suffix)
	practiceB := insertTheoryPractice(t, database, cardB)
	chunkA := insertTheoryChunk(t, database, libraryA, cardA, nil, "chunk-a-"+suffix)
	chunkB := insertTheoryChunk(t, database, libraryB, cardB, &practiceB, "chunk-b-"+suffix)
	releaseA := insertTheoryRelease(t, database, libraryA)

	t.Run("relation cards share a library", func(t *testing.T) {
		expectDeferredConstraintFailure(t, database, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO theory_card_relations (from_card_id, to_card_id, relation_type) VALUES ($1,$2,'supports')`, cardA, cardB)
			return err
		})
	})
	t.Run("card source card and work share a library", func(t *testing.T) {
		expectDeferredConstraintFailure(t, database, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO theory_card_sources (card_id, work_id, source_role) VALUES ($1,$2,'primary')`, cardA, workB)
			return err
		})
	})
	t.Run("card source file belongs to work", func(t *testing.T) {
		expectDeferredConstraintFailure(t, database, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO theory_card_sources (card_id, work_id, file_id, source_role) VALUES ($1,$2,$3,'primary')`, cardA, workA, fileB)
			return err
		})
	})
	t.Run("chunk card belongs to library", func(t *testing.T) {
		expectDeferredConstraintFailure(t, database, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO theory_chunks (library_id, card_id, chunk_key, chunk_kind, title, content, authority_level, evidence_level, clinical_safety, content_hash) VALUES ($1,$2,$3,'card','x','x',1,'unknown','general',$3)`, libraryB, cardA, "bad-library-"+suffix)
			return err
		})
	})
	t.Run("chunk practice belongs to card", func(t *testing.T) {
		expectDeferredConstraintFailure(t, database, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO theory_chunks (library_id, card_id, practice_id, chunk_key, chunk_kind, title, content, authority_level, evidence_level, clinical_safety, content_hash) VALUES ($1,$2,$3,$4,'practice','x','x',1,'unknown','general',$4)`, libraryA, cardA, practiceB, "bad-practice-"+suffix)
			return err
		})
	})
	t.Run("release card and chunk share ownership", func(t *testing.T) {
		expectDeferredConstraintFailure(t, database, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO theory_release_cards (release_id, card_id, chunk_id) VALUES ($1,$2,$3)`, releaseA, cardA, chunkB)
			return err
		})
	})
	t.Run("canonical work stays in the same library", func(t *testing.T) {
		expectDeferredConstraintFailure(t, database, func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE theory_source_works SET canonical_work_id=$1 WHERE id=$2`, workB, workA)
			return err
		})
	})
	t.Run("duplicate file stays in the same library", func(t *testing.T) {
		expectDeferredConstraintFailure(t, database, func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE theory_source_files SET duplicate_of_file_id=$1 WHERE id=$2`, fileB, fileA)
			return err
		})
	})

	if _, err := database.ExecContext(ctx, `INSERT INTO theory_release_cards (release_id, card_id, chunk_id) VALUES ($1,$2,$3)`, releaseA, cardA, chunkA); err != nil {
		t.Fatalf("valid ownership mapping rejected: %v", err)
	}

	t.Run("coordinated ownership updates can finish within one transaction", func(t *testing.T) {
		left := insertTheoryCard(t, database, libraryA, "move-left-"+suffix)
		right := insertTheoryCard(t, database, libraryA, "move-right-"+suffix)
		if _, err := database.ExecContext(ctx, `INSERT INTO theory_card_relations (from_card_id, to_card_id, relation_type) VALUES ($1,$2,'supports')`, left, right); err != nil {
			t.Fatal(err)
		}
		tx, err := database.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`UPDATE theory_cards SET library_id=$1 WHERE id=$2`, libraryB, left); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if _, err := tx.Exec(`UPDATE theory_cards SET library_id=$1 WHERE id=$2`, libraryB, right); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("coordinated deferred update rejected: %v", err)
		}
	})

	t.Run("published replacement supersedes old version first", func(t *testing.T) {
		key := "published-replacement-" + suffix
		oldID := insertPublishedTheoryCard(t, database, libraryA, key, 1)
		tx, err := database.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`
			INSERT INTO theory_cards
			  (library_id, canonical_key, canonical_name, card_kind, epistemic_status, evidence_level, clinical_safety, authority_level, status, version)
			VALUES ($1,$2,$2,'concept','source_text','unknown','general',1,'published',2)
		`, libraryA, key); err == nil {
			_ = tx.Rollback()
			t.Fatal("expected simultaneous published versions to be rejected")
		}
		_ = tx.Rollback()

		tx, err = database.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`UPDATE theory_cards SET status='superseded' WHERE id=$1`, oldID); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if _, err := tx.Exec(`
			INSERT INTO theory_cards
			  (library_id, canonical_key, canonical_name, card_kind, epistemic_status, evidence_level, clinical_safety, authority_level, status, version)
			VALUES ($1,$2,$2,'concept','source_text','unknown','general',1,'published',2)
		`, libraryA, key); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("ordered published replacement rejected: %v", err)
		}
	})
}

func TestTheoryLibraryArrayChecksUpgradeExistingTables(t *testing.T) {
	database := openTheorySchemaTestDB(t)
	ctx := context.Background()
	constraints := []struct {
		table string
		name  string
	}{
		{"theory_source_works", "ck_theory_source_works_authors_array"},
		{"theory_source_works", "ck_theory_source_works_editors_array"},
		{"theory_source_works", "ck_theory_source_works_translators_array"},
		{"theory_cards", "ck_theory_cards_aliases_array"},
		{"theory_cards", "ck_theory_cards_observable_signals_array"},
		{"theory_cards", "ck_theory_cards_common_triggers_array"},
		{"theory_practices", "ck_theory_practices_steps_array"},
		{"theory_practices", "ck_theory_practices_reflection_prompts_array"},
		{"theory_practices", "ck_theory_practices_expected_feedback_array"},
		{"theory_practices", "ck_theory_practices_stop_conditions_array"},
		{"theory_practices", "ck_theory_practices_professional_escalation_array"},
		{"theory_chunks", "ck_theory_chunks_keywords_array"},
		{"theory_chunks", "ck_theory_chunks_tags_array"},
	}
	for _, constraint := range constraints {
		if _, err := database.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s`, constraint.table, constraint.name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
		t.Fatalf("reapply schema after removing array checks: %v", err)
	}
	for _, constraint := range constraints {
		var count int
		if err := database.QueryRowContext(ctx, `SELECT count(*) FROM pg_constraint WHERE conname=$1`, constraint.name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("expected upgraded array check %s, got %d", constraint.name, count)
		}
	}
}

func TestTheoryLibraryConcurrentOwnershipWritesCannotBothCommit(t *testing.T) {
	database := openTheorySchemaTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var libraryA, libraryB int64
	if err := database.QueryRowContext(ctx, `INSERT INTO theory_libraries (key,name) VALUES ($1,$1) RETURNING id`, "concurrency-a-"+suffix).Scan(&libraryA); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO theory_libraries (key,name) VALUES ($1,$1) RETURNING id`, "concurrency-b-"+suffix).Scan(&libraryB); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM theory_libraries WHERE id IN ($1,$2)`, libraryA, libraryB)
	})
	workA := insertTheoryWork(t, database, libraryA, "concurrency-work-a-"+suffix)
	workB := insertTheoryWork(t, database, libraryB, "concurrency-work-b-"+suffix)
	fileA := insertTheoryFile(t, database, workA, "concurrency-file-a-"+suffix)
	cardA := insertTheoryCard(t, database, libraryA, "concurrency-card-a-"+suffix)

	sourceTx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourceTx.ExecContext(ctx, `INSERT INTO theory_card_sources (card_id,work_id,file_id,source_role) VALUES ($1,$2,$3,'primary')`, cardA, workA, fileA); err != nil {
		_ = sourceTx.Rollback()
		t.Fatal(err)
	}
	fileTx, err := database.BeginTx(ctx, nil)
	if err != nil {
		_ = sourceTx.Rollback()
		t.Fatal(err)
	}
	started := make(chan struct{})
	fileResult := make(chan error, 1)
	go func() {
		close(started)
		if _, err := fileTx.ExecContext(ctx, `UPDATE theory_source_files SET work_id=$1 WHERE id=$2`, workB, fileA); err != nil {
			_ = fileTx.Rollback()
			fileResult <- err
			return
		}
		fileResult <- fileTx.Commit()
	}()
	<-started
	if err := sourceTx.Commit(); err != nil {
		_ = fileTx.Rollback()
		t.Fatalf("first ownership transaction must commit: %v", err)
	}
	conflictErr := <-fileResult
	if conflictErr == nil {
		t.Fatal("conflicting ownership transaction unexpectedly committed")
	}
	assertOwnershipConstraintError(t, conflictErr)

	var invalid int
	if err := database.QueryRowContext(ctx, `
		SELECT count(*)
		FROM theory_card_sources source
		JOIN theory_source_files file ON file.id=source.file_id
		WHERE file.work_id IS DISTINCT FROM source.work_id
	`).Scan(&invalid); err != nil {
		t.Fatal(err)
	}
	if invalid != 0 {
		t.Fatalf("concurrent writes left %d mismatched card-source files", invalid)
	}
}

func TestTheoryLibraryUnrelatedConcurrentWritesBothCommit(t *testing.T) {
	database := openTheorySchemaTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var libraryA, libraryB int64
	if err := database.QueryRowContext(ctx, `INSERT INTO theory_libraries (key,name) VALUES ($1,$1) RETURNING id`, "unrelated-a-"+suffix).Scan(&libraryA); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO theory_libraries (key,name) VALUES ($1,$1) RETURNING id`, "unrelated-b-"+suffix).Scan(&libraryB); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM theory_libraries WHERE id IN ($1,$2)`, libraryA, libraryB)
	})

	txA, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	txB, err := database.BeginTx(ctx, nil)
	if err != nil {
		_ = txA.Rollback()
		t.Fatal(err)
	}
	if _, err := txA.ExecContext(ctx, `INSERT INTO theory_source_works (library_id,canonical_key,title,work_type,authority_level,epistemic_status,copyright_scope) VALUES ($1,$2,$2,'book',1,'source_text','metadata_only')`, libraryA, "unrelated-work-a-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := txB.ExecContext(ctx, `INSERT INTO theory_source_works (library_id,canonical_key,title,work_type,authority_level,epistemic_status,copyright_scope) VALUES ($1,$2,$2,'book',1,'source_text','metadata_only')`, libraryB, "unrelated-work-b-"+suffix); err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	go func() { results <- txA.Commit() }()
	go func() { results <- txB.Commit() }()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("unrelated library write failed: %v", err)
		}
	}
}

func TestTheoryLibraryPgTrgmCustomSchemaIsOptional(t *testing.T) {
	database := openTheorySchemaTestDB(t)
	ctx := context.Background()
	var ownsExtension bool
	if err := database.QueryRowContext(ctx, `
		SELECT pg_get_userbyid(extowner) = current_user
		FROM pg_extension
		WHERE extname='pg_trgm'
	`).Scan(&ownsExtension); err != nil {
		t.Fatal(err)
	}
	if !ownsExtension {
		t.Skip("custom-schema pg_trgm relocation requires the test user to own the extension")
	}
	if _, err := database.ExecContext(ctx, `
		DROP INDEX IF EXISTS idx_theory_chunks_lexical_trgm;
		DROP INDEX IF EXISTS idx_theory_cards_lexical_trgm;
		CREATE SCHEMA IF NOT EXISTS extensions;
		ALTER EXTENSION pg_trgm SET SCHEMA extensions;
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `ALTER EXTENSION pg_trgm SET SCHEMA public`)
	})
	if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
		t.Fatalf("schema must survive pg_trgm outside search_path: %v", err)
	}
	var count int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM pg_indexes WHERE indexname IN ('idx_theory_chunks_lexical_trgm','idx_theory_cards_lexical_trgm')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected both schema-qualified trigram indexes, got %d", count)
	}
}

func TestTheoryLibraryTriggerCreationIsTableScoped(t *testing.T) {
	database := openTheorySchemaTestDB(t)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS theory_trigger_name_probe (id BIGSERIAL PRIMARY KEY);
		CREATE OR REPLACE FUNCTION theory_trigger_name_probe_fn() RETURNS trigger AS $$ BEGIN RETURN NEW; END $$ LANGUAGE plpgsql;
		DROP TRIGGER IF EXISTS theory_card_sources_file_work_match ON theory_trigger_name_probe;
		CREATE TRIGGER theory_card_sources_file_work_match BEFORE INSERT ON theory_trigger_name_probe FOR EACH ROW EXECUTE FUNCTION theory_trigger_name_probe_fn();
		DROP TRIGGER IF EXISTS theory_card_sources_file_work_match ON theory_card_sources;
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DROP TABLE IF EXISTS theory_trigger_name_probe CASCADE`)
	})

	if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
		t.Fatalf("rerun schema with unrelated same-name trigger: %v", err)
	}
	var count int
	if err := database.QueryRowContext(ctx, `
		SELECT count(*) FROM pg_trigger
		WHERE tgname='theory_card_sources_file_work_match'
		  AND tgrelid='theory_card_sources'::regclass
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected target-table trigger to be recreated, got %d", count)
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

func openTheorySchemaTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run theory schema integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
		t.Fatalf("reapply schema: %v", err)
	}
	return database
}

func insertTheoryWork(t *testing.T, database *sql.DB, libraryID int64, key string) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`
		INSERT INTO theory_source_works
		  (library_id, canonical_key, title, work_type, authority_level, epistemic_status, copyright_scope)
		VALUES ($1,$2,$2,'book',1,'source_text','metadata_only') RETURNING id
	`, libraryID, key).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertTheoryFile(t *testing.T, database *sql.DB, workID int64, name string) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`
		INSERT INTO theory_source_files
		  (work_id, relative_path, original_filename, file_format, sha256, title_source, extraction_class)
		VALUES ($1,$2,$2,'pdf',$2,'filename','text_rich') RETURNING id
	`, workID, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertTheoryCard(t *testing.T, database *sql.DB, libraryID int64, key string) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`
		INSERT INTO theory_cards
		  (library_id, canonical_key, canonical_name, card_kind, epistemic_status, evidence_level, clinical_safety, authority_level)
		VALUES ($1,$2,$2,'concept','source_text','unknown','general',1) RETURNING id
	`, libraryID, key).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertPublishedTheoryCard(t *testing.T, database *sql.DB, libraryID int64, key string, version int) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`
		INSERT INTO theory_cards
		  (library_id, canonical_key, canonical_name, card_kind, epistemic_status, evidence_level, clinical_safety, authority_level, status, version)
		VALUES ($1,$2,$2,'concept','source_text','unknown','general',1,'published',$3) RETURNING id
	`, libraryID, key, version).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertTheoryPractice(t *testing.T, database *sql.DB, cardID int64) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`INSERT INTO theory_practices (card_id, goal) VALUES ($1,'goal') RETURNING id`, cardID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertTheoryChunk(t *testing.T, database *sql.DB, libraryID, cardID int64, practiceID *int64, key string) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`
		INSERT INTO theory_chunks
		  (library_id, card_id, practice_id, chunk_key, chunk_kind, title, content, authority_level, evidence_level, clinical_safety, content_hash)
		VALUES ($1,$2,$3,$4,'card',$4,$4,1,'unknown','general',$4) RETURNING id
	`, libraryID, cardID, practiceID, key).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertTheoryRelease(t *testing.T, database *sql.DB, libraryID int64) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`INSERT INTO theory_library_releases (library_id, version) VALUES ($1,1) RETURNING id`, libraryID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func expectDeferredConstraintFailure(t *testing.T, database *sql.DB, insert func(*sql.Tx) error) {
	t.Helper()
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := insert(tx); err != nil {
		_ = tx.Rollback()
		t.Fatalf("expected deferred check to allow statement, got %v", err)
	}
	if err := tx.Commit(); err == nil {
		t.Fatal("expected deferred ownership constraint failure at commit")
	}
}

func assertOwnershipConstraintError(t *testing.T, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected PostgreSQL ownership error, got %T: %v", err, err)
	}
	if pgErr.Code != "23514" {
		t.Fatalf("expected ownership SQLSTATE 23514, got %s: %v", pgErr.Code, err)
	}
	if !strings.Contains(pgErr.Message, "theory ownership constraint") {
		t.Fatalf("expected ownership constraint message, got %q", pgErr.Message)
	}
}
