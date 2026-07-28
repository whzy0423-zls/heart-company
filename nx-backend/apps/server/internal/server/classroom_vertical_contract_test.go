package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/config"
)

type classroomVerticalFixture struct {
	Series   json.RawMessage `json:"series"`
	Content  json.RawMessage `json:"content"`
	Playback struct {
		URL         string                `json:"url"`
		ExpiresIn   int                   `json:"expiresIn"`
		ContentType classroom.ContentType `json:"contentType"`
	} `json:"playback"`
}

func loadClassroomVerticalFixture(t *testing.T) classroomVerticalFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve classroom vertical fixture caller")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "docs", "superpowers", "fixtures", "classroom-public-response.json"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shared classroom fixture %s: %v", path, err)
	}
	var fixture classroomVerticalFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode shared classroom fixture: %v", err)
	}
	return fixture
}

type classroomVerticalState struct {
	mu            sync.Mutex
	seriesStatus  classroom.SeriesStatus
	contentStatus classroom.ContentStatus
	seriesAt      time.Time
	contentAt     time.Time
}

type classroomVerticalDriver struct{ state *classroomVerticalState }

func (d classroomVerticalDriver) Open(string) (driver.Conn, error) {
	return &classroomVerticalConn{state: d.state}, nil
}

type classroomVerticalConn struct{ state *classroomVerticalState }

func (*classroomVerticalConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*classroomVerticalConn) Close() error                        { return nil }
func (*classroomVerticalConn) Begin() (driver.Tx, error)           { return classroomVerticalTx{}, nil }
func (c *classroomVerticalConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	seriesValues := func() []driver.Value {
		return []driver.Value{
			int64(12), "九型人格入门", "建立基础地图", "https://cdn.example/covers/series-12.jpg", nil,
			"teacher-han", "韩老师", int64(1), string(c.state.seriesStatus), false, "paid", int64(2990),
			c.state.seriesAt, int64(7), int64(7), c.state.seriesAt.Add(-time.Hour), c.state.seriesAt,
		}
	}
	contentValues := func() []driver.Value {
		return []driver.Value{
			int64(21), int64(12), false, "第一课：认识三中心", "视频课程", "video", int64(31),
			"https://cdn.example/covers/content-21.jpg", "", "16:9", int64(1200), "teacher-han", "韩老师", nil,
			"课程回放", `["九型入门"]`, int64(1), int64(1), string(c.state.contentStatus), false, "inherit", int64(0),
			c.state.contentAt, int64(7), int64(7), c.state.contentAt.Add(-time.Hour), c.state.contentAt,
		}
	}
	mediaValues := []driver.Value{
		int64(31), "private", "classroom/private/content-21.mp4", "media-v1", "sha256:fixture", "video",
		int64(4096), int64(1200), int64(1920), int64(1080), "classroom/covers/content-21.jpg", "ready", int64(7),
		c.state.contentAt.Add(-time.Hour), c.state.contentAt,
	}

	switch {
	case strings.Contains(query, "UPDATE classroom_series"):
		c.state.seriesStatus = classroom.SeriesStatus(fmt.Sprint(args[7].Value))
		c.state.seriesAt = c.state.seriesAt.Add(time.Second)
		return &classroomRows{cols: []string{"created_at", "updated_at"}, values: [][]driver.Value{{c.state.seriesAt.Add(-time.Hour), c.state.seriesAt}}}, nil
	case strings.Contains(query, "UPDATE classroom_contents"):
		c.state.contentStatus = classroom.ContentStatus(fmt.Sprint(args[17].Value))
		c.state.contentAt = c.state.contentAt.Add(time.Second)
		return &classroomRows{cols: []string{"created_at", "updated_at"}, values: [][]driver.Value{{c.state.contentAt.Add(-time.Hour), c.state.contentAt}}}, nil
	case strings.Contains(query, "FROM classroom_series WHERE id=$1"):
		return &classroomRows{cols: make([]string, 17), values: [][]driver.Value{seriesValues()}}, nil
	case strings.Contains(query, "FROM classroom_contents WHERE id=$1"):
		return &classroomRows{cols: make([]string, 27), values: [][]driver.Value{contentValues()}}, nil
	case strings.Contains(query, "SELECT c.id,m.cover_object_key"):
		return &classroomRows{cols: make([]string, 5), values: [][]driver.Value{{int64(21), "classroom/covers/content-21.jpg", int64(12), "paid", int64(2990)}}}, nil
	case strings.Contains(query, "FROM classroom_media_assets WHERE id=$1"):
		return &classroomRows{cols: make([]string, 15), values: [][]driver.Value{mediaValues}}, nil
	case strings.Contains(query, "SELECT count(*) FROM classroom_series s"):
		count := int64(0)
		if c.state.seriesStatus == classroom.SeriesPublished && c.state.contentStatus == classroom.ContentPublished {
			count = 1
		}
		return &classroomRows{cols: []string{"count"}, values: [][]driver.Value{{count}}}, nil
	case strings.Contains(query, "SELECT s.id,s.title,s.summary"):
		return &classroomRows{cols: make([]string, 8), values: [][]driver.Value{{
			int64(12), "九型人格入门", "建立基础地图", "https://cdn.example/covers/series-12.jpg", "韩老师", "paid", int64(2990), false,
		}}}, nil
	case strings.Contains(query, "WHERE c.id=$1 AND c.status=$2 AND m.storage_status=$3"):
		return &classroomRows{cols: make([]string, 23), values: [][]driver.Value{{
			int64(21), int64(12), false, "第一课：认识三中心", "视频课程", "video", int64(31), "https://cdn.example/covers/content-21.jpg", int64(1200), "韩老师", "inherit", int64(0), false, string(c.state.contentStatus), "", "16:9", "classroom/covers/content-21.jpg", "media-v1",
			int64(12), string(c.state.seriesStatus), "paid", int64(2990), false,
		}}}, nil
	case strings.Contains(query, "WHERE c.id=$1 AND m.storage_status=$2"):
		return &classroomRows{cols: make([]string, 22), values: [][]driver.Value{{
			int64(21), int64(12), false, "第一课：认识三中心", "video", string(c.state.contentStatus), false, "inherit", int64(0), int64(31),
			int64(31), "private", "classroom/private/content-21.mp4", "media-v1", "video", "ready", int64(1200),
			int64(12), string(c.state.seriesStatus), false, "paid", int64(2990),
		}}}, nil
	case strings.Contains(query, "SELECT member_level,member_expires_at"):
		return &classroomRows{cols: []string{"member_level", "member_expires_at"}, values: [][]driver.Value{{int64(0), nil}}}, nil
	case strings.Contains(query, "SELECT series_id,content_id FROM classroom_entitlements"):
		return &classroomRows{cols: []string{"series_id", "content_id"}, values: [][]driver.Value{{int64(12), nil}}}, nil
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

type classroomContractSigner struct {
	t            *testing.T
	expectedKey  string
	expectedTTL  time.Duration
	fixtureURL   string
	callObserved bool
}

func (s *classroomContractSigner) PresignGetURL(_ context.Context, key string, ttl time.Duration) (string, error) {
	s.t.Helper()
	if key == "classroom/covers/content-21.jpg" && ttl == 30*time.Minute {
		return "https://cdn.example/covers/content-21.jpg", nil
	}
	if key != s.expectedKey || ttl != s.expectedTTL {
		s.t.Fatalf("playback signing key=%q ttl=%s", key, ttl)
	}
	s.callObserved = true
	return s.fixtureURL, nil
}

type classroomContractEnvelope struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Error   any             `json:"error"`
	Message string          `json:"message"`
}

func contractData(t *testing.T, response *httptest.ResponseRecorder) json.RawMessage {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("contract response=%d %s", response.Code, response.Body.String())
	}
	var envelope classroomContractEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response envelope: %v", err)
	}
	if envelope.Code != 0 || envelope.Message != "ok" || envelope.Error != nil {
		t.Fatalf("unexpected response envelope: %+v", envelope)
	}
	return envelope.Data
}

func assertExactJSON(t *testing.T, name string, actual, expected json.RawMessage) {
	t.Helper()
	var got, want any
	if err := json.Unmarshal(actual, &got); err != nil {
		t.Fatalf("decode %s actual: %v", name, err)
	}
	if err := json.Unmarshal(expected, &want); err != nil {
		t.Fatalf("decode %s fixture: %v", name, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s DTO mismatch\n got: %s\nwant: %s", name, actual, expected)
	}
}

func assertJSONKeys(t *testing.T, name string, raw json.RawMessage, expected ...string) {
	t.Helper()
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s fields: %v", name, err)
	}
	got := make([]string, 0, len(value))
	for key := range value {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(expected)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("%s fields=%v want=%v", name, got, expected)
	}
}

func TestClassroomVerticalContractAdminPublishToPublicPlayback(t *testing.T) {
	fixture := loadClassroomVerticalFixture(t)
	updatedAt := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	state := &classroomVerticalState{seriesStatus: classroom.SeriesDraft, contentStatus: classroom.ContentReady, seriesAt: updatedAt, contentAt: updatedAt}
	db := openClassroomVerticalDB(t, state)
	signer := &classroomContractSigner{t: t, expectedKey: "classroom/private/content-21.mp4", expectedTTL: 5 * time.Minute, fixtureURL: fixture.Playback.URL}
	s := &Server{
		classroomAdmin:          newClassroomAdminStore(db),
		classroomPublic:         newClassroomPublicDBWithCovers(db, signer, 30*time.Minute),
		classroomPlaybackSigner: signer,
		env:                     config.Env{JWTSecret: "vertical-secret"},
		mux:                     http.NewServeMux(),
	}
	registerClassroomAdminRoutes(s.mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	registerClassroomPublicRoutes(s.mux, s)

	publish := func(path string) {
		t.Helper()
		response := httptest.NewRecorder()
		req := classroomUser(httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"expectedUpdatedAt":"2026-07-27T10:00:00Z","reason":"contract publish"}`)))
		s.mux.ServeHTTP(response, req)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"published"`) {
			t.Fatalf("publish %s response=%d %s", path, response.Code, response.Body.String())
		}
	}
	publish("/api/admin/classroom/series/12/publish")
	publish("/api/admin/classroom/contents/21/publish")

	seriesResponse := httptest.NewRecorder()
	s.mux.ServeHTTP(seriesResponse, httptest.NewRequest(http.MethodGet, "/api/public/classroom/series", nil))
	seriesData := contractData(t, seriesResponse)
	assertExactJSON(t, "public series list", seriesData, fixture.Series)
	assertJSONKeys(t, "public series page", seriesData, "items", "limit", "offset", "total")
	var seriesPage struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(seriesData, &seriesPage); err != nil || len(seriesPage.Items) != 1 {
		t.Fatalf("decode public series items: %v %s", err, seriesData)
	}
	assertJSONKeys(t, "public series item", seriesPage.Items[0], "canPlay", "coverUrl", "effectiveAccess", "id", "playbackBlocked", "priceCents", "purchaseState", "summary", "teacherName", "title")

	detailResponse := httptest.NewRecorder()
	s.mux.ServeHTTP(detailResponse, httptest.NewRequest(http.MethodGet, "/api/public/classroom/content/21", nil))
	detailData := contractData(t, detailResponse)
	var expectedContent map[string]any
	if err := json.Unmarshal(fixture.Content, &expectedContent); err != nil {
		t.Fatal(err)
	}
	expectedContent["coverAspectRatio"] = "16:9"
	expectedContentRaw, _ := json.Marshal(expectedContent)
	assertExactJSON(t, "public content detail", detailData, expectedContentRaw)
	assertJSONKeys(t, "public content detail", detailData, "accessLevel", "canPlay", "contentType", "coverAspectRatio", "coverUrl", "description", "durationSeconds", "effectiveAccess", "id", "playbackBlocked", "priceCents", "purchaseState", "seriesId", "teacherName", "title")

	denied := httptest.NewRecorder()
	s.mux.ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/21/play", nil))
	if denied.Code != http.StatusNotFound {
		t.Fatalf("anonymous paid playback response=%d %s", denied.Code, denied.Body.String())
	}

	token, err := auth.Sign(auth.UserInfo{ID: 42, Roles: []string{miniappRole}, TokenKind: auth.TokenKindMiniapp}, "vertical-secret")
	if err != nil {
		t.Fatal(err)
	}
	playRequest := httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/21/play", nil)
	playRequest.Header.Set("Authorization", "Bearer "+token)
	play := httptest.NewRecorder()
	s.mux.ServeHTTP(play, playRequest)
	playData := contractData(t, play)
	expectedPlayback, err := json.Marshal(fixture.Playback)
	if err != nil {
		t.Fatal(err)
	}
	assertExactJSON(t, "playback", playData, expectedPlayback)
	assertJSONKeys(t, "playback", playData, "contentType", "expiresIn", "url")
	if !signer.callObserved || fixture.Playback.ExpiresIn != 300 || !strings.Contains(fixture.Playback.URL, "/temporary/") || strings.Contains(fixture.Playback.URL, signer.expectedKey) {
		t.Fatalf("playback URL is not a short-lived signed fixture: %+v", fixture.Playback)
	}
}
