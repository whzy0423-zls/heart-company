package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/storage"
)

type fakeClassroomUploadHandlerService struct {
	initiated   classroom.InitiateUploadResult
	signed      storage.SignPartResult
	completed   classroom.CompleteUploadResult
	completeErr error
	aborted     classroom.UploadTask
	calls       []string
}

func (f *fakeClassroomUploadHandlerService) Initiate(context.Context, classroom.InitiateUploadInput) (classroom.InitiateUploadResult, error) {
	f.calls = append(f.calls, "initiate")
	return f.initiated, nil
}
func (f *fakeClassroomUploadHandlerService) SignPart(context.Context, int64, int64, int) (storage.SignPartResult, error) {
	f.calls = append(f.calls, "sign")
	return f.signed, nil
}
func (f *fakeClassroomUploadHandlerService) Complete(context.Context, int64, int64, []storage.CompletedPart) (classroom.CompleteUploadResult, error) {
	f.calls = append(f.calls, "complete")
	return f.completed, f.completeErr
}

func TestClassroomUploadCompleteInProgressReturnsAccepted(t *testing.T) {
	f := &fakeClassroomUploadHandlerService{completeErr: classroom.ErrUploadInProgress}
	s := &Server{classroomUploads: f}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/classroom/uploads/3/complete", strings.NewReader(`{"parts":[{"partNumber":1,"etag":"e1"}]}`))
	req = req.WithContext(withUser(req.Context(), auth.UserInfo{ID: 42}))
	rr := httptest.NewRecorder()
	s.classroomUploadComplete(rr, req)
	if rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), "upload in progress") {
		t.Fatalf("unexpected %d %s", rr.Code, rr.Body.String())
	}
}
func (f *fakeClassroomUploadHandlerService) Abort(context.Context, int64, int64) (classroom.UploadTask, error) {
	f.calls = append(f.calls, "abort")
	return f.aborted, nil
}

func TestClassroomUploadRoutesRequireDedicatedPermission(t *testing.T) {
	f := &fakeClassroomUploadHandlerService{}
	s := &Server{classroomUploads: f}
	mux := http.NewServeMux()
	permissionCode := ""
	deny := func(code string, next http.HandlerFunc) http.HandlerFunc {
		permissionCode = code
		return func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "forbidden", http.StatusForbidden) }
	}
	registerClassroomUploadRoutes(mux, deny, s.classroomUploadInit, s.classroomUploadPart, s.classroomUploadComplete, s.classroomUploadAbort)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/classroom/uploads/initiate", strings.NewReader(`{"contentId":7,"filename":"lesson.mp4","contentType":"video/mp4","sizeBytes":10,"checksum":"sha256:x"}`))
	req = req.WithContext(withUser(req.Context(), auth.UserInfo{ID: 42}))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden || permissionCode != "Miniapp:Classroom:Upload" {
		t.Fatalf("permission=%q status=%d body=%s", permissionCode, rr.Code, rr.Body.String())
	}
}

func TestClassroomUploadHandlersParseInputAndReturnSignedPart(t *testing.T) {
	f := &fakeClassroomUploadHandlerService{signed: storage.SignPartResult{URL: "https://oss.test", PartNumber: 2, ExpiresAt: time.Now().Add(time.Minute)}}
	s := &Server{classroomUploads: f}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/classroom/uploads/9/parts/2/sign", nil)
	req = req.WithContext(withUser(req.Context(), auth.UserInfo{ID: 42}))
	rr := httptest.NewRecorder()
	s.classroomUploadPart(rr, req)
	if rr.Code != http.StatusOK || len(f.calls) != 1 || f.calls[0] != "sign" {
		t.Fatalf("unexpected response %d %s calls=%v", rr.Code, rr.Body.String(), f.calls)
	}
}

func TestClassroomUploadCompleteAcceptsPartEtagsAndIsJSON(t *testing.T) {
	f := &fakeClassroomUploadHandlerService{completed: classroom.CompleteUploadResult{Task: classroom.UploadTask{ID: 3, Status: classroom.UploadCompleted}}}
	s := &Server{classroomUploads: f}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/classroom/uploads/3/complete", strings.NewReader(`{"parts":[{"partNumber":1,"etag":"e1"}]}`))
	req = req.WithContext(withUser(req.Context(), auth.UserInfo{ID: 42}))
	rr := httptest.NewRecorder()
	s.classroomUploadComplete(rr, req)
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusOK || len(f.calls) != 1 || f.calls[0] != "complete" {
		t.Fatalf("unexpected %d %s", rr.Code, rr.Body.String())
	}
}

func TestClassroomUploadNilServiceHandlersReturn503(t *testing.T) {
	s := &Server{}
	for _, handler := range []http.HandlerFunc{s.classroomUploadPart, s.classroomUploadComplete, s.classroomUploadAbort} {
		r := httptest.NewRequest(http.MethodPost, "/api/admin/classroom/uploads/1/complete", strings.NewReader(`{"parts":[]}`))
		r = r.WithContext(withUser(r.Context(), auth.UserInfo{ID: 42}))
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}
}

type fakeClassroomMaintenance struct {
	calls int
	mu    sync.Mutex
}

func (f *fakeClassroomMaintenance) CleanupPending(context.Context, int) (int, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return 0, nil
}
func TestClassroomUploadMaintenanceIsRunnable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := &fakeClassroomMaintenance{}
	go startClassroomUploadMaintenance(ctx, svc, 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	cancel()
	svc.mu.Lock()
	calls := svc.calls
	svc.mu.Unlock()
	if calls < 1 {
		t.Fatalf("maintenance calls=%d", calls)
	}
}

func TestServerShutdownCancelsClassroomMaintenanceContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{maintenanceCancel: cancel}

	s.Shutdown()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("maintenance context was not canceled")
	}
}

func TestClassroomUploadResponsesUseSafeCamelCaseDTOs(t *testing.T) {
	f := &fakeClassroomUploadHandlerService{
		initiated: classroom.InitiateUploadResult{Task: classroom.UploadTask{ID: 5, ContentID: 7, ObjectKey: "private/secret.mp4", OSSUploadID: "upload-secret", Status: classroom.UploadInitiated}},
		aborted:   classroom.UploadTask{ID: 5, ContentID: 7, ObjectKey: "private/secret.mp4", OSSUploadID: "upload-secret", Status: classroom.UploadAborted},
	}
	s := &Server{classroomUploads: f}
	for _, tc := range []struct {
		path    string
		handler http.HandlerFunc
		body    string
	}{
		{"/api/admin/classroom/uploads/initiate", s.classroomUploadInit, `{"contentId":7,"filename":"lesson.mp4","contentType":"video/mp4","sizeBytes":10,"checksum":"sha256:x"}`},
		{"/api/admin/classroom/uploads/5/abort", s.classroomUploadAbort, ""},
		{"/api/admin/classroom/uploads/5/complete", s.classroomUploadComplete, `{"parts":[{"partNumber":1,"etag":"e1"}]}`},
	} {
		rr := httptest.NewRecorder()
		req := classroomUser(httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)))
		tc.handler(rr, req)
		raw := rr.Body.String()
		if strings.Contains(raw, "ObjectKey") || strings.Contains(raw, "objectKey") || strings.Contains(raw, "OSSUploadID") || strings.Contains(raw, "upload-secret") || strings.Contains(raw, "private/secret") {
			t.Fatalf("unsafe upload response: %s", raw)
		}
		if !strings.Contains(raw, `"contentId"`) || strings.Contains(raw, `"ContentID"`) {
			t.Fatalf("not camelCase: %s", raw)
		}
	}
}
