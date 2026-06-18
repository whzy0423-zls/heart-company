package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/storage"
)

func TestUploadRequiresAuth(t *testing.T) {
	handler := New(config.Env{JWTSecret: "test-secret"}, nil)

	body, contentType := multipartBody(t, "file", "logo.png", "image/png", "image")
	request := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestUploadStoresMultipartFile(t *testing.T) {
	uploader := &recordingUploader{
		result: storage.UploadResult{
			Key:         "uploads/site/logo.png",
			URL:         "https://cdn.example.com/uploads/site/logo.png",
			Name:        "logo.png",
			ContentType: "image/png",
			Size:        5,
		},
	}
	env := config.Env{
		JWTSecret:       "test-secret",
		ObjectUploader:  uploader,
		UploadMaxBytes:  1024,
		UploadPublicURL: "https://cdn.example.com",
	}
	handler := New(env, nil)
	token, err := auth.Sign(auth.UserInfo{ID: 1, Username: "admin"}, env.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}

	body, contentType := multipartBody(t, "file", "logo.png", "image/png", "image")
	request := httptest.NewRequest(http.MethodPost, "/api/upload?dir=site", body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	if uploader.dir != "site" {
		t.Fatalf("expected dir site, got %q", uploader.dir)
	}
	if uploader.name != "logo.png" || uploader.contentType != "image/png" || string(uploader.content) != "image" {
		t.Fatalf("unexpected uploaded file: name=%q contentType=%q content=%q", uploader.name, uploader.contentType, uploader.content)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			ContentType string `json:"contentType"`
			Key         string `json:"key"`
			Name        string `json:"name"`
			Size        int64  `json:"size"`
			URL         string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != 0 || payload.Data.URL != uploader.result.URL || payload.Data.Key != uploader.result.Key {
		t.Fatalf("unexpected response: %+v", payload)
	}
}

func TestUploadStoresFileInDatabaseWhenDBAvailable(t *testing.T) {
	database := openUploadTestDB(t)
	env := config.Env{
		JWTSecret:      "test-secret",
		ObjectUploader: &recordingUploader{},
		UploadMaxBytes: 1024,
	}
	handler := New(env, database)
	token, err := auth.Sign(auth.UserInfo{ID: 1, Username: "admin"}, env.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}

	body, contentType := multipartBody(t, "file", "logo.png", "image/png", "image")
	request := httptest.NewRequest(http.MethodPost, "/api/upload?dir=site", body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			AssetID   int64  `json:"assetId"`
			AssetKey  string `json:"assetKey"`
			Key       string `json:"key"`
			ObjectKey string `json:"objectKey"`
			ObjectURL string `json:"objectUrl"`
			URL       string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.AssetID <= 0 || !strings.HasPrefix(payload.Data.AssetKey, "upload-assets/") {
		t.Fatalf("expected database asset metadata, got %+v", payload.Data)
	}
	if payload.Data.Key != payload.Data.AssetKey || payload.Data.URL != "/api/"+payload.Data.AssetKey {
		t.Fatalf("expected preview url to point at stored upload asset, got %+v", payload.Data)
	}
	if payload.Data.ObjectURL != "/site/logo.png" || payload.Data.ObjectKey != "site/logo.png" {
		t.Fatalf("expected object storage url/key metadata, got %+v", payload.Data)
	}
	var objectKey string
	var objectURL string
	if err := database.QueryRow(
		`SELECT object_key, object_url FROM upload_assets WHERE id=$1`,
		payload.Data.AssetID,
	).Scan(&objectKey, &objectURL); err != nil {
		t.Fatalf("query upload asset: %v", err)
	}
	if objectKey != payload.Data.ObjectKey || objectURL != payload.Data.ObjectURL {
		t.Fatalf("expected database to keep object key/url, got key=%q url=%q", objectKey, objectURL)
	}

	assetRequest := httptest.NewRequest(http.MethodGet, "/api/"+payload.Data.AssetKey, nil)
	assetResponse := httptest.NewRecorder()
	handler.ServeHTTP(assetResponse, assetRequest)

	if assetResponse.Code != http.StatusOK {
		t.Fatalf("expected asset 200, got %d body=%s", assetResponse.Code, assetResponse.Body.String())
	}
	if assetResponse.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("expected image/png, got %q", assetResponse.Header().Get("Content-Type"))
	}
	if assetResponse.Body.String() != "image" {
		t.Fatalf("expected image bytes, got %q", assetResponse.Body.String())
	}
}

func TestUploadRejectsOversizedFiles(t *testing.T) {
	env := config.Env{
		JWTSecret:      "test-secret",
		ObjectUploader: &recordingUploader{},
		UploadMaxBytes: 4,
	}
	handler := New(env, nil)
	token, err := auth.Sign(auth.UserInfo{ID: 1, Username: "admin"}, env.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}

	body, contentType := multipartBody(t, "file", "logo.png", "image/png", "image")
	request := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", response.Code, response.Body.String())
	}
}

type recordingUploader struct {
	content     []byte
	contentType string
	dir         string
	name        string
	result      storage.UploadResult
}

func (u *recordingUploader) Upload(ctx context.Context, input storage.UploadInput) (storage.UploadResult, error) {
	content, err := io.ReadAll(input.Reader)
	if err != nil {
		return storage.UploadResult{}, err
	}
	u.content = content
	u.contentType = input.ContentType
	u.dir = input.Dir
	u.name = input.Filename
	if u.result.URL == "" {
		key := strings.Trim(input.Dir+"/"+input.Filename, "/")
		u.result = storage.UploadResult{
			Key:         key,
			URL:         "/" + key,
			Name:        input.Filename,
			ContentType: input.ContentType,
			Size:        input.Size,
		}
	}
	return u.result, nil
}

func multipartBody(t *testing.T, field string, filename string, contentType string, content string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreatePart(map[string][]string{
		"Content-Disposition": {`form-data; name="` + field + `"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

func openUploadTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run upload database integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database, err := db.Open(ctx, dsn, "admin", "123456")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
