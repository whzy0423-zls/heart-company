package skillchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/skillcatalog"
	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestSkillChatPostgresIsolationAndDeletion(t *testing.T) {
	sourceDir := os.Getenv("SKILL_TEST_SOURCE_DIR")
	if sourceDir == "" {
		t.Skip("set SKILL_TEST_SOURCE_DIR with TEST_DATABASE_URL to run skill chat PostgreSQL integration tests")
	}
	database, _ := testdb.OpenEnvIsolatedSchema(t, "skill_chat")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	schema, err := os.ReadFile(filepath.Join("..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	manifestPath := filepath.Join("..", "..", "config", "skill-review-manifest.json")
	initialCommand := skillcatalog.CatalogCommand{Action: "publish", Version: "1.0.0", SourceDir: sourceDir, ManifestPath: manifestPath}
	if _, err := skillcatalog.NewStore(database).ApplyLearningGrowthCatalog(ctx, initialCommand); err != nil {
		t.Fatalf("compile skill catalog: %v", err)
	}
	secondImport, err := skillcatalog.NewStore(database).ApplyLearningGrowthCatalog(ctx, initialCommand)
	if err != nil || secondImport.CategoryCount != 7 || secondImport.SkillCount != 35 || secondImport.PublishedCount != 32 || secondImport.SkippedCount != 35 {
		t.Fatalf("idempotent import=%+v err=%v", secondImport, err)
	}

	var userOne, userTwo int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, "skill-pg-user-1").Scan(&userOne); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, "skill-pg-user-2").Scan(&userTwo); err != nil {
		t.Fatal(err)
	}
	var publishedSkillID, disabledSkillID int64
	if err := database.QueryRowContext(ctx, `SELECT id FROM app_skills WHERE key='art-of-learning'`).Scan(&publishedSkillID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT id FROM app_skills WHERE key='deliberate-practice'`).Scan(&disabledSkillID); err != nil {
		t.Fatal(err)
	}

	store := NewStore(database)
	sessionOne, err := store.CreateSession(ctx, userOne, publishedSkillID, "会话一")
	if err != nil {
		t.Fatal(err)
	}
	sessionTwo, err := store.CreateSession(ctx, userOne, publishedSkillID, "会话二")
	if err != nil {
		t.Fatal(err)
	}
	otherSession, err := store.CreateSession(ctx, userTwo, publishedSkillID, "他人会话")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSession(ctx, userOne, disabledSkillID, "不可用技能"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled skill session err=%v, want ErrNotFound", err)
	}
	if _, err := store.GetSession(ctx, userOne, otherSession.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user get err=%v, want ErrNotFound", err)
	}
	if _, err := store.UpdateSession(ctx, userOne, otherSession.ID, nil, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user clear err=%v, want ErrNotFound", err)
	}
	if err := store.DeleteSession(ctx, userOne, otherSession.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user delete err=%v, want ErrNotFound", err)
	}

	traceOne := GenerationTrace{GenerationRevision: sessionOne.GenerationRevision, SkillVersionID: sessionOne.SkillVersionID, TheoryReleaseID: sessionOne.TheoryReleaseID, ChunkIDs: []int64{101, 102}}
	traceTwo := GenerationTrace{GenerationRevision: sessionTwo.GenerationRevision, SkillVersionID: sessionTwo.SkillVersionID, TheoryReleaseID: sessionTwo.TheoryReleaseID, ChunkIDs: []int64{201}}
	messageOne, err := store.SavePair(ctx, userOne, sessionOne.ID, traceOne, "SESSION_ONE_MARKER", "ONE", json.RawMessage("[]"))
	if err != nil {
		t.Fatal(err)
	}
	messageTwo, err := store.SavePair(ctx, userOne, sessionTwo.ID, traceTwo, "SESSION_TWO_MARKER", "TWO", json.RawMessage("[]"))
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := store.UpdateConversationSummary(ctx, userOne, sessionOne.ID, sessionOne.GenerationRevision, 0, "SUMMARY_ONE", messageOne); err != nil || !changed {
		t.Fatalf("update first summary changed=%v err=%v", changed, err)
	}
	if changed, err := store.UpdateConversationSummary(ctx, userOne, sessionTwo.ID, sessionTwo.GenerationRevision, 0, "SUMMARY_TWO", messageTwo); err != nil || !changed {
		t.Fatalf("update second summary changed=%v err=%v", changed, err)
	}
	stateOne, err := store.GetConversationState(ctx, userOne, sessionOne.ID)
	if err != nil || stateOne.Summary != "SUMMARY_ONE" {
		t.Fatalf("first state=%+v err=%v", stateOne, err)
	}
	stateTwo, err := store.GetConversationState(ctx, userOne, sessionTwo.ID)
	if err != nil || stateTwo.Summary != "SUMMARY_TWO" {
		t.Fatalf("second state=%+v err=%v", stateTwo, err)
	}
	firstMessages, err := store.ListMessages(ctx, userOne, sessionOne.ID)
	if err != nil || len(firstMessages) != 2 || firstMessages[0].Content != "SESSION_ONE_MARKER" {
		t.Fatalf("first messages=%+v err=%v", firstMessages, err)
	}
	if crossed, err := store.ListMessages(ctx, userTwo, sessionOne.ID); err != nil || len(crossed) != 0 {
		t.Fatalf("cross-user messages=%+v err=%v", crossed, err)
	}
	var tracedRevision, tracedVersion, tracedRelease int64
	var tracedChunks []int64
	var tracedChunksRaw []byte
	if err := database.QueryRowContext(ctx, `SELECT generation_revision,skill_version_id,theory_release_id,array_to_json(chunk_ids) FROM app_skill_chat_traces WHERE assistant_message_id=$1`, messageOne).Scan(&tracedRevision, &tracedVersion, &tracedRelease, &tracedChunksRaw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(tracedChunksRaw, &tracedChunks); err != nil {
		t.Fatal(err)
	}
	if tracedRevision != traceOne.GenerationRevision || tracedVersion != traceOne.SkillVersionID || tracedRelease != traceOne.TheoryReleaseID || fmt.Sprint(tracedChunks) != fmt.Sprint(traceOne.ChunkIDs) {
		t.Fatalf("trace revision=%d version=%d release=%d chunks=%v", tracedRevision, tracedVersion, tracedRelease, tracedChunks)
	}

	assetOne := insertSkillVoiceAsset(t, ctx, database, "skill-clear-asset")
	if _, err := store.SaveVoiceMessage(ctx, userOne, sessionOne.ID, sessionOne.GenerationRevision, assetOne, 1200, "清空测试"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateSession(ctx, userOne, sessionOne.ID, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SavePair(ctx, userOne, sessionOne.ID, traceOne, "STALE", "STALE", json.RawMessage("[]")); !errors.Is(err, ErrSessionChanged) {
		t.Fatalf("stale text save err=%v, want ErrSessionChanged", err)
	}
	staleAsset := insertSkillVoiceAsset(t, ctx, database, "skill-stale-asset")
	if _, err := store.SaveVoiceMessage(ctx, userOne, sessionOne.ID, sessionOne.GenerationRevision, staleAsset, 1200, "STALE"); !errors.Is(err, ErrSessionChanged) {
		t.Fatalf("stale voice save err=%v, want ErrSessionChanged", err)
	}
	if changed, err := store.UpdateConversationSummary(ctx, userOne, sessionOne.ID, sessionOne.GenerationRevision, 0, "STALE", messageOne); err != nil || changed {
		t.Fatalf("stale summary changed=%v err=%v", changed, err)
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM upload_assets WHERE id=$1`, staleAsset); err != nil {
		t.Fatal(err)
	}
	assertSkillSessionCounts(t, ctx, database, sessionOne.ID, assetOne, 0, "")

	assetTwo := insertSkillVoiceAsset(t, ctx, database, "skill-delete-asset")
	if _, err := store.SaveVoiceMessage(ctx, userOne, sessionTwo.ID, sessionTwo.GenerationRevision, assetTwo, 1200, "删除测试"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSession(ctx, userOne, sessionTwo.ID); err != nil {
		t.Fatal(err)
	}
	assertSkillSessionCounts(t, ctx, database, sessionTwo.ID, assetTwo, 0, "deleted")

	if _, err := database.ExecContext(ctx, `UPDATE app_skill_versions SET instructions=instructions || 'changed' WHERE id=$1`, sessionOne.SkillVersionID); err == nil {
		t.Fatal("published skill version update was not rejected")
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM app_skill_versions WHERE id=$1`, sessionOne.SkillVersionID); err == nil {
		t.Fatal("published skill version delete was not rejected")
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE theory_chunks SET content=content || ' changed'
		WHERE id=(
		  SELECT mapping.chunk_id FROM theory_release_cards mapping
		  JOIN app_skill_versions version ON version.theory_release_id=mapping.release_id
		  WHERE version.id=$1 LIMIT 1
		)`, sessionOne.SkillVersionID); err == nil {
		t.Fatal("published skill release chunk update was not rejected")
	}
	if _, err := database.ExecContext(ctx, `
		DELETE FROM theory_release_cards
		WHERE id=(
		  SELECT mapping.id FROM theory_release_cards mapping
		  JOIN app_skill_versions version ON version.theory_release_id=mapping.release_id
		  WHERE version.id=$1 LIMIT 1
		)`, sessionOne.SkillVersionID); err == nil {
		t.Fatal("published skill release mapping delete was not rejected")
	}
	assertOrdinaryTheoryReleaseRemainsEditable(t, ctx, database)

	manifestV101 := versionedReviewManifest(t, "1.0.1")
	if _, err := skillcatalog.NewStore(database).ApplyLearningGrowthCatalog(ctx, skillcatalog.CatalogCommand{Action: "ready", Version: "1.0.1", SourceDir: sourceDir, ManifestPath: manifestV101}); err != nil {
		t.Fatalf("prepare new catalog version: %v", err)
	}
	readySession, err := store.CreateSession(ctx, userOne, publishedSkillID, "ready版本不应切换")
	if err != nil || readySession.Version != "1.0.0" {
		t.Fatalf("ready action changed latest session version: %+v err=%v", readySession, err)
	}
	if _, err := skillcatalog.NewStore(database).ApplyLearningGrowthCatalog(ctx, skillcatalog.CatalogCommand{Action: "publish", Version: "1.0.1", SourceDir: sourceDir, ManifestPath: manifestV101}); err != nil {
		t.Fatalf("publish new catalog version: %v", err)
	}
	newSession, err := store.CreateSession(ctx, userOne, publishedSkillID, "新版本")
	if err != nil || newSession.Version != "1.0.1" {
		t.Fatalf("new session did not use published version: %+v err=%v", newSession, err)
	}
	fixedOldSession, err := store.GetSession(ctx, userOne, sessionOne.ID)
	if err != nil || fixedOldSession.Version != "1.0.0" || fixedOldSession.MinAppVersion != "1.0.1" {
		t.Fatalf("old session version changed: %+v err=%v", fixedOldSession, err)
	}
	if _, err := skillcatalog.NewStore(database).ApplyLearningGrowthCatalog(ctx, skillcatalog.CatalogCommand{Action: "rollback", Version: "1.0.0"}); err != nil {
		t.Fatalf("rollback catalog version: %v", err)
	}
	rollbackSession, err := store.CreateSession(ctx, userOne, publishedSkillID, "回滚版本")
	if err != nil || rollbackSession.Version != "1.0.0" {
		t.Fatalf("rollback did not restore latest pointer: %+v err=%v", rollbackSession, err)
	}
	if _, err := skillcatalog.NewStore(database).ApplyLearningGrowthCatalog(ctx, skillcatalog.CatalogCommand{Action: "retire", Version: "1.0.1"}); err != nil {
		t.Fatalf("retire catalog version: %v", err)
	}
	var retiredVersionStatus, retiredReleaseStatus string
	if err := database.QueryRowContext(ctx, `SELECT version.status,release.status FROM app_skill_versions version JOIN theory_library_releases release ON release.id=version.theory_release_id WHERE version.skill_id=$1 AND version.version='1.0.1'`, publishedSkillID).Scan(&retiredVersionStatus, &retiredReleaseStatus); err != nil {
		t.Fatal(err)
	}
	if retiredVersionStatus != "retired" || retiredReleaseStatus != "retired" {
		t.Fatalf("retire statuses version=%s release=%s", retiredVersionStatus, retiredReleaseStatus)
	}
}

func versionedReviewManifest(t *testing.T, version string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "skill-review-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(raw), `"catalogVersion": "1.0.0"`, `"catalogVersion": "`+version+`"`, 1)
	path := filepath.Join(t.TempDir(), "skill-review-"+version+".json")
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertOrdinaryTheoryReleaseRemainsEditable(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	key := fmt.Sprintf("ordinary-editable-%d", time.Now().UnixNano())
	var libraryID, releaseID, cardID, chunkID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO theory_libraries(key,name,status) VALUES($1,$1,'enabled') RETURNING id`, key).Scan(&libraryID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.Exec(`DELETE FROM theory_libraries WHERE id=$1`, libraryID) })
	if err := database.QueryRowContext(ctx, `INSERT INTO theory_library_releases(library_id,version,status) VALUES($1,1,'ready') RETURNING id`, libraryID).Scan(&releaseID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO theory_cards(library_id,canonical_key,canonical_name,card_kind,epistemic_status,
		  evidence_level,clinical_safety,authority_level,status)
		VALUES($1,$2,$2,'concept','author_interpretation','unknown','general',1,'draft') RETURNING id`, libraryID, key).Scan(&cardID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO theory_chunks(library_id,card_id,chunk_key,chunk_kind,title,content,authority_level,
		  evidence_level,clinical_safety,content_hash,status)
		VALUES($1,$2,$3,'card',$3,'before',1,'unknown','general',$3,'enabled') RETURNING id`, libraryID, cardID, key).Scan(&chunkID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO theory_release_cards(release_id,card_id,chunk_id) VALUES($1,$2,$3)`, releaseID, cardID, chunkID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE theory_chunks SET content='after' WHERE id=$1`, chunkID); err != nil {
		t.Fatalf("ordinary theory chunk became immutable: %v", err)
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM theory_release_cards WHERE release_id=$1`, releaseID); err != nil {
		t.Fatalf("ordinary theory mapping became immutable: %v", err)
	}
}

func insertSkillVoiceAsset(t *testing.T, ctx context.Context, database *sql.DB, key string) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRowContext(ctx, `
		INSERT INTO upload_assets(key,name,dir,content_type,size,data)
		VALUES($1,$1,'app/skill-chat/voice','audio/aac',3,$2) RETURNING id`, key, []byte("aac")).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func assertSkillSessionCounts(t *testing.T, ctx context.Context, database *sql.DB, sessionID, assetID int64, wantMessages int, sessionState string) {
	t.Helper()
	var messages, assets, sessions int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_chat_messages WHERE session_id=$1`, sessionID).Scan(&messages); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM upload_assets WHERE id=$1`, assetID).Scan(&assets); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_chat_sessions WHERE id=$1`, sessionID).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if messages != wantMessages || assets != 0 {
		t.Fatalf("messages=%d assets=%d, want messages=%d assets=0", messages, assets, wantMessages)
	}
	if sessionState == "deleted" && sessions != 0 {
		t.Fatalf("session count=%d, want deleted", sessions)
	}
	if sessionState == "" {
		var summary string
		if err := database.QueryRowContext(ctx, `SELECT context_summary FROM app_chat_sessions WHERE id=$1`, sessionID).Scan(&summary); err != nil {
			t.Fatal(err)
		}
		if summary != "" {
			t.Fatalf("summary=%q, want cleared", summary)
		}
	}
}
