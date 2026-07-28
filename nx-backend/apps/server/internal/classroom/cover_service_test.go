package classroom

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/storage"
)

var testPNG, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")

type fakeCoverStore struct {
	content Content
	setErr  error
	sets    []string
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

type fakeCoverStorage struct {
	uploadErr error
	deleteErr error
	uploads   []storage.UploadInput
	deleted   []string
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
func (f *fakeCoverStorage) DeleteObject(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
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
	if len(store.sets) != 0 || len(objects.deleted) != 0 {
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
