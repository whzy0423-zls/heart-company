package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"nine-xing/nx-backend/apps/server/internal/push"
	"strings"
	"testing"
)

func TestAdminPushSendRejectsOversizedBody(t *testing.T) {
	body := `{"title":"` + strings.Repeat("a", 9000) + `","content":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/push/send", strings.NewReader(body))
	res := httptest.NewRecorder()

	s := &Server{}
	s.adminPushSend(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected oversized push body to be rejected, got %d", res.Code)
	}
}

func TestAdminPushSendRejectsUnknownMemberLevel(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/push/send", strings.NewReader(`{
		"title":"会员推送",
		"content":"test",
		"targetType":"level",
		"targetValue":"gold"
	}`))
	res := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected unknown member level to return 400, panicked: %v", recovered)
		}
	}()

	s := &Server{}
	s.adminPushSend(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown member level to be rejected, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestAdminPushAudienceCountRejectsUnknownMemberLevel(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/push/audience-count?targetType=level&targetValue=gold", nil)
	res := httptest.NewRecorder()

	s := &Server{}
	s.adminPushAudienceCount(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown member level to be rejected, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestAdminPushAudienceCountReturnsDeviceAndUserCount(t *testing.T) {
	database, err := sql.Open("admin_push_count_test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := &Server{pushStore: push.NewStore(database, push.NoopPusher{})}
	req := httptest.NewRequest(http.MethodGet, "/api/push/audience-count?targetType=level&targetValue=vip", nil)
	res := httptest.NewRecorder()

	s.adminPushAudienceCount(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var body struct {
		Data struct {
			DeviceCount int64 `json:"deviceCount"`
			UserCount   int64 `json:"userCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.DeviceCount != 0 || body.Data.UserCount != 0 {
		t.Fatalf("expected zero counts from fixture, got %+v", body.Data)
	}
}

func init() {
	sql.Register("admin_push_count_test", adminPushCountDriver{})
}

type adminPushCountDriver struct{}

func (adminPushCountDriver) Open(string) (driver.Conn, error) {
	return adminPushCountConn{}, nil
}

type adminPushCountConn struct{}

func (adminPushCountConn) Prepare(string) (driver.Stmt, error) {
	return nil, nil
}

func (adminPushCountConn) Close() error {
	return nil
}

func (adminPushCountConn) Begin() (driver.Tx, error) {
	return nil, nil
}

func (adminPushCountConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	return &adminPushCountRows{columns: []string{"device_count", "user_count"}, values: []driver.Value{int64(0), int64(0)}}, nil
}

type adminPushCountRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *adminPushCountRows) Columns() []string { return r.columns }

func (r *adminPushCountRows) Close() error { return nil }

func (r *adminPushCountRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}

func init() {
	sql.Register("push_store_test", serverPushStoreTestDriver{})
}

type serverPushStoreTestDriver struct{}

func (serverPushStoreTestDriver) Open(string) (driver.Conn, error) {
	return serverPushStoreTestConn{}, nil
}

type serverPushStoreTestConn struct{}

func (serverPushStoreTestConn) Prepare(string) (driver.Stmt, error) { return nil, nil }
func (serverPushStoreTestConn) Close() error                        { return nil }
func (serverPushStoreTestConn) Begin() (driver.Tx, error)           { return nil, nil }
func (serverPushStoreTestConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}
func (serverPushStoreTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "COUNT(DISTINCT dt.registration_id)") {
		return &serverPushStoreOneRow{columns: []string{"device_count", "user_count"}, values: []driver.Value{int64(0), int64(0)}}, nil
	}
	return serverPushStoreEmptyRows{}, nil
}

type serverPushStoreEmptyRows struct{}

func (serverPushStoreEmptyRows) Columns() []string         { return nil }
func (serverPushStoreEmptyRows) Close() error              { return nil }
func (serverPushStoreEmptyRows) Next([]driver.Value) error { return io.EOF }

type serverPushStoreOneRow struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *serverPushStoreOneRow) Columns() []string { return r.columns }
func (r *serverPushStoreOneRow) Close() error      { return nil }
func (r *serverPushStoreOneRow) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}

var _ driver.ExecerContext = serverPushStoreTestConn{}
var _ driver.QueryerContext = serverPushStoreTestConn{}
