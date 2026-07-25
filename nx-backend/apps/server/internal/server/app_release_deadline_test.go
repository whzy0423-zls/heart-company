package server

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/apprelease"
)

func TestAppReleaseUploadClearsServerReadDeadline(t *testing.T) {
	service := slowUploadAppReleaseService{}
	handler := &Server{appReleases: service}
	testServer := httptest.NewUnstartedServer(http.HandlerFunc(handler.appReleaseUpload))
	testServer.Config.ReadTimeout = 20 * time.Millisecond
	testServer.Config.WriteTimeout = time.Second
	testServer.Start()
	defer testServer.Close()

	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	writeDone := make(chan error, 1)
	go func() {
		part, err := multipartWriter.CreateFormFile("file", "release.apk")
		if err == nil {
			_, err = part.Write([]byte("first"))
		}
		if err == nil {
			time.Sleep(40 * time.Millisecond)
			_, err = part.Write([]byte("second"))
		}
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		if closeErr := writer.CloseWithError(err); err == nil {
			err = closeErr
		}
		writeDone <- err
	}()

	request, err := http.NewRequest(http.MethodPost, testServer.URL, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	response, err := testServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("write multipart body: %v", err)
	}
}

type slowUploadAppReleaseService struct{}

func (slowUploadAppReleaseService) List(context.Context, int, int) (apprelease.ListResult, error) {
	return apprelease.ListResult{}, nil
}
func (slowUploadAppReleaseService) StageAPK(_ string, source io.Reader) (apprelease.StagedFile, error) {
	_, err := io.Copy(io.Discard, source)
	return apprelease.StagedFile{}, err
}
func (slowUploadAppReleaseService) DiscardStaged(apprelease.StagedFile) error { return nil }
func (slowUploadAppReleaseService) CreateDraftFromStaged(context.Context, apprelease.StagedFile, string) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}
func (slowUploadAppReleaseService) Publish(context.Context, int64) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}
func (slowUploadAppReleaseService) Archive(context.Context, int64) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}
func (slowUploadAppReleaseService) Latest(context.Context, string) (apprelease.Release, error) {
	return apprelease.Release{}, nil
}
func (slowUploadAppReleaseService) Open(context.Context, int64) (apprelease.Release, *os.File, error) {
	return apprelease.Release{}, nil, apprelease.ErrNotFound
}
func (slowUploadAppReleaseService) OpenIcon(context.Context, int64) (apprelease.Release, *os.File, error) {
	return apprelease.Release{}, nil, apprelease.ErrNotFound
}
func (slowUploadAppReleaseService) Maintain(context.Context, time.Time) error { return nil }
