package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestAdminAppChatMessagesExcludeSkillChatSessions(t *testing.T) {
	database := openAdminAppChatBoundaryDB(t)
	s := &Server{db: database}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/app-chat/messages", nil)

	s.adminAppChatMessages(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

const adminAppChatBoundaryDriverName = "admin_app_chat_boundary_test"

var registerAdminAppChatBoundaryDriver sync.Once

func openAdminAppChatBoundaryDB(t *testing.T) *sql.DB {
	t.Helper()
	registerAdminAppChatBoundaryDriver.Do(func() {
		sql.Register(adminAppChatBoundaryDriverName, adminAppChatBoundaryDriver{})
	})
	database, err := sql.Open(adminAppChatBoundaryDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

type adminAppChatBoundaryDriver struct{}

func (adminAppChatBoundaryDriver) Open(string) (driver.Conn, error) {
	return adminAppChatBoundaryConn{}, nil
}

type adminAppChatBoundaryConn struct{}

func (adminAppChatBoundaryConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (adminAppChatBoundaryConn) Close() error                        { return nil }
func (adminAppChatBoundaryConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (adminAppChatBoundaryConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "cs.scene = 'chat'") {
		return nil, errors.New("admin chat query includes non-chat sessions")
	}
	if strings.Contains(query, "SELECT count(*)") {
		return &adminAppChatBoundaryRows{columns: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
	}
	return &adminAppChatBoundaryRows{columns: []string{
		"id", "session_id", "app_user_id", "phone", "nickname", "card_id", "card_name",
		"role", "content", "sources", "favorite", "feedback", "create_time",
	}}, nil
}

type adminAppChatBoundaryRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *adminAppChatBoundaryRows) Columns() []string { return r.columns }
func (r *adminAppChatBoundaryRows) Close() error      { return nil }
func (r *adminAppChatBoundaryRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

var _ driver.QueryerContext = adminAppChatBoundaryConn{}
var _ driver.Rows = (*adminAppChatBoundaryRows)(nil)
