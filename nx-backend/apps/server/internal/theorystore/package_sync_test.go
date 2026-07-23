package theorystore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestTheoryDatabaseURLPriorityAndRedaction(t *testing.T) {
	getenv := func(key string) string {
		return map[string]string{
			"THEORY_DATABASE_URL": "postgres://theory:top-secret@db/theory",
			"DATABASE_URL":        "postgres://fallback:other-secret@db/default",
		}[key]
	}
	got, err := TheoryDatabaseURL(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgres://theory:top-secret@db/theory" {
		t.Fatalf("priority URL = %q", got)
	}
	if text := RedactDatabaseError(errors.New("connect postgres://theory:top-secret@db/theory failed")).Error(); strings.Contains(text, "top-secret") || strings.Contains(text, "postgres://") {
		t.Fatalf("redacted error leaked DSN: %q", text)
	}
}

func TestTheoryDatabaseURLFallsBackAndRequiresEnvironment(t *testing.T) {
	got, err := TheoryDatabaseURL(func(key string) string {
		if key == "DATABASE_URL" {
			return "postgres://fallback:secret@db/default"
		}
		return ""
	})
	if err != nil || got == "" {
		t.Fatalf("fallback URL = %q, err=%v", got, err)
	}
	if _, err := TheoryDatabaseURL(func(string) string { return "" }); err == nil {
		t.Fatal("missing database URL must fail")
	}
}

func TestValidateAndPlanRoundPackage(t *testing.T) {
	root := roundPackageRoot(t)
	plan, err := ValidatePackage(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PackageID != "xinzhili-round-001" || plan.ContentDigest == "" || plan.Cards != 40 || plan.Practices != 12 || plan.Sources != 24 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.WriteAllowed {
		t.Fatal("validation/plan must not authorize a write")
	}
}

func TestActivateRoundOneIsFailClosed(t *testing.T) {
	err := ActivatePackage(context.Background(), nil, "xinzhili-round-001", 1)
	if !errors.Is(err, ErrActivationBlocked) {
		t.Fatalf("activate error = %v", err)
	}
	for _, prerequisite := range []string{"not_runnable_for_activation", "milestone B", "milestone C"} {
		if !strings.Contains(err.Error(), prerequisite) {
			t.Fatalf("activation error misses %q: %v", prerequisite, err)
		}
	}
}

func TestPackageSyncPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	schemaPath := filepath.Join("..", "db", "schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}

	libraryKey := "xinzhili_test_package_sync"
	cleanupPackageFixture(t, db, libraryKey)
	defer cleanupPackageFixture(t, db, libraryKey)
	actorID := createPackageTestUser(t, db, libraryKey+"-actor")
	syncer := NewPackageSyncer(db)
	syncer.libraryKey = libraryKey

	plan, err := syncer.Plan(ctx, roundPackageRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if plan.WriteAllowed || plan.Operation != "stage" {
		t.Fatalf("dry-run plan = %+v", plan)
	}
	staged, err := syncer.Stage(ctx, roundPackageRoot(t), actorID)
	if err != nil {
		t.Fatal(err)
	}
	if staged.NoOp || staged.Cards != 40 || staged.Practices != 12 {
		t.Fatalf("stage = %+v", staged)
	}
	var totalCards, practiceHosts, practices, sources int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), count(*) FILTER (WHERE c.card_kind='practice')
		FROM theory_cards c JOIN theory_libraries l ON l.id=c.library_id WHERE l.key=$1`, libraryKey).Scan(&totalCards, &practiceHosts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM theory_practices p JOIN theory_cards c ON c.id=p.card_id JOIN theory_libraries l ON l.id=c.library_id WHERE l.key=$1`, libraryKey).Scan(&practices); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM theory_source_works w JOIN theory_libraries l ON l.id=w.library_id WHERE l.key=$1`, libraryKey).Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if totalCards != 52 || practiceHosts != 12 || practices != 12 || sources != 24 {
		t.Fatalf("stage counts cards=%d practiceHosts=%d practices=%d sources=%d", totalCards, practiceHosts, practices, sources)
	}
	var published int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM theory_cards c JOIN theory_libraries l ON l.id=c.library_id WHERE l.key=$1 AND c.status='published'`, libraryKey).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published != 0 {
		t.Fatalf("stage published %d cards", published)
	}
	again, err := syncer.Stage(ctx, roundPackageRoot(t), actorID)
	if err != nil || !again.NoOp {
		t.Fatalf("idempotent stage = %+v, err=%v", again, err)
	}
	if _, err := syncer.Promote(ctx, "xinzhili-round-001", actorID); !errors.Is(err, ErrReviewsIncomplete) {
		t.Fatalf("promote without reviews = %v", err)
	}
	sourceReviewer := createPackageReviewer(t, db, libraryKey+"-source", "theory_source_reviewer")
	theoryReviewer := createPackageReviewer(t, db, libraryKey+"-theory", "theory_content_reviewer")
	safetyReviewer := createPackageReviewer(t, db, libraryKey+"-safety", "theory_safety_reviewer")
	if _, err := syncer.RecordReview(ctx, "xinzhili-round-001", ReviewSourceVerification, theoryReviewer, "wrong role"); !errors.Is(err, ErrReviewerRole) {
		t.Fatalf("wrong reviewer role = %v", err)
	}
	for _, review := range []struct {
		kind ReviewType
		user int64
	}{{ReviewSourceVerification, sourceReviewer}, {ReviewTheory, theoryReviewer}, {ReviewSafety, safetyReviewer}} {
		if _, err := syncer.RecordReview(ctx, "xinzhili-round-001", review.kind, review.user, "已人工核验"); err != nil {
			t.Fatalf("record %s: %v", review.kind, err)
		}
	}
	if _, err := syncer.Promote(ctx, "xinzhili-round-001", sourceReviewer); !errors.Is(err, ErrActorSeparation) {
		t.Fatalf("reviewer promoted own package: %v", err)
	}
	promoted, err := syncer.Promote(ctx, "xinzhili-round-001", actorID)
	if err != nil {
		t.Fatal(err)
	}
	if promoted.NoOp || promoted.ReleaseID <= 0 || promoted.ReleaseStatus != ReleaseStatusReady || promoted.CardCount != 52 || promoted.ChunkCount != 52 {
		t.Fatalf("promotion = %+v", promoted)
	}
	var chunks, mappings int
	if err := db.QueryRow(`SELECT count(*) FROM theory_chunks c JOIN theory_libraries l ON l.id=c.library_id WHERE l.key=$1`, libraryKey).Scan(&chunks); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM theory_release_cards m JOIN theory_library_releases r ON r.id=m.release_id JOIN theory_libraries l ON l.id=r.library_id WHERE l.key=$1`, libraryKey).Scan(&mappings); err != nil {
		t.Fatal(err)
	}
	if chunks != 52 || mappings != 52 {
		t.Fatalf("promotion counts chunks=%d mappings=%d", chunks, mappings)
	}
	repeated, err := syncer.Promote(ctx, "xinzhili-round-001", actorID)
	if err != nil || !repeated.NoOp || repeated.ReleaseID != promoted.ReleaseID {
		t.Fatalf("idempotent promotion = %+v, err=%v", repeated, err)
	}
	if _, err := db.Exec(`UPDATE theory_chunks SET content='人工改写发布块' WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND chunk_key='belief.behavior_not_identity'`, libraryKey); err != nil {
		t.Fatal(err)
	}
	if _, err := syncer.Promote(ctx, "xinzhili-round-001", actorID); !errors.Is(err, ErrImportedContentChanged) {
		t.Fatalf("modified ready release was accepted: %v", err)
	}
}

func TestPackagePromotionRollbackAndManualEditProtection(t *testing.T) {
	db := openPackageTestDatabase(t)
	defer db.Close()
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		libraryKey string
		mutate     func(*testing.T, *sql.DB, string, *PackageSyncer)
		want       error
	}{
		{name: "manual edit", libraryKey: "xinzhili_test_manual_edit", mutate: func(t *testing.T, db *sql.DB, key string, _ *PackageSyncer) {
			if _, err := db.Exec(`UPDATE theory_cards SET summary='人工改写' WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='change.yin_yang_complementarity'`, key); err != nil {
				t.Fatal(err)
			}
		}, want: ErrImportedContentChanged},
		{name: "late failure", libraryKey: "xinzhili_test_rollback", mutate: func(_ *testing.T, _ *sql.DB, _ string, syncer *PackageSyncer) {
			syncer.beforePromotionCommit = func(*sql.Tx) error { return errors.New("injected promotion failure") }
		}, want: errors.New("injected promotion failure")},
		{name: "review role revoked", libraryKey: "xinzhili_test_revoked_review", mutate: func(t *testing.T, db *sql.DB, key string, _ *PackageSyncer) {
			if _, err := db.Exec(`DELETE FROM user_roles WHERE user_id=(SELECT id FROM users WHERE username=$1) AND role_id=(SELECT id FROM roles WHERE code='theory_safety_reviewer')`, "theorysync-test-"+key+"-review-role-revoked-safety"); err != nil {
				t.Fatal(err)
			}
		}, want: ErrReviewsIncomplete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanupPackageFixture(t, db, tc.libraryKey)
			defer cleanupPackageFixture(t, db, tc.libraryKey)
			actor := createPackageTestUser(t, db, tc.libraryKey+"-actor")
			syncer := NewPackageSyncer(db)
			syncer.libraryKey = tc.libraryKey
			if _, err := syncer.Stage(ctx, roundPackageRoot(t), actor); err != nil {
				t.Fatal(err)
			}
			for _, review := range []struct {
				kind         ReviewType
				suffix, role string
			}{
				{ReviewSourceVerification, tc.name + "-source", "theory_source_reviewer"},
				{ReviewTheory, tc.name + "-theory", "theory_content_reviewer"},
				{ReviewSafety, tc.name + "-safety", "theory_safety_reviewer"},
			} {
				reviewer := createPackageReviewer(t, db, tc.libraryKey+"-"+strings.ReplaceAll(review.suffix, " ", "-"), review.role)
				if _, err := syncer.RecordReview(ctx, "xinzhili-round-001", review.kind, reviewer, "已核验"); err != nil {
					t.Fatal(err)
				}
			}
			tc.mutate(t, db, tc.libraryKey, syncer)
			_, err := syncer.Promote(ctx, "xinzhili-round-001", actor)
			if tc.want == ErrImportedContentChanged || tc.want == ErrReviewsIncomplete {
				if !errors.Is(err, tc.want) {
					t.Fatalf("promote error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want.Error()) {
				t.Fatalf("promote error = %v", err)
			}
			var published, chunks, receipts int
			if err := db.QueryRow(`SELECT count(*) FROM theory_cards c JOIN theory_libraries l ON l.id=c.library_id WHERE l.key=$1 AND c.status='published'`, tc.libraryKey).Scan(&published); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM theory_chunks c JOIN theory_libraries l ON l.id=c.library_id WHERE l.key=$1`, tc.libraryKey).Scan(&chunks); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM theory_package_promotions p JOIN theory_package_imports i ON i.id=p.import_id JOIN theory_libraries l ON l.id=i.library_id WHERE l.key=$1`, tc.libraryKey).Scan(&receipts); err != nil {
				t.Fatal(err)
			}
			if published != 0 || chunks != 0 || receipts != 0 {
				t.Fatalf("partial promotion published=%d chunks=%d receipts=%d", published, chunks, receipts)
			}
		})
	}
}

func TestStageRejectsDigestDatabaseLibraryAndVersionConflicts(t *testing.T) {
	db := openPackageTestDatabase(t)
	defer db.Close()
	ctx := context.Background()
	tests := []struct {
		name, key, want string
		prepare         func(*testing.T, *sql.DB, string, int64, *PackageSyncer)
	}{
		{"digest", "xinzhili_test_digest_conflict", "identity mismatch", func(t *testing.T, db *sql.DB, key string, actor int64, syncer *PackageSyncer) {
			if _, err := syncer.Stage(ctx, roundPackageRoot(t), actor); err != nil {
				t.Fatal(err)
			}
			setImportContractTrigger(t, db, false)
			if _, err := db.Exec(`UPDATE theory_package_imports SET content_digest=repeat('a',64) WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1)`, key); err != nil {
				t.Fatal(err)
			}
			setImportContractTrigger(t, db, true)
		}},
		{"database", "xinzhili_test_database_conflict", "identity mismatch", func(t *testing.T, db *sql.DB, key string, actor int64, syncer *PackageSyncer) {
			if _, err := syncer.Stage(ctx, roundPackageRoot(t), actor); err != nil {
				t.Fatal(err)
			}
			setImportContractTrigger(t, db, false)
			if _, err := db.Exec(`UPDATE theory_package_imports SET target_database='another_database' WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1)`, key); err != nil {
				t.Fatal(err)
			}
			setImportContractTrigger(t, db, true)
		}},
		{"cross library", "xinzhili_test_cross_library", "another library", func(t *testing.T, db *sql.DB, key string, actor int64, _ *PackageSyncer) {
			other := key + "_other"
			if _, err := db.Exec(`INSERT INTO theory_libraries(key,name,created_by) VALUES($1,'other',$2)`, other, actor); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO theory_cards(library_id,canonical_key,canonical_name,card_kind,epistemic_status,evidence_level,clinical_safety,authority_level,status,version) VALUES((SELECT id FROM theory_libraries WHERE key=$1),'belief.behavior_not_identity','collision','concept','author_interpretation','limited','caution',3,'draft',1)`, other); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM theory_libraries WHERE key=$1`, other) })
		}},
		{"higher active", "xinzhili_test_higher_active", "behind active version", func(t *testing.T, db *sql.DB, key string, actor int64, _ *PackageSyncer) {
			var libraryID int64
			if err := db.QueryRow(`INSERT INTO theory_libraries(key,name,current_version,created_by) VALUES($1,'higher',2,$2) RETURNING id`, key, actor).Scan(&libraryID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO theory_library_releases(library_id,version,status) VALUES($1,2,'active')`, libraryID); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanupPackageFixture(t, db, tc.key)
			defer cleanupPackageFixture(t, db, tc.key)
			actor := createPackageTestUser(t, db, tc.key+"-actor")
			syncer := NewPackageSyncer(db)
			syncer.libraryKey = tc.key
			tc.prepare(t, db, tc.key, actor, syncer)
			_, err := syncer.Stage(ctx, roundPackageRoot(t), actor)
			if !errors.Is(err, ErrPackageConflict) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("stage conflict = %v", err)
			}
		})
	}
}

func TestConcurrentStageAndPromoteAreIdempotent(t *testing.T) {
	db := openPackageTestDatabase(t)
	defer db.Close()
	ctx := context.Background()
	key := "xinzhili_test_concurrent"
	cleanupPackageFixture(t, db, key)
	defer cleanupPackageFixture(t, db, key)
	actor1 := createPackageTestUser(t, db, key+"-actor-1")
	actor2 := createPackageTestUser(t, db, key+"-actor-2")
	newSyncer := func() *PackageSyncer { s := NewPackageSyncer(db); s.libraryKey = key; return s }
	type stageResult struct {
		plan PackagePlan
		err  error
	}
	start := make(chan struct{})
	results := make(chan stageResult, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			p, e := newSyncer().Stage(ctx, roundPackageRoot(t), actor1)
			results <- stageResult{p, e}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	noops := 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.plan.NoOp {
			noops++
		}
	}
	if noops != 1 {
		t.Fatalf("concurrent stage noops=%d", noops)
	}
	for _, review := range []struct {
		kind         ReviewType
		suffix, role string
	}{{ReviewSourceVerification, "source", "theory_source_reviewer"}, {ReviewTheory, "theory", "theory_content_reviewer"}, {ReviewSafety, "safety", "theory_safety_reviewer"}} {
		id := createPackageReviewer(t, db, key+"-"+review.suffix, review.role)
		if _, err := newSyncer().RecordReview(ctx, "xinzhili-round-001", review.kind, id, "已核验"); err != nil {
			t.Fatal(err)
		}
	}
	type promoteResult struct {
		receipt PromotionReceipt
		err     error
	}
	promotions := make(chan promoteResult, 2)
	start = make(chan struct{})
	for _, actor := range []int64{actor1, actor2} {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-start
			r, e := newSyncer().Promote(ctx, "xinzhili-round-001", id)
			promotions <- promoteResult{r, e}
		}(actor)
	}
	close(start)
	wg.Wait()
	close(promotions)
	var releaseID int64
	noops = 0
	for result := range promotions {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if releaseID == 0 {
			releaseID = result.receipt.ReleaseID
		}
		if result.receipt.ReleaseID != releaseID {
			t.Fatalf("release ids differ")
		}
		if result.receipt.NoOp {
			noops++
		}
	}
	if noops != 1 {
		t.Fatalf("concurrent promote noops=%d", noops)
	}
}

func TestPromoteRejectsCanonicalSnapshotTampering(t *testing.T) {
	db := openPackageTestDatabase(t)
	defer db.Close()
	ctx := context.Background()
	tests := []struct{ name, sql string }{
		{"practice definition", `UPDATE theory_cards SET definition=definition||'篡改' WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='practice.intention_center_resource'`},
		{"practice domain", `UPDATE theory_cards SET domain='tampered' WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='practice.intention_center_resource'`},
		{"practice card kind", `UPDATE theory_cards SET card_kind='warning' WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='practice.intention_center_resource'`},
		{"practice epistemic", `UPDATE theory_cards SET epistemic_status='hypothesis' WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='practice.intention_center_resource'`},
		{"practice evidence", `UPDATE theory_cards SET evidence_level='unknown' WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='practice.intention_center_resource'`},
		{"practice authority", `UPDATE theory_cards SET authority_level=2 WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='practice.intention_center_resource'`},
		{"practice clinical safety", `UPDATE theory_cards SET clinical_safety='general' WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='practice.intention_center_resource'`},
		{"concept fixed field", `UPDATE theory_cards SET automatic_pattern='篡改' WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='belief.behavior_not_identity'`},
		{"practice steps", `UPDATE theory_practices SET steps='["篡改"]'::jsonb WHERE card_id=(SELECT id FROM theory_cards WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='practice.intention_center_resource')`},
		{"practice goal", `UPDATE theory_practices SET goal=goal||'篡改' WHERE card_id=(SELECT id FROM theory_cards WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='practice.intention_center_resource')`},
		{"source locator", `UPDATE theory_card_sources SET location_label='篡改' WHERE card_id=(SELECT id FROM theory_cards WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='belief.behavior_not_identity')`},
		{"source hash", `UPDATE theory_source_files SET sha256=repeat('f',64) WHERE work_id IN (SELECT id FROM theory_source_works WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1)) AND id=(SELECT min(f.id) FROM theory_source_files f JOIN theory_source_works w ON w.id=f.work_id WHERE w.library_id=(SELECT id FROM theory_libraries WHERE key=$1))`},
		{"source role", `UPDATE theory_card_sources SET source_role='supporting' WHERE card_id=(SELECT id FROM theory_cards WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1) AND canonical_key='belief.behavior_not_identity')`},
		{"relation note", `UPDATE theory_card_relations SET note='篡改' WHERE id=(SELECT min(r.id) FROM theory_card_relations r JOIN theory_cards c ON c.id=r.from_card_id WHERE c.library_id=(SELECT id FROM theory_libraries WHERE key=$1))`},
		{"relation type", `UPDATE theory_card_relations SET relation_type='extends' WHERE id=(SELECT min(r.id) FROM theory_card_relations r JOIN theory_cards c ON c.id=r.from_card_id WHERE c.library_id=(SELECT id FROM theory_libraries WHERE key=$1))`},
	}
	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := fmt.Sprintf("xinzhili_test_snapshot_%02d", index)
			cleanupPackageFixture(t, db, key)
			defer cleanupPackageFixture(t, db, key)
			actor := createPackageTestUser(t, db, key+"-actor")
			syncer := NewPackageSyncer(db)
			syncer.libraryKey = key
			stageAndReviewPackage(t, db, syncer, key, actor)
			if _, err := db.Exec(tc.sql, key); err != nil {
				t.Fatal(err)
			}
			if _, err := syncer.Promote(ctx, "xinzhili-round-001", actor); !errors.Is(err, ErrImportedContentChanged) {
				t.Fatalf("tamper accepted: %v", err)
			}
		})
	}
}

func TestIdempotentPromoteRechecksReviewAndVersionGates(t *testing.T) {
	db := openPackageTestDatabase(t)
	defer db.Close()
	ctx := context.Background()
	key := "xinzhili_test_noop_gates"
	cleanupPackageFixture(t, db, key)
	defer cleanupPackageFixture(t, db, key)
	actor := createPackageTestUser(t, db, key+"-actor")
	syncer := NewPackageSyncer(db)
	syncer.libraryKey = key
	reviewers := stageAndReviewPackage(t, db, syncer, key, actor)
	first, err := syncer.Promote(ctx, "xinzhili-round-001", actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM user_roles WHERE user_id=$1 AND role_id=(SELECT id FROM roles WHERE code='theory_safety_reviewer')`, reviewers[ReviewSafety]); err != nil {
		t.Fatal(err)
	}
	if _, err := syncer.Promote(ctx, "xinzhili-round-001", actor); !errors.Is(err, ErrReviewsIncomplete) {
		t.Fatalf("revoked review accepted: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_roles(user_id,role_id) VALUES($1,(SELECT id FROM roles WHERE code='theory_safety_reviewer'))`, reviewers[ReviewSafety]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO theory_library_releases(library_id,version,status) VALUES((SELECT id FROM theory_libraries WHERE key=$1),2,'active')`, key); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE theory_libraries SET current_version=2 WHERE key=$1`, key); err != nil {
		t.Fatal(err)
	}
	if repeated, err := syncer.Promote(ctx, "xinzhili-round-001", actor); !errors.Is(err, ErrPackageConflict) || repeated.ReleaseID != 0 {
		t.Fatalf("higher version accepted after release %d: %+v %v", first.ReleaseID, repeated, err)
	}
}

func TestIdempotentPromoteRechecksStoredPackageDigests(t *testing.T) {
	db := openPackageTestDatabase(t)
	defer db.Close()
	ctx := context.Background()
	for _, tc := range []struct{ name, column string }{{"content digest", "content_digest"}, {"package digest", "package_digest"}} {
		t.Run(tc.name, func(t *testing.T) {
			key := "xinzhili_test_noop_" + strings.ReplaceAll(tc.name, " ", "_")
			cleanupPackageFixture(t, db, key)
			defer cleanupPackageFixture(t, db, key)
			actor := createPackageTestUser(t, db, key+"-actor")
			syncer := NewPackageSyncer(db)
			syncer.libraryKey = key
			stageAndReviewPackage(t, db, syncer, key, actor)
			if _, err := syncer.Promote(ctx, "xinzhili-round-001", actor); err != nil {
				t.Fatal(err)
			}
			setImportContractTrigger(t, db, false)
			query := fmt.Sprintf(`UPDATE theory_package_imports SET %s=repeat('f',64) WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1)`, tc.column)
			if _, err := db.Exec(query, key); err != nil {
				t.Fatal(err)
			}
			setImportContractTrigger(t, db, true)
			if _, err := syncer.Promote(ctx, "xinzhili-round-001", actor); !errors.Is(err, ErrPackageConflict) {
				t.Fatalf("digest tamper accepted: %v", err)
			}
		})
	}
}

func TestPromoteRejectsPayloadAndIndependentHashTampering(t *testing.T) {
	db := openPackageTestDatabase(t)
	defer db.Close()
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *sql.DB, string)
	}{
		{"payload", func(t *testing.T, db *sql.DB, key string) {
			text := "篡改后的正式检索文本"
			sum := sha256.Sum256([]byte(text))
			if _, err := db.Exec(`UPDATE theory_package_imports SET payload=jsonb_set(jsonb_set(payload,'{previews,0,text}',to_jsonb($2::text)),'{previews,0,contentHash}',to_jsonb($3::text)) WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1)`, key, text, fmt.Sprintf("%x", sum)); err != nil {
				t.Fatal(err)
			}
		}},
		{"payload sha column", func(t *testing.T, db *sql.DB, key string) {
			if _, err := db.Exec(`UPDATE theory_package_imports SET payload_sha256=repeat('f',64) WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1)`, key); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := "xinzhili_test_payload_" + strings.ReplaceAll(tc.name, " ", "_")
			cleanupPackageFixture(t, db, key)
			defer cleanupPackageFixture(t, db, key)
			actor := createPackageTestUser(t, db, key+"-actor")
			syncer := NewPackageSyncer(db)
			syncer.libraryKey = key
			stageAndReviewPackage(t, db, syncer, key, actor)
			if _, err := db.Exec(`UPDATE theory_package_imports SET package_digest=repeat('e',64) WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1)`, key); err == nil {
				t.Fatal("immutable trigger allowed staged contract update")
			}
			setImportContractTrigger(t, db, false)
			tc.mutate(t, db, key)
			setImportContractTrigger(t, db, true)
			if _, err := syncer.Promote(ctx, "xinzhili-round-001", actor); !errors.Is(err, ErrPackageConflict) {
				t.Fatalf("tampered payload contract accepted: %v", err)
			}
		})
	}
}

func TestPromoteRejectsFingerprintContractTampering(t *testing.T) {
	db := openPackageTestDatabase(t)
	defer db.Close()
	ctx := context.Background()
	tests := []struct {
		name, expression string
		promoteFirst     bool
	}{
		{"first promote extra field", `object_fingerprints||'{"tampered":true}'::jsonb`, false},
		{"noop extra field", `object_fingerprints||'{"tampered":true}'::jsonb`, true},
		{"known field modified", `jsonb_set(object_fingerprints,'{sha256}',to_jsonb(repeat('f',64)))`, false},
		{"known field deleted", `object_fingerprints-'payloadSha256'`, false},
	}
	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := fmt.Sprintf("xinzhili_test_fingerprint_%02d", index)
			cleanupPackageFixture(t, db, key)
			defer cleanupPackageFixture(t, db, key)
			actor := createPackageTestUser(t, db, key+"-actor")
			syncer := NewPackageSyncer(db)
			syncer.libraryKey = key
			stageAndReviewPackage(t, db, syncer, key, actor)
			if tc.promoteFirst {
				if _, err := syncer.Promote(ctx, "xinzhili-round-001", actor); err != nil {
					t.Fatal(err)
				}
			}
			setImportContractTrigger(t, db, false)
			if _, err := db.Exec(`UPDATE theory_package_imports SET object_fingerprints=`+tc.expression+` WHERE library_id=(SELECT id FROM theory_libraries WHERE key=$1)`, key); err != nil {
				t.Fatal(err)
			}
			setImportContractTrigger(t, db, true)
			if _, err := syncer.Promote(ctx, "xinzhili-round-001", actor); !errors.Is(err, ErrPackageConflict) {
				t.Fatalf("fingerprint tamper accepted: %v", err)
			}
		})
	}
}

func setImportContractTrigger(t *testing.T, db *sql.DB, enabled bool) {
	t.Helper()
	action := "DISABLE"
	if enabled {
		action = "ENABLE"
	}
	if _, err := db.Exec(`ALTER TABLE theory_package_imports ` + action + ` TRIGGER theory_package_imports_immutable`); err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Cleanup(func() {
			_, _ = db.Exec(`ALTER TABLE theory_package_imports ENABLE TRIGGER theory_package_imports_immutable`)
		})
	}
}

func stageAndReviewPackage(t *testing.T, db *sql.DB, syncer *PackageSyncer, key string, actor int64) map[ReviewType]int64 {
	t.Helper()
	ctx := context.Background()
	if _, err := syncer.Stage(ctx, roundPackageRoot(t), actor); err != nil {
		t.Fatal(err)
	}
	reviewers := map[ReviewType]int64{}
	for _, review := range []struct {
		kind         ReviewType
		suffix, role string
	}{{ReviewSourceVerification, "source", "theory_source_reviewer"}, {ReviewTheory, "theory", "theory_content_reviewer"}, {ReviewSafety, "safety", "theory_safety_reviewer"}} {
		id := createPackageReviewer(t, db, key+"-"+review.suffix, review.role)
		reviewers[review.kind] = id
		if _, err := syncer.RecordReview(ctx, "xinzhili-round-001", review.kind, id, "已核验"); err != nil {
			t.Fatal(err)
		}
	}
	return reviewers
}

func openPackageTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	schema, err := os.ReadFile(filepath.Join("..", "db", "schema.sql"))
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func roundPackageRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "data", "theory", "xinzhili", "round-001"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func cleanupPackageFixture(t *testing.T, db *sql.DB, libraryKey string) {
	t.Helper()
	if _, err := db.Exec(`DELETE FROM theory_card_sources WHERE card_id IN (SELECT c.id FROM theory_cards c JOIN theory_libraries l ON l.id=c.library_id WHERE l.key=$1)`, libraryKey); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM theory_package_promotions WHERE import_id IN (SELECT i.id FROM theory_package_imports i JOIN theory_libraries l ON l.id=i.library_id WHERE l.key=$1)`, libraryKey); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM theory_package_imports WHERE library_id IN (SELECT id FROM theory_libraries WHERE key=$1)`, libraryKey); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM theory_libraries WHERE key=$1`, libraryKey); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE username LIKE $1`, "theorysync-test-"+libraryKey+"-%"); err != nil {
		t.Fatal(err)
	}
}

func createPackageTestUser(t *testing.T, db *sql.DB, suffix string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`INSERT INTO users(username,password_hash,status) VALUES ($1,'test',1) RETURNING id`, "theorysync-test-"+suffix).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func createPackageReviewer(t *testing.T, db *sql.DB, suffix, roleCode string) int64 {
	t.Helper()
	userID := createPackageTestUser(t, db, suffix)
	var roleID int64
	if err := db.QueryRow(`INSERT INTO roles(code,name,status) VALUES($1,$1,1) ON CONFLICT(code) DO UPDATE SET status=1 RETURNING id`, roleCode).Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, userID, roleID); err != nil {
		t.Fatal(err)
	}
	return userID
}
