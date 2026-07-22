package theorystore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCardStoreCreateUsesValidatedParameterizedSQLAndScans(t *testing.T) {
	card := testCard(StatusDraft)
	script := &sqlScript{steps: []sqlStep{{kind: "query", contains: "INSERT INTO theory_cards", columns: cardColumns(), rows: [][]driver.Value{cardValues(card)}}}}
	store := testStore(t, script)
	created, err := store.CreateCard(context.Background(), card)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != card.ID || created.CanonicalKey != card.CanonicalKey {
		t.Fatalf("unexpected scan: %+v", created)
	}
	call := script.callsSnapshot()[0]
	if len(call.args) < 10 || call.args[0] != card.LibraryID || call.args[1] != card.CanonicalKey {
		t.Fatalf("parameterized card arguments missing: %#v", call.args)
	}
}

func TestCardStoreCreateDefaultsEmptyStatusButRejectsPublishedSnapshot(t *testing.T) {
	card := testCard("")
	card.Version = 1
	draft := card
	draft.Status = StatusDraft
	script := &sqlScript{steps: []sqlStep{{kind: "query", contains: "INSERT INTO theory_cards", columns: cardColumns(), rows: [][]driver.Value{cardValues(draft)}}}}
	created, err := testStore(t, script).CreateCard(context.Background(), card)
	if err != nil || created.Status != StatusDraft {
		t.Fatalf("empty status was not normalized to draft: %+v %v", created, err)
	}

	published := testCard(StatusPublished)
	published.Version = 3
	rejectScript := &sqlScript{}
	_, err = testStore(t, rejectScript).CreateCard(context.Background(), published)
	if !errors.Is(err, ErrCardNotEditable) {
		t.Fatalf("expected published create rejection, got %v", err)
	}
	if len(rejectScript.callsSnapshot()) != 0 {
		t.Fatal("published create reached SQL")
	}
}

func TestCardStoreRejectsInvalidTransitionBeforeSQL(t *testing.T) {
	script := &sqlScript{}
	store := testStore(t, script)
	_, err := store.TransitionCard(context.Background(), 11, StatusDraft, StatusPublished, 7)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if len(script.callsSnapshot()) != 0 {
		t.Fatal("invalid transition reached database")
	}
}

func TestCardStoreTransitionMatrixContainsOnlyReviewWorkflowEdges(t *testing.T) {
	allowed := map[[2]CardStatus]bool{
		{StatusDraft, StatusInReview}:       true,
		{StatusInReview, StatusDraft}:       true,
		{StatusInReview, StatusPublished}:   true,
		{StatusPublished, StatusSuperseded}: true,
		{StatusSuperseded, StatusRetired}:   true,
	}
	statuses := []CardStatus{StatusDraft, StatusInReview, StatusPublished, StatusSuperseded, StatusRetired}
	for _, from := range statuses {
		for _, to := range statuses {
			if got, want := allowedCardTransition(from, to), allowed[[2]CardStatus{from, to}]; got != want {
				t.Fatalf("transition %s -> %s: got %v want %v", from, to, got, want)
			}
		}
	}
}

func TestCardStoreUpdateUsesOptimisticVersionAndInvalidatesEmbeddings(t *testing.T) {
	card := testCard(StatusDraft)
	updated := card
	updated.Version++
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{card.LibraryID}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "UPDATE theory_cards", columns: cardColumns(), rows: [][]driver.Value{cardValues(updated)}},
		{kind: "exec", contains: "status = 'stale'", affected: 1},
		{kind: "commit"},
	}}
	saved, err := testStore(t, script).UpdateCard(context.Background(), card)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version != card.Version+1 {
		t.Fatalf("version not advanced: %+v", saved)
	}
	call := script.callsSnapshot()[3]
	if call.args[len(call.args)-2] != string(card.Status) || call.args[len(call.args)-1] != int64(card.Version) {
		t.Fatalf("missing optimistic status/version: %#v", call.args)
	}
	if !strings.Contains(script.callsSnapshot()[4].query, "pending") {
		t.Fatal("card change did not stale in-flight pending embeddings")
	}
}

func TestCardStoreUpdateRejectsEveryNonDraftSnapshotBeforeSQL(t *testing.T) {
	for _, status := range []CardStatus{StatusInReview, StatusPublished, StatusSuperseded, StatusRetired} {
		t.Run(string(status), func(t *testing.T) {
			script := &sqlScript{}
			_, err := testStore(t, script).UpdateCard(context.Background(), testCard(status))
			if !errors.Is(err, ErrCardNotEditable) {
				t.Fatalf("expected ErrCardNotEditable, got %v", err)
			}
			if len(script.callsSnapshot()) != 0 {
				t.Fatal("immutable snapshot reached SQL")
			}
		})
	}
}

func TestCardStorePublishRequiresPrimarySourceAndRollsBack(t *testing.T) {
	card := testCard(StatusInReview)
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{card.LibraryID}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE", columns: cardColumns(), rows: [][]driver.Value{cardValues(card)}},
		{kind: "query", contains: "FROM theory_card_sources", columns: cardSourceColumns()},
		{kind: "rollback"},
	}}
	store := testStore(t, script)
	_, err := store.TransitionCard(context.Background(), card.ID, StatusInReview, StatusPublished, 7)
	if err == nil || !strings.Contains(err.Error(), "primary source") {
		t.Fatalf("expected publish source validation failure, got %v", err)
	}
	script.assertDone(t)
}

func TestCardStorePublishSupersedesReplacementBeforePublishing(t *testing.T) {
	card := testCard(StatusInReview)
	source := testCardSource(card.ID)
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{card.LibraryID}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE", columns: cardColumns(), rows: [][]driver.Value{cardValues(card)}},
		{kind: "query", contains: "FROM theory_card_sources", columns: cardSourceColumns(), rows: [][]driver.Value{cardSourceValues(source)}},
		{kind: "exec", contains: "status = 'superseded'", affected: 1},
		{kind: "query", contains: "status=$2", columns: cardColumns(), rows: [][]driver.Value{cardValues(withCardStatus(card, StatusPublished))}},
		{kind: "commit"},
	}}
	store := testStore(t, script)
	published, err := store.TransitionCard(context.Background(), card.ID, StatusInReview, StatusPublished, 7)
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != StatusPublished || published.ReviewedBy == nil || *published.ReviewedBy != 7 {
		t.Fatalf("unexpected published card: %+v", published)
	}
	calls := script.callsSnapshot()
	if indexCall(calls, "status = 'superseded'") >= indexCall(calls, "status=$2") {
		t.Fatalf("replacement ordering wrong: %#v", calls)
	}
	script.assertDone(t)
}

func TestCardStoreReportsConcurrentStatusChange(t *testing.T) {
	card := testCard(StatusDraft)
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{card.LibraryID}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE", columns: cardColumns(), rows: [][]driver.Value{cardValues(withCardStatus(card, StatusInReview))}},
		{kind: "rollback"},
	}}
	store := testStore(t, script)
	_, err := store.TransitionCard(context.Background(), card.ID, StatusDraft, StatusInReview, 7)
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("expected ErrConcurrentUpdate, got %v", err)
	}
}

func TestCardStoreSupersedingPublishedSnapshotDoesNotChangeContentVersion(t *testing.T) {
	card := testCard(StatusPublished)
	superseded := withCardStatus(card, StatusSuperseded)
	superseded.Version = card.Version
	script := &sqlScript{steps: []sqlStep{
		{kind: "begin"},
		{kind: "query", contains: "SELECT library_id", columns: []string{"library_id"}, rows: [][]driver.Value{{card.LibraryID}}},
		{kind: "query", contains: "lock_theory_libraries", columns: []string{"locked"}, rows: [][]driver.Value{{nil}}},
		{kind: "query", contains: "FOR UPDATE", columns: cardColumns(), rows: [][]driver.Value{cardValues(card)}},
		{kind: "query", contains: "CASE WHEN $2='published' THEN version+1 ELSE version END", columns: cardColumns(), rows: [][]driver.Value{cardValues(superseded)}},
		{kind: "commit"},
	}}
	got, err := testStore(t, script).TransitionCard(context.Background(), card.ID, StatusPublished, StatusSuperseded, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != card.Version {
		t.Fatalf("superseding changed snapshot version: %d -> %d", card.Version, got.Version)
	}
}

func testCard(status CardStatus) Card {
	now := time.Unix(1700000000, 0).UTC()
	return Card{ID: 11, LibraryID: 3, CanonicalKey: "inner_observer", CanonicalName: "内在观察者", Aliases: []byte(`[]`), Domain: "self", Subdomain: "awareness", CardKind: CardKindConcept, Summary: "summary", Definition: "definition", CoreClaim: "claim", Mechanism: "mechanism", ApplicableContext: "context", NonApplicableContext: "not context", ObservableSignals: []byte(`[]`), CommonTriggers: []byte(`[]`), AutomaticPattern: "auto", ResourceState: "resource", ShadowOrRisk: "risk", GrowthDirection: "growth", EpistemicStatus: EpistemicEvidenceInformed, EvidenceLevel: EvidenceModerate, ClinicalSafety: ClinicalGeneral, ControversyNotes: "notes", CulturalContext: "culture", AuthorityLevel: 4, Language: "zh-CN", Status: status, Version: 2, CreatedBy: int64ptr(5), UpdatedBy: int64ptr(6), CreateTime: now, UpdateTime: now}
}

func withCardStatus(card Card, status CardStatus) Card {
	card.Status = status
	if status == StatusPublished {
		now := time.Unix(1700000100, 0).UTC()
		card.ReviewedBy, card.ReviewedAt, card.PublishedAt = int64ptr(7), &now, &now
		card.Version++
	}
	return card
}

func testCardSource(cardID int64) CardSource {
	now := time.Unix(1700000000, 0).UTC()
	return CardSource{ID: 21, CardID: cardID, WorkID: 31, FileID: int64ptr(41), SourceRole: SourceRolePrimary, Chapter: "1", PageStart: intptr(2), PageEnd: intptr(3), LocationLabel: "loc", Quotation: "quote", InterpretationNote: "note", ExtractionQuality: .95, QuoteVerified: true, VerifiedBy: int64ptr(7), VerifiedAt: &now, CreateTime: now, UpdateTime: now}
}

func cardColumns() []string {
	return []string{"id", "library_id", "canonical_key", "canonical_name", "aliases", "domain", "subdomain", "card_kind", "summary", "definition", "core_claim", "mechanism", "applicable_context", "non_applicable_context", "observable_signals", "common_triggers", "automatic_pattern", "resource_state", "shadow_or_risk", "growth_direction", "epistemic_status", "evidence_level", "clinical_safety", "controversy_notes", "cultural_context", "authority_level", "language", "status", "version", "reviewed_by", "reviewed_at", "published_at", "created_by", "updated_by", "create_time", "update_time"}
}

func cardValues(c Card) []driver.Value {
	return []driver.Value{c.ID, c.LibraryID, c.CanonicalKey, c.CanonicalName, []byte(c.Aliases), c.Domain, c.Subdomain, string(c.CardKind), c.Summary, c.Definition, c.CoreClaim, c.Mechanism, c.ApplicableContext, c.NonApplicableContext, []byte(c.ObservableSignals), []byte(c.CommonTriggers), c.AutomaticPattern, c.ResourceState, c.ShadowOrRisk, c.GrowthDirection, string(c.EpistemicStatus), string(c.EvidenceLevel), string(c.ClinicalSafety), c.ControversyNotes, c.CulturalContext, int64(c.AuthorityLevel), c.Language, string(c.Status), int64(c.Version), nullableInt(c.ReviewedBy), nullableTime(c.ReviewedAt), nullableTime(c.PublishedAt), nullableInt(c.CreatedBy), nullableInt(c.UpdatedBy), c.CreateTime, c.UpdateTime}
}

func cardSourceColumns() []string {
	return []string{"id", "card_id", "work_id", "file_id", "source_role", "chapter", "page_start", "page_end", "location_label", "quotation", "interpretation_note", "extraction_quality", "quote_verified", "verified_by", "verified_at", "create_time", "update_time"}
}

func cardSourceValues(s CardSource) []driver.Value {
	return []driver.Value{s.ID, s.CardID, s.WorkID, nullableInt(s.FileID), string(s.SourceRole), s.Chapter, nullableIntFromInt(s.PageStart), nullableIntFromInt(s.PageEnd), s.LocationLabel, s.Quotation, s.InterpretationNote, s.ExtractionQuality, s.QuoteVerified, nullableInt(s.VerifiedBy), nullableTime(s.VerifiedAt), s.CreateTime, s.UpdateTime}
}

func int64ptr(v int64) *int64 { return &v }
func intptr(v int) *int       { return &v }
func nullableInt(v *int64) driver.Value {
	if v == nil {
		return nil
	}
	return *v
}
func nullableIntFromInt(v *int) driver.Value {
	if v == nil {
		return nil
	}
	return int64(*v)
}
func nullableTime(v *time.Time) driver.Value {
	if v == nil {
		return nil
	}
	return *v
}

type sqlStep struct {
	kind, contains string
	columns        []string
	rows           [][]driver.Value
	affected       int64
	err            error
}

type sqlCall struct {
	kind, query string
	args        []any
}

type sqlScript struct {
	mu    sync.Mutex
	steps []sqlStep
	next  int
	calls []sqlCall
}

func (s *sqlScript) take(kind, query string, args []driver.NamedValue) (sqlStep, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]any, len(args))
	for i := range args {
		values[i] = args[i].Value
	}
	s.calls = append(s.calls, sqlCall{kind: kind, query: compactSQL(query), args: values})
	if s.next >= len(s.steps) {
		return sqlStep{}, fmt.Errorf("unexpected %s: %s", kind, compactSQL(query))
	}
	step := s.steps[s.next]
	if step.kind != kind || (step.contains != "" && !strings.Contains(query, step.contains)) {
		return sqlStep{}, fmt.Errorf("step %d: expected %s containing %q, got %s %q", s.next, step.kind, step.contains, kind, compactSQL(query))
	}
	s.next++
	return step, step.err
}

func (s *sqlScript) callsSnapshot() []sqlCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sqlCall(nil), s.calls...)
}
func (s *sqlScript) assertDone(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next != len(s.steps) {
		t.Fatalf("consumed %d/%d SQL steps; calls=%#v", s.next, len(s.steps), s.calls)
	}
}

var testDriverSequence atomic.Int64

func testStore(t *testing.T, script *sqlScript) *Store {
	t.Helper()
	name := fmt.Sprintf("theory_store_test_%d", testDriverSequence.Add(1))
	sql.Register(name, scriptedDriver{script})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db)
}

type scriptedDriver struct{ script *sqlScript }

func (d scriptedDriver) Open(string) (driver.Conn, error) {
	return &scriptedConn{script: d.script}, nil
}

type scriptedConn struct{ script *sqlScript }

func (*scriptedConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*scriptedConn) Close() error                        { return nil }
func (c *scriptedConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *scriptedConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	_, err := c.script.take("begin", "BEGIN", nil)
	return &scriptedTx{script: c.script}, err
}
func (c *scriptedConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	step, err := c.script.take("query", query, args)
	if err != nil {
		return nil, err
	}
	return &scriptedRows{columns: step.columns, rows: step.rows}, nil
}
func (c *scriptedConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	step, err := c.script.take("exec", query, args)
	if err != nil {
		return nil, err
	}
	return driver.RowsAffected(step.affected), nil
}

type scriptedTx struct{ script *sqlScript }

func (tx *scriptedTx) Commit() error { _, err := tx.script.take("commit", "COMMIT", nil); return err }
func (tx *scriptedTx) Rollback() error {
	_, err := tx.script.take("rollback", "ROLLBACK", nil)
	return err
}

type scriptedRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *scriptedRows) Columns() []string { return r.columns }
func (*scriptedRows) Close() error        { return nil }
func (r *scriptedRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}
func compactSQL(v string) string { return strings.Join(strings.Fields(v), " ") }
func indexCall(calls []sqlCall, contains string) int {
	for i, call := range calls {
		if strings.Contains(call.query, contains) {
			return i
		}
	}
	return -1
}
