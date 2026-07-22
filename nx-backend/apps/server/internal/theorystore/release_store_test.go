package theorystore

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReleaseStoreRejectsEmptyMapping(t *testing.T) {
	_, err := testStore(t, &sqlScript{}).BuildRelease(context.Background(), testRelease(RetrievalLexicalOnly), nil)
	if !errors.Is(err, ErrEmptyReleaseMapping) {
		t.Fatalf("expected ErrEmptyReleaseMapping, got %v", err)
	}
}

func TestReleaseStoreRejectsDuplicateChunkMappingBeforeSQL(t *testing.T) {
	mappings := []ReleaseMapping{{CardID: 11, ChunkID: 71}, {CardID: 11, ChunkID: 71}}
	_, err := testStore(t, &sqlScript{}).BuildRelease(context.Background(), testRelease(RetrievalLexicalOnly), mappings)
	if !errors.Is(err, ErrDuplicateReleaseMapping) {
		t.Fatalf("expected duplicate mapping error, got %v", err)
	}
}

func TestReleaseStoreRejectsChunkCardMismatchAndRollsBack(t *testing.T) {
	r := testRelease(RetrievalLexicalOnly)
	script := buildPrefix(r, []sqlStep{{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(12), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), string(StatusPublished), true}}}, {kind: "rollback"}})
	_, err := testStore(t, script).BuildRelease(context.Background(), r, []ReleaseMapping{{CardID: 11, ChunkID: 71}})
	if !errors.Is(err, ErrInvalidReleaseMapping) {
		t.Fatalf("expected mapping error, got %v", err)
	}
	script.assertDone(t)
}

func TestReleaseStoreBuildsLexicalReleaseWithoutEmbeddingsAndChecksCounts(t *testing.T) {
	r := testRelease(RetrievalLexicalOnly)
	ready := r
	ready.ID, ready.Status, ready.CardCount, ready.ChunkCount = 91, ReleaseStatusReady, 1, 1
	steps := []sqlStep{
		{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(11), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), string(StatusPublished), true}}},
		{kind: "exec", contains: "INSERT INTO theory_release_cards", affected: 1},
		{kind: "query", contains: "status = 'ready'", columns: releaseColumns(), rows: [][]driver.Value{releaseValues(ready)}},
		{kind: "commit"},
	}
	script := buildPrefix(r, steps)
	built, err := testStore(t, script).BuildRelease(context.Background(), r, []ReleaseMapping{{CardID: 11, ChunkID: 71}})
	if err != nil {
		t.Fatal(err)
	}
	if built.Status != ReleaseStatusReady || built.CardCount != 1 || built.ChunkCount != 1 {
		t.Fatalf("bad release counts: %+v", built)
	}
	for _, call := range script.callsSnapshot() {
		if strings.Contains(call.query, "theory_chunk_embeddings") {
			t.Fatal("lexical build queried embeddings")
		}
	}
}

func TestReleaseStoreHybridRejectsUnavailableVectorColumn(t *testing.T) {
	r := testRelease(RetrievalHybrid)
	steps := []sqlStep{
		{kind: "query", contains: "information_schema.columns", columns: []string{"exists"}, rows: [][]driver.Value{{false}}},
		{kind: "rollback"},
	}
	script := buildPrefix(r, steps)
	_, err := testStore(t, script).BuildRelease(context.Background(), r, []ReleaseMapping{{CardID: 11, ChunkID: 71}})
	if !errors.Is(err, ErrVectorUnavailable) {
		t.Fatalf("expected vector error, got %v", err)
	}
}

func TestReleaseStoreHybridRejectsMissingStaleWrongModelHashAndDimension(t *testing.T) {
	cases := []struct {
		name          string
		embeddingRows [][]driver.Value
		want          error
	}{
		{"missing", nil, ErrEmbeddingNotReady},
		{"stale", [][]driver.Value{{"text-embedding-3-small", int64(1536), strings.Repeat("b", 64), string(EmbeddingStatusReady), true}}, ErrStaleEmbedding},
		{"wrong_model", [][]driver.Value{{"other", int64(1536), strings.Repeat("a", 64), string(EmbeddingStatusReady), true}}, ErrEmbeddingNotReady},
		{"wrong_dimension", [][]driver.Value{{"text-embedding-3-small", int64(2), strings.Repeat("a", 64), string(EmbeddingStatusReady), true}}, ErrEmbeddingNotReady},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := testRelease(RetrievalHybrid)
			steps := []sqlStep{
				{kind: "query", contains: "information_schema.columns", columns: []string{"exists"}, rows: [][]driver.Value{{true}}},
				{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(11), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), string(StatusPublished), true}}},
				{kind: "query", contains: "FROM theory_chunk_embeddings", columns: []string{"embedding_model", "dimensions", "content_hash", "status", "has_embedding"}, rows: tc.embeddingRows},
				{kind: "rollback"},
			}
			script := buildPrefix(r, steps)
			_, err := testStore(t, script).BuildRelease(context.Background(), r, []ReleaseMapping{{CardID: 11, ChunkID: 71}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestReleaseStoreActivationLocksRetiresActivatesThenUpdatesLibrary(t *testing.T) {
	r := testRelease(RetrievalLexicalOnly)
	r.ID, r.Status, r.CardCount, r.ChunkCount = 91, ReleaseStatusReady, 1, 1
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_libraries", columns: []string{"id"}, rows: [][]driver.Value{{r.LibraryID}}},
		{kind: "query", contains: "FROM theory_library_releases", columns: releaseColumns(), rows: [][]driver.Value{releaseValues(r)}},
		{kind: "query", contains: "FROM theory_release_cards mapping", columns: []string{"card_id", "chunk_id"}, rows: [][]driver.Value{{int64(11), int64(71)}}},
		{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(11), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), string(StatusPublished), true}}},
		{kind: "exec", contains: "status = 'retired'", affected: 1},
		{kind: "exec", contains: "status = 'active'", affected: 1},
		{kind: "exec", contains: "current_version", affected: 1},
		{kind: "commit"},
	}}
	if err := testStore(t, script).ActivateRelease(context.Background(), r.LibraryID, r.ID); err != nil {
		t.Fatal(err)
	}
	calls := script.callsSnapshot()
	if !(indexCall(calls, "lock_theory_libraries") < indexCall(calls, "FROM theory_libraries") && indexCall(calls, "status = 'retired'") < indexCall(calls, "status = 'active'") && indexCall(calls, "status = 'active'") < indexCall(calls, "current_version")) {
		t.Fatalf("activation order wrong: %#v", calls)
	}
}

func TestReleaseStoreActivationFailureRollsBackBeforeOldActiveChanges(t *testing.T) {
	r := testRelease(RetrievalLexicalOnly)
	r.ID, r.Status, r.CardCount, r.ChunkCount = 91, ReleaseStatusReady, 1, 2
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_libraries", columns: []string{"id"}, rows: [][]driver.Value{{r.LibraryID}}},
		{kind: "query", contains: "FROM theory_library_releases", columns: releaseColumns(), rows: [][]driver.Value{releaseValues(r)}},
		{kind: "query", contains: "FROM theory_release_cards mapping", columns: []string{"card_id", "chunk_id"}, rows: [][]driver.Value{{int64(11), int64(71)}}},
		{kind: "rollback"},
	}}
	err := testStore(t, script).ActivateRelease(context.Background(), r.LibraryID, r.ID)
	if !errors.Is(err, ErrReleaseCountMismatch) {
		t.Fatalf("expected count mismatch, got %v", err)
	}
	if indexCall(script.callsSnapshot(), "status = 'retired'") >= 0 {
		t.Fatal("old active release changed before validation")
	}
}

func TestReleaseStoreActivationRejectsNonReadyOrDifferentLibrary(t *testing.T) {
	r := testRelease(RetrievalLexicalOnly)
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_libraries", columns: []string{"id"}, rows: [][]driver.Value{{r.LibraryID}}},
		{kind: "query", contains: "FROM theory_library_releases", columns: releaseColumns()},
		{kind: "rollback"},
	}}
	err := testStore(t, script).ActivateRelease(context.Background(), r.LibraryID, 999)
	if !errors.Is(err, ErrReleaseNotReady) {
		t.Fatalf("expected ErrReleaseNotReady, got %v", err)
	}
}

func testRelease(mode RetrievalMode) Release {
	now := time.Unix(1700000000, 0).UTC()
	model := ""
	if mode == RetrievalHybrid {
		model = "text-embedding-3-small"
	}
	return Release{LibraryID: 3, Version: 4, Status: ReleaseStatusDraft, EmbeddingModel: model, EmbeddingDimensions: 1536, RetrievalMode: mode, IndexVersion: "v1", CreateTime: now, UpdateTime: now}
}
func buildPrefix(r Release, tail []sqlStep) *sqlScript {
	building := r
	building.ID, building.Status = 91, ReleaseStatusBuilding
	steps := []sqlStep{{kind: "begin"}, {kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}}, {kind: "query", contains: "FROM theory_libraries", columns: []string{"id"}, rows: [][]driver.Value{{r.LibraryID}}}, {kind: "query", contains: "INSERT INTO theory_library_releases", columns: releaseColumns(), rows: [][]driver.Value{releaseValues(building)}}, {kind: "exec", contains: "DELETE FROM theory_release_cards", affected: 0}}
	steps = append(steps, tail...)
	return &sqlScript{steps: steps}
}
func releaseColumns() []string {
	return []string{"id", "library_id", "version", "status", "embedding_model", "embedding_dimensions", "retrieval_mode", "index_version", "card_count", "chunk_count", "build_error", "activated_by", "activated_at", "create_time", "update_time"}
}
func releaseValues(r Release) []driver.Value {
	return []driver.Value{r.ID, r.LibraryID, int64(r.Version), string(r.Status), r.EmbeddingModel, int64(r.EmbeddingDimensions), string(r.RetrievalMode), r.IndexVersion, int64(r.CardCount), int64(r.ChunkCount), r.BuildError, nullableInt(r.ActivatedBy), nullableTime(r.ActivatedAt), r.CreateTime, r.UpdateTime}
}
func mappingValidationColumns() []string {
	return []string{"card_id", "library_id", "chunk_status", "content_hash", "card_status", "has_primary"}
}
