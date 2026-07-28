package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/storage"
)

type fakeClassroomAdminService struct {
	series           classroom.Series
	content          classroom.Content
	updatedContent   classroom.Content
	calls            []string
	getSeriesErr     error
	deleteContentErr error
	uploadTasks      []classroom.UploadTask
}

func (f *fakeClassroomAdminService) ListContentContexts(_ context.Context, ids []int64) (map[int64]classroomContentContext, error) {
	f.calls = append(f.calls, "list-content-contexts")
	if f.getSeriesErr != nil && f.content.SeriesID != nil {
		return nil, f.getSeriesErr
	}
	result := make(map[int64]classroomContentContext, len(ids))
	for _, id := range ids {
		var parent *classroom.Series
		if f.content.SeriesID != nil {
			copy := f.series
			parent = &copy
		}
		result[id] = classroomContentContext{Parent: parent}
	}
	return result, nil
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
	f.updatedContent = value
	return value, nil
}
func (f *fakeClassroomAdminService) DeleteSeries(context.Context, int64, time.Time) error {
	f.calls = append(f.calls, "delete-series")
	return nil
}
func (f *fakeClassroomAdminService) DeleteContent(context.Context, int64, time.Time) error {
	f.calls = append(f.calls, "delete-content")
	return f.deleteContentErr
}

type fakeClassroomCoverManager struct {
	updated              classroom.Content
	uploadErr, deleteErr error
	uploads, deletes     int
}

func (f *fakeClassroomCoverManager) Upload(context.Context, int64, time.Time, *int64, string, io.Reader) (classroom.Content, error) {
	f.uploads++
	return f.updated, f.uploadErr
}
func (f *fakeClassroomCoverManager) Delete(context.Context, int64, time.Time, *int64) (classroom.Content, error) {
	f.deletes++
	return f.updated, f.deleteErr
}

type fakeClassroomCoverObjects struct {
	deleted []string
	err     error
}

func (*fakeClassroomCoverObjects) Upload(context.Context, storage.UploadInput) (storage.UploadResult, error) {
	return storage.UploadResult{}, nil
}
func (*fakeClassroomCoverObjects) PresignGetURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://signed/" + key, nil
}
func (f *fakeClassroomCoverObjects) DeleteObject(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return f.err
}
func (f *fakeClassroomAdminService) ListUploadTasks(context.Context, int, int) ([]classroom.UploadTask, int, error) {
	f.calls = append(f.calls, "list-tasks")
	return f.uploadTasks, len(f.uploadTasks), nil
}

func TestClassroomAdminUploadTasksExposeSafeIdentityAndPersistedProgress(t *testing.T) {
	f := &fakeClassroomAdminService{uploadTasks: []classroom.UploadTask{{ID: 4, ContentID: 8, OriginalFilename: "teacher-lesson.mp4", OSSUploadID: "upload-secret", ObjectKey: "private/secret.mp4", ExpectedSize: 200, Checksum: "crc64:123", CompletedParts: 3, CompletedBytes: 75, PartSize: 25, MaxParts: 8, Status: classroom.UploadUploading}}}
	s := &Server{classroomAdmin: f}
	rr := httptest.NewRecorder()
	s.classroomUploadTasks(rr, classroomUser(httptest.NewRequest(http.MethodGet, "/api/admin/classroom/upload-tasks", nil)))
	raw := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, raw)
	}
	for _, want := range []string{`"originalFilename":"teacher-lesson.mp4"`, `"expectedChecksum":"crc64:123"`, `"completedParts":3`, `"completedBytes":75`, `"totalBytes":200`, `"progressPercent":37.5`} {
		if !strings.Contains(raw, want) {
			t.Errorf("missing %s in %s", want, raw)
		}
	}
	for _, forbidden := range []string{"objectKey", "ossUploadId", "upload-secret", "private/secret.mp4"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("unsafe field %q in %s", forbidden, raw)
		}
	}
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

func TestClassroomAdminListPublishAndPriceRoutesStopAtPermissionDenial(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	seen := ""
	permission := func(code string, _ http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			seen = code
			http.Error(w, "Forbidden", http.StatusForbidden)
		}
	}
	registerClassroomAdminRoutes(mux, permission, s)
	cases := []struct {
		method, path, permission string
	}{
		{http.MethodGet, "/api/admin/classroom/series", "Miniapp:Classroom:List"},
		{http.MethodPost, "/api/admin/classroom/series/12/publish", "Miniapp:Classroom:Publish"},
		{http.MethodPost, "/api/admin/classroom/contents/21/price", "Miniapp:Classroom:Price"},
	}
	for _, test := range cases {
		seen = ""
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != http.StatusForbidden || seen != test.permission {
			t.Errorf("%s %s status=%d permission=%q body=%s", test.method, test.path, response.Code, seen, response.Body.String())
		}
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

func TestClassroomAdminContentListResolvesManualCoverAndExposesCoverMetadataWithoutNPlusOne(t *testing.T) {
	f := &fakeClassroomAdminService{
		series: classroom.Series{ID: 7, AccessLevel: classroom.AccessPaid, PriceCents: 2990},
		content: classroom.Content{
			ID: 3, SeriesID: ptrI64(7), Title: "Covered", ContentType: classroom.ContentVideo,
			ManualCoverObjectKey: "classroom/covers/manual/3/cover.webp",
			CoverAspectRatio:     classroom.CoverAspectRatio9x16,
			AccessLevel:          classroom.AccessInherit,
		},
	}
	s := &Server{classroomAdmin: f, classroomPlaybackSigner: fakeClassroomSigner{key: "manual-cover"}, env: config.Env{ClassroomMedia: config.ClassroomMediaConfig{CoverURLTTLSeconds: 1800}}}
	rr := httptest.NewRecorder()
	s.classroomContentList(rr, classroomUser(httptest.NewRequest(http.MethodGet, "/api/admin/classroom/contents", nil)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	raw := rr.Body.String()
	for _, want := range []string{`"coverUrl":"https://cdn.example/manual-cover"`, `"manualCoverObjectKey":"classroom/covers/manual/3/cover.webp"`, `"coverAspectRatio":"9:16"`, `"coverSource":"manual"`, `"effectiveAccessLevel":"paid"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("missing %s in %s", want, raw)
		}
	}
	if strings.Count(strings.Join(f.calls, ","), "get-series") != 0 {
		t.Fatalf("content list performed per-row series lookup: calls=%v", f.calls)
	}
	if rr.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cache-control=%q", rr.Header().Get("Cache-Control"))
	}
}

func TestClassroomAdminCoverSigningFailureReturns503WithoutObjectKey(t *testing.T) {
	key := "classroom/covers/manual/3/private.webp"
	f := &fakeClassroomAdminService{content: classroom.Content{ID: 3, ContentType: classroom.ContentVideo, ManualCoverObjectKey: key, AccessLevel: classroom.AccessPublic}}
	s := &Server{classroomAdmin: f, classroomPlaybackSigner: &recordingClassroomCoverSigner{err: errors.New("signer unavailable")}}
	rr := httptest.NewRecorder()
	s.classroomContentList(rr, classroomUser(httptest.NewRequest(http.MethodGet, "/api/admin/classroom/contents", nil)))
	if rr.Code != http.StatusServiceUnavailable || strings.Contains(rr.Body.String(), key) {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
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

func TestClassroomContentMetadataUpdatePreservesCoverSettings(t *testing.T) {
	updatedAt := time.Date(2026, 7, 28, 11, 0, 0, 0, time.UTC)
	f := &fakeClassroomAdminService{content: classroom.Content{
		ID: 18, Title: "原课件", ContentType: classroom.ContentVideo,
		ManualCoverObjectKey: "classroom/covers/manual/18/portrait.webp",
		CoverAspectRatio:     classroom.CoverAspectRatio9x16,
		Status:               classroom.ContentDraft, AccessLevel: classroom.AccessPublic, UpdatedAt: updatedAt,
	}}
	s := &Server{classroomAdmin: f, classroomPlaybackSigner: fakeClassroomSigner{key: "preserved-cover"}}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, s)

	rr := httptest.NewRecorder()
	body := `{"title":"新标题","contentType":"video","expectedUpdatedAt":"2026-07-28T11:00:00Z"}`
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodPut, "/api/admin/classroom/contents/18", strings.NewReader(body))))

	if rr.Code != http.StatusOK {
		t.Fatalf("metadata update status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.updatedContent.ManualCoverObjectKey != f.content.ManualCoverObjectKey || f.updatedContent.CoverAspectRatio != f.content.CoverAspectRatio {
		t.Fatalf("metadata update changed cover settings: got key=%q ratio=%q, want key=%q ratio=%q", f.updatedContent.ManualCoverObjectKey, f.updatedContent.CoverAspectRatio, f.content.ManualCoverObjectKey, f.content.CoverAspectRatio)
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

func classroomCoverMultipart(t *testing.T, expected string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	part, err := w.CreateFormFile("file", "cover.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if expected != "" {
		_ = w.WriteField("expectedUpdatedAt", expected)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return body, w.FormDataContentType()
}

func TestClassroomContentCoverUploadAndDeleteReturnLatestDTO(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	current := classroom.Content{ID: 11, Title: "课件", ContentType: classroom.ContentVideo, Status: classroom.ContentPublished, AccessLevel: classroom.AccessPublic, CoverAspectRatio: classroom.CoverAspectRatio16x9, UpdatedAt: now}
	updated := current
	updated.ManualCoverObjectKey = "classroom/covers/manual/11/new.png"
	updated.UpdatedAt = now.Add(time.Second)
	admin := &fakeClassroomAdminService{content: current}
	manager := &fakeClassroomCoverManager{updated: updated}
	objects := &fakeClassroomCoverObjects{}
	s := &Server{classroomAdmin: admin, classroomCovers: manager, classroomPlaybackSigner: objects}
	mux := http.NewServeMux()
	registerClassroomAdminRoutes(mux, func(_ string, h http.HandlerFunc) http.HandlerFunc { return h }, s)
	body, contentType := classroomCoverMultipart(t, now.Format(time.RFC3339Nano), []byte("png"))
	req := classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/contents/11/cover", body))
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || manager.uploads != 1 || !strings.Contains(rr.Body.String(), `"coverSource":"manual"`) || strings.Contains(rr.Body.String(), "objectKey") {
		t.Fatalf("status=%d uploads=%d body=%s", rr.Code, manager.uploads, rr.Body.String())
	}

	manager.updated = current
	manager.updated.UpdatedAt = updated.UpdatedAt.Add(time.Second)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, classroomUser(httptest.NewRequest(http.MethodDelete, "/api/admin/classroom/contents/11/cover?expectedUpdatedAt="+updated.UpdatedAt.Format(time.RFC3339Nano), nil)))
	if rr.Code != http.StatusOK || manager.deletes != 1 || !strings.Contains(rr.Body.String(), `"coverSource":"none"`) {
		t.Fatalf("status=%d deletes=%d body=%s", rr.Code, manager.deletes, rr.Body.String())
	}
}

func TestClassroomContentCoverRequiresVersionUsesWritePermissionAndMapsErrors(t *testing.T) {
	now := time.Now().UTC()
	current := classroom.Content{ID: 6, Title: "课件", ContentType: classroom.ContentVideo, Status: classroom.ContentReady, AccessLevel: classroom.AccessPublic, UpdatedAt: now}
	admin := &fakeClassroomAdminService{content: current}
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{{"invalid", classroom.ErrInvalidCoverImage, 400}, {"missing", classroom.ErrNotFound, 404}, {"conflict", classroom.ErrConflict, 409}, {"storage", classroom.ErrCoverStorageUnavailable, 503}} {
		t.Run(tc.name, func(t *testing.T) {
			manager := &fakeClassroomCoverManager{uploadErr: tc.err}
			s := &Server{classroomAdmin: admin, classroomCovers: manager}
			mux := http.NewServeMux()
			seen := ""
			registerClassroomAdminRoutes(mux, func(code string, h http.HandlerFunc) http.HandlerFunc { seen = code; return h }, s)
			body, typ := classroomCoverMultipart(t, now.Format(time.RFC3339Nano), []byte("png"))
			req := classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/contents/6/cover", body))
			req.Header.Set("Content-Type", typ)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code != tc.status || seen != "Miniapp:Classroom:Write" {
				t.Fatalf("status=%d permission=%s body=%s", rr.Code, seen, rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "secret") || strings.Contains(rr.Body.String(), "classroom/covers") {
				t.Fatalf("leaked detail: %s", rr.Body.String())
			}
		})
	}
	body, typ := classroomCoverMultipart(t, "", []byte("png"))
	req := classroomUser(httptest.NewRequest(http.MethodPost, "/api/admin/classroom/contents/6/cover", body))
	req.Header.Set("Content-Type", typ)
	rr := httptest.NewRecorder()
	(&Server{classroomAdmin: admin, classroomCovers: &fakeClassroomCoverManager{}}).classroomContentItem(rr, req)
	if rr.Code != 400 {
		t.Fatalf("missing version status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestClassroomContentDeleteCleansManualCoverOnlyAfterDatabaseCommit(t *testing.T) {
	now := time.Now().UTC()
	content := classroom.Content{ID: 19, Title: "草稿", ContentType: classroom.ContentVideo, Status: classroom.ContentDraft, AccessLevel: classroom.AccessPublic, UpdatedAt: now, ManualCoverObjectKey: "classroom/covers/manual/19/x.jpg"}
	for _, tc := range []struct {
		name             string
		dbErr, deleteErr error
		wantStatus       int
		wantDeletes      int
	}{{"success", nil, nil, 200, 1}, {"database failure", errors.New("db down"), nil, 500, 0}, {"already gone", nil, storage.ErrAlreadyGone, 200, 1}, {"cleanup failure", nil, errors.New("oss down"), 200, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			admin := &fakeClassroomAdminService{content: content, deleteContentErr: tc.dbErr}
			objects := &fakeClassroomCoverObjects{err: tc.deleteErr}
			s := &Server{classroomAdmin: admin, classroomCoverObjects: objects}
			rr := httptest.NewRecorder()
			s.classroomContentItem(rr, classroomUser(httptest.NewRequest(http.MethodDelete, "/api/admin/classroom/contents/19?expectedUpdatedAt="+now.Format(time.RFC3339Nano), nil)))
			if rr.Code != tc.wantStatus || len(objects.deleted) != tc.wantDeletes {
				t.Fatalf("status=%d deleted=%v body=%s", rr.Code, objects.deleted, rr.Body.String())
			}
		})
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
