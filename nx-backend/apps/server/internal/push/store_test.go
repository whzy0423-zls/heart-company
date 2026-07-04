package push

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestRegisterDeviceUpsertsByRegistrationID(t *testing.T) {
	execRecorder.reset()
	database, err := sql.Open("push_store_test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	store := NewStore(database, NoopPusher{})
	if err := store.RegisterDevice(context.Background(), 2, "same-registration-id", "android", "new-phone"); err != nil {
		t.Fatalf("register device: %v", err)
	}

	query := execRecorder.query()
	if !strings.Contains(query, "ON CONFLICT (registration_id) DO UPDATE") {
		t.Fatalf("expected upsert to be keyed by registration_id, query:\n%s", query)
	}
	if strings.Contains(query, "ON CONFLICT (app_user_id, registration_id)") {
		t.Fatalf("upsert must not allow one registration_id under multiple users, query:\n%s", query)
	}
}

func TestGetAllRegistrationIDsFiltersActiveUsers(t *testing.T) {
	execRecorder.reset()
	database, err := sql.Open("push_store_test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	store := NewStore(database, NoopPusher{})
	if _, err := store.GetAllRegistrationIDs(context.Background()); err != nil {
		t.Fatalf("get all registration ids: %v", err)
	}

	query := execRecorder.query()
	if !strings.Contains(query, "JOIN app_users") || !strings.Contains(query, "u.status = 'active'") {
		t.Fatalf("expected all-user push query to include active app user filter, query:\n%s", query)
	}
}

func TestForEachRegistrationIDBatchUsesKeysetPagination(t *testing.T) {
	execRecorder.reset()
	database, err := sql.Open("push_store_test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	store := NewStore(database, NoopPusher{})
	if err := store.ForEachRegistrationIDBatch(context.Background(), "all", "", 500, func([]string) error {
		t.Fatal("empty fixture should not call callback")
		return nil
	}); err != nil {
		t.Fatalf("iterate registration ids: %v", err)
	}

	query := execRecorder.query()
	if !strings.Contains(query, "dt.id > $1") ||
		!strings.Contains(query, "ORDER BY dt.id ASC") ||
		!strings.Contains(query, "LIMIT $2") {
		t.Fatalf("expected keyset-paginated token query, query:\n%s", query)
	}
}

func TestCountAudienceUsesDistinctDevicesAndUsers(t *testing.T) {
	execRecorder.reset()
	database, err := sql.Open("push_store_test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	store := NewStore(database, NoopPusher{})
	if _, _, err := store.CountAudience(context.Background(), "level", "vip"); err != nil {
		t.Fatalf("count audience: %v", err)
	}

	query := execRecorder.query()
	if !strings.Contains(query, "COUNT(DISTINCT dt.registration_id)") ||
		!strings.Contains(query, "COUNT(DISTINCT u.id)") ||
		!strings.Contains(query, "u.status = 'active'") ||
		!strings.Contains(query, "u.member_level = $1") {
		t.Fatalf("expected audience count query to count distinct active vip devices/users, query:\n%s", query)
	}
}

var execRecorder = &recordingExec{}

func init() {
	sql.Register("push_store_test", recordingDriver{})
}

type recordingExec struct {
	mu    sync.Mutex
	value string
}

func (r *recordingExec) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = ""
}

func (r *recordingExec) set(query string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = query
}

func (r *recordingExec) query() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.value
}

type recordingDriver struct{}

func (recordingDriver) Open(string) (driver.Conn, error) {
	return recordingConn{}, nil
}

type recordingConn struct{}

func (recordingConn) Prepare(string) (driver.Stmt, error) {
	return nil, nil
}

func (recordingConn) Close() error {
	return nil
}

func (recordingConn) Begin() (driver.Tx, error) {
	return nil, nil
}

func (recordingConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	execRecorder.set(query)
	return driver.RowsAffected(1), nil
}

func (recordingConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	execRecorder.set(query)
	if strings.Contains(query, "COUNT(DISTINCT dt.registration_id)") {
		return &oneRow{columns: []string{"device_count", "user_count"}, values: []driver.Value{int64(0), int64(0)}}, nil
	}
	return emptyRows{}, nil
}

type emptyRows struct{}

func (emptyRows) Columns() []string {
	return nil
}

func (emptyRows) Close() error {
	return nil
}

func (emptyRows) Next([]driver.Value) error {
	return io.EOF
}

type oneRow struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r oneRow) Columns() []string { return r.columns }

func (r oneRow) Close() error { return nil }

func (r *oneRow) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}
