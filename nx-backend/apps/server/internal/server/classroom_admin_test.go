package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
)

type fakeClassroomAdminService struct {
	series       classroom.Series
	content      classroom.Content
	calls        []string
	getSeriesErr error
}

func (f *fakeClassroomAdminService) ListSeries(context.Context, classroom.SeriesFilter) ([]classroom.Series, int, error) {
	f.calls = append(f.calls, "list-series")
	return []classroom.Series{f.series}, 1, nil
}
func (f *fakeClassroomAdminService) GetSeries(context.Context, int64) (classroom.Series, error) {
	f.calls = append(f.calls, "get-series")
	return f.series, f.getSeriesErr
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
func (f *fakeClassroomAdminService) DeleteSeries(context.Context, int64, time.Time) error {
	f.calls = append(f.calls, "delete-series")
	return nil
}
func (f *fakeClassroomAdminService) DeleteContent(context.Context, int64, time.Time) error {
	f.calls = append(f.calls, "delete-content")
	return nil
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
	mux.ServeHTTP(publish, classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/series/4/publish", strings.NewReader(`{"expectedUpdatedAt":"2026-07-26T10:00:00Z"}`))))
	if publish.Code != http.StatusOK || !strings.Contains(publish.Body.String(), `"status":"published"`) {
		t.Fatalf("publish status=%d body=%s", publish.Code, publish.Body.String())
	}
	offline := httptest.NewRecorder()
	f.series.Status = classroom.SeriesPublished
	mux.ServeHTTP(offline, classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/series/4/offline", strings.NewReader(`{"expectedUpdatedAt":"2026-07-26T10:00:00Z"}`))))
	if offline.Code != http.StatusOK || !strings.Contains(offline.Body.String(), `"status":"offline"`) {
		t.Fatalf("offline status=%d body=%s", offline.Code, offline.Body.String())
	}
}

type failingClassroomAudit struct{ calls int }

func (f *failingClassroomAudit) Record(context.Context, auditlog.Entry) error {
	f.calls++
	return errors.New("audit unavailable")
}

func TestClassroomWriteRejectsSensitiveFieldsAndPublishedEdits(t *testing.T) {
	f := &fakeClassroomAdminService{series: classroom.Series{ID: 8, Title: "published", Status: classroom.SeriesPublished, AccessLevel: classroom.AccessPublic, UpdatedAt: time.Now()}}
	s := &Server{classroomAdmin: f}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/series", strings.NewReader(`{"title":"S","accessLevel":"paid","priceCents":100}`))))
	if create.Code != http.StatusBadRequest {
		t.Fatalf("sensitive create status=%d body=%s", create.Code, create.Body.String())
	}
	update := httptest.NewRecorder()
	mux.ServeHTTP(update, classroomUser(httptest.NewRequest(http.MethodPut, "/api/admin/classroom/series/8", strings.NewReader(`{"title":"changed","expectedUpdatedAt":"2026-07-26T10:00:00Z"}`))))
	if update.Code != http.StatusConflict {
		t.Fatalf("published update status=%d body=%s", update.Code, update.Body.String())
	}
}

func TestClassroomActionsRequireExpectedUpdatedAt(t *testing.T) {
	f := &fakeClassroomAdminService{series: classroom.Series{ID: 4, Title: "S", Status: classroom.SeriesDraft, AccessLevel: classroom.AccessPublic, UpdatedAt: time.Now()}}
	s := &Server{classroomAdmin: f}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/series/4/publish", strings.NewReader(`{}`))))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestClassroomDeleteDraftRoutes(t *testing.T) {
	f := &fakeClassroomAdminService{series: classroom.Series{ID: 2, Title: "draft", Status: classroom.SeriesDraft, AccessLevel: classroom.AccessPublic, UpdatedAt: time.Now()}}
	s := &Server{classroomAdmin: f}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodDelete, "/api/admin/classroom/series/2?expectedUpdatedAt=2026-07-26T10:00:00Z", nil)))
	if rr.Code != http.StatusOK || !containsString(f.calls, "delete-series") {
		t.Fatalf("status=%d calls=%v body=%s", rr.Code, f.calls, rr.Body.String())
	}
}

func TestClassroomAuditFailureIsBestEffort(t *testing.T) {
	f := &fakeClassroomAdminService{series: classroom.Series{ID: 2, Title: "draft", Status: classroom.SeriesDraft, AccessLevel: classroom.AccessPublic, UpdatedAt: time.Now()}}
	audit := &failingClassroomAudit{}
	s := &Server{classroomAdmin: f, classroomAudit: audit}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodDelete, "/api/admin/classroom/series/2?expectedUpdatedAt=2026-07-26T10:00:00Z", nil)))
	if rr.Code != http.StatusOK || audit.calls != 1 {
		t.Fatalf("status=%d audit=%d body=%s", rr.Code, audit.calls, rr.Body.String())
	}
}

type capturingClassroomAudit struct{ entry auditlog.Entry }

func (c *capturingClassroomAudit) Record(_ context.Context, entry auditlog.Entry) error {
	c.entry = entry
	return nil
}

func TestClassroomAuditCapturesActorActionObjectAndReason(t *testing.T) {
	f := &fakeClassroomAdminService{series: classroom.Series{ID: 9, Title: "draft", Status: classroom.SeriesDraft, AccessLevel: classroom.AccessPublic, UpdatedAt: time.Now()}}
	audit := &capturingClassroomAudit{}
	s := &Server{classroomAdmin: f, classroomAudit: audit}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/series/9/price", strings.NewReader(`{"expectedUpdatedAt":"2026-07-26T10:00:00Z","accessLevel":"public","priceCents":0,"reason":"活动结束"}`))))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if audit.entry.OperatorID != 42 || audit.entry.OperatorName != "老师" || audit.entry.Action != "price" || audit.entry.TargetType != "classroom_series" || audit.entry.TargetID != "9" || !strings.Contains(audit.entry.Summary, "活动结束") {
		t.Fatalf("incomplete audit: %+v", audit.entry)
	}
}

func TestClassroomEffectiveAccessParentFailureAndPurchaseTarget(t *testing.T) {
	seriesID := int64(3)
	f := &fakeClassroomAdminService{content: classroom.Content{ID: 7, SeriesID: &seriesID, Title: "lesson", ContentType: classroom.ContentVideo, Status: classroom.ContentDraft, AccessLevel: classroom.AccessInherit}, getSeriesErr: errors.New("parent unavailable")}
	s := &Server{classroomAdmin: f}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodGet, "/api/admin/classroom/contents", nil)))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	f.getSeriesErr = nil
	f.series = classroom.Series{ID: seriesID, AccessLevel: classroom.AccessPublic}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodGet, "/api/admin/classroom/contents", nil)))
	if strings.Contains(rr.Body.String(), `"purchaseTarget"`) {
		t.Fatalf("public content must not set purchase target: %s", rr.Body.String())
	}
}

func TestClassroomPriceActionAllowsPublishedAndOfflineRecords(t *testing.T) {
	for _, status := range []classroom.SeriesStatus{classroom.SeriesPublished, classroom.SeriesOffline} {
		f := &fakeClassroomAdminService{series: classroom.Series{ID: 12, Title: "series", Status: status, AccessLevel: classroom.AccessPaid, PriceCents: 1000, UpdatedAt: time.Now()}}
		audit := &capturingClassroomAudit{}
		s := &Server{classroomAdmin: f, classroomAudit: audit}
		mux := http.NewServeMux()
		registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/series/12/price", strings.NewReader(`{"expectedUpdatedAt":"2026-07-26T10:00:00Z","accessLevel":"paid","priceCents":2999,"reason":"新一期定价"}`))))
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"priceCents":2999`) {
			t.Fatalf("status=%s code=%d body=%s", status, rr.Code, rr.Body.String())
		}
		if audit.entry.Action != "price" || !strings.Contains(audit.entry.Summary, "新一期定价") {
			t.Fatalf("status=%s audit=%+v", status, audit.entry)
		}
	}
}

func TestClassroomContentPriceActionAllowsPublishedRecord(t *testing.T) {
	f := &fakeClassroomAdminService{content: classroom.Content{ID: 13, Title: "lesson", ContentType: classroom.ContentAudio, Status: classroom.ContentPublished, AccessLevel: classroom.AccessPaid, PriceCents: 800, UpdatedAt: time.Now()}}
	audit := &capturingClassroomAudit{}
	s := &Server{classroomAdmin: f, classroomAudit: audit}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/contents/13/price", strings.NewReader(`{"expectedUpdatedAt":"2026-07-26T10:00:00Z","accessLevel":"paid","priceCents":1599,"reason":"单课调价"}`))))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"priceCents":1599`) {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestClassroomAuditFailureDoesNotReportBusinessFailure(t *testing.T) {
	f := &fakeClassroomAdminService{series: classroom.Series{ID: 22, Title: "draft", Status: classroom.SeriesDraft, AccessLevel: classroom.AccessPublic, UpdatedAt: time.Now()}}
	audit := &failingClassroomAudit{}
	s := &Server{classroomAdmin: f, classroomAudit: audit}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodDelete, "/api/admin/classroom/series/22?expectedUpdatedAt=2026-07-26T10:00:00Z", nil)))
	if rr.Code != http.StatusOK {
		t.Fatalf("business operation must remain successful after best-effort audit failure: %d %s", rr.Code, rr.Body.String())
	}
}

func TestClassroomAdminUnknownErrorDoesNotLeakDriverDetails(t *testing.T) {
	secretErr := errors.New("pq: password=super-secret host=internal-db")
	f := &fakeClassroomAdminService{getSeriesErr: secretErr}
	s := &Server{classroomAdmin: f}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodGet, "/api/admin/classroom/series/99", nil)))
	if rr.Code != http.StatusInternalServerError || strings.Contains(rr.Body.String(), "super-secret") || strings.Contains(rr.Body.String(), "internal-db") {
		t.Fatalf("unknown error leaked: %d %s", rr.Code, rr.Body.String())
	}
}

func TestClassroomAdminRejectsOverflowingPagination(t *testing.T) {
	f := &fakeClassroomAdminService{}
	s := &Server{classroomAdmin: f}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodGet, "/api/admin/classroom/series?page=1000001&pageSize=200", nil)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("overflow page status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(f.calls) != 0 {
		t.Fatalf("service called for invalid pagination: %v", f.calls)
	}
}

func TestClassroomAuditUsesNormalizedClientIP(t *testing.T) {
	f := &fakeClassroomAdminService{series: classroom.Series{ID: 31, Title: "draft", Status: classroom.SeriesDraft, AccessLevel: classroom.AccessPublic, UpdatedAt: time.Now()}}
	audit := &capturingClassroomAudit{}
	s := &Server{classroomAdmin: f, classroomAudit: audit}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)
	req := classroomUser(httptest.NewRequest(http.MethodDelete, "/api/admin/classroom/series/31?expectedUpdatedAt=2026-07-26T10:00:00Z", nil))
	req.RemoteAddr = "203.0.113.9:54321"
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || audit.entry.IP != "203.0.113.9" {
		t.Fatalf("status=%d auditIP=%q body=%s", rr.Code, audit.entry.IP, rr.Body.String())
	}
}
