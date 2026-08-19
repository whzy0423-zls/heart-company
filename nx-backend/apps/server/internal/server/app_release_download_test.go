package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/apprelease"
)

func TestPublicAppReleaseDownloadSupportsByteRanges(t *testing.T) {
	body := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	service := newDownloadAppReleaseService(t, body)
	server := &Server{appReleases: service}

	tests := []struct {
		name       string
		method     string
		rangeValue string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "GET returns requested bytes",
			method:     http.MethodGet,
			rangeValue: "bytes=10-14",
			wantStatus: http.StatusPartialContent,
			wantBody:   "abcde",
		},
		{
			name:       "HEAD returns range metadata without a body",
			method:     http.MethodHead,
			rangeValue: "bytes=10-14",
			wantStatus: http.StatusPartialContent,
			wantBody:   "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/api/public/app-releases/9/download", nil)
			request.Header.Set("Range", test.rangeValue)
			response := httptest.NewRecorder()

			server.publicAppReleaseDownload(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%q", response.Code, test.wantStatus, response.Body.String())
			}
			if got := response.Body.String(); got != test.wantBody {
				t.Fatalf("body = %q, want %q", got, test.wantBody)
			}
			if got, want := response.Header().Get("Content-Range"), "bytes 10-14/"+strconv.Itoa(len(body)); got != want {
				t.Fatalf("Content-Range = %q, want %q", got, want)
			}
			if got := response.Header().Get("Content-Length"); got != "5" {
				t.Fatalf("Content-Length = %q, want 5", got)
			}
			if got := response.Header().Get("Accept-Ranges"); got != "bytes" {
				t.Fatalf("Accept-Ranges = %q, want bytes", got)
			}
		})
	}
}

func TestPublicAppReleaseDownloadRejectsUnsatisfiableRange(t *testing.T) {
	body := []byte("0123456789")
	service := newDownloadAppReleaseService(t, body)
	server := &Server{appReleases: service}
	request := httptest.NewRequest(http.MethodGet, "/api/public/app-releases/9/download", nil)
	request.Header.Set("Range", "bytes=100-199")
	response := httptest.NewRecorder()

	server.publicAppReleaseDownload(response, request)

	if response.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status = %d, want %d; body=%q", response.Code, http.StatusRequestedRangeNotSatisfiable, response.Body.String())
	}
	if got, want := response.Header().Get("Content-Range"), "bytes */"+strconv.Itoa(len(body)); got != want {
		t.Fatalf("Content-Range = %q, want %q", got, want)
	}
}

func newDownloadAppReleaseService(t *testing.T, body []byte) *downloadAppReleaseService {
	t.Helper()
	path := filepath.Join(t.TempDir(), "xinzhili.apk")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, time.August, 19, 4, 16, 32, 0, time.UTC)
	return &downloadAppReleaseService{
		release: apprelease.Release{
			ID:        9,
			FileName:  "xinzhili.apk",
			SHA256:    "c9b3d3d115702d3227ec1a2fea8cc0ef76ab85fc17adb47064fbc0ce4e364480",
			Status:    apprelease.StatusPublished,
			CreatedAt: createdAt,
		},
		path: path,
	}
}

type downloadAppReleaseService struct {
	release apprelease.Release
	path    string
}

func (*downloadAppReleaseService) List(context.Context, int, int) (apprelease.ListResult, error) {
	return apprelease.ListResult{}, nil
}

func (*downloadAppReleaseService) StageAPK(string, io.Reader) (apprelease.StagedFile, error) {
	return apprelease.StagedFile{}, nil
}

func (*downloadAppReleaseService) DiscardStaged(apprelease.StagedFile) error { return nil }

func (*downloadAppReleaseService) CreateDraftFromStaged(context.Context, apprelease.StagedFile, string) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}

func (*downloadAppReleaseService) Publish(context.Context, int64) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}

func (*downloadAppReleaseService) Archive(context.Context, int64) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}

func (*downloadAppReleaseService) Latest(context.Context, string) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}

func (s *downloadAppReleaseService) Open(_ context.Context, id int64) (apprelease.Release, *os.File, error) {
	if id != s.release.ID {
		return apprelease.Release{}, nil, apprelease.ErrNotFound
	}
	file, err := os.Open(s.path)
	return s.release, file, err
}

func (*downloadAppReleaseService) OpenIcon(context.Context, int64) (apprelease.Release, *os.File, error) {
	return apprelease.Release{}, nil, apprelease.ErrNotFound
}

func (*downloadAppReleaseService) Maintain(context.Context, time.Time) error { return nil }
