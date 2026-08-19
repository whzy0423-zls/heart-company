package skillcatalog

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPublicCatalogQueriesRequireEnabledAndPublishedRows(t *testing.T) {
	store := NewStore(openCatalogBoundaryDB(t))
	libraries, err := store.ListLibraries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(libraries) != 1 || libraries[0].SkillCount != 35 {
		t.Fatalf("libraries=%+v", libraries)
	}
	categories, err := store.ListCategories(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(categories) != 1 || categories[0].SkillCount != 5 {
		t.Fatalf("categories=%+v", categories)
	}
	page, err := store.ListSkills(context.Background(), SkillFilter{LibraryID: 1, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Version != "1.1.0" {
		t.Fatalf("skills=%+v", page)
	}
	detail, err := store.GetSkill(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Key != "art-of-learning" || detail.Version.ID != 91 || detail.Version.TheoryReleaseID != 71 {
		t.Fatalf("detail=%+v", detail)
	}
	if strings.Contains(string(detail.Version.SourceMetadata), "private/source") || !strings.Contains(string(detail.Version.SourceMetadata), "product-baseline-v1") {
		t.Fatalf("public source metadata was not sanitized: %s", detail.Version.SourceMetadata)
	}
}

const catalogBoundaryDriverName = "skill_catalog_boundary_test"

var registerCatalogBoundaryDriver sync.Once

func openCatalogBoundaryDB(t *testing.T) *sql.DB {
	t.Helper()
	registerCatalogBoundaryDriver.Do(func() { sql.Register(catalogBoundaryDriverName, catalogBoundaryDriver{}) })
	database, err := sql.Open(catalogBoundaryDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

type catalogBoundaryDriver struct{}

func (catalogBoundaryDriver) Open(string) (driver.Conn, error) { return catalogBoundaryConn{}, nil }

type catalogBoundaryConn struct{}

func (catalogBoundaryConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (catalogBoundaryConn) Close() error                        { return nil }
func (catalogBoundaryConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (catalogBoundaryConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	required := []string{
		"library.status = 'enabled'",
		"category.status = 'enabled'",
		"skill.status = 'enabled'",
		"version.status = 'published'",
		"skill.latest_published_version_id = version.id",
	}
	for _, fragment := range required {
		if !strings.Contains(query, fragment) {
			return nil, errors.New("public catalog query missing boundary: " + fragment)
		}
	}
	switch {
	case strings.Contains(query, "GROUP BY library.id"):
		return &catalogRows{columns: []string{"id", "key", "name", "description", "icon_key", "skill_count"}, values: [][]driver.Value{{int64(1), "books", "学习成长", "", "book", int64(35)}}}, nil
	case strings.Contains(query, "GROUP BY category.id"):
		return &catalogRows{columns: []string{"id", "library_id", "key", "name", "icon_key", "color_token", "skill_count"}, values: [][]driver.Value{{int64(2), int64(1), "learning", "学习与成长", "school", "sand", int64(5)}}}, nil
	case strings.Contains(query, "WHERE skill.id = $1"):
		return &catalogRows{columns: skillDetailColumns(), values: [][]driver.Value{{int64(9), int64(2), int64(1), "art-of-learning", "学习之道", "摘要", "介绍", "school", "sand", "学习与成长", int64(91), "1.1.0", int64(1), "规则", []byte(`["从哪里开始？"]`), int64(71), "general-v1", "hash", "1.0.0", []byte(`{"reviewPolicy":"product-baseline-v1","reviewDecisionRef":"baseline","riskNotices":[],"source":"/private/source/SKILL.md","reviewManifestHash":"secret"}`), time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)}}}, nil
	default:
		return &catalogRows{columns: skillSummaryColumns(), values: [][]driver.Value{{int64(9), int64(2), "learning", "学习与成长", "art-of-learning", "学习之道", "摘要", "school", "sand", int64(91), "1.1.0", int64(10)}}}, nil
	}
}

type catalogRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *catalogRows) Columns() []string { return r.columns }
func (r *catalogRows) Close() error      { return nil }
func (r *catalogRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func skillSummaryColumns() []string {
	return []string{"id", "category_id", "category_key", "category_name", "key", "name", "summary", "icon_key", "color_token", "version_id", "version", "sort_order"}
}

func skillDetailColumns() []string {
	return []string{"id", "category_id", "library_id", "key", "name", "summary", "description", "icon_key", "color_token", "category_name", "version_id", "version", "runtime_version", "instructions", "opening_prompts", "theory_release_id", "safety_profile", "content_hash", "min_app_version", "source_metadata", "published_at"}
}

var _ driver.QueryerContext = catalogBoundaryConn{}
var _ driver.Rows = (*catalogRows)(nil)
