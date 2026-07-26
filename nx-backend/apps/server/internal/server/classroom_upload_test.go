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
	"nine-xing/nx-backend/apps/server/internal/storage"
)

type fakeClassroomUploadHandlerService struct {
	initiated classroom.InitiateUploadResult
	signed    storage.SignPartResult
	completed classroom.CompleteUploadResult
	aborted   classroom.UploadTask
	calls     []string
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
	return f.completed, nil
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
