package db

import (
	"context"
	"database/sql"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const theoryVerticalSliceSeedPath = "../../../../scripts/db/seed-xinzhili-theory-vertical-slice.sql"

func TestTheoryVerticalSliceSeedContract(t *testing.T) {
	raw, err := os.ReadFile(theoryVerticalSliceSeedPath)
	if err != nil {
		t.Fatalf("read vertical slice seed: %v", err)
	}
	seed := string(raw)
	normalized := strings.Join(strings.Fields(seed), " ")

	for _, required := range []string{
		"xinzhili",
		"inner_observer",
		"seed/han-teacher-course.md",
		"placeholder",
		"reviewed",
		"source_role",
		"primary",
		"quote_verified",
		"false",
		"extraction_quality",
		"published",
		"enabled",
		"lexical_only",
		"active",
		"embedding_dimensions",
		"1536",
		"card_count",
		"chunk_count",
		"current_version",
		"ON CONFLICT",
	} {
		if !strings.Contains(normalized, required) {
			t.Errorf("seed missing contract fragment %q", required)
		}
	}

	for _, stableHash := range regexp.MustCompile(`[a-f0-9]{64}`).FindAllString(seed, -1) {
		if stableHash != strings.ToLower(stableHash) {
			t.Errorf("hash is not lowercase: %q", stableHash)
		}
	}
	if len(regexp.MustCompile(`[a-f0-9]{64}`).FindAllString(seed, -1)) < 2 {
		t.Error("seed must contain stable SHA-256 values for file and chunk")
	}

	lower := strings.ToLower(seed)
	for _, forbidden := range []string{"openai", "embedding provider", "embeddings.create", "http://", "https://"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("seed must not call an embedding provider or external source: found %q", forbidden)
		}
	}
	if regexp.MustCompile(`(?is)quotation\s*[,)]`).FindString(seed) == "" ||
		!regexp.MustCompile(`(?is)quotation[^;]{0,500}''`).MatchString(seed) {
		t.Error("seed must explicitly leave quotation empty")
	}
	for _, match := range regexp.MustCompile(`'([^']|'')*'`).FindAllString(seed, -1) {
		if len([]rune(match)) > 240 {
			t.Errorf("seed contains an unexpectedly long SQL literal (%d runes); keep copyrighted text out of the seed", len([]rune(match)))
		}
	}
}

func TestTheoryVerticalSliceSeedExecutesTwice(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run the theory vertical slice seed integration test")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}

	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close PostgreSQL database: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)

	conn, err := database.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close PostgreSQL connection: %v", err)
		}
	})
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(782145901)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := conn.ExecContext(cleanupCtx, `SELECT pg_advisory_unlock(782145901)`); err != nil {
			t.Errorf("release theory seed advisory lock: %v", err)
		}
	})

	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(theoryVerticalSliceSeedPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := conn.ExecContext(ctx, string(schema)); err != nil {
			t.Fatalf("schema execution %d: %v", i+1, err)
		}
	}
	if err := cleanupTheoryVerticalSliceSeed(ctx, conn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if err := cleanupTheoryVerticalSliceSeed(cleanupCtx, database); err != nil {
			t.Errorf("clean up theory vertical slice seed: %v", err)
		}
	})
	for i := 0; i < 2; i++ {
		if _, err := conn.ExecContext(ctx, string(seed)); err != nil {
			t.Fatalf("seed execution %d: %v", i+1, err)
		}
	}

	var libraries, works, files, cards, sources, chunks, releases, mappings int
	err = conn.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM theory_libraries WHERE key='xinzhili'),
			(SELECT count(*) FROM theory_source_works w JOIN theory_libraries l ON l.id=w.library_id WHERE l.key='xinzhili'),
			(SELECT count(*) FROM theory_source_files f JOIN theory_source_works w ON w.id=f.work_id JOIN theory_libraries l ON l.id=w.library_id WHERE l.key='xinzhili'),
			(SELECT count(*) FROM theory_cards c JOIN theory_libraries l ON l.id=c.library_id WHERE l.key='xinzhili'),
			(SELECT count(*) FROM theory_card_sources s JOIN theory_cards c ON c.id=s.card_id JOIN theory_libraries l ON l.id=c.library_id WHERE l.key='xinzhili'),
			(SELECT count(*) FROM theory_chunks c JOIN theory_libraries l ON l.id=c.library_id WHERE l.key='xinzhili'),
			(SELECT count(*) FROM theory_library_releases r JOIN theory_libraries l ON l.id=r.library_id WHERE l.key='xinzhili'),
			(SELECT count(*) FROM theory_release_cards m JOIN theory_library_releases r ON r.id=m.release_id JOIN theory_libraries l ON l.id=r.library_id WHERE l.key='xinzhili')`).Scan(
		&libraries, &works, &files, &cards, &sources, &chunks, &releases, &mappings)
	if err != nil {
		t.Fatal(err)
	}
	for name, count := range map[string]int{
		"libraries": libraries, "works": works, "files": files, "cards": cards,
		"sources": sources, "chunks": chunks, "releases": releases, "mappings": mappings,
	} {
		if count != 1 {
			t.Errorf("%s count = %d, want 1 after two executions", name, count)
		}
	}

	var chainCount int
	err = conn.QueryRowContext(ctx, `
		SELECT count(*)
		FROM theory_libraries library
		JOIN theory_library_releases release ON release.library_id=library.id AND release.status='active'
		JOIN theory_release_cards mapping ON mapping.release_id=release.id
		JOIN theory_cards card ON card.id=mapping.card_id AND card.status='published'
		JOIN theory_chunks chunk ON chunk.id=mapping.chunk_id AND chunk.card_id=card.id AND chunk.status='enabled'
		JOIN theory_card_sources source ON source.card_id=card.id AND source.source_role='primary'
		JOIN theory_source_works work ON work.id=source.work_id AND work.library_id=library.id
		JOIN theory_source_files file ON file.id=source.file_id AND file.work_id=work.id
		WHERE library.key='xinzhili' AND library.current_version=1
		  AND release.version=1 AND release.retrieval_mode='lexical_only'
		  AND release.embedding_dimensions=1536 AND release.card_count=1 AND release.chunk_count=1
		  AND card.canonical_key='inner_observer' AND card.version=1
		  AND source.quotation='' AND source.quote_verified=false AND source.extraction_quality>=0.90`).Scan(&chainCount)
	if err != nil {
		t.Fatal(err)
	}
	if chainCount != 1 {
		t.Fatalf("active vertical slice chain count = %d, want 1", chainCount)
	}

	t.Run("seed preserves published card and enabled chunk snapshots", func(t *testing.T) {
		if _, err := conn.ExecContext(ctx, `
			UPDATE theory_cards
			SET definition='card-sentinel-definition'
			WHERE canonical_key='inner_observer' AND version=1 AND library_id=(SELECT id FROM theory_libraries WHERE key='xinzhili');
			UPDATE theory_chunks
			SET content='chunk-sentinel-content', content_hash=repeat('f', 64)
			WHERE chunk_key='inner_observer.card' AND version=1 AND library_id=(SELECT id FROM theory_libraries WHERE key='xinzhili')`); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, string(seed)); err != nil {
			t.Fatalf("seed execution after publishing sentinels: %v", err)
		}

		var definition, content, contentHash string
		if err := conn.QueryRowContext(ctx, `
			SELECT card.definition, chunk.content, chunk.content_hash
			FROM theory_libraries library
			JOIN theory_cards card ON card.library_id=library.id AND card.canonical_key='inner_observer' AND card.version=1
			JOIN theory_chunks chunk ON chunk.library_id=library.id AND chunk.chunk_key='inner_observer.card' AND chunk.version=1
			WHERE library.key='xinzhili'`).Scan(&definition, &content, &contentHash); err != nil {
			t.Fatal(err)
		}
		if definition != "card-sentinel-definition" || content != "chunk-sentinel-content" || contentHash != strings.Repeat("f", 64) {
			t.Fatalf("seed overwrote published snapshot: definition=%q content=%q hash=%q", definition, content, contentHash)
		}
	})

	t.Run("seed does not activate v1 after a higher current version", func(t *testing.T) {
		if _, err := conn.ExecContext(ctx, `
			UPDATE theory_library_releases release
			SET status='retired', update_time=now()
			FROM theory_libraries library
			WHERE release.library_id=library.id AND library.key='xinzhili' AND release.version=1;
			UPDATE theory_libraries SET current_version=2 WHERE key='xinzhili'`); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, string(seed)); err != nil {
			t.Fatalf("seed execution after retiring v1 at current version 2: %v", err)
		}

		var currentVersion, activeCount int
		if err := conn.QueryRowContext(ctx, `
			SELECT library.current_version,
			  count(release.id) FILTER (WHERE release.status='active')
			FROM theory_libraries library
			LEFT JOIN theory_library_releases release ON release.library_id=library.id
			WHERE library.key='xinzhili' GROUP BY library.id`).Scan(&currentVersion, &activeCount); err != nil {
			t.Fatal(err)
		}
		if currentVersion != 2 || activeCount != 0 {
			t.Fatalf("seed activated release after current version advanced: current=%d active=%d, want 2/0", currentVersion, activeCount)
		}
	})

	if _, err := conn.ExecContext(ctx, `
		UPDATE theory_library_releases release
		SET status='retired', update_time=now()
		FROM theory_libraries library
		WHERE release.library_id=library.id AND library.key='xinzhili' AND release.version=1;
		INSERT INTO theory_library_releases (
			library_id, version, status, embedding_model, embedding_dimensions, retrieval_mode,
			index_version, card_count, chunk_count, build_error, activated_at
		)
		SELECT id, 2, 'active', '', 1536, 'lexical_only', 'test-v2', 1, 1, '', now()
		FROM theory_libraries WHERE key='xinzhili';
		INSERT INTO theory_release_cards (release_id, card_id, chunk_id)
		SELECT release.id, card.id, chunk.id
		FROM theory_libraries library
		JOIN theory_library_releases release ON release.library_id=library.id AND release.version=2
		JOIN theory_cards card ON card.library_id=library.id
		  AND card.canonical_key='inner_observer' AND card.version=1
		JOIN theory_chunks chunk ON chunk.library_id=library.id
		  AND chunk.chunk_key='inner_observer.card' AND chunk.version=1
		WHERE library.key='xinzhili';
		UPDATE theory_libraries SET current_version=2 WHERE key='xinzhili'`); err != nil {
		t.Fatalf("create active v2 fixture: %v", err)
	}
	if _, err := conn.ExecContext(ctx, string(seed)); err != nil {
		t.Fatalf("seed execution with active v2: %v", err)
	}

	var currentVersion, activeVersion int
	if err := conn.QueryRowContext(ctx, `
		SELECT library.current_version, release.version
		FROM theory_libraries library
		JOIN theory_library_releases release ON release.library_id=library.id AND release.status='active'
		WHERE library.key='xinzhili'`).Scan(&currentVersion, &activeVersion); err != nil {
		t.Fatal(err)
	}
	if currentVersion != 2 || activeVersion != 2 {
		t.Fatalf("seed after active v2 left current/active version = %d/%d, want 2/2", currentVersion, activeVersion)
	}
}

type theoryVerticalSliceExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func cleanupTheoryVerticalSliceSeed(ctx context.Context, execer theoryVerticalSliceExecer) error {
	_, err := execer.ExecContext(ctx, `
		DELETE FROM theory_card_sources source
		USING theory_cards card, theory_libraries library
		WHERE source.card_id=card.id AND card.library_id=library.id AND library.key='xinzhili';
		DELETE FROM theory_libraries WHERE key='xinzhili'`)
	return err
}
