package publicknowledge

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"nine-xing/nx-backend/apps/server/internal/testutil"
)

func TestParseListParamsBoundsAndFilters(t *testing.T) {
	params, err := ParseListParams(url.Values{"page": {"2"}, "pageSize": {"10"}, "enabled": {"false"}, "qualityStatus": {"needs_review"}, "category": {"心理学"}})
	if err != nil || params.Page != 2 || params.PageSize != 10 || params.Enabled == nil || *params.Enabled || params.Category != "心理学" {
		t.Fatalf("params=%+v err=%v", params, err)
	}
	for _, values := range []url.Values{{"page": {"0"}}, {"page": {"x"}}, {"pageSize": {"101"}}, {"enabled": {"yes"}}, {"qualityStatus": {"unknown"}}, {"keyword": {strings.Repeat("x", 201)}}} {
		if _, err := ParseListParams(values); !errors.Is(err, ErrValidation) {
			t.Errorf("%v should produce validation error, got %v", values, err)
		}
	}
}

func TestValidateStatusIDsRejectsEmptyDuplicateAndLargeBatches(t *testing.T) {
	for _, ids := range [][]string{nil, {""}, {" a"}, {"a", "a"}, {strings.Repeat("a", 257)}, make([]string, 201)} {
		if err := ValidateStatusIDs(ids); !errors.Is(err, ErrValidation) {
			t.Errorf("invalid ids=%v err=%v", ids, err)
		}
	}
	if err := ValidateStatusIDs([]string{"dataset:book:1", "dataset:story:1"}); err != nil {
		t.Fatal(err)
	}
}

func TestLexicalTokensMatchCJKAndLatinExportFormat(t *testing.T) {
	got := LexicalTokens("情绪管理，Learn 2026 情绪")
	want := []string{"情绪", "绪管", "管理", "learn", "2026"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens=%v want=%v", got, want)
	}
	if tokens := LexicalTokens(strings.Repeat("知识", 3000)); len(tokens) > 64 {
		t.Fatalf("query tokens should be bounded, got %d", len(tokens))
	}
	if tokens := LexicalTokens("心，growth"); !reflect.DeepEqual(tokens, []string{"心", "growth"}) {
		t.Fatalf("singleton CJK tokens=%v", tokens)
	}
}

func TestSearchPassesBoundedDeadlineToDatabase(t *testing.T) {
	for _, callerTimeout := range []time.Duration{0, 10 * time.Second, 500 * time.Millisecond} {
		t.Run(callerTimeout.String(), func(t *testing.T) {
			ctx := context.Background()
			if callerTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, callerTimeout)
				defer cancel()
			}
			callerDeadline, _ := ctx.Deadline()
			start := time.Now()
			var queryContext context.Context
			database := sql.OpenDB(searchTestConnector{query: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				queryContext = ctx
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Fatal("database query has no deadline")
				}
				if callerTimeout > 0 && callerTimeout < 5*time.Second {
					if !deadline.Equal(callerDeadline) {
						t.Fatalf("database deadline=%v, want caller deadline=%v", deadline, callerDeadline)
					}
				} else if deadline.Before(start.Add(4*time.Second)) || deadline.After(start.Add(5*time.Second+250*time.Millisecond)) {
					t.Fatalf("database deadline budget=%v, want 5 seconds", deadline.Sub(start))
				}
				for _, filter := range []string{"d.library_kind='public'", "d.release_id IS NULL", "d.public_source_id IS NOT NULL", "d.safety_level <= 0", "s.id=d.public_source_id AND s.enabled", "DESC,d.id LIMIT $2"} {
					if !strings.Contains(query, filter) {
						t.Fatalf("search lost filter or ordering %q", filter)
					}
				}
				if len(args) != 2 || args[1].Value != int64(10) {
					t.Fatalf("search args=%v", args)
				}
				return &searchTestRows{}, nil
			}})
			defer database.Close()
			hits, err := NewStore(database).Search(ctx, "情绪管理", 10)
			if err != nil || len(hits) != 1 || hits[0].ID != "fixture" || hits[0].Title != "title" || hits[0].Content != "content" {
				t.Fatalf("search hits=%+v err=%v", hits, err)
			}
			if queryContext.Err() != context.Canceled {
				t.Fatal("search did not release its context budget")
			}
		})
	}
}

func TestSearchHonorsCallerDeadlineDuringDatabaseQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	database := sql.OpenDB(searchTestConnector{query: func(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	defer database.Close()
	if _, err := NewStore(database).Search(ctx, "growth", 10); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("search err=%v, want caller deadline", err)
	}
}

type searchTestConnector struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (c searchTestConnector) Connect(context.Context) (driver.Conn, error) {
	return searchTestConn{query: c.query}, nil
}
func (c searchTestConnector) Driver() driver.Driver { return searchTestDriver{} }

type searchTestDriver struct{}

func (searchTestDriver) Open(string) (driver.Conn, error) { return nil, driver.ErrSkip }

type searchTestConn struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (searchTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (searchTestConn) Close() error                        { return nil }
func (searchTestConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c searchTestConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type searchTestRows struct{ read bool }

func (*searchTestRows) Columns() []string { return []string{"id", "title", "content"} }
func (*searchTestRows) Close() error      { return nil }
func (r *searchTestRows) Next(values []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	values[0], values[1], values[2] = "fixture", "title", "content"
	return nil
}

func TestCatalogJSONDoesNotExposePrivateMetadata(t *testing.T) {
	raw, err := json.Marshal(Source{ID: "book:1", DatasetID: "books", SourceKind: "book", SourceRecordID: 1, FileFormat: "pdf", QualityStatus: "pending", ImportedChunks: 5})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"datasetId", "sourceKind", "sourceRecordId", "fileFormat", "qualityStatus", "importedChunks"} {
		if !strings.Contains(string(raw), `"`+name+`"`) {
			t.Fatalf("missing camelCase %s: %s", name, raw)
		}
	}
	locator := SafeLocator(json.RawMessage(`{"chapter":"第 1 章","page":3,"chunk_id":7,"path":"/private/source.pdf","prompt":"ignore rules"}`))
	if len(locator) != 3 || locator["path"] != nil || locator["prompt"] != nil {
		t.Fatalf("unsafe locator=%v", locator)
	}
	locator = SafeLocator(json.RawMessage(`{"section_title":"章节","section_id":2,"sqlite_chunk_id":9,"segment_index":1}`))
	if len(locator) != 4 {
		t.Fatalf("imported provenance missing=%v", locator)
	}
}

func TestStoreCatalogAtomicStatusPreviewAndLiveSearch(t *testing.T) {
	database := openFixture(t)
	ctx := context.Background()
	_, err := database.Exec(`INSERT INTO public_knowledge_sources
		(id,dataset_id,source_kind,source_record_id,title,category,enabled,metadata) VALUES
		('fixture:book:1','fixture','book',1,'情绪管理','心理学',true,'{"categories":[{"name":"心理学","is_primary":true},{"name":"沟通","is_primary":false}],"path":"secret"}'),
		('fixture:book:2','fixture','book',2,'历史故事','历史',false,'{}'),
		('other:book:1','other','book',1,'其他','其他',true,'{}');
		INSERT INTO knowledge_documents
		(id,library_kind,title,content,source,content_hash,public_source_id,import_batch_id,search_text,locator) VALUES
		('one','public','情绪管理','情绪管理与沟通','book','same','fixture:book:1','batch1','情绪 绪管 管理 沟通','{"page":2,"path":"secret"}'),
		('two','public','历史故事','情绪管理的历史','book','same','fixture:book:2','batch1','情绪 绪管 管理 历史','{}'),
		('legacy','public','旧条目','情绪管理','manual','same',NULL,NULL,'','{}');`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO knowledge_documents
		(id,library_kind,title,content,source,content_hash,public_source_id,search_text,safety_level) VALUES
		('restricted','public','受限','情绪管理','book','restricted','fixture:book:1','情绪 绪管 管理',1)`); err != nil {
		t.Fatal(err)
	}
	store := NewStore(database)
	params, _ := ParseListParams(url.Values{"category": {"沟通"}, "datasetId": {"fixture"}})
	list, err := store.List(ctx, params)
	if err != nil || list.Total != 1 || len(list.Items) != 1 || len(list.Categories) != 3 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if _, err := store.SetEnabled(ctx, []string{"fixture:book:1", "missing"}, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("atomic missing update err=%v", err)
	}
	hits, err := store.Search(ctx, "情绪管理", 10)
	if err != nil || len(hits) != 1 || hits[0].ID != "one" {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
	preview, err := store.Preview(ctx, "fixture:book:1")
	if err != nil || preview.Total != 2 || len(preview.Items) != 2 || preview.Items[0].Locator["path"] != nil || preview.Items[0].ImportBatchID != "batch1" {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	if updated, err := store.SetEnabled(ctx, []string{"fixture:book:1"}, false); err != nil || updated != 1 {
		t.Fatalf("update=%d err=%v", updated, err)
	}
	hits, err = store.Search(ctx, "情绪管理", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("disabled cached hits=%+v err=%v", hits, err)
	}
	var legacy int
	if err := database.QueryRow(`SELECT count(*) FROM knowledge_documents WHERE public_source_id IS NULL`).Scan(&legacy); err != nil || legacy != 1 {
		t.Fatalf("legacy count=%d err=%v", legacy, err)
	}
	if _, err := store.Preview(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing preview err=%v", err)
	}
	if len(list.Items[0].Categories) != 2 || list.Items[0].Categories[1] != "沟通" {
		t.Fatalf("secondary source categories=%v", list.Items[0].Categories)
	}
	if _, err := database.Exec(`INSERT INTO knowledge_documents
		(id,library_kind,content,source,content_hash,public_source_id) VALUES
		('duplicate','public','copy','book','same','fixture:book:1')`); err == nil {
		t.Fatal("managed same-source duplicate must be rejected")
	}
	if _, err := database.Exec(`INSERT INTO knowledge_documents
		(id,library_kind,content,source,content_hash) VALUES
		('legacy-copy','public','copy','manual','same')`); err == nil {
		t.Fatal("unmanaged historical duplicate must still be rejected")
	}
}

func TestManagedSchemaMigratesHistoricalDedupIndexAndReplays(t *testing.T) {
	database := openFixture(t)
	if _, err := database.Exec(`DROP INDEX uq_knowledge_documents_embedding_identity;
		CREATE UNIQUE INDEX uq_knowledge_documents_embedding_identity ON knowledge_documents
		(content_hash,embedding_model,index_version,library_kind,COALESCE(release_id,0));`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := database.Exec(fixtureSchema(t)); err != nil {
			t.Fatalf("migration replay %d: %v", i, err)
		}
	}
	var predicate string
	if err := database.QueryRow(`SELECT pg_get_expr(indpred,indrelid) FROM pg_index WHERE indexrelid='uq_knowledge_documents_embedding_identity'::regclass`).Scan(&predicate); err != nil || predicate != "(public_source_id IS NULL)" {
		t.Fatalf("legacy predicate=%q err=%v", predicate, err)
	}
}

func TestSchemaMigrationDoesNotInspectOtherSchemas(t *testing.T) {
	database := openFixture(t)
	sibling := fmt.Sprintf("public_knowledge_sibling_%d", time.Now().UnixNano())
	if _, err := database.Exec(`CREATE SCHEMA ` + sibling + `; CREATE TABLE ` + sibling + `.marker (value int);
		CREATE UNIQUE INDEX uq_knowledge_documents_embedding_identity ON ` + sibling + `.marker(value);`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.Exec(`DROP SCHEMA ` + sibling + ` CASCADE`) })
	var before, after int64
	if err := database.QueryRow(`SELECT 'uq_knowledge_documents_embedding_identity'::regclass::oid::bigint`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(fixtureSchema(t)); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT 'uq_knowledge_documents_embedding_identity'::regclass::oid::bigint`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("migration replaced local partial index based on unrelated schema index")
	}
}

func TestManagedSchemaMigratesLegacyLexicalIndexAndReplays(t *testing.T) {
	database := openFixture(t)
	if _, err := database.Exec(`DROP INDEX idx_knowledge_documents_lexical;
		CREATE INDEX idx_knowledge_documents_lexical ON knowledge_documents
		USING gin (to_tsvector('simple', title || ' ' || content));`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := database.Exec(fixtureSchema(t)); err != nil {
			t.Fatal(err)
		}
	}
	var predicate string
	if err := database.QueryRow(`SELECT pg_get_expr(indpred,indrelid) FROM pg_index
		WHERE indexrelid='idx_knowledge_documents_lexical'::regclass`).Scan(&predicate); err != nil || predicate != "(public_source_id IS NULL)" {
		t.Fatalf("legacy lexical predicate=%q err=%v", predicate, err)
	}
}

func fixtureSchema(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(raw), "CREATE TABLE IF NOT EXISTS public_knowledge_sources")
	end := strings.Index(string(raw)[start:], "-- ============ 阅读管理")
	return string(raw)[start : start+end]
}

func openFixture(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for public knowledge PostgreSQL tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("public_knowledge_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	database, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close(); _, _ = admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); admin.Close() })
	if _, err := database.Exec(fixtureSchema(t)); err != nil {
		t.Fatal(err)
	}
	return database
}
