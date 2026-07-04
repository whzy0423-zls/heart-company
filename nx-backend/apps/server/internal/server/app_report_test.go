package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
)

func TestCapReportEndDateDoesNotReturnFutureDate(t *testing.T) {
	now := time.Date(2026, 7, 3, 18, 30, 0, 0, time.Local)
	futureSunday := time.Date(2026, 7, 5, 0, 0, 0, 0, time.Local)

	got := capReportEndDate(futureSunday, now)
	if !got.Equal(now) {
		t.Fatalf("expected current time for future report end date, got %s", got)
	}
}

func TestCapReportEndDateKeepsPastDate(t *testing.T) {
	now := time.Date(2026, 7, 3, 18, 30, 0, 0, time.Local)
	pastSunday := time.Date(2026, 6, 28, 0, 0, 0, 0, time.Local)

	got := capReportEndDate(pastSunday, now)
	if !got.Equal(pastSunday) {
		t.Fatalf("expected past report end date to be unchanged, got %s", got)
	}
}

func TestAppReportListReturnsServerErrorWhenPrimaryCardQueryFails(t *testing.T) {
	driverName := "app-report-query-error"
	registerAppReportQueryErrorDriver(driverName)
	database, err := sql.Open(driverName, driverName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodGet, "/api/app/reports", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 42}))
	res := httptest.NewRecorder()

	s.appReportList(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected database query failure to return 500, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestAppReportListReturnsEmptyWhenPrimaryCardHasNoSessions(t *testing.T) {
	driverName := "app-report-no-sessions"
	registerAppReportNoSessionsDriver(driverName)
	database, err := sql.Open(driverName, driverName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodGet, "/api/app/reports", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 42}))
	res := httptest.NewRecorder()

	s.appReportList(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected empty report list to return 200, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"data":[]`) {
		t.Fatalf("expected empty data array, got %s", res.Body.String())
	}
}

func TestAppReportListReturnsServerErrorWhenFirstSessionQueryFails(t *testing.T) {
	driverName := "app-report-session-query-error"
	registerAppReportSessionQueryErrorDriver(driverName)
	database, err := sql.Open(driverName, driverName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodGet, "/api/app/reports", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 42}))
	res := httptest.NewRecorder()

	s.appReportList(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected first session query failure to return 500, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestAppReportListUsesSingleWeeklyAggregateQuery(t *testing.T) {
	driverName := "app-report-weekly-aggregate"
	registerAppReportWeeklyAggregateDriver(driverName)
	appReportWeeklyQueryCount = 0
	database, err := sql.Open(driverName, driverName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodGet, "/api/app/reports", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 42}))
	res := httptest.NewRecorder()

	s.appReportList(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected report list to return 200, got %d body=%s", res.Code, res.Body.String())
	}
	if appReportWeeklyQueryCount != 1 {
		t.Fatalf("expected one weekly aggregate query, got %d", appReportWeeklyQueryCount)
	}
	if !strings.Contains(res.Body.String(), "本周共进行 3 次对话") {
		t.Fatalf("expected aggregate message count in response, got %s", res.Body.String())
	}
}

func TestAppReportListUsesSequentialReportIDs(t *testing.T) {
	driverName := "app-report-sequential-ids"
	registerAppReportSequentialIDsDriver(driverName)
	appReportSequentialFirstWeek = getWeekStart(time.Now().AddDate(0, 0, -14))
	database, err := sql.Open(driverName, driverName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodGet, "/api/app/reports", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 42}))
	res := httptest.NewRecorder()

	s.appReportList(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected report list to return 200, got %d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Data []struct {
			ID        int64  `json:"id"`
			StartDate string `json:"startDate"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 2 {
		t.Fatalf("expected two weekly reports, got %+v", payload.Data)
	}
	if payload.Data[0].ID != 2 || payload.Data[1].ID != 1 {
		t.Fatalf("expected newest-to-oldest sequential ids [2,1], got %+v", payload.Data)
	}
}

func registerAppReportQueryErrorDriver(name string) {
	defer func() {
		_ = recover()
	}()
	sql.Register(name, appReportQueryErrorDriver{})
}

func registerAppReportNoSessionsDriver(name string) {
	defer func() {
		_ = recover()
	}()
	sql.Register(name, appReportNoSessionsDriver{})
}

func registerAppReportSessionQueryErrorDriver(name string) {
	defer func() {
		_ = recover()
	}()
	sql.Register(name, appReportSessionQueryErrorDriver{})
}

func registerAppReportWeeklyAggregateDriver(name string) {
	defer func() {
		_ = recover()
	}()
	sql.Register(name, appReportWeeklyAggregateDriver{})
}

func registerAppReportSequentialIDsDriver(name string) {
	defer func() {
		_ = recover()
	}()
	sql.Register(name, appReportSequentialIDsDriver{})
}

type appReportQueryErrorDriver struct{}

func (appReportQueryErrorDriver) Open(string) (driver.Conn, error) {
	return appReportQueryErrorConn{}, nil
}

type appReportQueryErrorConn struct{}

func (appReportQueryErrorConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare should not be called")
}

func (appReportQueryErrorConn) Close() error { return nil }

func (appReportQueryErrorConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin should not be called")
}

func (appReportQueryErrorConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return nil, errors.New("database unavailable")
}

func (appReportQueryErrorConn) CheckNamedValue(*driver.NamedValue) error {
	return nil
}

type appReportEmptyRows struct{}

func (appReportEmptyRows) Columns() []string { return nil }
func (appReportEmptyRows) Close() error      { return nil }
func (appReportEmptyRows) Next([]driver.Value) error {
	return io.EOF
}

var _ driver.QueryerContext = appReportQueryErrorConn{}
var _ driver.NamedValueChecker = appReportQueryErrorConn{}
var _ driver.Rows = appReportEmptyRows{}

type appReportNoSessionsDriver struct{}

func (appReportNoSessionsDriver) Open(string) (driver.Conn, error) {
	return appReportNoSessionsConn{}, nil
}

type appReportNoSessionsConn struct{}

func (appReportNoSessionsConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare should not be called")
}

func (appReportNoSessionsConn) Close() error { return nil }

func (appReportNoSessionsConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin should not be called")
}

func (appReportNoSessionsConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT id FROM app_user_cards"):
		return &appReportRows{
			columns: []string{"id"},
			values:  [][]driver.Value{{int64(7)}},
		}, nil
	case strings.Contains(query, "SELECT MIN(create_time) FROM app_chat_sessions"):
		return &appReportRows{
			columns: []string{"min"},
			values:  [][]driver.Value{{nil}},
		}, nil
	default:
		return nil, errors.New("unexpected query: " + query)
	}
}

func (appReportNoSessionsConn) CheckNamedValue(*driver.NamedValue) error {
	return nil
}

type appReportRows struct {
	columns []string
	index   int
	values  [][]driver.Value
}

func (r *appReportRows) Columns() []string { return r.columns }
func (r *appReportRows) Close() error      { return nil }
func (r *appReportRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

var _ driver.QueryerContext = appReportNoSessionsConn{}
var _ driver.NamedValueChecker = appReportNoSessionsConn{}
var _ driver.Rows = (*appReportRows)(nil)

type appReportSessionQueryErrorDriver struct{}

func (appReportSessionQueryErrorDriver) Open(string) (driver.Conn, error) {
	return appReportSessionQueryErrorConn{}, nil
}

type appReportSessionQueryErrorConn struct{}

func (appReportSessionQueryErrorConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare should not be called")
}

func (appReportSessionQueryErrorConn) Close() error { return nil }

func (appReportSessionQueryErrorConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin should not be called")
}

func (appReportSessionQueryErrorConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "SELECT id FROM app_user_cards") {
		return &appReportRows{
			columns: []string{"id"},
			values:  [][]driver.Value{{int64(7)}},
		}, nil
	}
	if strings.Contains(query, "SELECT MIN(create_time) FROM app_chat_sessions") {
		return nil, errors.New("session query failed")
	}
	return nil, errors.New("unexpected query: " + query)
}

func (appReportSessionQueryErrorConn) CheckNamedValue(*driver.NamedValue) error {
	return nil
}

var _ driver.QueryerContext = appReportSessionQueryErrorConn{}
var _ driver.NamedValueChecker = appReportSessionQueryErrorConn{}

var appReportWeeklyQueryCount int

type appReportWeeklyAggregateDriver struct{}

func (appReportWeeklyAggregateDriver) Open(string) (driver.Conn, error) {
	return appReportWeeklyAggregateConn{}, nil
}

type appReportWeeklyAggregateConn struct{}

func (appReportWeeklyAggregateConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare should not be called")
}

func (appReportWeeklyAggregateConn) Close() error { return nil }

func (appReportWeeklyAggregateConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin should not be called")
}

func (appReportWeeklyAggregateConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT id FROM app_user_cards"):
		return &appReportRows{
			columns: []string{"id"},
			values:  [][]driver.Value{{int64(7)}},
		}, nil
	case strings.Contains(query, "SELECT MIN(create_time) FROM app_chat_sessions"):
		return &appReportRows{
			columns: []string{"min"},
			values: [][]driver.Value{{
				time.Date(2026, 6, 1, 8, 0, 0, 0, time.Local),
			}},
		}, nil
	case strings.Contains(query, "date_trunc('week'"):
		appReportWeeklyQueryCount++
		return &appReportRows{
			columns: []string{"week_start", "msg_count"},
			values: [][]driver.Value{{
				getWeekStart(time.Now()),
				int64(3),
			}},
		}, nil
	case strings.Contains(query, "SELECT COUNT(*) FROM app_chat_messages"):
		return nil, errors.New("report list must not query each week individually")
	default:
		return nil, errors.New("unexpected query: " + query)
	}
}

func (appReportWeeklyAggregateConn) CheckNamedValue(*driver.NamedValue) error {
	return nil
}

var _ driver.QueryerContext = appReportWeeklyAggregateConn{}
var _ driver.NamedValueChecker = appReportWeeklyAggregateConn{}

var appReportSequentialFirstWeek time.Time

type appReportSequentialIDsDriver struct{}

func (appReportSequentialIDsDriver) Open(string) (driver.Conn, error) {
	return appReportSequentialIDsConn{}, nil
}

type appReportSequentialIDsConn struct{}

func (appReportSequentialIDsConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare should not be called")
}

func (appReportSequentialIDsConn) Close() error { return nil }

func (appReportSequentialIDsConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin should not be called")
}

func (appReportSequentialIDsConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT id FROM app_user_cards"):
		return &appReportRows{
			columns: []string{"id"},
			values:  [][]driver.Value{{int64(7)}},
		}, nil
	case strings.Contains(query, "SELECT MIN(create_time) FROM app_chat_sessions"):
		return &appReportRows{
			columns: []string{"min"},
			values:  [][]driver.Value{{appReportSequentialFirstWeek.Add(8 * time.Hour)}},
		}, nil
	case strings.Contains(query, "date_trunc('week'"):
		return &appReportRows{
			columns: []string{"week_start", "msg_count"},
			values: [][]driver.Value{
				{appReportSequentialFirstWeek, int64(2)},
				{appReportSequentialFirstWeek.AddDate(0, 0, 7), int64(4)},
			},
		}, nil
	default:
		return nil, errors.New("unexpected query: " + query)
	}
}

func (appReportSequentialIDsConn) CheckNamedValue(*driver.NamedValue) error {
	return nil
}

var _ driver.QueryerContext = appReportSequentialIDsConn{}
var _ driver.NamedValueChecker = appReportSequentialIDsConn{}

func TestWeekLabelDoesNotContainWhitespace(t *testing.T) {
	label := time.Date(2026, 7, 3, 18, 30, 0, 0, time.Local).Format("2006年第") + getWeekOfYear(time.Date(2026, 7, 3, 18, 30, 0, 0, time.Local)) + "周"
	if strings.Contains(label, " ") {
		t.Fatalf("expected compact week label, got %q", label)
	}
}
