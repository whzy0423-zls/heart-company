package appnotification

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

func TestNormalizePage(t *testing.T) {
	tests := []struct {
		page, pageSize         int
		wantPage, wantPageSize int
	}{
		{0, 0, 1, 20},
		{-1, -1, 1, 20},
		{2, 50, 2, 50},
		{3, 101, 3, 20},
	}
	for _, tt := range tests {
		page, pageSize := normalizePage(tt.page, tt.pageSize)
		if page != tt.wantPage || pageSize != tt.wantPageSize {
			t.Fatalf("normalizePage(%d, %d)=(%d,%d), want (%d,%d)",
				tt.page, tt.pageSize, page, pageSize, tt.wantPage, tt.wantPageSize)
		}
	}
}

func TestCreateForUserLocksActiveUserInInsertStatement(t *testing.T) {
	recorder := &notificationQueryRecorder{}
	database := openNotificationQueryTestDB(t, recorder)
	store := NewStore(database)

	if _, err := store.CreateForUser(context.Background(), 42, "life_story", "ready", "content", "/life-stories/7/read", "story:7"); err != nil {
		t.Fatal(err)
	}
	query := strings.Join(strings.Fields(recorder.query), " ")
	for _, fragment := range []string{"FROM app_users", "status = 'active'", "FOR UPDATE", "INSERT INTO app_notifications"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("notification insert must serialize with account deletion; missing %q in %s", fragment, query)
		}
	}
}

var notificationQueryDriverSequence atomic.Int64

type notificationQueryRecorder struct{ query string }

func openNotificationQueryTestDB(t *testing.T, recorder *notificationQueryRecorder) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("app_notification_query_test_%d", notificationQueryDriverSequence.Add(1))
	sql.Register(name, notificationQueryDriver{recorder: recorder})
	database, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

type notificationQueryDriver struct{ recorder *notificationQueryRecorder }

func (d notificationQueryDriver) Open(string) (driver.Conn, error) {
	return &notificationQueryConn{recorder: d.recorder}, nil
}

type notificationQueryConn struct{ recorder *notificationQueryRecorder }

func (*notificationQueryConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*notificationQueryConn) Close() error                        { return nil }
func (*notificationQueryConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (c *notificationQueryConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.recorder.query = query
	return &notificationIDRows{}, nil
}

type notificationIDRows struct{ done bool }

func (*notificationIDRows) Columns() []string { return []string{"id"} }
func (*notificationIDRows) Close() error      { return nil }
func (r *notificationIDRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	dest[0] = int64(9)
	r.done = true
	return nil
}

var _ driver.QueryerContext = (*notificationQueryConn)(nil)
