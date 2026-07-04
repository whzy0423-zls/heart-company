package server

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestAppAnalyticsEventRejectsBlankEvent(t *testing.T) {
	database := newAppAnalyticsUnitDB(t, "blank_event")
	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodPost, "/api/app/analytics/event", strings.NewReader(`{"event":"   "}`))
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 42}))
	res := httptest.NewRecorder()

	s.appAnalyticsEvent(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestAppAnalyticsRouteRequiresAuthAndWritesEvent(t *testing.T) {
	atomic.StoreInt64(&appAnalyticsExecCount, 0)
	database := newAppAnalyticsUnitDB(t, "route")
	s := &Server{
		env:      config.Env{JWTSecret: "test-secret"},
		mux:      http.NewServeMux(),
		db:       database,
		appUsers: appuser.NewStore(database),
	}
	s.routes()

	noAuth := performAppAPI(s.mux, http.MethodPost, "/api/app/analytics/event", "", map[string]any{
		"event":  "app_open",
		"params": map[string]any{"from": "test"},
	})
	if noAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d body=%s", noAuth.Code, noAuth.Body.String())
	}

	token, err := auth.SignWithExpiry(auth.UserInfo{
		ID:        42,
		Phone:     "13800009005",
		TokenKind: auth.TokenKindApp,
	}, "test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	res := performAppAPI(s.mux, http.MethodPost, "/api/app/analytics/event", token, map[string]any{
		"event":  "app_open",
		"params": map[string]any{"from": "test"},
		"ts":     "2026-07-03T10:00:00Z",
	})
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if got := atomic.LoadInt64(&appAnalyticsExecCount); got != 1 {
		t.Fatalf("expected analytics insert to run once, got %d", got)
	}
}

func TestAppAnalyticsIntegrationStoresEvent(t *testing.T) {
	handler, database := newAppAPITestServer(t)
	token, _, userID := appAPILogin(t, handler, "13800009005")

	noAuth := performAppAPI(handler, http.MethodPost, "/api/app/analytics/event", "", map[string]any{
		"event":  "app_open",
		"params": map[string]any{"from": "test"},
	})
	if noAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d body=%s", noAuth.Code, noAuth.Body.String())
	}

	res := performAppAPI(handler, http.MethodPost, "/api/app/analytics/event", token, map[string]any{
		"event":  "app_open",
		"params": map[string]any{"from": "test"},
		"ts":     "2026-07-03T10:00:00Z",
	})
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_analytics_events WHERE app_user_id = $1 AND event = 'app_open'`, userID); got != 1 {
		t.Fatalf("expected app analytics event to be stored, got %d", got)
	}
}

func newAppAnalyticsUnitDB(t *testing.T, mode string) *sql.DB {
	t.Helper()
	registerAppAnalyticsTestDriver()
	database, err := sql.Open(appAnalyticsTestDriverName, mode)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

const appAnalyticsTestDriverName = "app_analytics_test"

var registerAppAnalyticsTestDriverOnce sync.Once

func registerAppAnalyticsTestDriver() {
	registerAppAnalyticsTestDriverOnce.Do(func() {
		sql.Register(appAnalyticsTestDriverName, appAnalyticsTestDriver{})
	})
}

type appAnalyticsTestDriver struct{}

func (appAnalyticsTestDriver) Open(name string) (driver.Conn, error) {
	return &appAnalyticsTestConn{mode: name}, nil
}

type appAnalyticsTestConn struct {
	mode string
}

func (c *appAnalyticsTestConn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (c *appAnalyticsTestConn) Close() error {
	return nil
}

func (c *appAnalyticsTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (c *appAnalyticsTestConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "app_analytics_events") {
		atomic.AddInt64(&appAnalyticsExecCount, 1)
	}
	return driver.RowsAffected(1), nil
}

func (c *appAnalyticsTestConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "FROM app_users") {
		now := time.Now()
		return &appAnalyticsRows{
			columns: []string{"id", "phone", "nickname", "avatar", "status", "member_level", "register_source", "last_login_at", "create_time", "update_time"},
			values: [][]driver.Value{{
				int64(42),
				"13800009005",
				"Test User",
				"",
				"active",
				"free",
				"sms",
				nil,
				now,
				now,
			}},
		}, nil
	}
	return nil, errors.New("not implemented")
}

var _ driver.ExecerContext = (*appAnalyticsTestConn)(nil)
var _ driver.QueryerContext = (*appAnalyticsTestConn)(nil)

var appAnalyticsExecCount int64

func performAppAnalyticsHandler(t *testing.T, handler http.HandlerFunc, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/app/analytics/event", &body)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 42}))
	res := httptest.NewRecorder()
	handler(res, req)
	return res
}

type appAnalyticsRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *appAnalyticsRows) Columns() []string {
	return r.columns
}

func (r *appAnalyticsRows) Close() error {
	return nil
}

func (r *appAnalyticsRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
