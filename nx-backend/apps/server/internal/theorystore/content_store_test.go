package theorystore

import (
	"context"
	"database/sql/driver"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestContentStoreSaveCardSourcePersistsAuditFields(t *testing.T) {
	source := testCardSource(11)
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		validCardSourceScopeStep(source),
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		validLockedCardSourceStep(StatusDraft),
		{kind: "query", contains: "FROM theory_source_files file", columns: []string{"file_work_id"}, rows: [][]driver.Value{{source.WorkID}}},
		{kind: "query", contains: "INSERT INTO theory_card_sources", columns: cardSourceColumns(), rows: [][]driver.Value{cardSourceValues(source)}},
		{kind: "commit"},
	}}
	saved, err := testStore(t, script).SaveCardSource(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if saved.VerifiedBy == nil || saved.VerifiedAt == nil || saved.Quotation == "" {
		t.Fatalf("audit fields not scanned: %+v", saved)
	}
	call := script.callsSnapshot()[5]
	if len(call.args) != 14 || call.args[0] != source.CardID || call.args[1] != source.WorkID || call.args[11] != source.QuoteVerified {
		t.Fatalf("source arguments incomplete: %#v", call.args)
	}
	if strings.Contains(script.callsSnapshot()[3].query, "file") {
		t.Fatal("attempted to row-lock nullable file join")
	}
}

func TestContentStoreRejectsMutationOwnedByPublishedCard(t *testing.T) {
	source := testCardSource(11)
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		validCardSourceScopeStep(source),
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		validLockedCardSourceStep(StatusPublished),
		{kind: "rollback"},
	}}
	_, err := testStore(t, script).SaveCardSource(context.Background(), source)
	if !errors.Is(err, ErrCardNotEditable) {
		t.Fatalf("expected ErrCardNotEditable, got %v", err)
	}
	script.assertDone(t)
}

func TestContentStorePracticeAndRelationAlsoRequireDraftOwners(t *testing.T) {
	t.Run("practice", func(t *testing.T) {
		p := testPractice()
		script := &sqlScript{steps: []sqlStep{
			{kind: "begin"},
			{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{int64(3)}}},
			{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
			{kind: "query", contains: "FOR UPDATE", columns: []string{"status"}, rows: [][]driver.Value{{string(StatusPublished)}}},
			{kind: "rollback"},
		}}
		_, err := testStore(t, script).SavePractice(context.Background(), p)
		if !errors.Is(err, ErrCardNotEditable) {
			t.Fatalf("expected ErrCardNotEditable, got %v", err)
		}
	})
	t.Run("relation", func(t *testing.T) {
		r := testRelation()
		script := &sqlScript{steps: []sqlStep{
			{kind: "begin"},
			{kind: "query", contains: "WHERE id IN", columns: []string{"id", "library_id"}, rows: [][]driver.Value{{r.FromCardID, int64(3)}, {r.ToCardID, int64(3)}}},
			{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
			{kind: "query", contains: "FOR UPDATE", columns: []string{"id", "library_id", "status"}, rows: [][]driver.Value{{r.FromCardID, int64(3), string(StatusDraft)}, {r.ToCardID, int64(3), string(StatusPublished)}}},
			{kind: "rollback"},
		}}
		_, err := testStore(t, script).SaveRelation(context.Background(), r)
		if !errors.Is(err, ErrCardNotEditable) {
			t.Fatalf("expected ErrCardNotEditable, got %v", err)
		}
	})
}

func TestContentStoreCardSourceLocksFullOwnershipScopeBeforeRows(t *testing.T) {
	source := testCardSource(11)
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "JOIN theory_source_works work", columns: []string{"card_library_id", "work_library_id", "file_library_id", "file_work_id"}, rows: [][]driver.Value{{int64(9), int64(2), int64(1), source.WorkID}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE OF card", columns: []string{"status", "card_library_id", "work_library_id"}, rows: [][]driver.Value{{string(StatusDraft), int64(9), int64(2)}}},
		{kind: "rollback"},
	}}
	_, err := testStore(t, script).SaveCardSource(context.Background(), source)
	if !errors.Is(err, ErrInvalidContentOwnership) {
		t.Fatalf("expected ownership rejection, got %v", err)
	}
	calls := script.callsSnapshot()
	if !(indexCall(calls, "JOIN theory_source_works work") < indexCall(calls, "lock_theory_libraries") && indexCall(calls, "lock_theory_libraries") < indexCall(calls, "FOR UPDATE OF card")) {
		t.Fatalf("source lock order wrong: %#v", calls)
	}
}

func TestContentStoreRejectsInvalidPracticeRelationAndEmbedding(t *testing.T) {
	store := testStore(t, &sqlScript{})
	source := testCardSource(11)
	source.FileID = int64ptr(0)
	if _, err := store.SaveCardSource(context.Background(), source); err == nil {
		t.Fatal("invalid source file id accepted")
	}
	practice := testPractice()
	practice.Steps = []byte(`{}`)
	if _, err := store.SavePractice(context.Background(), practice); err == nil {
		t.Fatal("invalid practice accepted")
	}
	relation := testRelation()
	relation.ToCardID = relation.FromCardID
	if _, err := store.SaveRelation(context.Background(), relation); err == nil {
		t.Fatal("self relation accepted")
	}
	record := testEmbedding()
	record.Embedding[0] = float32(math.Inf(1))
	if _, err := store.SaveEmbeddingRecord(context.Background(), record); err == nil {
		t.Fatal("non-finite embedding accepted")
	}
}

func TestContentStorePracticeChangeMarksEmbeddingsStale(t *testing.T) {
	p := testPractice()
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{int64(3)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE", columns: []string{"status"}, rows: [][]driver.Value{{string(StatusDraft)}}},
		{kind: "query", contains: "INSERT INTO theory_practices", columns: practiceColumns(), rows: [][]driver.Value{practiceValues(p)}},
		{kind: "exec", contains: "status = 'stale'", affected: 2},
		{kind: "commit"},
	}}
	if _, err := testStore(t, script).SavePractice(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if indexCall(script.callsSnapshot(), "INSERT INTO theory_practices") >= indexCall(script.callsSnapshot(), "status = 'stale'") {
		t.Fatal("embeddings invalidated before practice save")
	}
}

func TestContentStoreSavePracticeRejectsNonDraftStatusBeforeSQL(t *testing.T) {
	for _, status := range []CardStatus{StatusInReview, StatusPublished, StatusSuperseded, StatusRetired} {
		p := testPractice()
		p.Status = status
		script := &sqlScript{}
		_, err := testStore(t, script).SavePractice(context.Background(), p)
		if !errors.Is(err, ErrPracticeNotEditable) {
			t.Fatalf("status %s: expected ErrPracticeNotEditable, got %v", status, err)
		}
		if len(script.callsSnapshot()) != 0 {
			t.Fatalf("status %s reached SQL", status)
		}
	}
}

func TestContentStoreSaveRelationUsesValidatorAndAllFields(t *testing.T) {
	r := testRelation()
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "WHERE id IN", columns: []string{"id", "library_id"}, rows: [][]driver.Value{{r.FromCardID, int64(3)}, {r.ToCardID, int64(3)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE", columns: []string{"id", "library_id", "status"}, rows: [][]driver.Value{{r.FromCardID, int64(3), string(StatusDraft)}, {r.ToCardID, int64(3), string(StatusDraft)}}},
		{kind: "query", contains: "INSERT INTO theory_card_relations", columns: relationColumns(), rows: [][]driver.Value{relationValues(r)}},
		{kind: "commit"},
	}}
	saved, err := testStore(t, script).SaveRelation(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Confidence != r.Confidence || len(script.callsSnapshot()[4].args) != 8 {
		t.Fatalf("relation fields missing: %+v %#v", saved, script.callsSnapshot()[4].args)
	}
}

func TestContentStoreRelationRejectsOwnershipScopeChangeAfterAdvisoryLock(t *testing.T) {
	r := testRelation()
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "WHERE id IN", columns: []string{"id", "library_id"}, rows: [][]driver.Value{{r.FromCardID, int64(3)}, {r.ToCardID, int64(3)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE", columns: []string{"id", "library_id", "status"}, rows: [][]driver.Value{{r.FromCardID, int64(3), string(StatusDraft)}, {r.ToCardID, int64(2), string(StatusDraft)}}},
		{kind: "rollback"},
	}}
	_, err := testStore(t, script).SaveRelation(context.Background(), r)
	if !errors.Is(err, ErrOwnershipChanged) {
		t.Fatalf("expected ErrOwnershipChanged, got %v", err)
	}
	script.assertDone(t)
}

func TestContentStoreChunkCreatesImmutableVersionWithoutTouchingOldEmbeddings(t *testing.T) {
	c := testChunk()
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		validChunkScopeStep(c),
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		validLockedChunkStep(StatusPublished, c.Version),
		{kind: "query", contains: "INSERT INTO theory_chunks", columns: chunkColumns(), rows: [][]driver.Value{chunkValues(c)}},
		{kind: "commit"},
	}}
	saved, err := testStore(t, script).SaveChunk(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ContentHash != c.ContentHash || saved.Version != c.Version {
		t.Fatalf("chunk hash/version lost: %+v", saved)
	}
}

func TestContentStoreChunkVersionConflictCannotOverwriteSnapshot(t *testing.T) {
	c := testChunk()
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		validChunkScopeStep(c),
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		validLockedChunkStep(StatusPublished, c.Version),
		{kind: "query", contains: "ON CONFLICT", columns: chunkColumns()},
		{kind: "rollback"},
	}}
	_, err := testStore(t, script).SaveChunk(context.Background(), c)
	if !errors.Is(err, ErrChunkVersionConflict) {
		t.Fatalf("expected ErrChunkVersionConflict, got %v", err)
	}
	for _, call := range script.callsSnapshot() {
		if strings.Contains(call.query, "DO UPDATE") {
			t.Fatalf("chunk snapshot was overwriteable: %s", call.query)
		}
	}
}

func TestContentStorePracticeChunkLocksPracticeLibraryBeforeRows(t *testing.T) {
	c := testChunk()
	c.ChunkKind = ChunkKindPractice
	c.PracticeID = int64ptr(51)
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "LEFT JOIN theory_practices", columns: []string{"card_library_id", "practice_card_id", "practice_library_id"}, rows: [][]driver.Value{{int64(9), int64(12), int64(2)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE OF card", columns: []string{"status", "card_library_id", "card_version"}, rows: [][]driver.Value{{string(StatusPublished), int64(9), int64(c.Version)}}},
		{kind: "query", contains: "FROM theory_practices practice", columns: []string{"practice_card_id", "practice_library_id", "practice_status", "practice_version"}, rows: [][]driver.Value{{int64(12), int64(2), string(StatusPublished), int64(c.Version)}}},
		{kind: "rollback"},
	}}
	_, err := testStore(t, script).SaveChunk(context.Background(), c)
	if !errors.Is(err, ErrInvalidContentOwnership) {
		t.Fatalf("expected practice ownership rejection, got %v", err)
	}
}

func TestContentStorePracticeChunkRequiresPublishedMatchingPracticeVersion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  CardStatus
		version int
	}{
		{name: "draft", status: StatusDraft, version: 2},
		{name: "wrong_version", status: StatusPublished, version: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testChunk()
			c.ChunkKind = ChunkKindPractice
			c.PracticeID = int64ptr(51)
			script := &sqlScript{steps: []sqlStep{
				{kind: "begin"},
				{kind: "query", contains: "LEFT JOIN theory_practices", columns: []string{"card_library_id", "practice_card_id", "practice_library_id"}, rows: [][]driver.Value{{int64(3), c.CardID, int64(3)}}},
				{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
				validLockedChunkStep(StatusPublished, c.Version),
				{kind: "query", contains: "FROM theory_practices practice", columns: []string{"practice_card_id", "practice_library_id", "practice_status", "practice_version"}, rows: [][]driver.Value{{c.CardID, int64(3), string(tc.status), int64(tc.version)}}},
				{kind: "rollback"},
			}}
			_, err := testStore(t, script).SaveChunk(context.Background(), c)
			if !errors.Is(err, ErrPracticeNotPublishable) {
				t.Fatalf("expected ErrPracticeNotPublishable, got %v", err)
			}
		})
	}
}

func TestContentStoreChunkRequiresPublishedCardAndMatchingVersion(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      CardStatus
		cardVersion int
		want        error
	}{
		{name: "draft", status: StatusDraft, cardVersion: 2, want: ErrCardNotEditable},
		{name: "version_mismatch", status: StatusPublished, cardVersion: 3, want: ErrChunkCardVersionMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testChunk()
			script := &sqlScript{steps: []sqlStep{
				{kind: "begin"}, validChunkScopeStep(c),
				{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
				validLockedChunkStep(tc.status, tc.cardVersion), {kind: "rollback"},
			}}
			_, err := testStore(t, script).SaveChunk(context.Background(), c)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestContentStoreEmbeddingRejectsAsyncResultForOldChunkHashUnderLibraryLock(t *testing.T) {
	r := testEmbedding()
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{int64(3)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE", columns: []string{"library_id", "content_hash"}, rows: [][]driver.Value{{int64(3), strings.Repeat("b", 64)}}},
		{kind: "rollback"},
	}}
	_, err := testStore(t, script).SaveEmbeddingRecord(context.Background(), r)
	if !errors.Is(err, ErrStaleEmbedding) {
		t.Fatalf("expected ErrStaleEmbedding, got %v", err)
	}
	calls := script.callsSnapshot()
	if !(indexCall(calls, "SELECT library_id") < indexCall(calls, "lock_theory_libraries") && indexCall(calls, "lock_theory_libraries") < indexCall(calls, "FOR UPDATE")) {
		t.Fatalf("embedding lock order wrong: %#v", calls)
	}
}

func TestContentStoreEmbeddingCannotReviveStaleGenerationWithSameChunkHash(t *testing.T) {
	r := testEmbedding()
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{int64(3)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_chunks", columns: []string{"library_id", "content_hash"}, rows: [][]driver.Value{{int64(3), r.ContentHash}}},
		{kind: "query", contains: "SELECT status FROM theory_chunk_embeddings", columns: []string{"status"}, rows: [][]driver.Value{{string(EmbeddingStatusStale)}}},
		{kind: "rollback"},
	}}
	_, err := testStore(t, script).SaveEmbeddingRecord(context.Background(), r)
	if !errors.Is(err, ErrStaleEmbedding) {
		t.Fatalf("expected stale generation rejection, got %v", err)
	}
}

func TestContentStoreLexicalEmbeddingMetadataDoesNotPretendVectorExists(t *testing.T) {
	r := testEmbedding()
	r.Status, r.Embedding, r.EmbeddedAt = EmbeddingStatusPending, nil, nil
	script := embeddingSaveScript(r, false)
	saved, err := testStore(t, script).SaveEmbeddingRecord(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != EmbeddingStatusPending {
		t.Fatalf("unexpected status: %s", saved.Status)
	}
	if strings.Contains(script.callsSnapshot()[5].query, " embedding,") {
		t.Fatal("metadata-only write referenced vector column")
	}
}

func TestContentStorePendingEmbeddingStateMachine(t *testing.T) {
	base := testEmbedding()
	base.Status, base.Embedding, base.EmbeddedAt = EmbeddingStatusPending, nil, nil
	t.Run("ready_rejected", func(t *testing.T) {
		script := embeddingGenerationScript(base, EmbeddingStatusReady, nil)
		_, err := testStore(t, script).SaveEmbeddingRecord(context.Background(), base)
		if !errors.Is(err, ErrEmbeddingAlreadyReady) {
			t.Fatalf("expected ErrEmbeddingAlreadyReady, got %v", err)
		}
	})
	t.Run("pending_idempotent", func(t *testing.T) {
		existing := base
		existing.ID = 81
		script := embeddingGenerationScript(base, EmbeddingStatusPending, &existing)
		saved, err := testStore(t, script).SaveEmbeddingRecord(context.Background(), base)
		if err != nil || saved.ID != existing.ID || saved.Status != EmbeddingStatusPending {
			t.Fatalf("pending idempotency failed: %+v %v", saved, err)
		}
		if indexCall(script.callsSnapshot(), "INSERT INTO theory_chunk_embeddings") >= 0 {
			t.Fatal("pending idempotency rewrote generation")
		}
	})
	t.Run("stale_restarts", func(t *testing.T) {
		restarted := base
		restarted.ID = 81
		script := embeddingGenerationScript(base, EmbeddingStatusStale, &restarted)
		saved, err := testStore(t, script).SaveEmbeddingRecord(context.Background(), base)
		if err != nil || saved.Status != EmbeddingStatusPending {
			t.Fatalf("stale restart failed: %+v %v", saved, err)
		}
		if !strings.Contains(script.callsSnapshot()[6].query, "embedding=NULL") {
			t.Fatal("vector-capable stale restart did not clear old vector")
		}
	})
}

func TestContentStoreVectorWriteRequiresCapability(t *testing.T) {
	r := testEmbedding()
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{int64(3)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE", columns: []string{"library_id", "content_hash"}, rows: [][]driver.Value{{int64(3), r.ContentHash}}},
		{kind: "query", contains: "SELECT status FROM theory_chunk_embeddings", columns: []string{"status"}, rows: [][]driver.Value{{string(EmbeddingStatusPending)}}},
		{kind: "query", contains: "information_schema.columns", columns: []string{"exists"}, rows: [][]driver.Value{{false}}},
		{kind: "rollback"},
	}}
	_, err := testStore(t, script).SaveEmbeddingRecord(context.Background(), r)
	if !errors.Is(err, ErrVectorUnavailable) {
		t.Fatalf("expected ErrVectorUnavailable, got %v", err)
	}
}

func TestContentStoreReadyVectorWrites1536ValuesOnlyWhenColumnExists(t *testing.T) {
	r := testEmbedding()
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{int64(3)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE", columns: []string{"library_id", "content_hash"}, rows: [][]driver.Value{{int64(3), r.ContentHash}}},
		{kind: "query", contains: "SELECT status FROM theory_chunk_embeddings", columns: []string{"status"}, rows: [][]driver.Value{{string(EmbeddingStatusPending)}}},
		{kind: "query", contains: "information_schema.columns", columns: []string{"exists"}, rows: [][]driver.Value{{true}}},
		{kind: "query", contains: "UPDATE theory_chunk_embeddings", columns: embeddingColumns(), rows: [][]driver.Value{embeddingValues(r)}},
		{kind: "commit"},
	}}
	if _, err := testStore(t, script).SaveEmbeddingRecord(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	calls := script.callsSnapshot()
	vector, ok := calls[6].args[4].(string)
	if !ok || strings.Count(vector, ",") != 1535 {
		t.Fatalf("vector argument is not 1536-dimensional: %T", calls[6].args[4])
	}
}

func TestContentStoreReadyEmbeddingRequiresExistingPendingGeneration(t *testing.T) {
	r := testEmbedding()
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{int64(3)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_chunks", columns: []string{"library_id", "content_hash"}, rows: [][]driver.Value{{int64(3), r.ContentHash}}},
		{kind: "query", contains: "SELECT status FROM theory_chunk_embeddings", columns: []string{"status"}},
		{kind: "rollback"},
	}}
	_, err := testStore(t, script).SaveEmbeddingRecord(context.Background(), r)
	if !errors.Is(err, ErrEmbeddingNotPending) {
		t.Fatalf("expected pending generation requirement, got %v", err)
	}
}

func TestContentStoreFailedEmbeddingAlsoRequiresPendingGeneration(t *testing.T) {
	r := testEmbedding()
	r.Status, r.Embedding = EmbeddingStatusFailed, nil
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{int64(3)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_chunks", columns: []string{"library_id", "content_hash"}, rows: [][]driver.Value{{int64(3), r.ContentHash}}},
		{kind: "query", contains: "SELECT status FROM theory_chunk_embeddings", columns: []string{"status"}},
		{kind: "rollback"},
	}}
	_, err := testStore(t, script).SaveEmbeddingRecord(context.Background(), r)
	if !errors.Is(err, ErrEmbeddingNotPending) {
		t.Fatalf("expected ErrEmbeddingNotPending, got %v", err)
	}
}

func TestContentStorePendingOrFailedEmbeddingCannotCarryVector(t *testing.T) {
	for _, status := range []EmbeddingStatus{EmbeddingStatusPending, EmbeddingStatusFailed, EmbeddingStatusStale} {
		r := testEmbedding()
		r.Status = status
		script := &sqlScript{}
		_, err := testStore(t, script).SaveEmbeddingRecord(context.Background(), r)
		if err == nil || !strings.Contains(err.Error(), "must be empty") {
			t.Fatalf("status %s accepted vector: %v", status, err)
		}
		if len(script.callsSnapshot()) != 0 {
			t.Fatalf("status %s reached SQL", status)
		}
	}
}

func embeddingSaveScript(r EmbeddingRecord, vector bool) *sqlScript {
	steps := []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{int64(3)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE", columns: []string{"library_id", "content_hash"}, rows: [][]driver.Value{{int64(3), r.ContentHash}}},
		{kind: "query", contains: "SELECT status FROM theory_chunk_embeddings", columns: []string{"status"}},
	}
	if vector {
		steps = append(steps, sqlStep{kind: "query", contains: "information_schema.columns", columns: []string{"exists"}, rows: [][]driver.Value{{true}}})
	}
	steps = append(steps, sqlStep{kind: "query", contains: "INSERT INTO theory_chunk_embeddings", columns: embeddingColumns(), rows: [][]driver.Value{embeddingValues(r)}}, sqlStep{kind: "commit"})
	return &sqlScript{steps: steps}
}

func embeddingGenerationScript(r EmbeddingRecord, existing EmbeddingStatus, result *EmbeddingRecord) *sqlScript {
	steps := []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{int64(3)}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FROM theory_chunks", columns: []string{"library_id", "content_hash"}, rows: [][]driver.Value{{int64(3), r.ContentHash}}},
		{kind: "query", contains: "SELECT status FROM theory_chunk_embeddings", columns: []string{"status"}, rows: [][]driver.Value{{string(existing)}}},
	}
	switch existing {
	case EmbeddingStatusReady:
		steps = append(steps, sqlStep{kind: "rollback"})
	case EmbeddingStatusPending:
		steps = append(steps, sqlStep{kind: "query", contains: "FROM theory_chunk_embeddings", columns: embeddingColumns(), rows: [][]driver.Value{embeddingValues(*result)}}, sqlStep{kind: "commit"})
	case EmbeddingStatusStale, EmbeddingStatusFailed:
		steps = append(steps, sqlStep{kind: "query", contains: "information_schema.columns", columns: []string{"exists"}, rows: [][]driver.Value{{true}}}, sqlStep{kind: "query", contains: "UPDATE theory_chunk_embeddings", columns: embeddingColumns(), rows: [][]driver.Value{embeddingValues(*result)}}, sqlStep{kind: "commit"})
	}
	return &sqlScript{steps: steps}
}

func validCardSourceScopeStep(source CardSource) sqlStep {
	return sqlStep{kind: "query", contains: "JOIN theory_source_works work", columns: []string{"card_library_id", "work_library_id", "file_library_id", "file_work_id"}, rows: [][]driver.Value{{int64(3), int64(3), int64(3), source.WorkID}}}
}
func validLockedCardSourceStep(status CardStatus) sqlStep {
	return sqlStep{kind: "query", contains: "FOR UPDATE OF card", columns: []string{"status", "card_library_id", "work_library_id"}, rows: [][]driver.Value{{string(status), int64(3), int64(3)}}}
}
func validChunkScopeStep(c Chunk) sqlStep {
	return sqlStep{kind: "query", contains: "LEFT JOIN theory_practices", columns: []string{"card_library_id", "practice_card_id", "practice_library_id"}, rows: [][]driver.Value{{c.LibraryID, nil, nil}}}
}
func validLockedChunkStep(status CardStatus, version int) sqlStep {
	return sqlStep{kind: "query", contains: "FOR UPDATE OF card", columns: []string{"status", "card_library_id", "card_version"}, rows: [][]driver.Value{{string(status), int64(3), int64(version)}}}
}

func testPractice() Practice {
	now := time.Unix(1700000000, 0).UTC()
	return Practice{ID: 51, CardID: 11, Goal: "observe", EstimatedMinutes: 5, Steps: []byte(`["pause"]`), ReflectionPrompts: []byte(`["what?"]`), ExpectedFeedback: []byte(`["calm"]`), StopConditions: []byte(`[]`), ProfessionalEscalation: []byte(`[]`), Contraindications: "none", PracticeSchemaVersion: PracticeSchemaV1, Status: StatusDraft, Version: 2, CreateTime: now, UpdateTime: now}
}
func testRelation() Relation {
	now := time.Unix(1700000000, 0).UTC()
	return Relation{ID: 61, FromCardID: 11, ToCardID: 12, RelationType: RelationSupports, Note: "note", Confidence: .8, Status: RelationStatusPublished, CreatedBy: int64ptr(5), ReviewedBy: int64ptr(7), CreateTime: now, UpdateTime: now}
}
func testChunk() Chunk {
	now := time.Unix(1700000000, 0).UTC()
	return Chunk{ID: 71, LibraryID: 3, CardID: 11, ChunkKey: "inner_observer/card", ChunkKind: ChunkKindCard, Title: "title", Content: "content", Keywords: []byte(`["observe"]`), Tags: []byte(`["self"]`), AuthorityLevel: 4, EvidenceLevel: EvidenceModerate, ClinicalSafety: ClinicalGeneral, TokenCount: 10, ContentHash: strings.Repeat("a", 64), Version: 2, Status: ChunkStatusEnabled, CreateTime: now, UpdateTime: now}
}
func testEmbedding() EmbeddingRecord {
	now := time.Unix(1700000000, 0).UTC()
	return EmbeddingRecord{ID: 81, ChunkID: 71, EmbeddingModel: "text-embedding-3-small", Dimensions: 1536, Embedding: make([]float32, 1536), ContentHash: strings.Repeat("a", 64), EmbeddedAt: &now, Status: EmbeddingStatusReady}
}

func practiceColumns() []string {
	return []string{"id", "card_id", "goal", "estimated_minutes", "steps", "reflection_prompts", "expected_feedback", "stop_conditions", "professional_escalation", "contraindications", "practice_schema_version", "status", "version", "create_time", "update_time"}
}
func practiceValues(p Practice) []driver.Value {
	return []driver.Value{p.ID, p.CardID, p.Goal, int64(p.EstimatedMinutes), []byte(p.Steps), []byte(p.ReflectionPrompts), []byte(p.ExpectedFeedback), []byte(p.StopConditions), []byte(p.ProfessionalEscalation), p.Contraindications, p.PracticeSchemaVersion, string(p.Status), int64(p.Version), p.CreateTime, p.UpdateTime}
}
func relationColumns() []string {
	return []string{"id", "from_card_id", "to_card_id", "relation_type", "note", "confidence", "status", "created_by", "reviewed_by", "create_time", "update_time"}
}
func relationValues(r Relation) []driver.Value {
	return []driver.Value{r.ID, r.FromCardID, r.ToCardID, string(r.RelationType), r.Note, r.Confidence, string(r.Status), nullableInt(r.CreatedBy), nullableInt(r.ReviewedBy), r.CreateTime, r.UpdateTime}
}
func chunkColumns() []string {
	return []string{"id", "library_id", "card_id", "practice_id", "chunk_key", "chunk_kind", "title", "content", "keywords", "tags", "authority_level", "evidence_level", "clinical_safety", "token_count", "content_hash", "version", "status", "create_time", "update_time"}
}
func chunkValues(c Chunk) []driver.Value {
	return []driver.Value{c.ID, c.LibraryID, c.CardID, nullableInt(c.PracticeID), c.ChunkKey, string(c.ChunkKind), c.Title, c.Content, []byte(c.Keywords), []byte(c.Tags), int64(c.AuthorityLevel), string(c.EvidenceLevel), string(c.ClinicalSafety), int64(c.TokenCount), c.ContentHash, int64(c.Version), string(c.Status), c.CreateTime, c.UpdateTime}
}
func embeddingColumns() []string {
	return []string{"id", "chunk_id", "embedding_model", "dimensions", "content_hash", "embedded_at", "status", "error_message"}
}
func embeddingValues(r EmbeddingRecord) []driver.Value {
	return []driver.Value{r.ID, r.ChunkID, r.EmbeddingModel, int64(r.Dimensions), r.ContentHash, nullableTime(r.EmbeddedAt), string(r.Status), r.ErrorMessage}
}
