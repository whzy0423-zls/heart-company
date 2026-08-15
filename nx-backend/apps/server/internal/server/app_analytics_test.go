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
		t.Fatalf("expected 200, got %d body=%s lastQuery=%s", res.Code, res.Body.String(), appAnalyticsLastQuery)
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
		t.Fatalf("expected 200, got %d body=%s lastQuery=%s", res.Code, res.Body.String(), appAnalyticsLastQuery)
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_analytics_events WHERE app_user_id = $1 AND event = 'app_open'`, userID); got != 1 {
		t.Fatalf("expected app analytics event to be stored, got %d", got)
	}
}

func TestAppAnalyticsOverviewReturnsEmptySafePayload(t *testing.T) {
	database := newAppAnalyticsUnitDB(t, "overview_empty")
	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodGet, "/api/app-analytics/overview", nil)
	res := httptest.NewRecorder()

	s.appAnalyticsOverview(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s lastQuery=%s", res.Code, res.Body.String(), appAnalyticsLastQuery)
	}
	var body struct {
		Data struct {
			TotalUsers         int64            `json:"totalUsers"`
			NewUsersToday      int64            `json:"newUsersToday"`
			ActiveUsers        int64            `json:"activeUsers"`
			EnabledUsers       int64            `json:"enabledUsers"`
			MemberUsers        int64            `json:"memberUsers"`
			DisabledUsers      int64            `json:"disabledUsers"`
			ExtractedUsers     int64            `json:"extractedUsers"`
			QuizSubmissions    int64            `json:"quizSubmissions"`
			Cards              int64            `json:"cards"`
			Memories           int64            `json:"memories"`
			ChatSessions       int64            `json:"chatSessions"`
			ChatMessages       int64            `json:"chatMessages"`
			Compatibility      int64            `json:"compatibilityReports"`
			RecentUsers        []map[string]any `json:"recentUsers"`
			RecentExtracted    []map[string]any `json:"recentExtractedUsers"`
			RecentMemoryUsers  []map[string]any `json:"recentMemoryUsers"`
			MemberDistribution map[string]int64 `json:"memberDistribution"`
			StatusDistribution map[string]int64 `json:"statusDistribution"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.TotalUsers != 0 || body.Data.NewUsersToday != 0 || body.Data.ActiveUsers != 0 ||
		body.Data.EnabledUsers != 0 ||
		body.Data.MemberUsers != 0 || body.Data.DisabledUsers != 0 || body.Data.ExtractedUsers != 0 {
		t.Fatalf("expected empty user counts, got %+v", body.Data)
	}
	if body.Data.QuizSubmissions != 0 || body.Data.Cards != 0 || body.Data.Memories != 0 ||
		body.Data.ChatSessions != 0 || body.Data.ChatMessages != 0 || body.Data.Compatibility != 0 {
		t.Fatalf("expected empty activity counts, got %+v", body.Data)
	}
	if body.Data.RecentUsers == nil || body.Data.RecentExtracted == nil || body.Data.RecentMemoryUsers == nil ||
		body.Data.MemberDistribution == nil || body.Data.StatusDistribution == nil {
		t.Fatalf("expected empty arrays/maps instead of nil: %+v", body.Data)
	}
}

func TestAppAnalyticsOverviewReturns500WhenDatabaseMissing(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/app-analytics/overview", nil)
	res := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected missing database to return 500, panicked: %v", recovered)
		}
	}()

	s.appAnalyticsOverview(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for missing database, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestAppAnalyticsOverviewUsesShanghaiDayBoundary(t *testing.T) {
	resetAppAnalyticsOverviewCaptures()
	database := newAppAnalyticsUnitDB(t, "overview_empty")
	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodGet, "/api/app-analytics/overview", nil)
	res := httptest.NewRecorder()

	s.appAnalyticsOverview(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(appAnalyticsOverviewUserCountQuery, "AT TIME ZONE 'Asia/Shanghai'") {
		t.Fatalf("expected newUsersToday query to use explicit Asia/Shanghai boundary, got %s", appAnalyticsOverviewUserCountQuery)
	}
}

func TestFormatAppAnalyticsTimeUsesShanghaiTimezone(t *testing.T) {
	got := formatAppAnalyticsTime(time.Date(2026, 7, 4, 16, 30, 0, 0, time.UTC))
	if got != "2026/07/05 00:30:00" {
		t.Fatalf("expected Shanghai time formatting, got %s", got)
	}
}

func TestAppAnalyticsOverviewUsesEventWindowForActiveUsersAndKeepsEnabledUsers(t *testing.T) {
	resetAppAnalyticsOverviewCaptures()
	database := newAppAnalyticsUnitDB(t, "overview_window_and_extracts")
	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodGet, "/api/app-analytics/overview?days=365&limit=999", nil)
	res := httptest.NewRecorder()

	s.appAnalyticsOverview(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s lastQuery=%s", res.Code, res.Body.String(), appAnalyticsLastQuery)
	}
	var body struct {
		Data struct {
			ActiveUsers          int64 `json:"activeUsers"`
			EnabledUsers         int64 `json:"enabledUsers"`
			ExtractedUsers       int64 `json:"extractedUsers"`
			RecentExtractedUsers []struct {
				ID              int64  `json:"id"`
				LastExtractedAt string `json:"lastExtractedAt"`
				LastMemoryAt    string `json:"lastMemoryAt"`
				MemoryCount     int64  `json:"memoryCount"`
				Phone           string `json:"phone"`
				PrimaryType     int    `json:"primaryType"`
			} `json:"recentExtractedUsers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.EnabledUsers != 9 {
		t.Fatalf("expected enabledUsers to keep status=active count 9, got %+v", body.Data)
	}
	if body.Data.ActiveUsers != 2 {
		t.Fatalf("expected activeUsers to use analytics events distinct users 2, got %+v", body.Data)
	}
	if body.Data.ExtractedUsers != 4 {
		t.Fatalf("expected extractedUsers to include quiz/card/memory/compatibility users, got %+v", body.Data)
	}
	if len(body.Data.RecentExtractedUsers) != 4 {
		t.Fatalf("expected quiz/card/memory/compatibility users in recentExtractedUsers, got %+v", body.Data.RecentExtractedUsers)
	}
	if body.Data.RecentExtractedUsers[0].Phone != "13800009011" || body.Data.RecentExtractedUsers[0].PrimaryType != 8 {
		t.Fatalf("expected most recently extracted compatibility user first with primary type, got %+v", body.Data.RecentExtractedUsers[0])
	}
	if body.Data.RecentExtractedUsers[0].LastExtractedAt == "" || body.Data.RecentExtractedUsers[0].LastMemoryAt == "" {
		t.Fatalf("expected recent extracted user to expose lastExtractedAt while keeping lastMemoryAt compatibility, got %+v", body.Data.RecentExtractedUsers[0])
	}
	if appAnalyticsLastActiveWindowDays != 30 {
		t.Fatalf("expected days query to be capped at 30, got %d", appAnalyticsLastActiveWindowDays)
	}
	if appAnalyticsRecentUsersLimit != 50 || appAnalyticsRecentExtractedLimit != 50 {
		t.Fatalf("expected recent query limits to be capped at 50, got users=%d extracted=%d", appAnalyticsRecentUsersLimit, appAnalyticsRecentExtractedLimit)
	}
}

func TestAppAnalyticsOverviewRecentMemoryUsersIncludePrimaryType(t *testing.T) {
	database := newAppAnalyticsUnitDB(t, "overview_memory_user")
	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodGet, "/api/app-analytics/overview", nil)
	res := httptest.NewRecorder()

	s.appAnalyticsOverview(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var body struct {
		Data struct {
			RecentMemoryUsers []struct {
				MemoryCount int64  `json:"memoryCount"`
				Phone       string `json:"phone"`
				PrimaryType int    `json:"primaryType"`
			} `json:"recentMemoryUsers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.RecentMemoryUsers) != 1 {
		t.Fatalf("expected one recent memory user, got %+v", body.Data.RecentMemoryUsers)
	}
	item := body.Data.RecentMemoryUsers[0]
	if item.Phone != "13800009006" || item.PrimaryType != 5 || item.MemoryCount != 2 {
		t.Fatalf("expected recent memory user to include latest primary type, got %+v", item)
	}
}

func TestAppAnalyticsOverviewSQLErrorReturns500(t *testing.T) {
	database := newAppAnalyticsUnitDB(t, "overview_error")
	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodGet, "/api/app-analytics/overview", nil)
	res := httptest.NewRecorder()

	s.appAnalyticsOverview(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", res.Code, res.Body.String())
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
	return nil, errors.New("not implemented: " + strings.Join(strings.Fields(query), " "))
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
	appAnalyticsLastQuery = strings.Join(strings.Fields(query), " ")
	if c.mode == "overview_error" {
		return nil, errors.New("query failed")
	}
	if strings.Contains(query, "FROM app_users") && strings.Contains(query, "COUNT(*) FILTER") {
		appAnalyticsOverviewUserCountQuery = strings.Join(strings.Fields(query), " ")
		if c.mode == "overview_window_and_extracts" {
			return &appAnalyticsRows{
				columns: []string{"total_users", "new_users_today", "enabled_users", "member_users", "disabled_users"},
				values:  [][]driver.Value{{int64(10), int64(1), int64(9), int64(3), int64(1)}},
			}, nil
		}
		return &appAnalyticsRows{
			columns: []string{"total_users", "new_users_today", "active_users", "member_users", "disabled_users"},
			values:  [][]driver.Value{{int64(0), int64(0), int64(0), int64(0), int64(0)}},
		}, nil
	}
	if strings.Contains(query, "FROM app_analytics_events") && strings.Contains(query, "COUNT(DISTINCT") {
		if len(args) > 0 {
			appAnalyticsLastActiveWindowDays = int(args[0].Value.(int64))
		}
		if c.mode == "overview_window_and_extracts" {
			return &appAnalyticsRows{columns: []string{"count"}, values: [][]driver.Value{{int64(2)}}}, nil
		}
		return &appAnalyticsRows{columns: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
	}
	if strings.Contains(query, "WITH extracted AS") && strings.Contains(query, "COUNT(DISTINCT") {
		if c.mode == "overview_window_and_extracts" && strings.Contains(query, "app_compatibility_reports") {
			return &appAnalyticsRows{columns: []string{"count"}, values: [][]driver.Value{{int64(4)}}}, nil
		}
		return &appAnalyticsRows{columns: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
	}
	if strings.Contains(query, "last_extracted_at") {
		if len(args) > 0 {
			appAnalyticsRecentExtractedLimit = int(args[0].Value.(int64))
		}
		if c.mode == "overview_window_and_extracts" {
			base := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
			return &appAnalyticsRows{
				columns: []string{"id", "phone", "nickname", "avatar", "status", "member_level", "primary_type", "last_memory_at", "memory_count"},
				values: [][]driver.Value{
					{int64(51), "13800009011", "Compat User", "", "active", "svip", int64(8), base, int64(0)},
					{int64(52), "13800009012", "Memory User", "", "active", "vip", int64(5), base.Add(-time.Hour), int64(2)},
					{int64(53), "13800009013", "Card User", "", "active", "free", int64(3), base.Add(-2 * time.Hour), int64(0)},
					{int64(54), "13800009014", "Quiz User", "", "active", "free", int64(1), base.Add(-3 * time.Hour), int64(0)},
				},
			}, nil
		}
		return &appAnalyticsRows{
			columns: []string{"id", "phone", "nickname", "avatar", "status", "member_level", "primary_type", "last_memory_at", "memory_count"},
			values:  nil,
		}, nil
	}
	if strings.Contains(query, "MAX(m.update_time)") {
		if c.mode == "overview_memory_user" {
			now := time.Now()
			if strings.Contains(query, "primary_type") {
				return &appAnalyticsRows{
					columns: []string{"id", "phone", "nickname", "avatar", "status", "member_level", "primary_type", "last_memory_at", "memory_count"},
					values: [][]driver.Value{{
						int64(43),
						"13800009006",
						"Insight User",
						"",
						"active",
						"vip",
						int64(5),
						now,
						int64(2),
					}},
				}, nil
			}
			return &appAnalyticsRows{
				columns: []string{"id", "phone", "nickname", "avatar", "status", "member_level", "last_memory_at", "memory_count"},
				values: [][]driver.Value{{
					int64(43),
					"13800009006",
					"Insight User",
					"",
					"active",
					"vip",
					now,
					int64(2),
				}},
			}, nil
		}
		return &appAnalyticsRows{
			columns: []string{"id", "phone", "nickname", "avatar", "status", "member_level", "last_memory_at", "memory_count"},
			values:  nil,
		}, nil
	}
	for _, table := range []string{"app_quiz_submissions", "app_user_cards", "app_memories", "app_chat_sessions", "app_chat_messages", "app_compatibility_reports"} {
		if strings.Contains(query, table) && strings.Contains(query, "COUNT(*)") {
			return &appAnalyticsRows{columns: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
		}
	}
	if strings.Contains(query, "GROUP BY member_level") {
		return &appAnalyticsRows{columns: []string{"member_level", "count"}, values: nil}, nil
	}
	if strings.Contains(query, "GROUP BY status") {
		return &appAnalyticsRows{columns: []string{"status", "count"}, values: nil}, nil
	}
	if strings.Contains(query, "ORDER BY create_time DESC") {
		if len(args) > 0 {
			appAnalyticsRecentUsersLimit = int(args[0].Value.(int64))
		}
		return &appAnalyticsRows{
			columns: []string{"id", "phone", "nickname", "avatar", "status", "member_level", "create_time"},
			values:  nil,
		}, nil
	}
	if strings.Contains(query, "FROM app_users") {
		now := time.Now()
		return &appAnalyticsRows{
			columns: []string{"id", "phone", "account", "nickname", "avatar", "status", "member_level", "member_started_at", "member_expires_at", "register_source", "last_login_at", "create_time", "update_time"},
			values: [][]driver.Value{{
				int64(42),
				"13800009005",
				"",
				"Test User",
				"",
				"active",
				"free",
				nil,
				nil,
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
var appAnalyticsLastActiveWindowDays int
var appAnalyticsRecentUsersLimit int
var appAnalyticsRecentExtractedLimit int
var appAnalyticsLastQuery string
var appAnalyticsOverviewUserCountQuery string

func resetAppAnalyticsOverviewCaptures() {
	appAnalyticsLastActiveWindowDays = 0
	appAnalyticsRecentUsersLimit = 0
	appAnalyticsRecentExtractedLimit = 0
	appAnalyticsLastQuery = ""
	appAnalyticsOverviewUserCountQuery = ""
}

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
