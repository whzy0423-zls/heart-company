package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/config"
)

type classroomVerticalState struct {
	mu        sync.Mutex
	status    classroom.ContentStatus
	published *time.Time
	updatedAt time.Time
}

type classroomVerticalDriver struct{ state *classroomVerticalState }

func (d classroomVerticalDriver) Open(string) (driver.Conn, error) {
	return &classroomVerticalConn{state: d.state}, nil
}

type classroomVerticalConn struct{ state *classroomVerticalState }

func (*classroomVerticalConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*classroomVerticalConn) Close() error                        { return nil }
func (c *classroomVerticalConn) Begin() (driver.Tx, error) {
	return classroomVerticalTx{}, nil
}
func (c *classroomVerticalConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	contentValues := func() []driver.Value {
		mediaID := int64(31)
		return []driver.Value{
			int64(21), nil, false, "第一课：认识三中心", "视频课程", "video", mediaID,
			"https://cdn.example/covers/content-21.jpg", int64(1200), "teacher-han", "韩老师", nil,
			"课程回放", `["九型入门"]`, int64(1), int64(1), string(c.state.status), false, "login", int64(0),
			c.state.published, int64(7), int64(7), c.state.updatedAt.Add(-time.Hour), c.state.updatedAt,
		}
	}
	mediaValues := []driver.Value{
		int64(31), "private", "classroom/private/content-21.mp4", "media-v1", "sha256:fixture", "video",
		int64(4096), int64(1200), int64(1920), int64(1080), "classroom/covers/content-21.jpg", "ready", int64(7),
		c.state.updatedAt.Add(-time.Hour), c.state.updatedAt,
	}

	switch {
	case strings.Contains(query, "UPDATE classroom_contents"):
		if len(args) != 23 || args[21].Value != int64(21) {
			return nil, fmt.Errorf("unexpected classroom update args: %+v", args)
		}
		c.state.status = classroom.ContentStatus(fmt.Sprint(args[15].Value))
		if value, ok := args[19].Value.(time.Time); ok {
			c.state.published = &value
		}
		c.state.updatedAt = c.state.updatedAt.Add(time.Second)
		return &classroomRows{cols: []string{"created_at", "updated_at"}, values: [][]driver.Value{{c.state.updatedAt.Add(-time.Hour), c.state.updatedAt}}}, nil
	case strings.Contains(query, "FROM classroom_contents WHERE id=$1"):
		return &classroomRows{cols: make([]string, 25), values: [][]driver.Value{contentValues()}}, nil
	case strings.Contains(query, "FROM classroom_media_assets WHERE id=$1"):
		return &classroomRows{cols: make([]string, 15), values: [][]driver.Value{mediaValues}}, nil
	case strings.Contains(query, "SELECT count(*) FROM classroom_contents c JOIN classroom_media_assets"):
		count := int64(0)
		if c.state.status == classroom.ContentPublished {
			count = 1
		}
		return &classroomRows{cols: []string{"count"}, values: [][]driver.Value{{count}}}, nil
	case strings.Contains(query, "SELECT c.id,c.series_id,c.show_as_standalone,c.title") && strings.Contains(query, "ORDER BY c.sort_order,c.id"):
		if c.state.status != classroom.ContentPublished {
			return &classroomRows{cols: make([]string, 18)}, nil
		}
		return &classroomRows{cols: make([]string, 18), values: [][]driver.Value{{
			int64(21), nil, false, "第一课：认识三中心", "视频课程", "video", int64(31),
			"https://cdn.example/covers/content-21.jpg", int64(1200), "韩老师", "login", int64(0), false,
			nil, nil, nil, nil, nil,
		}}}, nil
	case strings.Contains(query, "WHERE c.id=$1 AND m.storage_status=$2"):
		return &classroomRows{cols: make([]string, 22), values: [][]driver.Value{{
			int64(21), nil, false, "第一课：认识三中心", "video", string(c.state.status), false, "login", int64(0), int64(31),
			int64(31), "private", "classroom/private/content-21.mp4", "media-v1", "video", "ready", int64(1200),
			nil, nil, nil, nil, nil,
		}}}, nil
	case strings.Contains(query, "SELECT member_level,member_expires_at"):
		return &classroomRows{cols: []string{"member_level", "member_expires_at"}, values: [][]driver.Value{{int64(0), nil}}}, nil
	case strings.Contains(query, "SELECT series_id,content_id FROM classroom_entitlements"):
		return &classroomRows{cols: []string{"series_id", "content_id"}}, nil
	default:
		return nil, fmt.Errorf("unexpected classroom vertical query: %s", query)
	}
}

type classroomVerticalTx struct{}

func (classroomVerticalTx) Commit() error   { return nil }
func (classroomVerticalTx) Rollback() error { return nil }

func openClassroomVerticalDB(t *testing.T, state *classroomVerticalState) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("classroom-vertical-%d", classroomDriverSequence.Add(1))
	sql.Register(name, classroomVerticalDriver{state: state})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestClassroomVerticalContractAdminPublishToPublicPlayback(t *testing.T) {
	updatedAt := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	state := &classroomVerticalState{status: classroom.ContentReady, updatedAt: updatedAt}
	db := openClassroomVerticalDB(t, state)
	s := &Server{
		classroomAdmin:          newClassroomAdminStore(db),
		classroomPublic:         newClassroomPublicDB(db),
		classroomPlaybackSigner: fakeClassroomSigner{key: "vertical-contract"},
		env:                     config.Env{JWTSecret: "vertical-secret"},
		mux:                     http.NewServeMux(),
	}
	registerClassroomAdminRoutes(s.mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	registerClassroomPublicRoutes(s.mux, s)

	publish := httptest.NewRecorder()
	publishRequest := classroomUser(httptest.NewRequest(
		http.MethodPost,
		"/api/admin/classroom/contents/21/publish",
		strings.NewReader(`{"expectedUpdatedAt":"2026-07-27T10:00:00Z","reason":"contract publish"}`),
	))
	s.mux.ServeHTTP(publish, publishRequest)
	if publish.Code != http.StatusOK || !strings.Contains(publish.Body.String(), `"status":"published"`) {
		t.Fatalf("admin publish response=%d %s", publish.Code, publish.Body.String())
	}

	assertSafeMetadata := func(name, path string) {
		t.Helper()
		response := httptest.NewRecorder()
		s.mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		body := response.Body.String()
		if response.Code != http.StatusOK || !strings.Contains(body, "第一课：认识三中心") {
			t.Fatalf("%s response=%d %s", name, response.Code, body)
		}
		for _, secret := range []string{"classroom/private/content-21.mp4", "objectKey", "mediaUrl", "media-v1"} {
			if strings.Contains(body, secret) {
				t.Fatalf("%s leaked permanent media metadata %q: %s", name, secret, body)
			}
		}
	}
	assertSafeMetadata("public list", "/api/public/classroom/standalone")
	assertSafeMetadata("public detail", "/api/public/classroom/content/21")

	denied := httptest.NewRecorder()
	s.mux.ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/21/play", nil))
	if denied.Code != http.StatusNotFound {
		t.Fatalf("anonymous login-only playback response=%d %s", denied.Code, denied.Body.String())
	}

	token, err := auth.Sign(auth.UserInfo{ID: 42, Roles: []string{miniappRole}, TokenKind: auth.TokenKindMiniapp}, "vertical-secret")
	if err != nil {
		t.Fatal(err)
	}
	playRequest := httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/21/play", nil)
	playRequest.Header.Set("Authorization", "Bearer "+token)
	play := httptest.NewRecorder()
	s.mux.ServeHTTP(play, playRequest)
	if play.Code != http.StatusOK || !strings.Contains(play.Body.String(), "vertical-contract") || strings.Contains(play.Body.String(), "classroom/private/content-21.mp4") {
		t.Fatalf("authenticated playback response=%d %s", play.Code, play.Body.String())
	}
}
