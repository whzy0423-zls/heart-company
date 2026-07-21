package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/apprelease"
)

func TestAppReleaseIconGETAndHEAD(t *testing.T) {
	icon := []byte("\x89PNG\r\n\x1a\nicon")
	service := newAppReleaseIconService(t, icon)
	server := &Server{appReleases: service}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/app-release-icons/7", nil)
	getResponse := httptest.NewRecorder()
	server.appReleaseIcon(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d; body=%s", getResponse.Code, http.StatusOK, getResponse.Body.String())
	}
	if got := getResponse.Body.Bytes(); string(got) != string(icon) {
		t.Fatalf("GET body = %q, want PNG bytes", got)
	}
	assertAppReleaseIconHeaders(t, getResponse, len(icon))

	headRequest := httptest.NewRequest(http.MethodHead, "/api/app-release-icons/7", nil)
	headResponse := httptest.NewRecorder()
	server.appReleaseIcon(headResponse, headRequest)
	if headResponse.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want %d", headResponse.Code, http.StatusOK)
	}
	if headResponse.Body.Len() != 0 {
		t.Fatalf("HEAD body length = %d, want 0", headResponse.Body.Len())
	}
	assertAppReleaseIconHeaders(t, headResponse, len(icon))
}

func TestAppReleaseIconETagAndNotModified(t *testing.T) {
	service := newAppReleaseIconService(t, []byte("\x89PNG\r\n\x1a\nicon"))
	server := &Server{appReleases: service}

	first := httptest.NewRecorder()
	server.appReleaseIcon(first, httptest.NewRequest(http.MethodGet, "/api/app-release-icons/7", nil))
	etag := first.Header().Get("ETag")
	if etag == "" || !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
		t.Fatalf("ETag = %q, want quoted validator", etag)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/app-release-icons/7", nil)
	request.Header.Set("If-None-Match", etag)
	response := httptest.NewRecorder()
	server.appReleaseIcon(response, request)
	if response.Code != http.StatusNotModified {
		t.Fatalf("conditional GET status = %d, want %d", response.Code, http.StatusNotModified)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("304 body length = %d, want 0", response.Body.Len())
	}
	if got := response.Header().Get("ETag"); got != etag {
		t.Fatalf("304 ETag = %q, want %q", got, etag)
	}
}

func TestAppReleaseIconRejectsInvalidIDsAndPaths(t *testing.T) {
	service := newAppReleaseIconService(t, []byte("icon"))
	server := &Server{appReleases: service}

	paths := []string{
		"/api/app-release-icons/",
		"/api/app-release-icons/0",
		"/api/app-release-icons/-1",
		"/api/app-release-icons/not-a-number",
		"/api/app-release-icons/7/extra",
		"/api/app-release-icons/../../etc/passwd",
		"/api/app-release-icons/%2e%2e%2fetc%2fpasswd",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.appReleaseIcon(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
		})
	}
	if service.openIconCalls != 0 {
		t.Fatalf("OpenIcon calls = %d, want 0 for invalid paths", service.openIconCalls)
	}
}

func TestAppReleaseIconMapsMissingAndUnavailable(t *testing.T) {
	tests := []struct {
		name       string
		service    appReleaseService
		wantStatus int
	}{
		{name: "service unavailable", service: nil, wantStatus: http.StatusServiceUnavailable},
		{name: "missing release", service: &stubAppReleaseService{openIconErr: apprelease.ErrNotFound}, wantStatus: http.StatusNotFound},
		{name: "missing icon metadata", service: &stubAppReleaseService{openIconErr: apprelease.ErrNotFound}, wantStatus: http.StatusNotFound},
		{name: "missing icon file", service: &stubAppReleaseService{openIconErr: apprelease.ErrNotFound}, wantStatus: http.StatusNotFound},
		{name: "file unavailable", service: &stubAppReleaseService{openIconErr: errors.New("storage unavailable")}, wantStatus: http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{appReleases: test.service}
			response := httptest.NewRecorder()
			server.appReleaseIcon(response, httptest.NewRequest(http.MethodGet, "/api/app-release-icons/7", nil))
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

func TestAppReleaseIconRouteIsProtectedAndUsesReadOnlyMethods(t *testing.T) {
	server := &Server{mux: http.NewServeMux()}
	server.routes()

	unauthorized := httptest.NewRecorder()
	server.mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/app-release-icons/7", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("GET without permission status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	post := httptest.NewRecorder()
	server.mux.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/app-release-icons/7", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d; icon route may have been missed", post.Code, http.StatusMethodNotAllowed)
	}

	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	iconRoute := `s.mux.HandleFunc("/api/app-release-icons/", s.getOrHead(s.requirePermission("Website:AppReleases:List", s.appReleaseIcon)))`
	mutationRoute := `s.mux.HandleFunc("/api/app-releases/", s.requirePermission("Website:AppReleases:Write", s.appReleaseMutation))`
	iconIndex := strings.Index(string(source), iconRoute)
	mutationIndex := strings.Index(string(source), mutationRoute)
	if iconIndex < 0 {
		t.Fatalf("protected icon route registration not found")
	}
	if mutationIndex < 0 || iconIndex > mutationIndex {
		t.Fatalf("icon route must be registered before app release mutation catch-all")
	}
}

func assertAppReleaseIconHeaders(t *testing.T, response *httptest.ResponseRecorder, size int) {
	t.Helper()
	if got := response.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "private, max-age=300, must-revalidate" {
		t.Fatalf("Cache-Control = %q, want private revalidation policy", got)
	}
	if got := response.Header().Get("Content-Length"); got != strconv.Itoa(size) {
		t.Fatalf("Content-Length = %q, want %d", got, size)
	}
	if response.Header().Get("ETag") == "" {
		t.Fatal("ETag is empty")
	}
}

func newAppReleaseIconService(t *testing.T, body []byte) *stubAppReleaseService {
	t.Helper()
	path := filepath.Join(t.TempDir(), "icon.png")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return &stubAppReleaseService{
		openIconRelease: apprelease.Release{ID: 7, SHA256: strings.Repeat("a", 64)},
		openIconPath:    path,
	}
}

type stubAppReleaseService struct {
	openIconRelease apprelease.Release
	openIconPath    string
	openIconErr     error
	openIconCalls   int
}

func (s *stubAppReleaseService) List(context.Context, int, int) (apprelease.ListResult, error) {
	return apprelease.ListResult{}, nil
}

func (s *stubAppReleaseService) StageAPK(string, io.Reader) (apprelease.StagedFile, error) {
	return apprelease.StagedFile{}, nil
}

func (s *stubAppReleaseService) DiscardStaged(apprelease.StagedFile) error { return nil }

func (s *stubAppReleaseService) CreateDraftFromStaged(context.Context, apprelease.StagedFile, string) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}

func (s *stubAppReleaseService) Publish(context.Context, int64) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}

func (s *stubAppReleaseService) Archive(context.Context, int64) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}

func (s *stubAppReleaseService) Latest(context.Context, string) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}

func (s *stubAppReleaseService) Open(context.Context, int64) (apprelease.Release, *os.File, error) {
	return apprelease.Release{}, nil, apprelease.ErrNotFound
}

func (s *stubAppReleaseService) OpenIcon(_ context.Context, id int64) (apprelease.Release, *os.File, error) {
	s.openIconCalls++
	if s.openIconErr != nil {
		return apprelease.Release{}, nil, s.openIconErr
	}
	if id != s.openIconRelease.ID {
		return apprelease.Release{}, nil, apprelease.ErrNotFound
	}
	file, err := os.Open(s.openIconPath)
	return s.openIconRelease, file, err
}

func (s *stubAppReleaseService) Maintain(context.Context, time.Time) error { return nil }
