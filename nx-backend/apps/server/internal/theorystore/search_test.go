package theorystore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestSearchActiveChunksUsesCurrentActiveReleaseAndMinScore(t *testing.T) {
	database := openTheorySearchTestDB(t)
	docs, err := NewStore(database).SearchActiveChunks(context.Background(), "冲突时如何不伤害关系", 4, 0.35)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs=%+v, want exactly one relevant active chunk", docs)
	}
	if docs[0].ID != "theory:11" || docs[0].Title != "冲突中的非暴力沟通" {
		t.Fatalf("unexpected document: %+v", docs[0])
	}
}

func TestTheoryRelevanceDoesNotInjectEnneagramIntoRecipeQuestion(t *testing.T) {
	database := openTheorySearchTestDB(t)
	docs, err := NewStore(database).SearchActiveChunks(context.Background(), "推荐三道家常菜做法", 4, 0.35)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("recipe query must not receive theory chunks: %+v", docs)
	}
}

const theorySearchDriverName = "theory_active_search_test"

var registerTheorySearchDriver sync.Once

func openTheorySearchTestDB(t *testing.T) *sql.DB {
	t.Helper()
	registerTheorySearchDriver.Do(func() { sql.Register(theorySearchDriverName, theorySearchDriver{}) })
	database, err := sql.Open(theorySearchDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

type theorySearchDriver struct{}

func (theorySearchDriver) Open(string) (driver.Conn, error) { return theorySearchConn{}, nil }

type theorySearchConn struct{}

func (theorySearchConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (theorySearchConn) Close() error                        { return nil }
func (theorySearchConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (theorySearchConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	required := []string{
		"JOIN theory_library_releases",
		"release.version = library.current_version",
		"release.status = 'active'",
		"JOIN theory_release_cards",
		"chunk.status = 'enabled'",
	}
	for _, fragment := range required {
		if !strings.Contains(query, fragment) {
			return nil, errors.New("active release query missing: " + fragment)
		}
	}
	if len(args) != 2 {
		return nil, errors.New("expected query and candidate limit arguments")
	}
	return &theorySearchRows{values: [][]driver.Value{
		{int64(11), "冲突中的非暴力沟通", "在冲突中先描述事实，再表达感受和需要，最后提出清晰请求。", []byte(`["冲突","关系","沟通"]`), []byte(`["communication"]`)},
		{int64(12), "九型人格注意力", "观察注意力落点，模式不是人的固定身份。", []byte(`["九型","人格"]`), []byte(`["enneagram"]`)},
	}}, nil
}

type theorySearchRows struct {
	values [][]driver.Value
	index  int
}

func (r *theorySearchRows) Columns() []string {
	return []string{"id", "title", "content", "keywords", "tags"}
}
func (r *theorySearchRows) Close() error { return nil }
func (r *theorySearchRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
