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
	t.Cleanup(func() { _ = database.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)

	conn, err := database.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(782145901)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(782145901)`) })

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
		_ = cleanupTheoryVerticalSliceSeed(context.Background(), database)
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
