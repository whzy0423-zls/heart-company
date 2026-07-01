package server

import (
	"context"
	"io"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
	"nine-xing/nx-backend/apps/server/internal/videoanalysis"
)

func TestNewPanicsForUnknownSMSProvider(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected sms sender config to panic for unknown provider")
		}
	}()

	mustSMSSender(config.SMSConfig{Provider: "unknown"})
}

func TestNewPanicsForProductionWxPayWithoutCompleteConfig(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected wxpay config to panic when production config is incomplete")
		}
	}()

	mustWxPayClient(config.Env{
		AppEnv: "production",
		WxPay:  config.WxPayConfig{Dev: false},
	})
}

func TestNewAllowsExplicitDevWxPay(t *testing.T) {
	client := mustWxPayClient(config.Env{WxPay: config.WxPayConfig{Dev: true}})
	if !client.DevMode() {
		t.Fatal("expected explicit dev wxpay to stay in dev mode")
	}
}

func TestAnalysisVideoURLUsesSignedObjectURLWhenAvailable(t *testing.T) {
	signer := &recordingObjectSigner{url: "https://cdn.example.com/private.mp4?signature=ok"}
	s := &Server{uploader: signer}
	job := uploadasset.Asset{
		ObjectKey: "uploads/video/analysis/demo.mp4",
		ObjectURL: "https://cdn.example.com/uploads/video/analysis/demo.mp4",
	}

	got := s.analysisVideoURL(context.Background(), job)

	if got != signer.url {
		t.Fatalf("expected signed url, got %q", got)
	}
	if signer.objectKey != job.ObjectKey || signer.expires != 30*time.Minute {
		t.Fatalf("unexpected presign call: key=%q expires=%s", signer.objectKey, signer.expires)
	}
}

func TestAnalysisVideoURLFallsBackToObjectURLWithoutSigner(t *testing.T) {
	s := &Server{}
	job := uploadasset.Asset{
		ObjectKey: "uploads/video/analysis/demo.mp4",
		ObjectURL: "https://cdn.example.com/uploads/video/analysis/demo.mp4",
	}

	got := s.analysisVideoURL(context.Background(), job)

	if got != job.ObjectURL {
		t.Fatalf("expected object url fallback, got %q", got)
	}
}

func TestAnalysisJobVideoURLFallsBackToStoredURLWithoutAssetStore(t *testing.T) {
	s := &Server{}
	job := videoanalysis.Job{
		VideoAssetID: "12",
		VideoURL:     "https://cdn.example.com/uploads/video/analysis/demo.mp4",
	}

	got := s.analysisJobVideoURL(context.Background(), job)

	if got != job.VideoURL {
		t.Fatalf("expected stored video url fallback, got %q", got)
	}
}

func TestBackfillUploadAssetObjectURLReuploadsLegacyAsset(t *testing.T) {
	uploader := &recordingUploadResultUploader{
		result: storage.UploadResult{
			Key:         "upload/video/20260701/demo.png",
			URL:         "https://bucket.example.com/upload/video/20260701/demo.png",
			Name:        "demo.png",
			ContentType: "image/png",
			Size:        5,
		},
	}
	updater := &fakeUploadAssetObjectUpdater{}
	asset := uploadasset.Asset{
		ID:          34,
		Key:         "upload-assets/34",
		Name:        "demo.png",
		ContentType: "image/png",
		Data:        []byte("image"),
		ObjectKey:   "",
		ObjectURL:   "",
		Size:        5,
	}

	got, err := backfillUploadAssetObjectURL(context.Background(), updater, uploader, 34, asset)
	if err != nil {
		t.Fatalf("backfillUploadAssetObjectURL returned error: %v", err)
	}
	if got != uploader.result.URL {
		t.Fatalf("expected backfilled object url %q, got %q", uploader.result.URL, got)
	}
	if uploader.dir != "video/reference" || uploader.name != "demo.png" || uploader.content != "image" {
		t.Fatalf("unexpected upload input dir=%q name=%q content=%q", uploader.dir, uploader.name, uploader.content)
	}
	if updater.updatedID != 34 || updater.updatedKey != uploader.result.Key || updater.updatedURL != uploader.result.URL {
		t.Fatalf("expected upload asset object metadata to be backfilled, got id=%d key=%q url=%q", updater.updatedID, updater.updatedKey, updater.updatedURL)
	}
}

type recordingObjectSigner struct {
	expires   time.Duration
	objectKey string
	url       string
}

func (s *recordingObjectSigner) Upload(context.Context, storage.UploadInput) (storage.UploadResult, error) {
	return storage.UploadResult{}, nil
}

func (s *recordingObjectSigner) PresignGetURL(_ context.Context, objectKey string, expires time.Duration) (string, error) {
	s.objectKey = objectKey
	s.expires = expires
	return s.url, nil
}

type recordingUploadResultUploader struct {
	content string
	dir     string
	name    string
	result  storage.UploadResult
}

func (u *recordingUploadResultUploader) Upload(_ context.Context, input storage.UploadInput) (storage.UploadResult, error) {
	data, err := io.ReadAll(input.Reader)
	if err != nil {
		return storage.UploadResult{}, err
	}
	u.content = string(data)
	u.dir = input.Dir
	u.name = input.Filename
	return u.result, nil
}

type fakeUploadAssetObjectUpdater struct {
	updatedID  int64
	updatedKey string
	updatedURL string
}

func (s *fakeUploadAssetObjectUpdater) UpdateObjectMetadata(_ context.Context, id int64, objectKey string, objectURL string) error {
	s.updatedID = id
	s.updatedKey = objectKey
	s.updatedURL = objectURL
	return nil
}
