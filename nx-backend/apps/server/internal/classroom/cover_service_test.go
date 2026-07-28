package classroom

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/storage"
)

var testPNG, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
var testWebP, _ = base64.StdEncoding.DecodeString("UklGRjwAAABXRUJQVlA4IDAAAADQAQCdASoBAAEAAUAmJaACdLoB+AADsAD+8ut//NgVzXPv9//S4P0uD9Lg/9KQAAA=")

type fakeCoverStore struct {
	content     Content
	media       MediaAsset
	setErr      error
	settingsErr error
	sets        []string
}

func (f *fakeCoverStore) GetContent(context.Context, int64) (Content, error) {
	if f.content.ID == 0 {
		return Content{}, ErrNotFound
	}
	return f.content, nil
}
func (f *fakeCoverStore) SetContentManualCover(_ context.Context, _ int64, key string, expected time.Time, updatedBy *int64) (Content, error) {
	f.sets = append(f.sets, key)
	if f.setErr != nil {
		return Content{}, f.setErr
	}
	if !f.content.UpdatedAt.Equal(expected) {
		return Content{}, ErrConflict
	}
	f.content.ManualCoverObjectKey = key
	f.content.UpdatedBy = updatedBy
	f.content.UpdatedAt = expected.Add(time.Second)
	return f.content, nil
}
func (f *fakeCoverStore) GetMediaAsset(context.Context, int64) (MediaAsset, error) {
	if f.media.ID == 0 {
		return MediaAsset{}, ErrNotFound
	}
	return f.media, nil
}
func (f *fakeCoverStore) SetContentCoverSettings(_ context.Context, _ int64, ratio CoverAspectRatio, expected time.Time, updatedBy *int64, mediaID *int64, expectedGeneratedKey, generatedKey string) (Content, error) {
	if f.settingsErr != nil {
		return Content{}, f.settingsErr
	}
	if !f.content.UpdatedAt.Equal(expected) || (mediaID != nil && (f.media.ID != *mediaID || f.media.CoverObjectKey != expectedGeneratedKey)) {
		return Content{}, ErrConflict
	}
	if mediaID != nil {
		f.media.CoverObjectKey = generatedKey
	}
	f.content.CoverAspectRatio = ratio
	f.content.UpdatedBy = updatedBy
	f.content.UpdatedAt = expected.Add(time.Second)
	return f.content, nil
}

func classroomTestPtrI64(v int64) *int64 { return &v }

type fakeCoverStorage struct {
	uploadErr     error
	deleteErr     error
	uploads       []storage.UploadInput
	deleted       []string
	deleteCtxErrs []error
}

func (f *fakeCoverStorage) Upload(_ context.Context, in storage.UploadInput) (storage.UploadResult, error) {
	b, _ := io.ReadAll(in.Reader)
	in.Reader = bytes.NewReader(b)
	f.uploads = append(f.uploads, in)
	if f.uploadErr != nil {
		return storage.UploadResult{}, f.uploadErr
	}
	return storage.UploadResult{Key: strings.Trim(in.Dir, "/") + "/" + in.Filename, ObjectKey: strings.Trim(in.Dir, "/") + "/" + in.Filename}, nil
}
func (*fakeCoverStorage) PresignGetURL(context.Context, string, time.Duration) (string, error) {
	return "signed", nil
}
func (f *fakeCoverStorage) DeleteObject(ctx context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	f.deleteCtxErrs = append(f.deleteCtxErrs, ctx.Err())
	return f.deleteErr
}

func TestCoverServiceUploadsAndReplacesManualCover(t *testing.T) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	actor := int64(42)
	store := &fakeCoverStore{content: Content{ID: 7, Status: ContentPublished, UpdatedAt: now, ManualCoverObjectKey: "classroom/covers/manual/7/old.jpg"}}
	objects := &fakeCoverStorage{}
	svc := NewCoverService(store, objects, 1024)
	got, err := svc.Upload(context.Background(), 7, now, &actor, "fake.exe", bytes.NewReader(testPNG))
	if err != nil {
		t.Fatal(err)
	}
	if got.ManualCoverObjectKey == "" || !strings.HasPrefix(got.ManualCoverObjectKey, "classroom/covers/manual/7/") || !strings.HasSuffix(got.ManualCoverObjectKey, ".png") {
		t.Fatalf("key=%q", got.ManualCoverObjectKey)
	}
	if len(objects.uploads) != 1 || objects.uploads[0].ContentType != "image/png" || objects.uploads[0].Size != int64(len(testPNG)) {
		t.Fatalf("uploads=%+v", objects.uploads)
	}
	if len(objects.deleted) != 1 || objects.deleted[0] != "classroom/covers/manual/7/old.jpg" {
		t.Fatalf("deleted=%v", objects.deleted)
	}
}

func TestCoverServiceCleansNewObjectWhenCASFails(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeCoverStore{content: Content{ID: 3, Status: ContentReady, UpdatedAt: now}, setErr: ErrConflict}
	objects := &fakeCoverStorage{}
	_, err := NewCoverService(store, objects, 1024).Upload(context.Background(), 3, now, nil, "cover.png", bytes.NewReader(testPNG))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	if len(objects.deleted) != 1 || !strings.Contains(objects.deleted[0], "/3/") {
		t.Fatalf("cleanup=%v", objects.deleted)
	}
}

func TestCoverServiceDeleteIsIdempotentAndNeverDeletesGeneratedCover(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeCoverStore{content: Content{ID: 5, Status: ContentOffline, UpdatedAt: now}}
	objects := &fakeCoverStorage{}
	got, err := NewCoverService(store, objects, 1024).Delete(context.Background(), 5, now, nil)
	if err != nil || got.ID != 5 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if len(store.sets) != 1 || store.sets[0] != "" || len(objects.deleted) != 0 || !got.UpdatedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("sets=%v deleted=%v", store.sets, objects.deleted)
	}
	store.content.ManualCoverObjectKey = "classroom/covers/manual/5/x.webp"
	store.content.UpdatedAt = now
	got, err = NewCoverService(store, objects, 1024).Delete(context.Background(), 5, now, nil)
	if err != nil || got.ManualCoverObjectKey != "" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if len(objects.deleted) != 1 || objects.deleted[0] != "classroom/covers/manual/5/x.webp" {
		t.Fatalf("deleted=%v", objects.deleted)
	}
}

func TestCoverServiceEmptyDeleteStillUsesCAS(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeCoverStore{content: Content{ID: 9, UpdatedAt: now}, setErr: ErrConflict}
	_, err := NewCoverService(store, &fakeCoverStorage{}, 1024).Delete(context.Background(), 9, now, nil)
	if !errors.Is(err, ErrConflict) || len(store.sets) != 1 || store.sets[0] != "" {
		t.Fatalf("err=%v sets=%v", err, store.sets)
	}
}

func TestCoverServiceDeleteAcceptsAlreadyGoneAndKeepsDatabaseResultOnCleanupFailure(t *testing.T) {
	now := time.Now().UTC()
	for _, cleanupErr := range []error{storage.ErrAlreadyGone, errors.New("oss unavailable")} {
		store := &fakeCoverStore{content: Content{ID: 8, Status: ContentProcessing, UpdatedAt: now, ManualCoverObjectKey: "classroom/covers/manual/8/x.jpg"}}
		objects := &fakeCoverStorage{deleteErr: cleanupErr}
		got, err := NewCoverService(store, objects, 1024).Delete(context.Background(), 8, now, nil)
		if err != nil || got.ManualCoverObjectKey != "" {
			t.Fatalf("cleanup=%v got=%+v err=%v", cleanupErr, got, err)
		}
	}
}

func TestCoverServiceRejectsEmptyOversizedAndNonImageBodies(t *testing.T) {
	now := time.Now().UTC()
	for name, body := range map[string][]byte{"empty": nil, "oversized": bytes.Repeat([]byte{'a'}, 33), "non-image": []byte("not an image"), "broken-png": append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{'x'}, 20)...), "broken-jpeg": append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{'x'}, 20)...), "forged-webp": append([]byte("RIFFxxxxWEBP"), bytes.Repeat([]byte{'x'}, 8)...)} {
		store := &fakeCoverStore{content: Content{ID: 2, Status: ContentDraft, UpdatedAt: now}}
		objects := &fakeCoverStorage{}
		_, err := NewCoverService(store, objects, 32).Upload(context.Background(), 2, now, nil, "cover.jpg", bytes.NewReader(body))
		if !errors.Is(err, ErrInvalidCoverImage) {
			t.Errorf("%s err=%v", name, err)
		}
		if len(objects.uploads) != 0 {
			t.Errorf("%s uploaded", name)
		}
	}
}

func TestCoverServiceStorageUploadFailureIsTyped(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeCoverStore{content: Content{ID: 2, Status: ContentReady, UpdatedAt: now}}
	objects := &fakeCoverStorage{uploadErr: errors.New("secret oss cause")}
	_, err := NewCoverService(store, objects, 1024).Upload(context.Background(), 2, now, nil, "cover.png", bytes.NewReader(testPNG))
	if !errors.Is(err, ErrCoverStorageUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

func TestCoverServiceUsesFullWebPDecodeInsteadOfTrustingVP8XHeader(t *testing.T) {
	now := time.Now().UTC()
	forged := make([]byte, 30)
	copy(forged[0:4], "RIFF")
	binary.LittleEndian.PutUint32(forged[4:8], uint32(len(forged)-8))
	copy(forged[8:12], "WEBP")
	copy(forged[12:16], "VP8X")
	binary.LittleEndian.PutUint32(forged[16:20], 10)
	forged[24], forged[27] = 1, 1

	badStore := &fakeCoverStore{content: Content{ID: 12, Status: ContentReady, UpdatedAt: now}}
	badObjects := &fakeCoverStorage{}
	_, err := NewCoverService(badStore, badObjects, 1024).Upload(context.Background(), 12, now, nil, "forged.webp", bytes.NewReader(forged))
	if !errors.Is(err, ErrInvalidCoverImage) || len(badObjects.uploads) != 0 {
		t.Fatalf("forged VP8X accepted: err=%v uploads=%d", err, len(badObjects.uploads))
	}

	goodStore := &fakeCoverStore{content: Content{ID: 13, Status: ContentReady, UpdatedAt: now}}
	goodObjects := &fakeCoverStorage{}
	got, err := NewCoverService(goodStore, goodObjects, 1024).Upload(context.Background(), 13, now, nil, "real.webp", bytes.NewReader(testWebP))
	if err != nil || !strings.HasSuffix(got.ManualCoverObjectKey, ".webp") || len(goodObjects.uploads) != 1 || goodObjects.uploads[0].ContentType != "image/webp" {
		t.Fatalf("real WebP rejected: got=%+v err=%v uploads=%+v", got, err, goodObjects.uploads)
	}
}

func TestCoverServiceRejectsTruncatedAndOversizedDimensionImagesBeforeUpload(t *testing.T) {
	now := time.Now().UTC()
	validJPEG := encodeTestJPEG(t)
	oversizedPNG := append([]byte(nil), testPNG...)
	binary.BigEndian.PutUint32(oversizedPNG[16:20], 10_000)
	binary.BigEndian.PutUint32(oversizedPNG[20:24], 5_000)
	binary.BigEndian.PutUint32(oversizedPNG[29:33], crc32.ChecksumIEEE(oversizedPNG[12:29]))

	for name, body := range map[string][]byte{
		"truncated-png":   testPNG[:len(testPNG)-20],
		"truncated-jpeg":  validJPEG[:len(validJPEG)/2],
		"too-many-pixels": oversizedPNG,
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeCoverStore{content: Content{ID: 21, UpdatedAt: now}}
			objects := &fakeCoverStorage{}
			_, err := NewCoverService(store, objects, 2048).Upload(context.Background(), 21, now, nil, name, bytes.NewReader(body))
			if !errors.Is(err, ErrInvalidCoverImage) || len(objects.uploads) != 0 {
				t.Fatalf("err=%v uploads=%d", err, len(objects.uploads))
			}
		})
	}

	for name, body := range map[string][]byte{"png": testPNG, "jpeg": validJPEG, "webp": testWebP} {
		t.Run("valid-"+name, func(t *testing.T) {
			store := &fakeCoverStore{content: Content{ID: 22, UpdatedAt: now}}
			objects := &fakeCoverStorage{}
			if _, err := NewCoverService(store, objects, 4096).Upload(context.Background(), 22, now, nil, name, bytes.NewReader(body)); err != nil || len(objects.uploads) != 1 {
				t.Fatalf("err=%v uploads=%d", err, len(objects.uploads))
			}
		})
	}
}

func encodeTestJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestCoverServiceCleanupSurvivesCanceledRequestContext(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name  string
		store *fakeCoverStore
		run   func(*CoverService, context.Context) error
	}{
		{"upload-cas-rollback", &fakeCoverStore{content: Content{ID: 31, UpdatedAt: now}, setErr: ErrConflict}, func(s *CoverService, ctx context.Context) error {
			_, err := s.Upload(ctx, 31, now, nil, "x.png", bytes.NewReader(testPNG))
			return err
		}},
		{"replace-old-cover", &fakeCoverStore{content: Content{ID: 32, UpdatedAt: now, ManualCoverObjectKey: "old"}}, func(s *CoverService, ctx context.Context) error {
			_, err := s.Upload(ctx, 32, now, nil, "x.png", bytes.NewReader(testPNG))
			return err
		}},
		{"delete-manual-cover", &fakeCoverStore{content: Content{ID: 33, UpdatedAt: now, ManualCoverObjectKey: "old"}}, func(s *CoverService, ctx context.Context) error { _, err := s.Delete(ctx, 33, now, nil); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			objects := &fakeCoverStorage{}
			_ = tc.run(NewCoverService(tc.store, objects, 4096), ctx)
			if len(objects.deleteCtxErrs) != 1 || objects.deleteCtxErrs[0] != nil {
				t.Fatalf("cleanup contexts=%v deleted=%v", objects.deleteCtxErrs, objects.deleted)
			}
		})
	}
}

func TestCoverServiceCleanupLogsDoNotExposeObjectKeys(t *testing.T) {
	key := "classroom/covers/manual/34/private.png"
	oldOutput := log.Writer()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(oldOutput) })

	NewCoverService(&fakeCoverStore{}, nil, 1024).cleanupObject(context.Background(), 34, "delete_manual", key)
	NewCoverService(&fakeCoverStore{}, &fakeCoverStorage{deleteErr: errors.New("storage unavailable")}, 1024).cleanupObject(context.Background(), 34, "delete_manual", key)

	got := logs.String()
	if !strings.Contains(got, "deleter unavailable") || !strings.Contains(got, "cleanup failed") || strings.Contains(got, key) {
		t.Fatalf("unexpected cleanup logs: %s", got)
	}
}

type fakeGeneratedCoverExtractor struct {
	key    string
	err    error
	calls  int
	ratio  CoverAspectRatio
	object string
}

func (f *fakeGeneratedCoverExtractor) Extract(_ context.Context, objectKey string, _ int64, ratio CoverAspectRatio) (string, error) {
	f.calls++
	f.object, f.ratio = objectKey, ratio
	return f.key, f.err
}

func TestCoverServiceChangesAspectRatioByReplacingGeneratedCoverAfterCAS(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	actor := int64(42)
	store := &fakeCoverStore{content: Content{ID: 40, ContentType: ContentVideo, Status: ContentPublished, MediaAssetID: classroomTestPtrI64(90), ManualCoverObjectKey: "classroom/covers/manual/40/keep.webp", CoverAspectRatio: CoverAspectRatio16x9, UpdatedAt: now}, media: MediaAsset{ID: 90, ObjectKey: "classroom/media/40/video.mp4", CoverObjectKey: "classroom/covers/generated/40/old.jpg"}}
	objects := &fakeCoverStorage{}
	extractor := &fakeGeneratedCoverExtractor{key: "classroom/covers/generated/40/new.jpg"}
	svc := NewCoverService(store, objects, 1024)
	svc.extractor = extractor
	got, err := svc.UpdateSettings(context.Background(), 40, CoverAspectRatio9x16, now, &actor)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ContentPublished || got.ManualCoverObjectKey != "classroom/covers/manual/40/keep.webp" || got.CoverAspectRatio != CoverAspectRatio9x16 {
		t.Fatalf("updated=%+v", got)
	}
	if extractor.calls != 1 || extractor.object != "classroom/media/40/video.mp4" || extractor.ratio != CoverAspectRatio9x16 {
		t.Fatalf("extractor=%+v", extractor)
	}
	if store.media.CoverObjectKey != extractor.key || len(objects.deleted) != 1 || objects.deleted[0] != "classroom/covers/generated/40/old.jpg" {
		t.Fatalf("media=%+v deleted=%v", store.media, objects.deleted)
	}
}

func TestCoverServiceCleansNewGeneratedCoverWhenSettingsCASFails(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeCoverStore{content: Content{ID: 41, ContentType: ContentVideo, MediaAssetID: classroomTestPtrI64(91), CoverAspectRatio: CoverAspectRatio16x9, UpdatedAt: now}, media: MediaAsset{ID: 91, ObjectKey: "video.mp4"}, settingsErr: ErrConflict}
	objects := &fakeCoverStorage{}
	svc := NewCoverService(store, objects, 1024)
	svc.extractor = &fakeGeneratedCoverExtractor{key: "classroom/covers/generated/41/new.jpg"}
	_, err := svc.UpdateSettings(context.Background(), 41, CoverAspectRatio1x1, now, nil)
	if !errors.Is(err, ErrConflict) || len(objects.deleted) != 1 || objects.deleted[0] != "classroom/covers/generated/41/new.jpg" {
		t.Fatalf("err=%v cleanup=%v", err, objects.deleted)
	}
}

func TestCoverServiceChangesAudioAspectRatioWithoutGeneratedMedia(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeCoverStore{content: Content{ID: 42, ContentType: ContentAudio, Status: ContentOffline, CoverAspectRatio: CoverAspectRatio16x9, UpdatedAt: now}}
	svc := NewCoverService(store, &fakeCoverStorage{}, 1024)
	got, err := svc.UpdateSettings(context.Background(), 42, CoverAspectRatio1x1, now, nil)
	if err != nil || got.Status != ContentOffline || got.CoverAspectRatio != CoverAspectRatio1x1 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
