package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/teacher"
)

func TestAppProfileUpdateReturnsEnrichedRoles(t *testing.T) {
	registerAppProfileTestDriver()
	database, err := sql.Open(appProfileTestDriverName, "profile-update")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	server := &Server{
		appUsers: appuser.NewStore(database),
		teachers: teacher.NewStore(database),
	}
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/app/profile",
		strings.NewReader(`{"avatar":"/api/app/profile/avatar/42"}`),
	)
	request = request.WithContext(context.WithValue(
		request.Context(),
		appContextKey{},
		auth.UserInfo{ID: 7},
	))
	response := httptest.NewRecorder()

	server.appProfileUpdate(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("profile update status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			ID          int64    `json:"id"`
			Phone       string   `json:"phone"`
			Nickname    string   `json:"nickname"`
			Avatar      string   `json:"avatar"`
			MemberLevel string   `json:"memberLevel"`
			Roles       []string `json:"roles"`
			TeacherKey  string   `json:"teacherKey"`
			AgentID     int64    `json:"agentId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Avatar != "/api/app/profile/avatar/42" {
		t.Fatalf("avatar=%q", body.Data.Avatar)
	}
	if body.Data.ID != 7 || body.Data.Phone != "18800001234" || body.Data.Nickname != "木木" {
		t.Fatalf("unexpected identity: %+v", body.Data)
	}
	if body.Data.MemberLevel != "year" {
		t.Fatalf("memberLevel=%q", body.Data.MemberLevel)
	}
	if got := strings.Join(body.Data.Roles, ","); got != "teacher,agent" {
		t.Fatalf("roles=%q", got)
	}
	if body.Data.TeacherKey != "teacher-mumu" || body.Data.AgentID != 88 {
		t.Fatalf("teacherKey=%q agentId=%d", body.Data.TeacherKey, body.Data.AgentID)
	}
}

func TestAppProfileUpdateReturnsExplicitEmptyRoles(t *testing.T) {
	registerAppProfileTestDriver()
	database, err := sql.Open(appProfileTestDriverName, "profile-update-no-roles")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	server := &Server{
		appUsers: appuser.NewStore(database),
		teachers: teacher.NewStore(database),
	}
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/app/profile",
		strings.NewReader(`{"nickname":"木木"}`),
	)
	request = request.WithContext(context.WithValue(
		request.Context(),
		appContextKey{},
		auth.UserInfo{ID: 7},
	))
	response := httptest.NewRecorder()

	server.appProfileUpdate(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("profile update status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	roles, ok := body.Data["roles"].([]any)
	if !ok || len(roles) != 0 {
		t.Fatalf("roles=%#v", body.Data["roles"])
	}
}

func TestAppProfileUpdateFailsWhenRolesCannotBeLoaded(t *testing.T) {
	registerAppProfileTestDriver()
	database, err := sql.Open(appProfileTestDriverName, "profile-update-role-error")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	server := &Server{
		appUsers: appuser.NewStore(database),
		teachers: teacher.NewStore(database),
	}
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/app/profile",
		strings.NewReader(`{"avatar":"/api/app/profile/avatar/42"}`),
	)
	request = request.WithContext(context.WithValue(
		request.Context(),
		appContextKey{},
		auth.UserInfo{ID: 7},
	))
	response := httptest.NewRecorder()

	server.appProfileUpdate(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("profile update status=%d body=%s", response.Code, response.Body.String())
	}
}

const appProfileTestDriverName = "app_profile_test"

var registerAppProfileDriverOnce sync.Once

func registerAppProfileTestDriver() {
	registerAppProfileDriverOnce.Do(func() {
		sql.Register(appProfileTestDriverName, appProfileTestDriver{})
	})
}

type appProfileTestDriver struct{}

func (appProfileTestDriver) Open(name string) (driver.Conn, error) {
	return appProfileTestConn{
		failRoles: name == "profile-update-role-error",
		noRoles:   name == "profile-update-no-roles",
	}, nil
}

type appProfileTestConn struct {
	failRoles bool
	noRoles   bool
}

func (appProfileTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (appProfileTestConn) Close() error                        { return nil }
func (appProfileTestConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (appProfileTestConn) CheckNamedValue(*driver.NamedValue) error {
	return nil
}

func (c appProfileTestConn) QueryContext(
	_ context.Context,
	query string,
	_ []driver.NamedValue,
) (driver.Rows, error) {
	normalized := strings.Join(strings.Fields(query), " ")
	switch {
	case strings.Contains(normalized, "UPDATE app_users"):
		now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
		return &appProfileTestRows{
			columns: []string{
				"id", "phone", "account", "nickname", "avatar", "status",
				"member_level", "member_started_at", "member_expires_at",
				"register_source", "last_login_at", "create_time", "update_time",
			},
			rows: [][]driver.Value{{
				int64(7), "18800001234", "mumu", "木木",
				"/api/app/profile/avatar/42", "active", "year", nil, nil,
				"app", nil, now, now,
			}},
		}, nil
	case strings.Contains(normalized, "FROM app_user_roles"):
		if c.failRoles {
			return nil, errors.New("role lookup failed")
		}
		if c.noRoles {
			return &appProfileTestRows{
				columns: []string{"role", "teacher_key"},
			}, nil
		}
		return &appProfileTestRows{
			columns: []string{"role", "teacher_key"},
			rows:    [][]driver.Value{{"teacher", "teacher-mumu"}},
		}, nil
	case strings.Contains(normalized, "FROM distribution_agents"):
		if c.noRoles {
			return &appProfileTestRows{columns: []string{"id"}}, nil
		}
		return &appProfileTestRows{
			columns: []string{"id"},
			rows:    [][]driver.Value{{int64(88)}},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected profile test query: %s", normalized)
	}
}

type appProfileTestRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *appProfileTestRows) Columns() []string { return r.columns }
func (r *appProfileTestRows) Close() error      { return nil }
func (r *appProfileTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}

var _ driver.QueryerContext = appProfileTestConn{}
var _ driver.NamedValueChecker = appProfileTestConn{}
