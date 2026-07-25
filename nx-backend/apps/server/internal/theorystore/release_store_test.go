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

func TestReleaseStoreRejectsNonMonotonicBuildAndActivationVersions(t *testing.T) {
	r := testRelease(RetrievalLexicalOnly)
	build := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_libraries", columns: []string{"id", "current_version"}, rows: [][]driver.Value{{r.LibraryID, int64(r.Version)}}},
		{kind: "rollback"},
	}}
	_, err := testStore(t, build).BuildRelease(context.Background(), r, []ReleaseMapping{{CardID: 11, ChunkID: 71}})
	if !errors.Is(err, ErrReleaseVersionConflict) {
		t.Fatalf("expected build version conflict, got %v", err)
	}

	r.ID, r.Status, r.CardCount, r.ChunkCount = 91, ReleaseStatusReady, 1, 1
	activate := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_libraries", columns: []string{"id", "current_version"}, rows: [][]driver.Value{{r.LibraryID, int64(r.Version)}}},
		{kind: "query", contains: "SELECT library_id, status", columns: []string{"library_id", "status"}, rows: [][]driver.Value{{r.LibraryID, string(ReleaseStatusReady)}}},
		{kind: "query", contains: "FROM theory_library_releases", columns: releaseColumns(), rows: [][]driver.Value{releaseValues(r)}},
		{kind: "rollback"},
	}}
	err = testStore(t, activate).ActivateRelease(context.Background(), r.LibraryID, r.ID, 7)
	if !errors.Is(err, ErrReleaseVersionConflict) {
		t.Fatalf("expected activation version conflict, got %v", err)
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
	script := buildPrefix(r, []sqlStep{validReleaseCardStep(), validReleaseSourcesStep(), {kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(12), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), int64(2), int64(2), nil, nil, nil, nil, string(StatusPublished), true}}}, {kind: "rollback"}})
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
		validReleaseCardStep(),
		validReleaseSourcesStep(),
		{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(11), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), int64(2), int64(2), nil, nil, nil, nil, string(StatusPublished), true}}},
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

func TestReleaseStoreBuildValidatesEverySourceWithPublishValidator(t *testing.T) {
	r := testRelease(RetrievalLexicalOnly)
	card := testCard(StatusPublished)
	primary := testCardSource(card.ID)
	supporting := testCardSource(card.ID)
	supporting.ID = 22
	supporting.SourceRole = SourceRoleSupporting
	supporting.ExtractionQuality = .50
	steps := []sqlStep{
		{kind: "query", contains: "FROM theory_cards", columns: cardColumns(), rows: [][]driver.Value{cardValues(card)}},
		{kind: "query", contains: "FROM theory_card_sources", columns: cardSourceColumns(), rows: [][]driver.Value{cardSourceValues(primary), cardSourceValues(supporting)}},
		{kind: "rollback"},
	}
	script := buildPrefix(r, steps)
	_, err := testStore(t, script).BuildRelease(context.Background(), r, []ReleaseMapping{{CardID: card.ID, ChunkID: 71}})
	if err == nil || !strings.Contains(err.Error(), "extraction_quality") {
		t.Fatalf("expected full source validation failure, got %v", err)
	}
	script.assertDone(t)
}

func TestReleaseStoreRejectsMappedChunkFromDifferentCardVersion(t *testing.T) {
	r := testRelease(RetrievalLexicalOnly)
	build := buildPrefix(r, []sqlStep{validReleaseCardStep(), validReleaseSourcesStep(),
		{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(11), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), int64(1), int64(2), nil, nil, nil, nil, string(StatusPublished), true}}},
		{kind: "rollback"}})
	_, err := testStore(t, build).BuildRelease(context.Background(), r, []ReleaseMapping{{CardID: 11, ChunkID: 71}})
	if !errors.Is(err, ErrInvalidReleaseMapping) {
		t.Fatalf("build expected version mismatch, got %v", err)
	}

	r.ID, r.Status, r.CardCount, r.ChunkCount = 91, ReleaseStatusReady, 1, 1
	steps := activationValidationPrefix(r)
	steps = append(steps, validReleaseCardStep(), validReleaseSourcesStep(),
		sqlStep{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(11), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), int64(1), int64(2), nil, nil, nil, nil, string(StatusPublished), true}}},
		sqlStep{kind: "rollback"})
	err = testStore(t, &sqlScript{steps: steps}).ActivateRelease(context.Background(), r.LibraryID, r.ID, 7)
	if !errors.Is(err, ErrInvalidReleaseMapping) {
		t.Fatalf("activation expected version mismatch, got %v", err)
	}
}

func TestReleaseStoreRejectsMappedPracticeChunkUnlessPublishedAndVersionAligned(t *testing.T) {
	r := testRelease(RetrievalLexicalOnly)
	invalid := []driver.Value{int64(11), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), int64(2), int64(2), int64(51), int64(11), string(StatusDraft), int64(2), string(StatusPublished), true}
	build := buildPrefix(r, []sqlStep{validReleaseCardStep(), validReleaseSourcesStep(),
		{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{invalid}}, {kind: "rollback"}})
	_, err := testStore(t, build).BuildRelease(context.Background(), r, []ReleaseMapping{{CardID: 11, ChunkID: 71}})
	if !errors.Is(err, ErrInvalidReleaseMapping) {
		t.Fatalf("build accepted draft practice chunk: %v", err)
	}

	r.ID, r.Status, r.CardCount, r.ChunkCount = 91, ReleaseStatusReady, 1, 1
	steps := activationValidationPrefix(r)
	steps = append(steps, validReleaseCardStep(), validReleaseSourcesStep(), sqlStep{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{invalid}}, sqlStep{kind: "rollback"})
	err = testStore(t, &sqlScript{steps: steps}).ActivateRelease(context.Background(), r.LibraryID, r.ID, 7)
	if !errors.Is(err, ErrInvalidReleaseMapping) {
		t.Fatalf("activation accepted draft practice chunk: %v", err)
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
		{"not_ready", [][]driver.Value{{"text-embedding-3-small", int64(1536), strings.Repeat("a", 64), string(EmbeddingStatusPending), true}}, ErrEmbeddingNotReady},
		{"null_vector", [][]driver.Value{{"text-embedding-3-small", int64(1536), strings.Repeat("a", 64), string(EmbeddingStatusReady), false}}, ErrEmbeddingNotReady},
		{"stale", [][]driver.Value{{"text-embedding-3-small", int64(1536), strings.Repeat("b", 64), string(EmbeddingStatusReady), true}}, ErrStaleEmbedding},
		{"wrong_model", [][]driver.Value{{"other", int64(1536), strings.Repeat("a", 64), string(EmbeddingStatusReady), true}}, ErrEmbeddingNotReady},
		{"wrong_dimension", [][]driver.Value{{"text-embedding-3-small", int64(2), strings.Repeat("a", 64), string(EmbeddingStatusReady), true}}, ErrEmbeddingNotReady},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := testRelease(RetrievalHybrid)
			steps := []sqlStep{
				{kind: "query", contains: "information_schema.columns", columns: []string{"exists"}, rows: [][]driver.Value{{true}}},
				validReleaseCardStep(),
				validReleaseSourcesStep(),
				{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(11), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), int64(2), int64(2), nil, nil, nil, nil, string(StatusPublished), true}}},
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
		{kind: "query", contains: "FROM theory_libraries", columns: []string{"id", "current_version"}, rows: [][]driver.Value{{r.LibraryID, int64(3)}}},
		{kind: "query", contains: "SELECT library_id, status", columns: []string{"library_id", "status"}, rows: [][]driver.Value{{r.LibraryID, string(ReleaseStatusReady)}}},
		{kind: "query", contains: "FROM theory_library_releases", columns: releaseColumns(), rows: [][]driver.Value{releaseValues(r)}},
		{kind: "query", contains: "FROM theory_release_cards mapping", columns: []string{"card_id", "chunk_id"}, rows: [][]driver.Value{{int64(11), int64(71)}}},
		validReleaseCardStep(),
		validReleaseSourcesStep(),
		{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(11), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), int64(2), int64(2), nil, nil, nil, nil, string(StatusPublished), true}}},
		{kind: "exec", contains: "status = 'retired'", affected: 1},
		{kind: "exec", contains: "status = 'active'", affected: 1},
		{kind: "exec", contains: "current_version", affected: 1},
		{kind: "commit"},
	}}
	if err := testStore(t, script).ActivateRelease(context.Background(), r.LibraryID, r.ID, 7); err != nil {
		t.Fatal(err)
	}
	calls := script.callsSnapshot()
	activeCall := calls[indexCall(calls, "status = 'active'")]
	if activeCall.args[len(activeCall.args)-1] != int64(7) {
		t.Fatalf("activated_by parameter missing: %#v", activeCall.args)
	}
	if !(indexCall(calls, "lock_theory_libraries") < indexCall(calls, "FROM theory_libraries") && indexCall(calls, "status = 'retired'") < indexCall(calls, "status = 'active'") && indexCall(calls, "status = 'active'") < indexCall(calls, "SET current_version")) {
		t.Fatalf("activation order wrong: %#v", calls)
	}
}

func TestReleaseStoreActivationRejectsInvalidActorBeforeSQL(t *testing.T) {
	script := &sqlScript{}
	err := testStore(t, script).ActivateRelease(context.Background(), 3, 91, 0)
	if err == nil || !strings.Contains(err.Error(), "activated by") {
		t.Fatalf("expected invalid actor rejection, got %v", err)
	}
	if len(script.callsSnapshot()) != 0 {
		t.Fatal("invalid actor reached SQL")
	}
}

func TestReleaseStoreActivationFailureRollsBackBeforeOldActiveChanges(t *testing.T) {
	r := testRelease(RetrievalLexicalOnly)
	r.ID, r.Status, r.CardCount, r.ChunkCount = 91, ReleaseStatusReady, 1, 2
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_libraries", columns: []string{"id", "current_version"}, rows: [][]driver.Value{{r.LibraryID, int64(3)}}},
		{kind: "query", contains: "SELECT library_id, status", columns: []string{"library_id", "status"}, rows: [][]driver.Value{{r.LibraryID, string(ReleaseStatusReady)}}},
		{kind: "query", contains: "FROM theory_library_releases", columns: releaseColumns(), rows: [][]driver.Value{releaseValues(r)}},
		{kind: "query", contains: "FROM theory_release_cards mapping", columns: []string{"card_id", "chunk_id"}, rows: [][]driver.Value{{int64(11), int64(71)}}},
		{kind: "rollback"},
	}}
	err := testStore(t, script).ActivateRelease(context.Background(), r.LibraryID, r.ID, 7)
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
		{kind: "query", contains: "FROM theory_libraries", columns: []string{"id", "current_version"}, rows: [][]driver.Value{{r.LibraryID, int64(3)}}},
		{kind: "query", contains: "SELECT library_id, status", columns: []string{"library_id", "status"}, rows: [][]driver.Value{{r.LibraryID, string(ReleaseStatusReady)}}},
		{kind: "query", contains: "FROM theory_library_releases", columns: releaseColumns()},
		{kind: "rollback"},
	}}
	err := testStore(t, script).ActivateRelease(context.Background(), r.LibraryID, 999, 7)
	if !errors.Is(err, ErrReleaseNotReady) {
		t.Fatalf("expected ErrReleaseNotReady, got %v", err)
	}
}

func TestReleaseStoreActivationReturnsDistinctStableLookupErrors(t *testing.T) {
	cases := []struct {
		name string
		rows [][]driver.Value
		want error
	}{
		{name: "missing", want: ErrReleaseNotFound},
		{name: "different_library", rows: [][]driver.Value{{int64(9), string(ReleaseStatusReady)}}, want: ErrReleaseLibraryMismatch},
		{name: "not_ready", rows: [][]driver.Value{{int64(3), string(ReleaseStatusRetired)}}, want: ErrReleaseNotReady},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := &sqlScript{steps: []sqlStep{
				{kind: "begin"},
				{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
				{kind: "query", contains: "FROM theory_libraries", columns: []string{"id", "current_version"}, rows: [][]driver.Value{{int64(3), int64(3)}}},
				{kind: "query", contains: "SELECT library_id, status", columns: []string{"library_id", "status"}, rows: tc.rows},
				{kind: "rollback"},
			}}
			err := testStore(t, script).ActivateRelease(context.Background(), 3, 91, 7)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestReleaseStoreActivationReturnsLibraryNotFound(t *testing.T) {
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_libraries", columns: []string{"id", "current_version"}},
		{kind: "rollback"},
	}}
	err := testStore(t, script).ActivateRelease(context.Background(), 3, 91, 7)
	if !errors.Is(err, ErrLibraryNotFound) {
		t.Fatalf("expected ErrLibraryNotFound, got %v", err)
	}
}

func TestReleaseStoreActivationRevalidatesHybridEmbedding(t *testing.T) {
	cases := []struct {
		name string
		row  []driver.Value
		want error
	}{
		{"status", []driver.Value{"text-embedding-3-small", int64(1536), strings.Repeat("a", 64), string(EmbeddingStatusPending), true}, ErrEmbeddingNotReady},
		{"null", []driver.Value{"text-embedding-3-small", int64(1536), strings.Repeat("a", 64), string(EmbeddingStatusReady), false}, ErrEmbeddingNotReady},
		{"model", []driver.Value{"other", int64(1536), strings.Repeat("a", 64), string(EmbeddingStatusReady), true}, ErrEmbeddingNotReady},
		{"hash", []driver.Value{"text-embedding-3-small", int64(1536), strings.Repeat("b", 64), string(EmbeddingStatusReady), true}, ErrStaleEmbedding},
		{"dimension", []driver.Value{"text-embedding-3-small", int64(2), strings.Repeat("a", 64), string(EmbeddingStatusReady), true}, ErrEmbeddingNotReady},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := testRelease(RetrievalHybrid)
			r.ID, r.Status, r.CardCount, r.ChunkCount = 91, ReleaseStatusReady, 1, 1
			steps := activationValidationPrefix(r)
			steps = append(steps,
				sqlStep{kind: "query", contains: "information_schema.columns", columns: []string{"exists"}, rows: [][]driver.Value{{true}}},
				validReleaseCardStep(), validReleaseSourcesStep(),
				sqlStep{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(11), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), int64(2), int64(2), nil, nil, nil, nil, string(StatusPublished), true}}},
				sqlStep{kind: "query", contains: "FROM theory_chunk_embeddings", columns: []string{"embedding_model", "dimensions", "content_hash", "status", "has_embedding"}, rows: [][]driver.Value{tc.row}},
				sqlStep{kind: "rollback"},
			)
			err := testStore(t, &sqlScript{steps: steps}).ActivateRelease(context.Background(), r.LibraryID, r.ID, 7)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestReleaseStoreActivationRollsBackFailuresAfterRetiringOldActive(t *testing.T) {
	failure := errors.New("injected activation failure")
	for _, stage := range []string{"activate", "library"} {
		t.Run(stage, func(t *testing.T) {
			r := testRelease(RetrievalLexicalOnly)
			r.ID, r.Status, r.CardCount, r.ChunkCount = 91, ReleaseStatusReady, 1, 1
			steps := activationValidationPrefix(r)
			steps = append(steps, validReleaseCardStep(), validReleaseSourcesStep(),
				sqlStep{kind: "query", contains: "FROM theory_chunks chunk", columns: mappingValidationColumns(), rows: [][]driver.Value{{int64(11), r.LibraryID, string(ChunkStatusEnabled), strings.Repeat("a", 64), int64(2), int64(2), nil, nil, nil, nil, string(StatusPublished), true}}},
				sqlStep{kind: "exec", contains: "status = 'retired'", affected: 1})
			if stage == "activate" {
				steps = append(steps, sqlStep{kind: "exec", contains: "status = 'active'", err: failure})
			} else {
				steps = append(steps, sqlStep{kind: "exec", contains: "status = 'active'", affected: 1}, sqlStep{kind: "exec", contains: "current_version", err: failure})
			}
			steps = append(steps, sqlStep{kind: "rollback"})
			script := &sqlScript{steps: steps}
			err := testStore(t, script).ActivateRelease(context.Background(), r.LibraryID, r.ID, 7)
			if !errors.Is(err, failure) {
				t.Fatalf("expected injected failure, got %v", err)
			}
			calls := script.callsSnapshot()
			if indexCall(calls, "status = 'retired'") < 0 || indexCall(calls, "ROLLBACK") <= indexCall(calls, "status = 'retired'") {
				t.Fatalf("rollback did not follow retire: %#v", calls)
			}
		})
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
	steps := []sqlStep{{kind: "begin"}, {kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}}, {kind: "query", contains: "FROM theory_libraries", columns: []string{"id", "current_version"}, rows: [][]driver.Value{{r.LibraryID, int64(3)}}}, {kind: "query", contains: "INSERT INTO theory_library_releases", columns: releaseColumns(), rows: [][]driver.Value{releaseValues(building)}}, {kind: "exec", contains: "DELETE FROM theory_release_cards", affected: 0}}
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
	return []string{"card_id", "library_id", "chunk_status", "content_hash", "chunk_version", "card_version", "practice_id", "practice_card_id", "practice_status", "practice_version", "card_status", "has_primary"}
}

func validReleaseCardStep() sqlStep {
	card := testCard(StatusPublished)
	return sqlStep{kind: "query", contains: "FROM theory_cards", columns: cardColumns(), rows: [][]driver.Value{cardValues(card)}}
}

func validReleaseSourcesStep() sqlStep {
	source := testCardSource(11)
	return sqlStep{kind: "query", contains: "FROM theory_card_sources", columns: cardSourceColumns(), rows: [][]driver.Value{cardSourceValues(source)}}
}

func activationValidationPrefix(r Release) []sqlStep {
	return []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_libraries", columns: []string{"id", "current_version"}, rows: [][]driver.Value{{r.LibraryID, int64(3)}}},
		{kind: "query", contains: "SELECT library_id, status", columns: []string{"library_id", "status"}, rows: [][]driver.Value{{r.LibraryID, string(ReleaseStatusReady)}}},
		{kind: "query", contains: "FROM theory_library_releases", columns: releaseColumns(), rows: [][]driver.Value{releaseValues(r)}},
		{kind: "query", contains: "FROM theory_release_cards mapping", columns: []string{"card_id", "chunk_id"}, rows: [][]driver.Value{{int64(11), int64(71)}}},
	}
}
