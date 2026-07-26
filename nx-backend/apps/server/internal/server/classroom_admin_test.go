package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
)

type fakeClassroomAdminService struct {
	series  classroom.Series
	content classroom.Content
	calls   []string
}

func (f *fakeClassroomAdminService) ListSeries(context.Context, classroom.SeriesFilter) ([]classroom.Series, int, error) {
	f.calls = append(f.calls, "list-series")
	return []classroom.Series{f.series}, 1, nil
}
func (f *fakeClassroomAdminService) GetSeries(context.Context, int64) (classroom.Series, error) {
	f.calls = append(f.calls, "get-series")
	return f.series, nil
}
func (f *fakeClassroomAdminService) CreateSeries(context.Context, classroom.Series) (classroom.Series, error) {
	f.calls = append(f.calls, "create-series")
	return f.series, nil
}
func (f *fakeClassroomAdminService) UpdateSeries(_ context.Context, value classroom.Series, _ time.Time) (classroom.Series, error) {
	f.calls = append(f.calls, "update-series")
	return value, nil
}
func (f *fakeClassroomAdminService) ListContents(context.Context, classroom.ContentFilter) ([]classroom.Content, int, error) {
	f.calls = append(f.calls, "list-content")
	return []classroom.Content{f.content}, 1, nil
}
func (f *fakeClassroomAdminService) GetContent(context.Context, int64) (classroom.Content, error) {
	f.calls = append(f.calls, "get-content")
	return f.content, nil
}
func (f *fakeClassroomAdminService) CreateContent(context.Context, classroom.Content) (classroom.Content, error) {
	f.calls = append(f.calls, "create-content")
	return f.content, nil
}
func (f *fakeClassroomAdminService) UpdateContent(_ context.Context, value classroom.Content, _ time.Time) (classroom.Content, error) {
	f.calls = append(f.calls, "update-content")
	return value, nil
}
func (f *fakeClassroomAdminService) ListUploadTasks(context.Context, int, int) ([]classroom.UploadTask, int, error) {
	f.calls = append(f.calls, "list-tasks")
	return nil, 0, nil
}

func classroomUser(r *http.Request) *http.Request {
	return r.WithContext(withUser(r.Context(), auth.UserInfo{ID: 42, RealName: "老师"}))
}

func TestClassroomAdminRoutesUseDedicatedPermissions(t *testing.T) {
	f := &fakeClassroomAdminService{series: classroom.Series{ID: 1, Title: "S", Status: classroom.SeriesDraft, AccessLevel: classroom.AccessPublic}}
	s := &Server{classroomAdmin: f}
	mux := http.NewServeMux()
	var codes []string
	permission := func(code string, next http.HandlerFunc) http.HandlerFunc { codes = append(codes, code); return next }
	registerClassroomAdminRoutes(mux, permission, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodGet, "/api/admin/classroom/series?page=2&pageSize=10&status=draft", nil)))
	if rr.Code != http.StatusOK || len(codes) == 0 || !containsString(codes, "Miniapp:Classroom:List") {
		t.Fatalf("status=%d codes=%v body=%s", rr.Code, codes, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"title":"S"`) {
		t.Fatalf("missing series: %s", rr.Body.String())
	}
}

func TestClassroomAdminPaidMetadataDoesNotExposeMediaObjectKey(t *testing.T) {
	mediaKey := "private/classroom/secret.mp4"
	f := &fakeClassroomAdminService{content: classroom.Content{ID: 3, Title: "Paid", Status: classroom.ContentPublished, ContentType: classroom.ContentVideo, AccessLevel: classroom.AccessPaid, PriceCents: 1999, MediaAssetID: ptrI64(8)}}
	s := &Server{classroomAdmin: f}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodGet, "/api/admin/classroom/contents?status=published", nil)))
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	raw := rr.Body.String()
	if strings.Contains(raw, "objectKey") || strings.Contains(raw, mediaKey) {
		t.Fatalf("unsafe metadata leaked: %s", raw)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, raw)
	}
}

func TestClassroomAdminPriceEndpointRejectsInvalidCNY(t *testing.T) {
	f := &fakeClassroomAdminService{}
	s := &Server{classroomAdmin: f}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/series/1/price", strings.NewReader(`{"accessLevel":"paid","priceCents":0}`))))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
func ptrI64(v int64) *int64 { return &v }

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestClassroomAdminPublishOfflineAndDraftCRUD(t *testing.T) {
	f := &fakeClassroomAdminService{series: classroom.Series{ID: 4, Title: "S", Status: classroom.SeriesDraft, AccessLevel: classroom.AccessPublic, UpdatedAt: time.Now()}}
	s := &Server{classroomAdmin: f}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	publish := httptest.NewRecorder()
	mux.ServeHTTP(publish, classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/series/4/publish", strings.NewReader(`{}`))))
	if publish.Code != http.StatusOK || !strings.Contains(publish.Body.String(), `"status":"published"`) {
		t.Fatalf("publish status=%d body=%s", publish.Code, publish.Body.String())
	}
	offline := httptest.NewRecorder()
	f.series.Status = classroom.SeriesPublished
	mux.ServeHTTP(offline, classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/series/4/offline", strings.NewReader(`{}`))))
	if offline.Code != http.StatusOK || !strings.Contains(offline.Body.String(), `"status":"offline"`) {
		t.Fatalf("offline status=%d body=%s", offline.Code, offline.Body.String())
	}
}
