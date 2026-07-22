package theorystore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var postgresFixtureSequence atomic.Uint64

type postgresVerticalFixture struct {
	libraryID  int64
	libraryKey string
	actorID    int64
	work       SourceWork
	file       SourceFile
	card       Card
	chunk      Chunk
	release    Release
}

func TestTheoryStorePostgresVerticalSlice(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run theorystore PostgreSQL integration tests")
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

	executeTheorySchemaTwice(t, ctx, database)
	store := NewStore(database)
	fixture := createPostgresVerticalFixture(t, ctx, database, store, "lexical")

	t.Run("store API creates an active queryable chain", func(t *testing.T) {
		assertActivePostgresChain(t, ctx, database, fixture.libraryID, fixture.release.Version)
	})

	t.Run("database constraints reject invalid and cross-library rows", func(t *testing.T) {
		assertPostgresConstraintFailures(t, ctx, database, store, fixture)
	})

	t.Run("concurrent activation keeps highest successful version active", func(t *testing.T) {
		low := buildPostgresRelease(t, ctx, store, fixture, 2, RetrievalLexicalOnly, "")
		high := buildPostgresRelease(t, ctx, store, fixture, 3, RetrievalLexicalOnly, "")
		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for _, release := range []Release{low, high} {
			wg.Add(1)
			go func(release Release) {
				defer wg.Done()
				<-start
				errs <- store.ActivateRelease(ctx, fixture.libraryID, release.ID, fixture.actorID)
			}(release)
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil && !errors.Is(err, ErrReleaseVersionConflict) {
				t.Fatalf("concurrent ActivateRelease error = %v", err)
			}
		}
		var currentVersion, activeCount, activeVersion int
		if err := database.QueryRowContext(ctx, `
			SELECT library.current_version,
			  count(*) FILTER (WHERE release.status='active'),
			  COALESCE(max(release.version) FILTER (WHERE release.status='active'), 0)
			FROM theory_libraries library
			LEFT JOIN theory_library_releases release ON release.library_id=library.id
			WHERE library.id=$1 GROUP BY library.id`, fixture.libraryID).Scan(&currentVersion, &activeCount, &activeVersion); err != nil {
			t.Fatal(err)
		}
		if activeCount != 1 || currentVersion != 3 || activeVersion != 3 {
			t.Fatalf("current=%d active_count=%d active_version=%d, want 3/1/3", currentVersion, activeCount, activeVersion)
		}
	})

	t.Run("activation rollback restores old active release", func(t *testing.T) {
		candidate := buildPostgresRelease(t, ctx, store, fixture, 4, RetrievalLexicalOnly, "")
		suffix := postgresFixtureSequence.Add(1)
		functionName := fmt.Sprintf("test_fail_theory_library_update_%d", suffix)
		triggerName := fmt.Sprintf("test_fail_theory_library_update_%d", suffix)
		ddl := fmt.Sprintf(`
			CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
			  IF NEW.id = %d AND NEW.current_version <> OLD.current_version THEN
			    RAISE EXCEPTION 'injected current_version failure';
			  END IF;
			  RETURN NEW;
			END $$;
			CREATE TRIGGER %s BEFORE UPDATE ON theory_libraries
			FOR EACH ROW EXECUTE FUNCTION %s()`, functionName, fixture.libraryID, triggerName, functionName)
		if _, err := database.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = database.ExecContext(context.Background(), fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON theory_libraries; DROP FUNCTION IF EXISTS %s()", triggerName, functionName))
		})

		err := store.ActivateRelease(ctx, fixture.libraryID, candidate.ID, fixture.actorID)
		if err == nil {
			t.Fatal("ActivateRelease succeeded despite injected library update failure")
		}
		if _, err := database.ExecContext(ctx, fmt.Sprintf("DROP TRIGGER %s ON theory_libraries; DROP FUNCTION %s()", triggerName, functionName)); err != nil {
			t.Fatal(err)
		}
		var currentVersion int
		var oldStatus, newStatus ReleaseStatus
		if err := database.QueryRowContext(ctx, `
			SELECT library.current_version,
			  (SELECT status FROM theory_library_releases WHERE library_id=library.id AND version=3),
			  (SELECT status FROM theory_library_releases WHERE id=$2)
			FROM theory_libraries library WHERE library.id=$1`, fixture.libraryID, candidate.ID).Scan(&currentVersion, &oldStatus, &newStatus); err != nil {
			t.Fatal(err)
		}
		if currentVersion != 3 || oldStatus != ReleaseStatusActive || newStatus != ReleaseStatusReady {
			t.Fatalf("rollback state current=%d old=%s new=%s, want 3/active/ready", currentVersion, oldStatus, newStatus)
		}
	})

	t.Run("hybrid release uses deterministic local vector when pgvector exists", func(t *testing.T) {
		var vectorColumn bool
		if err := database.QueryRowContext(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM information_schema.columns
			  WHERE table_schema=current_schema() AND table_name='theory_chunk_embeddings' AND column_name='embedding'
			)`).Scan(&vectorColumn); err != nil {
			t.Fatal(err)
		}
		if !vectorColumn {
			t.Skip("pgvector embedding column is unavailable; hybrid path intentionally skipped")
		}
		model := "test-fixture-1536"
		pending, err := store.SaveEmbeddingRecord(ctx, EmbeddingRecord{
			ChunkID: fixture.chunk.ID, EmbeddingModel: model, Dimensions: 1536,
			ContentHash: fixture.chunk.ContentHash, Status: EmbeddingStatusPending,
		})
		if err != nil {
			t.Fatal(err)
		}
		vector := make([]float32, 1536)
		for i := range vector {
			vector[i] = float32((i%17)-8) / 100
		}
		now := time.Now().UTC()
		ready, err := store.SaveEmbeddingRecord(ctx, EmbeddingRecord{
			ID: pending.ID, ChunkID: fixture.chunk.ID, EmbeddingModel: model, Dimensions: 1536,
			Embedding: vector, ContentHash: fixture.chunk.ContentHash, EmbeddedAt: &now, Status: EmbeddingStatusReady,
		})
		if err != nil {
			t.Fatal(err)
		}
		if ready.Status != EmbeddingStatusReady {
			t.Fatalf("embedding status = %s, want ready", ready.Status)
		}
		release := buildPostgresRelease(t, ctx, store, fixture, 5, RetrievalHybrid, model)
		if err := store.ActivateRelease(ctx, fixture.libraryID, release.ID, fixture.actorID); err != nil {
			t.Fatal(err)
		}
		assertActivePostgresChain(t, ctx, database, fixture.libraryID, 5)
	})
}

func executeTheorySchemaTwice(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := database.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(782145901)`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(782145901)`) }()
	for i := 0; i < 2; i++ {
		if _, err := conn.ExecContext(ctx, string(raw)); err != nil {
			t.Fatalf("schema execution %d: %v", i+1, err)
		}
	}
}

func createPostgresVerticalFixture(t *testing.T, ctx context.Context, database *sql.DB, store *Store, label string) postgresVerticalFixture {
	t.Helper()
	suffix := fmt.Sprintf("%d_%d", time.Now().UnixNano(), postgresFixtureSequence.Add(1))
	key := "test_theory_" + label + "_" + suffix
	username := "test_theory_actor_" + suffix
	var actorID, libraryID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO users (username,password_hash,nickname,status) VALUES ($1,'test','test',1) RETURNING id`, username).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO theory_libraries (key,name,status,default_language) VALUES ($1,$2,'enabled','zh-CN') RETURNING id`, key, key).Scan(&libraryID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM theory_libraries WHERE key=$1`, key)
		_, _ = database.ExecContext(context.Background(), `DELETE FROM users WHERE username=$1`, username)
	})

	work, err := store.RegisterWork(ctx, SourceWork{
		LibraryID: libraryID, CanonicalKey: "work", Title: "测试课程", Authors: json.RawMessage(`["测试作者"]`),
		Editors: json.RawMessage(`[]`), Translators: json.RawMessage(`[]`), WorkType: WorkTypeCourse,
		AuthorityLevel: 5, EpistemicStatus: EpistemicCourseAdaptation, CopyrightScope: CopyrightMetadataOnly,
		Metadata: json.RawMessage(`{"test":true}`), Status: SourceWorkStatusReviewed,
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.RegisterFile(ctx, SourceFile{
		WorkID: work.ID, RelativePath: "test/fixture.md", OriginalFilename: "fixture.md", FileFormat: "md",
		MIMEType: "text/markdown", SHA256: strings.Repeat("1", 64), TitleSource: TitleSourceManual,
		ExtractionClass: ExtractionClassTextRich, ExtractionStatus: ExtractionStatusExtracted,
		ExtractionQuality: 0.95, Metadata: json.RawMessage(`{"test":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	card, err := store.CreateCard(ctx, Card{
		LibraryID: libraryID, CanonicalKey: "observer", CanonicalName: "观察者", Aliases: json.RawMessage(`[]`),
		CardKind: CardKindConcept, Summary: "观察反应", Definition: "观察此刻的自动反应。",
		CoreClaim: "觉察带来选择空间。", Mechanism: "暂停后识别反应。", ApplicableContext: "日常压力。",
		NonApplicableContext: "不替代专业诊疗。", ObservableSignals: json.RawMessage(`[]`), CommonTriggers: json.RawMessage(`[]`),
		EpistemicStatus: EpistemicCourseAdaptation, EvidenceLevel: EvidenceExperiential,
		ClinicalSafety: ClinicalGeneral, AuthorityLevel: 5, Language: "zh-CN", Status: StatusDraft, Version: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveCardSource(ctx, CardSource{
		CardID: card.ID, WorkID: work.ID, FileID: &file.ID, SourceRole: SourceRolePrimary,
		InterpretationNote: "测试自有摘要", ExtractionQuality: 0.95,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionCard(ctx, card.ID, StatusDraft, StatusInReview, actorID); err != nil {
		t.Fatal(err)
	}
	card, err = store.TransitionCard(ctx, card.ID, StatusInReview, StatusPublished, actorID)
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := store.SaveChunk(ctx, Chunk{
		LibraryID: libraryID, CardID: card.ID, ChunkKey: "observer.card", ChunkKind: ChunkKindCard,
		Title: "观察者", Content: "观察当下反应并恢复选择空间。", Keywords: json.RawMessage(`["观察"]`), Tags: json.RawMessage(`[]`),
		AuthorityLevel: 5, EvidenceLevel: EvidenceExperiential, ClinicalSafety: ClinicalGeneral,
		TokenCount: 16, ContentHash: strings.Repeat("2", 64), Version: card.Version, Status: ChunkStatusEnabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := postgresVerticalFixture{libraryID: libraryID, libraryKey: key, actorID: actorID, work: work, file: file, card: card, chunk: chunk}
	fixture.release = buildPostgresRelease(t, ctx, store, fixture, 1, RetrievalLexicalOnly, "")
	if err := store.ActivateRelease(ctx, libraryID, fixture.release.ID, actorID); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func buildPostgresRelease(t *testing.T, ctx context.Context, store *Store, fixture postgresVerticalFixture, version int, mode RetrievalMode, model string) Release {
	t.Helper()
	release, err := store.BuildRelease(ctx, Release{
		LibraryID: fixture.libraryID, Version: version, Status: ReleaseStatusDraft,
		EmbeddingModel: model, EmbeddingDimensions: 1536, RetrievalMode: mode,
		IndexVersion: fmt.Sprintf("test-v%d", version),
	}, []ReleaseMapping{{CardID: fixture.card.ID, ChunkID: fixture.chunk.ID}})
	if err != nil {
		t.Fatal(err)
	}
	return release
}

func assertActivePostgresChain(t *testing.T, ctx context.Context, database *sql.DB, libraryID int64, version int) {
	t.Helper()
	var count int
	err := database.QueryRowContext(ctx, `
		SELECT count(*) FROM theory_libraries library
		JOIN theory_library_releases release ON release.library_id=library.id AND release.status='active'
		JOIN theory_release_cards mapping ON mapping.release_id=release.id
		JOIN theory_cards card ON card.id=mapping.card_id AND card.status='published'
		JOIN theory_chunks chunk ON chunk.id=mapping.chunk_id AND chunk.card_id=card.id AND chunk.status='enabled'
		JOIN theory_card_sources source ON source.card_id=card.id AND source.source_role='primary'
		JOIN theory_source_works work ON work.id=source.work_id AND work.library_id=library.id
		JOIN theory_source_files file ON file.id=source.file_id AND file.work_id=work.id
		WHERE library.id=$1 AND library.current_version=$2 AND release.version=$2`, libraryID, version).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("active chain count = %d, want 1", count)
	}
}

func assertPostgresConstraintFailures(t *testing.T, ctx context.Context, database *sql.DB, store *Store, fixture postgresVerticalFixture) {
	t.Helper()
	_, err := database.ExecContext(ctx, `INSERT INTO theory_libraries (key,name,status) VALUES ($1,'bad','invalid')`, fixture.libraryKey+"_bad")
	requirePostgresCode(t, err, "23514")

	_, err = database.ExecContext(ctx, `
		INSERT INTO theory_source_files (work_id,relative_path,original_filename,file_format,page_count,sha256,title_source,extraction_class,extraction_status,extraction_quality)
		VALUES ($1,'bad-page.md','bad-page.md','md',0,$2,'manual','text_rich','extracted',0.95)`, fixture.work.ID, strings.Repeat("3", 64))
	requirePostgresCode(t, err, "23514")
	_, err = database.ExecContext(ctx, `
		INSERT INTO theory_source_files (work_id,relative_path,original_filename,file_format,sha256,title_source,extraction_class,extraction_status,extraction_quality)
		VALUES ($1,'bad-quality.md','bad-quality.md','md',$2,'manual','text_rich','extracted',1.1)`, fixture.work.ID, strings.Repeat("4", 64))
	requirePostgresCode(t, err, "23514")

	other := createPostgresVerticalFixture(t, ctx, database, store, "cross")
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO theory_source_files (work_id,relative_path,original_filename,file_format,sha256,duplicate_of_file_id,title_source,extraction_class,extraction_status,extraction_quality)
		VALUES ($1,'duplicate.md','duplicate.md','md',$2,$3,'manual','text_rich','extracted',0.95)`, other.work.ID, fixture.file.SHA256, fixture.file.ID)
	if err == nil {
		err = tx.Commit()
	} else {
		_ = tx.Rollback()
	}
	requirePostgresCode(t, err, "P0001")

	tx, err = database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO theory_card_relations (from_card_id,to_card_id,relation_type,confidence,status) VALUES ($1,$2,'supports',0.8,'published')`, fixture.card.ID, other.card.ID)
	if err == nil {
		err = tx.Commit()
	} else {
		_ = tx.Rollback()
	}
	requirePostgresCode(t, err, "P0001")

	tx, err = database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO theory_release_cards (release_id,card_id,chunk_id) VALUES ($1,$2,$3)`, fixture.release.ID, other.card.ID, other.chunk.ID)
	if err == nil {
		err = tx.Commit()
	} else {
		_ = tx.Rollback()
	}
	requirePostgresCode(t, err, "P0001")

	err = store.MarkDuplicate(ctx, other.file.ID, fixture.file.ID)
	if !errors.Is(err, ErrDuplicateCrossLibrary) {
		t.Fatalf("MarkDuplicate error = %v, want ErrDuplicateCrossLibrary", err)
	}
}

func requirePostgresCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("operation succeeded, want PostgreSQL SQLSTATE %s", code)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("error %v is not a PostgreSQL error", err)
	}
	if pgErr.Code != code {
		t.Fatalf("SQLSTATE = %s (%v), want %s", pgErr.Code, err, code)
	}
}
