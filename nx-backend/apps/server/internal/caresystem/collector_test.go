package caresystem

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCollectIncludesMainAndFriendMessagesInThirtyDayWindow(t *testing.T) {
	queries := []string{}
	db := openCareTestDB(t, func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
		queries = append(queries, query)
		if strings.Contains(query, "SELECT 'main'") {
			return &careRows{columns: []string{"source", "role", "content", "created_at"}, values: [][]driver.Value{{"main", "user", "最近有压力", time.Now()}}}, nil
		}
		return &careRows{columns: []string{"care_level", "care_evaluated_at"}, values: [][]driver.Value{{nil, nil}}}, nil
	})
	items, _, err := NewCollector(db).Collect(context.Background(), 42, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Source != "main" {
		t.Fatalf("evidence = %+v", items)
	}
	if !strings.Contains(queries[0], "direct_messages") || !strings.Contains(queries[0], "m.create_time >= $2") {
		t.Fatalf("collector query must combine direct messages and cutoff: %s", queries[0])
	}
}

type careRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r careRows) Columns() []string { return r.columns }
func (r careRows) Close() error      { return nil }

var _ = sql.ErrNoRows

type careDriver struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (d careDriver) Open(string) (driver.Conn, error) { return careConn{query: d.query}, nil }

type careConn struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (c careConn) Prepare(string) (driver.Stmt, error) { return nil, nil }
func (c careConn) Close() error                        { return nil }
func (c careConn) Begin() (driver.Tx, error)           { return nil, nil }
func (c careConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, q, args)
}
func (c careConn) CheckNamedValue(*driver.NamedValue) error { return nil }

var _ driver.QueryerContext = careConn{}
var _ driver.NamedValueChecker = careConn{}

func openCareTestDB(t *testing.T, query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)) *sql.DB {
	t.Helper()
	name := "care_test_" + strings.ReplaceAll(t.Name(), "/", "_")
	sql.Register(name, careDriver{query: query})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func (r *careRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
