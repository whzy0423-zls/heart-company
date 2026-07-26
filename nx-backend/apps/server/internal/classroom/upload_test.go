package classroom

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/storage"
)

type fakeMultipartStorage struct {
	initiated                                storage.InitiateMultipartInput
	signed                                   storage.SignPartInput
	completed                                storage.CompleteMultipartInput
	aborted                                  storage.AbortMultipartInput
	head                                     storage.ObjectMetadata
	parts                                    []storage.MultipartPart
	initiateCalls, completeCalls, abortCalls int
}

func (f *fakeMultipartStorage) InitiateMultipart(_ context.Context, in storage.InitiateMultipartInput) (storage.InitiateMultipartResult, error) {
	f.initiated = in
	f.initiateCalls++
	return storage.InitiateMultipartResult{UploadID: "oss-upload-1"}, nil
}
func (f *fakeMultipartStorage) SignMultipartPart(_ context.Context, in storage.SignPartInput) (storage.SignPartResult, error) {
	f.signed = in
	return storage.SignPartResult{URL: "https://oss.test/part", ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (f *fakeMultipartStorage) CompleteMultipart(_ context.Context, in storage.CompleteMultipartInput) (storage.CompleteMultipartResult, error) {
	f.completed = in
	f.completeCalls++
	return storage.CompleteMultipartResult{ETag: "final-etag"}, nil
}
func (f *fakeMultipartStorage) AbortMultipart(_ context.Context, in storage.AbortMultipartInput) error {
	f.aborted = in
	f.abortCalls++
	return nil
}
func (f *fakeMultipartStorage) ListMultipartParts(context.Context, storage.ListPartsInput) ([]storage.MultipartPart, error) {
	return f.parts, nil
}
func (f *fakeMultipartStorage) HeadObject(context.Context, string) (storage.ObjectMetadata, error) {
	return f.head, nil
}

type fakeUploadRepo struct {
	content Content
	task    UploadTask
	media   MediaAsset
	nextID  int64
}

func (r *fakeUploadRepo) GetContent(context.Context, int64) (Content, error) {
	if r.content.ID == 0 {
		return Content{}, ErrNotFound
	}
	return r.content, nil
}
func (r *fakeUploadRepo) FindUploadTaskByContent(context.Context, int64) (UploadTask, error) {
	if r.task.ID == 0 {
		return UploadTask{}, ErrNotFound
	}
	return r.task, nil
}
func (r *fakeUploadRepo) CreateUploadTask(_ context.Context, v UploadTask) (UploadTask, error) {
	r.nextID++
	v.ID = r.nextID
	r.task = v
	return v, nil
}
func (r *fakeUploadRepo) GetUploadTask(context.Context, int64) (UploadTask, error) {
	if r.task.ID == 0 {
		return UploadTask{}, ErrNotFound
	}
	return r.task, nil
}
func (r *fakeUploadRepo) SaveUploadTask(_ context.Context, v UploadTask) (UploadTask, error) {
	r.task = v
	return v, nil
}
func (r *fakeUploadRepo) CreateMediaAsset(_ context.Context, v MediaAsset) (MediaAsset, error) {
	v.ID = 91
	r.media = v
	return v, nil
}
func (r *fakeUploadRepo) AttachMediaToContent(_ context.Context, contentID int64, media MediaAsset) (Content, error) {
	r.content.MediaAssetID = &media.ID
	r.content.DurationSeconds = media.DurationSeconds
	r.content.Status = ContentReady
	return r.content, nil
}

type fakeProbe struct {
	result MediaProbeResult
	err    error
}

func (p fakeProbe) Probe(context.Context, ProbeInput) (MediaProbeResult, error) {
	return p.result, p.err
}

func newUploadService(repo *fakeUploadRepo, store *fakeMultipartStorage, now time.Time) *UploadService {
	return NewUploadService(repo, store, fakeProbe{result: MediaProbeResult{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", DurationSeconds: 125, Width: 1920, Height: 1080}}, UploadConfig{Bucket: "private-classroom", Prefix: "classroom", PartSize: 5 << 20, MaxParts: 10000, CredentialTTL: 15 * time.Minute, TaskTTL: time.Hour, MaxVideoBytes: 2 << 30, MaxAudioBytes: 512 << 20, MaxAttempts: 3}, func() time.Time { return now })
}

func TestClassroomUploadInitiateGeneratesPrivateObjectKeyAndBindsDraft(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	repo := &fakeUploadRepo{content: Content{ID: 7, Title: "lesson", ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}}
	store := &fakeMultipartStorage{}
	svc := newUploadService(repo, store, now)
	got, err := svc.Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "../teacher class.mp4", ContentType: "video/mp4", SizeBytes: 30 << 20, Checksum: "sha256:abc"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Task.CreatorID != 42 || got.Task.ContentID != 7 || got.Task.AttemptCount != 1 {
		t.Fatalf("unexpected task: %+v", got.Task)
	}
	if !strings.HasPrefix(got.Task.ObjectKey, "classroom/video/2026/07/26/content-7/") || strings.Contains(got.Task.ObjectKey, "..") {
		t.Fatalf("unsafe server object key %q", got.Task.ObjectKey)
	}
	if got.Task.ObjectKey != store.initiated.ObjectKey || store.initiated.ContentType != "video/mp4" || store.initiated.Checksum != "sha256:abc" {
		t.Fatalf("storage initiate mismatch: %+v", store.initiated)
	}
}

func TestClassroomUploadRejectsNonDraftDuplicateAndOversize(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		repo *fakeUploadRepo
		size int64
	}{
		{"published", &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentPublished, AccessLevel: AccessPublic}}, 10},
		{"active duplicate", &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 3, Status: UploadUploading}}, 10},
		{"oversize", &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}}, (2 << 30) + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newUploadService(tc.repo, &fakeMultipartStorage{}, now).Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "a.mp4", ContentType: "video/mp4", SizeBytes: tc.size, Checksum: "sha256:x"})
			if err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestClassroomUploadSignChecksOwnershipExpiryPartAndAttempts(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 20 << 20, PartSize: 5 << 20, MaxParts: 4, Status: UploadInitiated, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	store := &fakeMultipartStorage{}
	svc := newUploadService(repo, store, now)
	signed, err := svc.SignPart(context.Background(), 1, 42, 2)
	if err != nil {
		t.Fatal(err)
	}
	if signed.PartNumber != 2 || store.signed.UploadID != "u" {
		t.Fatalf("bad signed part %+v %+v", signed, store.signed)
	}
	if _, err := svc.SignPart(context.Background(), 1, 99, 2); !errors.Is(err, ErrUploadOwnership) {
		t.Fatalf("ownership: %v", err)
	}
	if _, err := svc.SignPart(context.Background(), 1, 42, 5); !errors.Is(err, ErrInvalidUploadPart) {
		t.Fatalf("part: %v", err)
	}
	repo.task.ExpiresAt = now.Add(-time.Second)
	if _, err := svc.SignPart(context.Background(), 1, 42, 1); !errors.Is(err, ErrUploadExpired) {
		t.Fatalf("expiry: %v", err)
	}
}

func TestClassroomUploadCompleteValidatesPartsHeadChecksumAndProbe(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "classroom/video/x.mp4", ExpectedSize: 10, Checksum: "sha256:abc", PartSize: 5, MaxParts: 2, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	store := &fakeMultipartStorage{parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e1", Size: 5}, {PartNumber: 2, ETag: "e2", Size: 5}}, head: storage.ObjectMetadata{ObjectKey: "classroom/video/x.mp4", ETag: "final-etag", Size: 10, Checksum: "sha256:abc", ContentType: "video/mp4"}}
	svc := newUploadService(repo, store, now)
	got, err := svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e1"}, {PartNumber: 2, ETag: "e2"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Task.Status != UploadCompleted || got.Media.StorageStatus != MediaReady || got.Content.Status != ContentReady || got.Media.DurationSeconds != 125 {
		t.Fatalf("not ready: %+v %+v %+v", got.Task, got.Media, got.Content)
	}
	again, err := svc.Complete(context.Background(), 1, 42, nil)
	if err != nil || again.Task.Status != UploadCompleted || store.completeCalls != 1 {
		t.Fatalf("complete not idempotent calls=%d err=%v", store.completeCalls, err)
	}
}

func TestClassroomUploadCompleteMarksFailedOnHeadOrProbeMismatch(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name  string
		head  storage.ObjectMetadata
		probe fakeProbe
	}{
		{"size", storage.ObjectMetadata{ETag: "x", Size: 9, Checksum: "sha256:abc", ContentType: "video/mp4"}, fakeProbe{}},
		{"checksum", storage.ObjectMetadata{ETag: "x", Size: 10, Checksum: "sha256:wrong", ContentType: "video/mp4"}, fakeProbe{}},
		{"codec", storage.ObjectMetadata{ETag: "x", Size: 10, Checksum: "sha256:abc", ContentType: "video/mp4"}, fakeProbe{result: MediaProbeResult{Container: "mp4", VideoCodec: "hevc", AudioCodec: "aac"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, Checksum: "sha256:abc", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 2}}
			st := &fakeMultipartStorage{parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: tc.head}
			svc := NewUploadService(repo, st, tc.probe, UploadConfig{Bucket: "b", PartSize: 10, MaxParts: 1, TaskTTL: time.Hour, MaxVideoBytes: 100, MaxAudioBytes: 100, MaxAttempts: 3}, func() time.Time { return now })
			if _, err := svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}}); err == nil {
				t.Fatal("expected mismatch")
			}
			if repo.task.Status != UploadFailed || repo.task.AttemptCount != 3 || repo.task.FailureReason == "" {
				t.Fatalf("failure state not persisted: %+v", repo.task)
			}
		})
	}
}

func TestClassroomUploadAbortIsIdempotentAndOwned(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, PartSize: 5, MaxParts: 2, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	st := &fakeMultipartStorage{}
	svc := newUploadService(repo, st, now)
	if _, err := svc.Abort(context.Background(), 1, 99); !errors.Is(err, ErrUploadOwnership) {
		t.Fatalf("ownership %v", err)
	}
	got, err := svc.Abort(context.Background(), 1, 42)
	if err != nil || got.Status != UploadAborted || got.CleanupStatus != "clean" {
		t.Fatalf("abort %+v %v", got, err)
	}
	if _, err := svc.Abort(context.Background(), 1, 42); err != nil || st.abortCalls != 1 {
		t.Fatalf("idempotent abort calls=%d err=%v", st.abortCalls, err)
	}
}

func TestValidateMediaProbeSupportsRequiredFormats(t *testing.T) {
	valid := []struct {
		typ   ContentType
		probe MediaProbeResult
	}{{ContentVideo, MediaProbeResult{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", DurationSeconds: 1}}, {ContentAudio, MediaProbeResult{Container: "mp3", AudioCodec: "mp3", DurationSeconds: 1}}, {ContentAudio, MediaProbeResult{Container: "m4a", AudioCodec: "aac", DurationSeconds: 1}}}
	for _, v := range valid {
		if err := ValidateMediaProbe(v.typ, v.probe); err != nil {
			t.Errorf("expected valid %+v: %v", v, err)
		}
	}
	if err := ValidateMediaProbe(ContentVideo, MediaProbeResult{Container: "mp4", VideoCodec: "hevc", AudioCodec: "aac"}); err == nil {
		t.Fatal("expected HEVC rejection")
	}
	if err := ValidateMediaProbe(ContentAudio, MediaProbeResult{Container: "wav", AudioCodec: "pcm"}); err == nil {
		t.Fatal("expected WAV rejection")
	}
}

func TestClassroomUploadRejectsOSSCompletedChecksumMismatch(t *testing.T) {
	task := UploadTask{ExpectedSize: 10, Checksum: "crc64:123"}
	err := validateHead(task, storage.CompleteMultipartResult{ETag: "etag", Checksum: "crc64:999"}, storage.ObjectMetadata{ETag: "etag", Size: 10, Checksum: "crc64:123"})
	if err == nil {
		t.Fatal("expected OSS completed checksum mismatch")
	}
}
