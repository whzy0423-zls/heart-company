package theorystore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCatalogRegisterWorkUpsertsTrimmedParametersAndScansResult(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	canonicalID := int64(8)
	db, script := openCatalogDB(t,
		queryStep("INSERT INTO theory_source_works", []any{
			int64(3), "book-key", "Book Title", "Original", `[{"name":"Author"}]`, `[]`, `[]`,
			"Publisher", 1999, "First", "ISBN", "book", 4, "source_text", "licensed", canonicalID,
			`{"language":"zh"}`, "registered",
		}, workColumns(), []driver.Value{
			int64(11), int64(3), "book-key", "Book Title", "Original", []byte(`[{"name":"Author"}]`), []byte(`[]`), []byte(`[]`),
			"Publisher", int64(1999), "First", "ISBN", "book", int64(4), "source_text", "licensed", canonicalID,
			[]byte(`{"language":"zh"}`), "registered", now, now,
		}),
	)

	work, err := NewStore(db).RegisterWork(nil, SourceWork{
		LibraryID: 3, CanonicalKey: " book-key ", Title: " Book Title ", OriginalTitle: " Original ",
		Authors: []byte(`[{"name":"Author"}]`), Publisher: " Publisher ", PublishedYear: intPtr(1999),
		Edition: " First ", ISBN: " ISBN ", WorkType: WorkTypeBook, AuthorityLevel: 4,
		EpistemicStatus: EpistemicSourceText, CopyrightScope: CopyrightLicensed, CanonicalWorkID: &canonicalID,
		Metadata: []byte(`{"language":"zh"}`), Status: SourceWorkStatusRegistered,
	})
	if err != nil {
		t.Fatalf("RegisterWork: %v", err)
	}
	if work.ID != 11 || work.CanonicalKey != "book-key" || work.CanonicalWorkID == nil || *work.CanonicalWorkID != canonicalID || !work.CreateTime.Equal(now) {
		t.Fatalf("unexpected scanned work: %+v", work)
	}
	script.assertDone(t)
	if query := script.calls[0].query; !strings.Contains(query, "ON CONFLICT (library_id, canonical_key) DO UPDATE") || !strings.Contains(query, "canonical_work_id = EXCLUDED.canonical_work_id") || strings.Contains(query, `'{`) {
		t.Fatalf("work upsert must preserve inputs and parameterize JSON, query:\n%s", query)
	}
}

func TestCatalogRegisterFileAlwaysInsertsIndependentPhysicalRows(t *testing.T) {
	now := time.Unix(1_700_000_001, 0).UTC()
	hash := strings.Repeat("a", 64)
	db, script := openCatalogDB(t,
		beginStep(),
		queryStep("FROM theory_source_works", []any{int64(10)}, []string{"library_id"}, []driver.Value{int64(2)}),
		queryStep("INSERT INTO theory_source_files", fileInsertArgs(10, "one/file.pdf", hash, nil), fileColumns(), fileValues(101, 10, "one/file.pdf", hash, nil, now)),
		commitStep(),
		beginStep(),
		queryStep("FROM theory_source_works", []any{int64(20)}, []string{"library_id"}, []driver.Value{int64(2)}),
		queryStep("INSERT INTO theory_source_files", fileInsertArgs(20, "two/file.pdf", hash, nil), fileColumns(), fileValues(102, 20, "two/file.pdf", hash, nil, now)),
		commitStep(),
	)
	store := NewStore(db)
	first, err := store.RegisterFile(context.Background(), catalogFile(10, " one/file.pdf ", hash))
	if err != nil {
		t.Fatalf("first RegisterFile: %v", err)
	}
	second, err := store.RegisterFile(context.Background(), catalogFile(20, " two/file.pdf ", hash))
	if err != nil {
		t.Fatalf("second RegisterFile: %v", err)
	}
	if first.ID != 101 || second.ID != 102 || first.WorkID == second.WorkID {
		t.Fatalf("same hash should produce independent rows: first=%+v second=%+v", first, second)
	}
	for _, call := range script.calls {
		if strings.Contains(call.query, "INSERT INTO theory_source_files") && strings.Contains(call.query, "ON CONFLICT") {
			t.Fatalf("file insert must not deduplicate by hash: %s", call.query)
		}
	}
	script.assertDone(t)
}

func TestCatalogRegisterFileRejectsMissingWork(t *testing.T) {
	db, script := openCatalogDB(t, beginStep(), queryEmptyStep("FROM theory_source_works", []any{int64(99)}, []string{"library_id"}), rollbackStep())
	_, err := NewStore(db).RegisterFile(context.Background(), catalogFile(99, "file.pdf", strings.Repeat("b", 64)))
	if !errors.Is(err, ErrWorkNotFound) {
		t.Fatalf("expected ErrWorkNotFound, got %v", err)
	}
	script.assertDone(t)
}

func TestCatalogRegisterFileExplicitDuplicateUsesSameValidation(t *testing.T) {
	hash := strings.Repeat("c", 64)
	duplicateID := int64(7)
	now := time.Unix(1_700_000_002, 0).UTC()
	db, script := openCatalogDB(t,
		beginStep(),
		queryStep("FROM theory_source_works", []any{int64(20)}, []string{"library_id"}, []driver.Value{int64(4)}),
		queryStep("FOR UPDATE OF file", []any{duplicateID}, []string{"id", "library_id", "sha256"}, []driver.Value{duplicateID, int64(4), hash}),
		queryStep("INSERT INTO theory_source_files", fileInsertArgs(20, "copy.pdf", hash, &duplicateID), fileColumns(), fileValues(21, 20, "copy.pdf", hash, &duplicateID, now)),
		commitStep(),
	)
	file := catalogFile(20, "copy.pdf", hash)
	file.DuplicateOfFileID = &duplicateID
	got, err := NewStore(db).RegisterFile(context.Background(), file)
	if err != nil {
		t.Fatalf("RegisterFile duplicate: %v", err)
	}
	if got.DuplicateOfFileID == nil || *got.DuplicateOfFileID != duplicateID {
		t.Fatalf("duplicate target not scanned: %+v", got)
	}
	script.assertDone(t)
}

func TestCatalogFindFileBySHA256ScopesLibraryOrdersAndScans(t *testing.T) {
	hash := strings.Repeat("d", 64)
	now := time.Unix(1_700_000_003, 0).UTC()
	db, script := openCatalogDB(t,
		queryStep("JOIN theory_source_works", []any{int64(6), hash}, fileColumns(), fileValues(31, 30, "found.pdf", hash, nil, now)),
	)
	file, found, err := NewStore(db).FindFileBySHA256(context.Background(), 6, hash)
	if err != nil || !found || file.ID != 31 || file.RelativePath != "found.pdf" {
		t.Fatalf("FindFileBySHA256 = (%+v, %v, %v)", file, found, err)
	}
	query := script.calls[0].query
	if !strings.Contains(query, "work.library_id = $1") || !strings.Contains(query, "file.sha256 = $2") || !strings.Contains(query, "file.duplicate_of_file_id IS NULL DESC") || !strings.Contains(query, "file.id ASC") {
		t.Fatalf("find query is not stably scoped/ordered:\n%s", query)
	}
	script.assertDone(t)
}

func TestCatalogFindFileBySHA256NotFound(t *testing.T) {
	hash := strings.Repeat("e", 64)
	db, script := openCatalogDB(t, queryEmptyStep("JOIN theory_source_works", []any{int64(1), hash}, fileColumns()))
	_, found, err := NewStore(db).FindFileBySHA256(nil, 1, hash)
	if err != nil || found {
		t.Fatalf("expected clean not-found result, found=%v err=%v", found, err)
	}
	script.assertDone(t)
}

func TestDuplicateMarkSameLibrarySameHashTransactionOrder(t *testing.T) {
	hash := strings.Repeat("f", 64)
	db, script := openCatalogDB(t,
		beginStep(),
		queryRowsStep("FOR UPDATE", []any{int64(10), int64(20)}, []string{"id", "work_id", "library_id", "sha256"}, [][]driver.Value{
			{int64(10), int64(1), int64(5), hash}, {int64(20), int64(2), int64(5), hash},
		}),
		queryStep("lock_theory_libraries", []any{int64(5)}, []string{"lock_theory_libraries"}, []driver.Value{nil}),
		queryStep("WITH RECURSIVE", []any{int64(20), int64(10)}, []string{"exists"}, []driver.Value{false}),
		execStep("UPDATE theory_source_files", []any{int64(20), int64(10)}, 1),
		commitStep(),
	)
	if err := NewStore(db).MarkDuplicate(context.Background(), 10, 20); err != nil {
		t.Fatalf("MarkDuplicate: %v", err)
	}
	script.assertDone(t)
}

func TestDuplicateMarkRejectsInvalidRelations(t *testing.T) {
	hashA := strings.Repeat("1", 64)
	hashB := strings.Repeat("2", 64)
	tests := []struct {
		name     string
		fileID   int64
		targetID int64
		rows     [][]driver.Value
		cycle    *bool
		want     error
	}{
		{name: "self", fileID: 10, targetID: 10, want: ErrDuplicateSelf},
		{name: "source missing", fileID: 10, targetID: 20, rows: [][]driver.Value{{int64(20), int64(2), int64(5), hashA}}, want: ErrFileNotFound},
		{name: "target missing", fileID: 10, targetID: 20, rows: [][]driver.Value{{int64(10), int64(1), int64(5), hashA}}, want: ErrFileNotFound},
		{name: "cross library", fileID: 10, targetID: 20, rows: duplicateRows(hashA, hashA, 5, 6), want: ErrDuplicateCrossLibrary},
		{name: "hash mismatch", fileID: 10, targetID: 20, rows: duplicateRows(hashA, hashB, 5, 5), want: ErrDuplicateHashMismatch},
		{name: "cycle", fileID: 10, targetID: 20, rows: duplicateRows(hashA, hashA, 5, 5), cycle: boolPtr(true), want: ErrDuplicateCycle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := []catalogStep{beginStep()}
			if tt.fileID == tt.targetID {
				steps = append(steps, rollbackStep())
			} else {
				steps = append(steps, queryRowsStep("FOR UPDATE", []any{tt.fileID, tt.targetID}, []string{"id", "work_id", "library_id", "sha256"}, tt.rows))
				if tt.cycle != nil {
					steps = append(steps, queryStep("lock_theory_libraries", []any{int64(5)}, []string{"lock_theory_libraries"}, []driver.Value{nil}))
					steps = append(steps, queryStep("WITH RECURSIVE", []any{tt.targetID, tt.fileID}, []string{"exists"}, []driver.Value{*tt.cycle}))
				}
				steps = append(steps, rollbackStep())
			}
			db, script := openCatalogDB(t, steps...)
			err := NewStore(db).MarkDuplicate(context.Background(), tt.fileID, tt.targetID)
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
			script.assertDone(t)
		})
	}
}

func TestCatalogUpdateExtractionStatusValidatesAndUpdates(t *testing.T) {
	db, script := openCatalogDB(t,
		execStep("UPDATE theory_source_files", []any{"failed", 0.25, "OCR_TIMEOUT", "timed out", int64(42)}, 1),
		execStep("UPDATE theory_source_files", []any{"extracted", 0.9, "", "", int64(42)}, 1),
	)
	store := NewStore(db)
	if err := store.UpdateExtractionStatus(nil, 42, ExtractionStatusFailed, .25, " OCR_TIMEOUT ", " timed out "); err != nil {
		t.Fatalf("failed update: %v", err)
	}
	if err := store.UpdateExtractionStatus(context.Background(), 42, ExtractionStatusExtracted, .9, "stale", "stale"); err != nil {
		t.Fatalf("successful update: %v", err)
	}
	if !strings.Contains(script.calls[0].query, "update_time = now()") {
		t.Fatalf("status update must touch update_time: %s", script.calls[0].query)
	}
	script.assertDone(t)
}

func TestCatalogUpdateExtractionStatusRejectsInvalidInputAndMissingFile(t *testing.T) {
	store := NewStore(nil)
	if err := store.UpdateExtractionStatus(nil, 1, ExtractionStatusPending, 0, "", ""); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("nil db: %v", err)
	}
	db, script := openCatalogDB(t, execStep("UPDATE theory_source_files", []any{"pending", 0.5, "", "", int64(99)}, 0))
	store = NewStore(db)
	if err := store.UpdateExtractionStatus(nil, 99, ExtractionStatusPending, .5, "", ""); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("missing file: %v", err)
	}
	for _, tc := range []struct {
		name    string
		status  ExtractionStatus
		quality float64
		code    string
		message string
	}{
		{name: "invalid status", status: "unknown", quality: .5},
		{name: "nan quality", status: ExtractionStatusPending, quality: math.NaN()},
		{name: "high quality", status: ExtractionStatusPending, quality: 1.01},
		{name: "failed missing details", status: ExtractionStatusFailed, quality: .1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.UpdateExtractionStatus(nil, 1, tc.status, tc.quality, tc.code, tc.message); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	script.assertDone(t)
}

func TestCatalogInvalidSHAAndNilStoresReturnSentinels(t *testing.T) {
	invalid := strings.Repeat("A", 64)
	if _, _, err := (*Store)(nil).FindFileBySHA256(nil, 1, invalid); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("nil receiver: %v", err)
	}
	if _, err := NewStore(nil).RegisterWork(nil, catalogWork()); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("nil db RegisterWork: %v", err)
	}
	db, script := openCatalogDB(t)
	if _, _, err := NewStore(db).FindFileBySHA256(nil, 1, invalid); !errors.Is(err, ErrInvalidSHA256) {
		t.Fatalf("invalid hash: %v", err)
	}
	if _, err := NewStore(db).RegisterFile(nil, catalogFile(1, "file.pdf", invalid)); !errors.Is(err, ErrInvalidSHA256) {
		t.Fatalf("invalid file hash: %v", err)
	}
	script.assertDone(t)
}

func catalogWork() SourceWork {
	return SourceWork{LibraryID: 1, CanonicalKey: "work", Title: "Work", WorkType: WorkTypeBook, AuthorityLevel: 3, EpistemicStatus: EpistemicSourceText, CopyrightScope: CopyrightMetadataOnly, Status: SourceWorkStatusRegistered}
}

func catalogFile(workID int64, path, hash string) SourceFile {
	return SourceFile{WorkID: workID, RelativePath: path, OriginalFilename: " file.pdf ", FileFormat: " pdf ", MIMEType: " application/pdf ", SHA256: hash, TitleSource: TitleSourceFilename, ExtractionClass: ExtractionClassTextRich, ExtractionStatus: ExtractionStatusPending, Metadata: []byte(`{}`)}
}

func intPtr(value int) *int    { return &value }
func boolPtr(value bool) *bool { return &value }

func duplicateRows(sourceHash, targetHash string, sourceLibrary, targetLibrary int64) [][]driver.Value {
	return [][]driver.Value{{int64(10), int64(1), sourceLibrary, sourceHash}, {int64(20), int64(2), targetLibrary, targetHash}}
}

func workColumns() []string {
	return []string{"id", "library_id", "canonical_key", "title", "original_title", "authors", "editors", "translators", "publisher", "published_year", "edition", "isbn", "work_type", "authority_level", "epistemic_status", "copyright_scope", "canonical_work_id", "metadata", "status", "create_time", "update_time"}
}

func fileColumns() []string {
	return []string{"id", "work_id", "relative_path", "original_filename", "file_format", "mime_type", "byte_size", "page_count", "sha256", "duplicate_of_file_id", "title_source", "extraction_class", "extraction_status", "extraction_quality", "extracted_text_uri", "ocr_text_uri", "extractor_name", "extractor_version", "error_code", "error_message", "metadata", "create_time", "update_time"}
}

func fileInsertArgs(workID int64, path, hash string, duplicateID *int64) []any {
	var duplicate any
	if duplicateID != nil {
		duplicate = *duplicateID
	}
	return []any{workID, path, "file.pdf", "pdf", "application/pdf", int64(0), nil, hash, duplicate, "filename", "text_rich", "pending", float64(0), "", "", "", "", "", "", `{}`}
}

func fileValues(id, workID int64, path, hash string, duplicateID *int64, now time.Time) []driver.Value {
	var duplicate driver.Value
	if duplicateID != nil {
		duplicate = *duplicateID
	}
	return []driver.Value{id, workID, path, "file.pdf", "pdf", "application/pdf", int64(0), nil, hash, duplicate, "filename", "text_rich", "pending", float64(0), "", "", "", "", "", "", []byte(`{}`), now, now}
}

type catalogStep struct {
	op       string
	contains string
	args     []any
	columns  []string
	rows     [][]driver.Value
	affected int64
}

func beginStep() catalogStep    { return catalogStep{op: "begin"} }
func commitStep() catalogStep   { return catalogStep{op: "commit"} }
func rollbackStep() catalogStep { return catalogStep{op: "rollback"} }
func queryStep(contains string, args []any, columns []string, row []driver.Value) catalogStep {
	return queryRowsStep(contains, args, columns, [][]driver.Value{row})
}
func queryEmptyStep(contains string, args []any, columns []string) catalogStep {
	return queryRowsStep(contains, args, columns, nil)
}
func queryRowsStep(contains string, args []any, columns []string, rows [][]driver.Value) catalogStep {
	return catalogStep{op: "query", contains: contains, args: args, columns: columns, rows: rows}
}
func execStep(contains string, args []any, affected int64) catalogStep {
	return catalogStep{op: "exec", contains: contains, args: args, affected: affected}
}

type catalogCall struct {
	op, query string
	args      []driver.NamedValue
}
type catalogScript struct {
	mu      sync.Mutex
	steps   []catalogStep
	calls   []catalogCall
	failure error
}

func (s *catalogScript) next(op, query string, args []driver.NamedValue) (catalogStep, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, catalogCall{op: op, query: query, args: append([]driver.NamedValue(nil), args...)})
	if s.failure != nil {
		return catalogStep{}, s.failure
	}
	if len(s.steps) == 0 {
		s.failure = fmt.Errorf("unexpected %s: %s", op, query)
		return catalogStep{}, s.failure
	}
	step := s.steps[0]
	s.steps = s.steps[1:]
	if step.op != op {
		s.failure = fmt.Errorf("expected %s, got %s (%s)", step.op, op, query)
		return catalogStep{}, s.failure
	}
	if step.contains != "" && !strings.Contains(query, step.contains) {
		s.failure = fmt.Errorf("%s query missing %q: %s", op, step.contains, query)
		return catalogStep{}, s.failure
	}
	if len(args) != len(step.args) {
		s.failure = fmt.Errorf("%s args length = %d, want %d: %+v", op, len(args), len(step.args), args)
		return catalogStep{}, s.failure
	}
	for i := range args {
		if fmt.Sprint(args[i].Value) != fmt.Sprint(step.args[i]) {
			s.failure = fmt.Errorf("%s arg %d = %#v, want %#v", op, i+1, args[i].Value, step.args[i])
			return catalogStep{}, s.failure
		}
	}
	return step, nil
}

func (s *catalogScript) assertDone(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		t.Fatal(s.failure)
	}
	if len(s.steps) != 0 {
		t.Fatalf("%d database steps not executed; next=%s", len(s.steps), s.steps[0].op)
	}
}

var catalogDriverSequence atomic.Int64

func openCatalogDB(t *testing.T, steps ...catalogStep) (*sql.DB, *catalogScript) {
	t.Helper()
	script := &catalogScript{steps: append([]catalogStep(nil), steps...)}
	name := fmt.Sprintf("theorystore_catalog_%d", catalogDriverSequence.Add(1))
	sql.Register(name, catalogDriver{script: script})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db, script
}

type catalogDriver struct{ script *catalogScript }

func (d catalogDriver) Open(string) (driver.Conn, error) { return &catalogConn{script: d.script}, nil }

type catalogConn struct{ script *catalogScript }

func (*catalogConn) Prepare(string) (driver.Stmt, error)                            { return nil, driver.ErrSkip }
func (*catalogConn) Close() error                                                   { return nil }
func (c *catalogConn) Begin() (driver.Tx, error)                                    { return c.begin() }
func (c *catalogConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) { return c.begin() }
func (c *catalogConn) begin() (driver.Tx, error) {
	if _, err := c.script.next("begin", "", nil); err != nil {
		return nil, err
	}
	return &catalogTx{script: c.script}, nil
}
func (c *catalogConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	step, err := c.script.next("query", query, args)
	if err != nil {
		return nil, err
	}
	return &catalogRows{columns: step.columns, rows: step.rows}, nil
}
func (c *catalogConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	step, err := c.script.next("exec", query, args)
	if err != nil {
		return nil, err
	}
	return driver.RowsAffected(step.affected), nil
}

type catalogTx struct{ script *catalogScript }

func (tx *catalogTx) Commit() error   { _, err := tx.script.next("commit", "", nil); return err }
func (tx *catalogTx) Rollback() error { _, err := tx.script.next("rollback", "", nil); return err }

type catalogRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *catalogRows) Columns() []string { return r.columns }
func (*catalogRows) Close() error        { return nil }
func (r *catalogRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}
