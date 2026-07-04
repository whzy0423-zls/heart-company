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

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/quiz"
)

func TestAppCompatibilityCreateReturnsReportWithCompatibleFieldNames(t *testing.T) {
	s := newAppCompatibilityTestServer(t, "compatibility")
	response := performAppCompatibilityRequest(t, s.appCompatibilityRouter, http.MethodPost, "/api/app/compatibility", map[string]any{
		"cardAId": 1,
		"cardBId": 2,
	})

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 creating compatibility report, got %d body=%s", response.Code, response.Body.String())
	}
	body := decodeAppCompatibilityResponse(t, response)
	if body.Code != 0 {
		t.Fatalf("expected ok response, got %+v", body)
	}
	data, ok := body.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected object data, got %T", body.Data)
	}
	if data["cardAName"] != "本人" || data["card_a_name"] != "本人" {
		t.Fatalf("expected camel and snake card A names, got %+v", data)
	}
	if data["cardBName"] != "朋友" || data["card_b_name"] != "朋友" {
		t.Fatalf("expected camel and snake card B names, got %+v", data)
	}
	if data["title"] == "" || data["dynamics"] == "" || data["strengths"] == "" || data["advice"] == "" {
		t.Fatalf("expected app detail text fields, got %+v", data)
	}
	if data["createdAt"] == "" || data["created_at"] == "" {
		t.Fatalf("expected createdAt aliases, got %+v", data)
	}
	if data["algorithmVersion"] != "v1" || data["algorithm_version"] != "v1" {
		t.Fatalf("expected algorithm version aliases, got %+v", data)
	}
	if data["relationLevel"] == "" || data["relation_level"] == "" {
		t.Fatalf("expected relation level aliases, got %+v", data)
	}
	if scores, ok := data["scores"].(map[string]any); !ok || scores["stability"] == nil {
		t.Fatalf("expected scores with stability, got %+v", data["scores"])
	}
	if tags, ok := data["explainTags"].([]any); !ok || len(tags) == 0 {
		t.Fatalf("expected explainTags, got %+v", data["explainTags"])
	} else if !containsAny(tags, "cross_center_complement") {
		t.Fatalf("expected API to use compatibility engine tags, got %+v", tags)
	}
	if tags, ok := data["explain_tags"].([]any); !ok || len(tags) == 0 {
		t.Fatalf("expected explain_tags, got %+v", data["explain_tags"])
	}
	if evidence, ok := data["evidence"].([]any); !ok || len(evidence) == 0 {
		t.Fatalf("expected evidence, got %+v", data["evidence"])
	}
	if _, ok := data["conflictPoints"].([]any); !ok {
		t.Fatalf("expected camel conflictPoints array, got %+v", data["conflictPoints"])
	}
	if _, ok := data["conflict_points"].([]any); !ok {
		t.Fatalf("expected snake conflict_points array, got %+v", data["conflict_points"])
	}
	if data["isFull"] != true || data["is_full"] != true {
		t.Fatalf("expected camel and snake isFull flags, got %+v", data)
	}
}

func containsAny(items []any, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestAppCompatibilityCreateRejectsCardsOutsideCurrentUser(t *testing.T) {
	s := newAppCompatibilityTestServer(t, "compatibility_other_user")
	response := performAppCompatibilityRequest(t, s.appCompatibilityRouter, http.MethodPost, "/api/app/compatibility", map[string]any{
		"cardAId": 1,
		"cardBId": 99,
	})

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for card outside current user, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestAppCompatibilityListAndDetailScopeToCurrentUser(t *testing.T) {
	s := newAppCompatibilityTestServer(t, "compatibility")

	listResponse := performAppCompatibilityRequest(t, s.appCompatibilityRouter, http.MethodGet, "/api/app/compatibility", nil)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected 200 listing compatibility reports, got %d body=%s", listResponse.Code, listResponse.Body.String())
	}
	listBody := decodeAppCompatibilityResponse(t, listResponse)
	items, ok := listBody.Data.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one list item, got %+v", listBody.Data)
	}

	detailResponse := performAppCompatibilityRequest(t, s.appCompatibilityRouter, http.MethodGet, "/api/app/compatibility/11", nil)
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("expected 200 fetching compatibility detail, got %d body=%s", detailResponse.Code, detailResponse.Body.String())
	}
	detailBody := decodeAppCompatibilityResponse(t, detailResponse)
	detail, ok := detailBody.Data.(map[string]any)
	if !ok || detail["id"] != float64(11) || detail["cardAName"] != "本人" {
		t.Fatalf("expected scoped detail with compatibility aliases, got %+v", detailBody.Data)
	}
	if detail["algorithmVersion"] != "v1" || detail["relationLevel"] == "" {
		t.Fatalf("expected detail algorithm metadata, got %+v", detail)
	}
}

func newAppCompatibilityTestServer(t *testing.T, mode string) *Server {
	t.Helper()
	registerAppCompatibilityTestDriver()
	db, err := sql.Open(appCompatibilityTestDriverName, mode)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Server{
		db:   db,
		quiz: quiz.NewStore(db),
	}
}

func performAppCompatibilityRequest(t *testing.T, handler http.HandlerFunc, method, path string, payload any) *httptest.ResponseRecorder {
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

func decodeAppCompatibilityResponse(t *testing.T, response *httptest.ResponseRecorder) struct {
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

const appCompatibilityTestDriverName = "app_compatibility_test"

var registerAppCompatibilityTestDriverOnce sync.Once

func registerAppCompatibilityTestDriver() {
	registerAppCompatibilityTestDriverOnce.Do(func() {
		sql.Register(appCompatibilityTestDriverName, appCompatibilityTestDriver{})
	})
}

type appCompatibilityTestDriver struct{}

func (appCompatibilityTestDriver) Open(name string) (driver.Conn, error) {
	return &appCompatibilityTestConn{mode: name}, nil
}

type appCompatibilityTestConn struct {
	mode string
}

func (c *appCompatibilityTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *appCompatibilityTestConn) Close() error                        { return nil }
func (c *appCompatibilityTestConn) Begin() (driver.Tx, error)           { return appCompatibilityTestTx{}, nil }

func (c *appCompatibilityTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return appCompatibilityTestTx{}, nil
}

func (c *appCompatibilityTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	switch {
	case strings.Contains(query, "FROM app_user_cards") && strings.Contains(query, "WHERE id = $1"):
		cardID, _ := args[0].Value.(int64)
		if c.mode == "compatibility_other_user" && cardID == 99 {
			return appCompatibilityCardRows(nil), nil
		}
		switch cardID {
		case 1:
			return appCompatibilityCardRows([][]driver.Value{{int64(1), int64(7), "primary", "本人", "self", int64(1), int64(9), []byte(`{}`), "active", now, now}}), nil
		case 2:
			return appCompatibilityCardRows([][]driver.Value{{int64(2), int64(7), "secondary", "朋友", "friend", int64(5), int64(6), []byte(`{}`), "active", now, now}}), nil
		default:
			return appCompatibilityCardRows(nil), nil
		}
	case strings.Contains(query, "INSERT INTO app_compatibility_reports"):
		return &appCompatibilityTestRows{
			columns: []string{"id", "create_time", "update_time"},
			values:  [][]driver.Value{{int64(11), now, now}},
		}, nil
	case strings.Contains(query, "FROM app_compatibility_reports"):
		return appCompatibilityReportRows(now), nil
	default:
		return nil, driver.ErrSkip
	}
}

type appCompatibilityTestTx struct{}

func (appCompatibilityTestTx) Commit() error   { return nil }
func (appCompatibilityTestTx) Rollback() error { return nil }

func appCompatibilityCardRows(values [][]driver.Value) driver.Rows {
	return &appCompatibilityTestRows{
		columns: []string{
			"id", "app_user_id", "card_type", "name", "relation", "enneagram", "wing", "profile", "status", "create_time", "update_time",
		},
		values: values,
	}
}

func appCompatibilityReportRows(now time.Time) driver.Rows {
	return &appCompatibilityTestRows{
		columns: []string{
			"id", "app_user_id", "card_a_id", "card_b_id", "card_a_name", "card_b_name", "card_a_type", "card_b_type",
			"summary", "highlights", "conflict_points", "suggestions", "is_full",
			"algorithm_version", "relation_level", "scores", "explain_tags", "evidence",
			"create_time", "update_time",
		},
		values: [][]driver.Value{{
			int64(11), int64(7), int64(1), int64(2), "本人", "朋友", int64(1), int64(5),
			"本人与朋友的关系合盘显示：这段关系的关键在于看见彼此的节奏差异。",
			[]byte(`["彼此能互补"]`), []byte(`["节奏不同"]`), []byte(`["先确认期待"]`), true,
			"v1", "balanced", []byte(`{"stability":74,"resonance":72}`), []byte(`["cross_center"]`), []byte(`[{"code":"type_pair","title":"型号组合","detail":"基于双方主型计算"}]`),
			now, now,
		}},
	}
}

type appCompatibilityTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *appCompatibilityTestRows) Columns() []string {
	return r.columns
}

func (r *appCompatibilityTestRows) Close() error {
	return nil
}

func (r *appCompatibilityTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
