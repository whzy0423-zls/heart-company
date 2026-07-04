package auditlog

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

func TestStoreRecordInsertsAuditLog(t *testing.T) {
	var seenQuery string
	var seenArgs []driver.NamedValue
	db := openAuditLogTestDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
		seenQuery = query
		seenArgs = args
		return driver.RowsAffected(1), nil
	})
	store := NewStore(db)

	err := store.Record(context.Background(), Entry{
		OperatorID:   7,
		OperatorName: "admin",
		Action:       "app_user.update",
		TargetType:   "app_user",
		TargetID:     "42",
		IP:           "127.0.0.1",
		UserAgent:    "test-agent",
		Before:       map[string]any{"status": "active"},
		After:        map[string]any{"status": "disabled"},
		Summary:      "禁用用户",
	})
	if err != nil {
		t.Fatalf("record audit log: %v", err)
	}
	if !strings.Contains(seenQuery, "INSERT INTO admin_operation_logs") {
		t.Fatalf("expected insert into admin_operation_logs, got %q", seenQuery)
	}
	if len(seenArgs) != 10 {
		t.Fatalf("expected 10 args, got %+v", seenArgs)
	}
	if seenArgs[1].Value != "admin" || seenArgs[2].Value != "app_user.update" || seenArgs[3].Value != "app_user" || seenArgs[4].Value != "42" {
		t.Fatalf("unexpected args: %+v", seenArgs)
	}
	if !strings.Contains(seenArgs[7].Value.(string), "active") || !strings.Contains(seenArgs[8].Value.(string), "disabled") {
		t.Fatalf("expected before/after json, got %+v", seenArgs)
	}
}

func TestStoreRecordNoopsWithoutDatabase(t *testing.T) {
	store := NewStore(nil)
	if err := store.Record(context.Background(), Entry{Action: "noop"}); err != nil {
		t.Fatalf("nil db record should not fail: %v", err)
	}
}

var auditLogDriverSeq atomic.Int64

func openAuditLogTestDB(t *testing.T, exec func(context.Context, string, []driver.NamedValue) (driver.Result, error)) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("audit_log_test_%d", auditLogDriverSeq.Add(1))
	sql.Register(name, auditLogDriver{exec: exec})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type auditLogDriver struct {
	exec func(context.Context, string, []driver.NamedValue) (driver.Result, error)
}

func (d auditLogDriver) Open(string) (driver.Conn, error) { return auditLogConn{exec: d.exec}, nil }

type auditLogConn struct {
	exec func(context.Context, string, []driver.NamedValue) (driver.Result, error)
}

func (auditLogConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (auditLogConn) Close() error                        { return nil }
func (auditLogConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c auditLogConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.exec(ctx, query, args)
}

func TestStoreListReturnsPagedAuditLogs(t *testing.T) {
	var seenQuery string
	var seenArgs []driver.NamedValue
	db := openAuditLogQueryTestDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		seenQuery = query
		seenArgs = args
		if strings.Contains(query, "count(*)") {
			return &auditLogRows{columns: []string{"count"}, values: [][]driver.Value{{int64(1)}}}, nil
		}
		return &auditLogRows{columns: []string{"id", "operator_id", "operator_name", "action", "target_type", "target_id", "ip", "user_agent", "before_data", "after_data", "summary", "create_time"}, values: [][]driver.Value{{int64(9), int64(7), "admin", "app_user.update", "app_user", "42", "127.0.0.1", "agent", []byte(`{"status":"active"}`), []byte(`{"status":"disabled"}`), "更新", "2026/07/04 14:00:00"}}}, nil
	})
	store := NewStore(db)

	result, err := store.List(context.Background(), map[string]string{"action": "app_user.update", "page": "2", "pageSize": "10"})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Items[0].ID != 9 || result.Items[0].Action != "app_user.update" || result.Items[0].TargetID != "42" {
		t.Fatalf("unexpected item: %+v", result.Items[0])
	}
	if !strings.Contains(seenQuery, "admin_operation_logs") || !strings.Contains(seenQuery, "LIMIT") {
		t.Fatalf("expected list query, got %q", seenQuery)
	}
	if len(seenArgs) == 0 {
		t.Fatal("expected query args")
	}
}

func openAuditLogQueryTestDB(t *testing.T, query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("audit_log_query_test_%d", auditLogDriverSeq.Add(1))
	sql.Register(name, auditLogQueryDriver{query: query})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type auditLogQueryDriver struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (d auditLogQueryDriver) Open(string) (driver.Conn, error) {
	return auditLogQueryConn{query: d.query}, nil
}

type auditLogQueryConn struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (auditLogQueryConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (auditLogQueryConn) Close() error                        { return nil }
func (auditLogQueryConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c auditLogQueryConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type auditLogRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *auditLogRows) Columns() []string { return r.columns }
func (r *auditLogRows) Close() error      { return nil }
func (r *auditLogRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
