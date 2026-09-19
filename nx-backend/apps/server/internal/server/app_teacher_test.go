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
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/teacher"
	"reflect"
	"strings"
	"testing"
	"time"
)

type teacherVideoTestDriver struct {
	queries []string
}

type teacherVideoTestConn struct{ driver *teacherVideoTestDriver }

type teacherVideoTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (d *teacherVideoTestDriver) Open(string) (driver.Conn, error) {
	return &teacherVideoTestConn{driver: d}, nil
}
func (c *teacherVideoTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (c *teacherVideoTestConn) Close() error { return nil }
func (c *teacherVideoTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin is not supported")
}
func (c *teacherVideoTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.driver.queries = append(c.driver.queries, query)
	now := time.Date(2026, time.September, 18, 12, 0, 0, 0, time.UTC)
	if strings.Contains(query, "FROM teacher_profiles") {
		return &teacherVideoTestRows{
			columns: strings.Split("id,teacher_key,name,title,avatar,cover,short_intro,detail_intro,expertise,intro_video_url,customer_service,offline_service,show_on_home,show_in_drawer,sort_order,enabled,created_at,updated_at", ","),
			values:  [][]driver.Value{{int64(1), "han", "老韩", "导师", "", "", "", "", []byte(`[]`), "", []byte(`null`), []byte(`null`), false, false, int64(0), true, now, now}},
		}, nil
	}
	if strings.Contains(query, "FROM classroom_contents") {
		if !strings.Contains(query, "feed_type IN ('course','daily')") ||
			!strings.Contains(query, "review_status='published'") ||
			!strings.Contains(query, "status='published'") ||
			!strings.Contains(query, "ORDER BY sort_order,COALESCE(published_at,created_at) DESC,id DESC") {
			return nil, errors.New("teacher video query does not enforce visibility and ordering")
		}
		columns := strings.Split("id,series_id,show_as_standalone,title,description,teacher_key,feed_type,review_status,review_reason,replaces_content_id,published_at,created_at,updated_at", ",")
		return &teacherVideoTestRows{columns: columns, values: [][]driver.Value{
			{int64(2), nil, false, "日常二", "", "han", "daily", "published", "", nil, now, now, now},
			{int64(1), nil, false, "课程一", "", "han", "course", "published", "", nil, now.Add(-time.Hour), now.Add(-time.Hour), now},
		}}, nil
	}
	return nil, errors.New("unexpected query")
}
func (r *teacherVideoTestRows) Columns() []string { return r.columns }
func (r *teacherVideoTestRows) Close() error      { return nil }
func (r *teacherVideoTestRows) Next(dst []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dst, r.values[r.index])
	r.index++
	return nil
}

type teacherVideoPublicStub struct{ classroomPublicService }

func (teacherVideoPublicStub) GetContent(_ context.Context, id, _ int64) (classroomPublicContent, error) {
	return classroomPublicContent{ID: id, Title: map[int64]string{1: "课程一", 2: "日常二"}[id], CoverURL: "https://cdn.example/cover.jpg", ContentType: classroom.ContentVideo, DurationSeconds: 90}, nil
}

func TestTeacherVideosAggregatesVisibleCourseAndDailyUsingPublicDTO(t *testing.T) {
	drv := &teacherVideoTestDriver{}
	name := "teacher-video-test-" + strings.ReplaceAll(t.Name(), "/", "-")
	sql.Register(name, drv)
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := &Server{teachers: teacher.NewStore(db), classroomPublic: teacherVideoPublicStub{}}
	r := httptest.NewRequest(http.MethodGet, "/api/app/teachers/han/videos", nil)
	r = r.WithContext(context.WithValue(r.Context(), appContextKey{}, auth.UserInfo{ID: 9}))
	rr := httptest.NewRecorder()
	s.appTeacherRouter(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Items []classroomPublicContent `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	gotIDs := make([]int64, 0, len(body.Data.Items))
	for _, item := range body.Data.Items {
		gotIDs = append(gotIDs, item.ID)
	}
	if !reflect.DeepEqual(gotIDs, []int64{2, 1}) {
		t.Fatalf("video ids=%v body=%s", gotIDs, rr.Body.String())
	}
	if len(body.Data.Items) != 2 || body.Data.Items[0].CoverURL == "" || body.Data.Items[0].DurationSeconds != 90 {
		t.Fatalf("unified endpoint must use classroom public DTO: %#v", body.Data.Items)
	}
}

func TestTeacherRoutesAreLoginProtected(t *testing.T) {
	s := &Server{env: config.Env{JWTSecret: "test"}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/app/teachers", s.requireAppAuth(s.appTeacherCollection))
	req := httptest.NewRequest(http.MethodGet, "/api/app/teachers", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTeacherReviewReasonRequiredForRejection(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/teachers/content/12/review", strings.NewReader(`{"status":"rejected"}`))
	rr := httptest.NewRecorder()
	s.adminTeacherRouter(rr, req)
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestParseTeacherReviewPageDefaultsAndCaps(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/teacher-reviews?page=0&pageSize=999", nil)
	page, pageSize := parseTeacherReviewPage(r)
	if page != 1 || pageSize != 200 {
		t.Fatalf("page=%d pageSize=%d", page, pageSize)
	}
}

func TestTeacherReviewActionRejectsUnknownAction(t *testing.T) {
	s := &Server{teachers: teacher.NewStore(nil)}
	r := httptest.NewRequest(http.MethodPost, "/api/admin/teacher-reviews/12/archive", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.adminTeacherReviewAction(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFilterAdminTeachersSupportsKeywordAndEnabled(t *testing.T) {
	items := []teacher.Teacher{
		{Key: "laohan", Name: "老韩", Title: "导师", Enabled: true},
		{Key: "other", Name: "其他老师", Enabled: false},
	}
	r := httptest.NewRequest(http.MethodGet, "/api/admin/teachers?keyword=%E8%80%81&enabled=true", nil)
	got := filterAdminTeachers(items, r)
	if len(got) != 1 || got[0].Key != "laohan" {
		t.Fatalf("unexpected filtered teachers: %#v", got)
	}
}

func TestParseTeacherAdminPageDefaultsAndCaps(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/teachers?page=0&pageSize=999", nil)
	page, pageSize := parseTeacherAdminPage(r)
	if page != 1 || pageSize != 200 {
		t.Fatalf("page=%d pageSize=%d", page, pageSize)
	}
}
