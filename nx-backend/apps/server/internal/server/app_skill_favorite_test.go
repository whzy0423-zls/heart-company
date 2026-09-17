package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/skillchat"
)

func TestAppSkillMessagesRouterTogglesFavorite(t *testing.T) {
	registerAppSkillFavoriteDriver.Do(func() { sql.Register("app_skill_favorite_test", appSkillFavoriteDriver{}) })
	database, err := sql.Open("app_skill_favorite_test", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	server := &Server{skillChat: skillchat.NewStore(database)}
	request := httptest.NewRequest(http.MethodPost, "/api/app/skill-messages/31/favorite", nil)
	request = request.WithContext(context.WithValue(request.Context(), appContextKey{}, auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	server.appSkillMessagesRouter(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"favorite":true`) {
		t.Fatalf("body=%s", response.Body.String())
	}
}

var registerAppSkillFavoriteDriver sync.Once

type appSkillFavoriteDriver struct{}

func (appSkillFavoriteDriver) Open(string) (driver.Conn, error) { return appSkillFavoriteConn{}, nil }

type appSkillFavoriteConn struct{}

func (appSkillFavoriteConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (appSkillFavoriteConn) Close() error                        { return nil }
func (appSkillFavoriteConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (appSkillFavoriteConn) QueryContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	return &appSkillFavoriteRows{}, nil
}

type appSkillFavoriteRows struct{ sent bool }

func (r *appSkillFavoriteRows) Columns() []string { return []string{"favorite"} }
func (r *appSkillFavoriteRows) Close() error      { return nil }
func (r *appSkillFavoriteRows) Next(dest []driver.Value) error {
	if r.sent {
		return io.EOF
	}
	r.sent = true
	dest[0] = true
	return nil
}

var _ driver.QueryerContext = appSkillFavoriteConn{}
