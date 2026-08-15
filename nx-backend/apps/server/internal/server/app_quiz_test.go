package server

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/quiz"
)

func TestAppQuizSubmissionReturnsOKWhenNoSubmission(t *testing.T) {
	s := newAppQuizTestServer(t, "not_found")
	response := performAppQuizRequest(t, s.appQuizSubmission, http.MethodGet, "/api/app/quiz/submission", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 when submission is missing, got %d body=%s", response.Code, response.Body.String())
	}
	body := decodeAppQuizResponse(t, response)
	if body.Code != 0 || body.Data != nil {
		t.Fatalf("expected nil success response, got %+v", body)
	}
}

func TestAppCardPrimaryReturnsOKWhenNoPrimaryCard(t *testing.T) {
	s := newAppQuizTestServer(t, "not_found")
	response := performAppQuizRequest(t, s.appCardPrimary, http.MethodGet, "/api/app/cards/primary", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 when primary card is missing, got %d body=%s", response.Code, response.Body.String())
	}
	body := decodeAppQuizResponse(t, response)
	if body.Code != 0 || body.Data != nil {
		t.Fatalf("expected nil success response, got %+v", body)
	}
}

func TestAppCardsCreateReturnsBadRequestWhenSecondaryCardLimitReached(t *testing.T) {
	s := newAppQuizTestServer(t, "card_limit")
	response := performAppQuizRequest(t, s.appCards, http.MethodPost, "/api/app/cards", map[string]any{
		"name":     "Friend",
		"relation": "friend",
		"mainType": 1,
		"wingType": 2,
	})

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when secondary card limit is reached, got %d body=%s", response.Code, response.Body.String())
	}
	body := decodeAppQuizResponse(t, response)
	if body.Code != -1 || body.Message != quiz.ErrCardLimit.Error() {
		t.Fatalf("expected ErrCardLimit response, got %+v", body)
	}
}

func TestAppCardTrendRejectsNonGETMethods(t *testing.T) {
	s := newAppQuizTestServer(t, "trend")
	response := performAppQuizRequest(t, s.appCardByID, http.MethodPost, "/api/app/cards/1/trend", nil)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for non-GET trend request, got %d body=%s", response.Code, response.Body.String())
	}
}

func newAppQuizTestServer(t *testing.T, mode string) *Server {
	t.Helper()
	registerAppQuizTestDriver()
	db, err := sql.Open(appQuizTestDriverName, mode)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Server{
		db:       db,
		appUsers: appuser.NewStore(db),
		quiz:     quiz.NewStore(db),
	}
}

func performAppQuizRequest(t *testing.T, handler http.HandlerFunc, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &body)
	request = request.WithContext(contextWithAppUser(request.Context(), auth.UserInfo{ID: 7, Phone: "13800000000"}))
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func decodeAppQuizResponse(t *testing.T, response *httptest.ResponseRecorder) struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
} {
	t.Helper()
	var body struct {
		Code    int    `json:"code"`
		Data    any    `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

const appQuizTestDriverName = "app_quiz_test"

var registerAppQuizTestDriverOnce sync.Once

func registerAppQuizTestDriver() {
	registerAppQuizTestDriverOnce.Do(func() {
		sql.Register(appQuizTestDriverName, appQuizTestDriver{})
	})
}

type appQuizTestDriver struct{}

func (appQuizTestDriver) Open(name string) (driver.Conn, error) {
	return &appQuizTestConn{mode: name}, nil
}

type appQuizTestConn struct {
	mode string
}

func (c *appQuizTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *appQuizTestConn) Close() error                        { return nil }
func (c *appQuizTestConn) Begin() (driver.Tx, error)           { return appQuizTestTx{}, nil }

func (c *appQuizTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return appQuizTestTx{}, nil
}

func (c *appQuizTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	switch {
	case strings.Contains(query, "FROM app_quiz_submissions"):
		return &appQuizTestRows{columns: []string{
			"id", "app_user_id", "answers", "result", "primary_type", "second_type", "wing_type",
			"gender", "quiz_version", "score", "adjusted_score", "centers", "create_time",
		}}, nil
	case strings.Contains(query, "FROM app_user_cards") && strings.Contains(query, "card_type='primary'"):
		return &appQuizTestRows{columns: []string{
			"id", "app_user_id", "card_type", "name", "relation", "enneagram", "wing", "profile", "status", "create_time", "update_time",
		}}, nil
	case strings.Contains(query, "SELECT app_user_id FROM app_user_cards"):
		return &appQuizTestRows{
			columns: []string{"app_user_id"},
			values:  [][]driver.Value{{int64(7)}},
		}, nil
	case strings.Contains(query, "FROM app_chat_sessions"):
		return &appQuizTestRows{columns: []string{"id", "create_time"}}, nil
	case strings.Contains(query, "FROM app_users WHERE id"):
		return &appQuizTestRows{
			columns: []string{"id", "phone", "account", "nickname", "avatar", "status", "member_level", "member_started_at", "member_expires_at", "register_source", "last_login_at", "create_time", "update_time"},
			values:  [][]driver.Value{{int64(7), "13800000000", "", "Test User", "", "active", "free", nil, nil, "sms", nil, now, now}},
		}, nil
	case strings.Contains(query, "SELECT count(*) FROM app_user_cards"):
		return &appQuizTestRows{
			columns: []string{"count"},
			values:  [][]driver.Value{{int64(1)}},
		}, nil
	default:
		return nil, driver.ErrSkip
	}
}

type appQuizTestTx struct{}

func (appQuizTestTx) Commit() error   { return nil }
func (appQuizTestTx) Rollback() error { return nil }

type appQuizTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *appQuizTestRows) Columns() []string {
	return r.columns
}

func (r *appQuizTestRows) Close() error {
	return nil
}

func (r *appQuizTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
