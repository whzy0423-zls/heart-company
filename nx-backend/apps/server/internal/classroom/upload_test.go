package classroom

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/storage"
)

type fakeMultipartStorage struct {
	mu                                       sync.Mutex
	initiated                                storage.InitiateMultipartInput
	signed                                   storage.SignPartInput
	completed                                storage.CompleteMultipartInput
	aborted                                  storage.AbortMultipartInput
	head                                     storage.ObjectMetadata
	parts                                    []storage.MultipartPart
	initiateCalls, completeCalls, abortCalls int
	completeDelay                            time.Duration
	deleteCalls                              []string
	abortErr                                 error
	initiateErr                              error
	initiateHook                             func()
	deleteHook                               func(context.Context, string) error
	deleteErr                                error
}

func (f *fakeMultipartStorage) InitiateMultipart(_ context.Context, in storage.InitiateMultipartInput) (storage.InitiateMultipartResult, error) {
	f.mu.Lock()
	f.initiated = in
	f.initiateCalls++
	hook, err := f.initiateHook, f.initiateErr
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	return storage.InitiateMultipartResult{UploadID: "oss-upload-1"}, err
}
func (f *fakeMultipartStorage) SignMultipartPart(_ context.Context, in storage.SignPartInput) (storage.SignPartResult, error) {
	f.signed = in
	return storage.SignPartResult{URL: "https://oss.test/part", ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (f *fakeMultipartStorage) CompleteMultipart(_ context.Context, in storage.CompleteMultipartInput) (storage.CompleteMultipartResult, error) {
	f.completed = in
	f.completeCalls++
	if f.completeDelay > 0 {
		time.Sleep(f.completeDelay)
	}
	return storage.CompleteMultipartResult{ETag: "final-etag"}, nil
}
func (f *fakeMultipartStorage) AbortMultipart(_ context.Context, in storage.AbortMultipartInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aborted = in
	f.abortCalls++
	return f.abortErr
}
func (f *fakeMultipartStorage) DeleteObject(ctx context.Context, key string) error {
	f.deleteCalls = append(f.deleteCalls, key)
	if f.deleteHook != nil {
		return f.deleteHook(ctx, key)
	}
	return f.deleteErr
}
func (f *fakeMultipartStorage) ListMultipartParts(context.Context, storage.ListPartsInput) ([]storage.MultipartPart, error) {
	return f.parts, nil
}
func (f *fakeMultipartStorage) HeadObject(context.Context, string) (storage.ObjectMetadata, error) {
	return f.head, nil
}

type fakeUploadRepo struct {
	mu              sync.Mutex
	content         Content
	task            UploadTask
	media           MediaAsset
	nextID          int64
	mediaStatuses   []MediaStatus
	contentStatuses []ContentStatus
	expired         []UploadTask
	finalizeCalls   int
	updateErr       error
	confirmErr      error
	finishErr       error
	reserveCalls    int
	confirmCalls    int
	rejectCanceled  bool
}

func (r *fakeUploadRepo) rejectCanceledContext(ctx context.Context) error {
	if r.rejectCanceled {
		return ctx.Err()
	}
	return nil
}

func (r *fakeUploadRepo) GetContent(ctx context.Context, _ int64) (Content, error) {
	if err := r.rejectCanceledContext(ctx); err != nil {
		return Content{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.content.ID == 0 {
		return Content{}, ErrNotFound
	}
	return r.content, nil
}
func (r *fakeUploadRepo) FindUploadTaskByContent(context.Context, int64) (UploadTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID == 0 {
		return UploadTask{}, ErrNotFound
	}
	return r.task, nil
}
func (r *fakeUploadRepo) SaveUploadTask(ctx context.Context, v UploadTask) (UploadTask, error) {
	if err := r.rejectCanceledContext(ctx); err != nil {
		return UploadTask{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.task = v
	return v, nil
}
func (r *fakeUploadRepo) UpdateUploadProgress(_ context.Context, id, creatorID int64, completedParts int, completedBytes int64) (UploadTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID != id {
		return UploadTask{}, ErrNotFound
	}
	if r.task.CreatorID != creatorID {
		return UploadTask{}, ErrUploadOwnership
	}
	r.task.CompletedParts = completedParts
	r.task.CompletedBytes = completedBytes
	if r.task.Status == UploadInitiated {
		r.task.Status = UploadUploading
	}
	return r.task, nil
}
func (r *fakeUploadRepo) ReserveUploadInitiation(_ context.Context, v UploadTask, expected *UploadTask) (UploadTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reserveCalls++
	if expected == nil {
		if r.task.ID != 0 {
			return r.task, false, nil
		}
		r.nextID++
		v.ID = r.nextID
		v.CreatedAt = time.Now()
		v.UpdatedAt = v.CreatedAt
		r.task = v
		return v, true, nil
	}
	if r.task.ID != expected.ID || r.task.Status != expected.Status || !r.task.UpdatedAt.Equal(expected.UpdatedAt) {
		return r.task, false, nil
	}
	v.ID = expected.ID
	v.CreatedAt = expected.CreatedAt
	v.UpdatedAt = time.Now()
	r.task = v
	return v, true, nil
}
func (r *fakeUploadRepo) ConfirmUploadInitiation(_ context.Context, expected UploadTask, uploadID string) (UploadTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.confirmCalls++
	if r.confirmErr != nil {
		return UploadTask{}, false, r.confirmErr
	}
	if r.task.ID != expected.ID || r.task.Status != UploadInitiating || !r.task.UpdatedAt.Equal(expected.UpdatedAt) {
		return r.task, false, nil
	}
	r.task.OSSUploadID = uploadID
	r.task.Status = UploadInitiated
	r.task.UpdatedAt = time.Now()
	return r.task, true, nil
}
func (r *fakeUploadRepo) FailUploadInitiation(_ context.Context, expected UploadTask, uploadID, cleanupStatus, reason string) (UploadTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID != expected.ID || r.task.Status != UploadInitiating || !r.task.UpdatedAt.Equal(expected.UpdatedAt) {
		return r.task, false, nil
	}
	if uploadID != "" {
		r.task.OSSUploadID = uploadID
	}
	r.task.Status = UploadFailed
	r.task.CleanupStatus = cleanupStatus
	r.task.FailureReason = reason
	r.task.UpdatedAt = time.Now()
	return r.task, true, nil
}
func (r *fakeUploadRepo) MarkUploadUploading(_ context.Context, id int64) (UploadTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID != id {
		return UploadTask{}, ErrNotFound
	}
	if r.task.Status == UploadInitiated {
		r.task.Status = UploadUploading
	}
	return r.task, nil
}
func (r *fakeUploadRepo) ClaimUploadCleanup(_ context.Context, expected UploadTask, status UploadStatus) (UploadTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID != expected.ID || r.task.Status != expected.Status || !r.task.UpdatedAt.Equal(expected.UpdatedAt) {
		return UploadTask{}, false, nil
	}
	r.task.Status = UploadCleaning
	r.task.CleanupStatus = "pending"
	r.task.UpdatedAt = time.Now()
	return r.task, true, nil
}
func (r *fakeUploadRepo) FinishUploadCleanup(_ context.Context, expected UploadTask, status UploadStatus, cleanupStatus, failureReason string) (UploadTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finishErr != nil {
		return UploadTask{}, false, r.finishErr
	}
	if r.task.ID != expected.ID || r.task.Status != UploadCleaning || !r.task.UpdatedAt.Equal(expected.UpdatedAt) {
		return UploadTask{}, false, nil
	}
	r.task.Status = status
	r.task.CleanupStatus = cleanupStatus
	r.task.FailureReason = failureReason
	r.task.UpdatedAt = time.Now()
	return r.task, true, nil
}
func (r *fakeUploadRepo) ClaimUploadCompletion(_ context.Context, id int64) (UploadTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID != id {
		return UploadTask{}, false, ErrNotFound
	}
	if r.task.Status != UploadInitiated && r.task.Status != UploadUploading {
		return r.task, false, nil
	}
	r.task.Status = UploadCompleting
	return r.task, true, nil
}
func (r *fakeUploadRepo) ClaimUploadAbort(_ context.Context, id int64) (UploadTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID != id {
		return UploadTask{}, false, ErrNotFound
	}
	if r.task.Status != UploadInitiated && r.task.Status != UploadUploading && r.task.Status != UploadFailed {
		return r.task, false, nil
	}
	r.task.Status = UploadAborted
	r.task.CleanupStatus = "pending"
	return r.task, true, nil
}

func (r *fakeUploadRepo) GetMediaAsset(ctx context.Context, _ int64) (MediaAsset, error) {
	if err := r.rejectCanceledContext(ctx); err != nil {
		return MediaAsset{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.media, nil
}
func (r *fakeUploadRepo) CreateMediaAsset(ctx context.Context, v MediaAsset) (MediaAsset, error) {
	if err := r.rejectCanceledContext(ctx); err != nil {
		return MediaAsset{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v.ID = 91
	r.media = v
	r.mediaStatuses = append(r.mediaStatuses, v.StorageStatus)
	return v, nil
}
func (r *fakeUploadRepo) UpdateMediaAsset(ctx context.Context, v MediaAsset) (MediaAsset, error) {
	if err := r.rejectCanceledContext(ctx); err != nil {
		return MediaAsset{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.updateErr != nil {
		return MediaAsset{}, r.updateErr
	}
	r.media = v
	r.mediaStatuses = append(r.mediaStatuses, v.StorageStatus)
	return v, nil
}
func (r *fakeUploadRepo) SetContentMediaState(ctx context.Context, _ int64, mediaID *int64, status ContentStatus, duration int) (Content, error) {
	if err := r.rejectCanceledContext(ctx); err != nil {
		return Content{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.content.MediaAssetID = mediaID
	r.content.Status = status
	r.content.DurationSeconds = duration
	r.contentStatuses = append(r.contentStatuses, status)
	return r.content, nil
}
func (r *fakeUploadRepo) FinalizeUpload(_ context.Context, task UploadTask, media MediaAsset) (UploadTask, MediaAsset, Content, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeCalls++
	r.task = task
	r.media = media
	r.mediaStatuses = append(r.mediaStatuses, media.StorageStatus)
	r.content.MediaAssetID = &media.ID
	r.content.Status = ContentReady
	r.content.DurationSeconds = media.DurationSeconds
	r.contentStatuses = append(r.contentStatuses, ContentReady)
	return task, media, r.content, nil
}
func (r *fakeUploadRepo) ListExpiredUploadTasks(context.Context, int) ([]UploadTask, error) {
	return r.expired, nil
}

type fakeProbe struct {
	result MediaProbeResult
	err    error
}

func (p fakeProbe) Probe(context.Context, ProbeInput) (MediaProbeResult, error) {
	return p.result, p.err
}

func newUploadService(repo *fakeUploadRepo, store *fakeMultipartStorage, now time.Time) *UploadService {
	return NewUploadService(repo, store, fakeProbe{result: MediaProbeResult{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", DurationSeconds: 125, Width: 1920, Height: 1080}}, UploadConfig{Bucket: "private-classroom", Prefix: "classroom", PartSize: 5 << 20, MaxParts: 10000, CredentialTTL: 15 * time.Minute, TaskTTL: time.Hour, MaxVideoBytes: 2 << 30, MaxAudioBytes: 512 << 20, MaxAttempts: 3}, func() time.Time { return now }).WithCoverExtractor(&fakeCoverExtractor{key: "classroom/covers/default.jpg"})
}

func TestClassroomUploadInitiateGeneratesPrivateObjectKeyAndBindsDraft(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	repo := &fakeUploadRepo{content: Content{ID: 7, Title: "lesson", ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}}
	store := &fakeMultipartStorage{}
	svc := newUploadService(repo, store, now)
	got, err := svc.Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "../teacher class.mp4", ContentType: "video/mp4", SizeBytes: 30 << 20, Checksum: "crc64:123"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Task.CreatorID != 42 || got.Task.ContentID != 7 || got.Task.AttemptCount != 1 {
		t.Fatalf("unexpected task: %+v", got.Task)
	}
	if !strings.HasPrefix(got.Task.ObjectKey, "classroom/video/2026/07/26/content-7/") || strings.Contains(got.Task.ObjectKey, "..") {
		t.Fatalf("unsafe server object key %q", got.Task.ObjectKey)
	}
	if got.Task.ObjectKey != store.initiated.ObjectKey || store.initiated.ContentType != "video/mp4" || store.initiated.Checksum != "crc64:123" {
		t.Fatalf("storage initiate mismatch: %+v", store.initiated)
	}
}

func TestClassroomUploadConcurrentInitiateReservesBeforeOSS(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, Title: "lesson", ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}}
	entered, release := make(chan struct{}), make(chan struct{})
	store := &fakeMultipartStorage{initiateHook: func() {
		close(entered)
		<-release
	}}
	svc := newUploadService(repo, store, now)
	firstDone := make(chan error, 1)
	go func() {
		_, err := svc.Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "a.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"})
		firstDone <- err
	}()
	<-entered
	_, secondErr := svc.Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "b.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"})
	if !errors.Is(secondErr, ErrUploadConflict) {
		t.Fatalf("second initiate err=%v", secondErr)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	calls := store.initiateCalls
	store.mu.Unlock()
	if calls != 1 {
		t.Fatalf("OSS initiate calls=%d", calls)
	}
}

func TestClassroomUploadStaleConfirmDoesNotOverwriteAndAbortsOSS(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, Title: "lesson", ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}}
	store := &fakeMultipartStorage{}
	store.initiateHook = func() {
		repo.mu.Lock()
		repo.task.Status = UploadCleaning
		repo.task.UpdatedAt = repo.task.UpdatedAt.Add(time.Second)
		repo.mu.Unlock()
	}
	_, err := newUploadService(repo, store, now).Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "a.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"})
	if !errors.Is(err, ErrUploadConflict) {
		t.Fatalf("initiate err=%v", err)
	}
	repo.mu.Lock()
	status, uploadID := repo.task.Status, repo.task.OSSUploadID
	repo.mu.Unlock()
	if status != UploadCleaning || uploadID == "oss-upload-1" {
		t.Fatalf("stale confirm overwrote task: %+v", repo.task)
	}
	store.mu.Lock()
	abortCalls := store.abortCalls
	store.mu.Unlock()
	if abortCalls != 1 {
		t.Fatalf("abort calls=%d", abortCalls)
	}
}

func TestClassroomUploadInitiateFailureReleasesReservationByCAS(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, Title: "lesson", ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}}
	store := &fakeMultipartStorage{initiateErr: errors.New("oss down")}
	_, err := newUploadService(repo, store, now).Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "a.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"})
	if err == nil {
		t.Fatal("expected initiate failure")
	}
	repo.mu.Lock()
	task := repo.task
	repo.mu.Unlock()
	if task.Status != UploadFailed || task.CleanupStatus != "cleaned" {
		t.Fatalf("reservation not compensated: %+v", task)
	}
}

func TestClassroomUploadConfirmFailurePersistsRealUploadForMaintenanceWhenAbortFails(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, Title: "lesson", ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, confirmErr: errors.New("db unavailable")}
	store := &fakeMultipartStorage{abortErr: errors.New("oss abort unavailable")}
	_, err := newUploadService(repo, store, now).Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "a.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"})
	if err == nil {
		t.Fatal("expected confirm failure")
	}
	repo.mu.Lock()
	task := repo.task
	repo.mu.Unlock()
	if task.Status != UploadFailed || task.CleanupStatus != "pending" || task.OSSUploadID != "oss-upload-1" {
		t.Fatalf("multipart cleanup reference lost: %+v", task)
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
			_, err := newUploadService(tc.repo, &fakeMultipartStorage{}, now).Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "a.mp4", ContentType: "video/mp4", SizeBytes: tc.size, Checksum: "crc64:123"})
			if err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestClassroomUploadReportProgressPersistsOnlyMonotonicOwnedProgress(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 100, Checksum: "crc64:123", CompletedParts: 1, CompletedBytes: 25, PartSize: 25, MaxParts: 4, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	svc := newUploadService(repo, &fakeMultipartStorage{}, now)
	got, err := svc.ReportProgress(context.Background(), 1, 42, 2, 50)
	if err != nil || got.CompletedParts != 2 || got.CompletedBytes != 50 {
		t.Fatalf("progress=%+v err=%v", got, err)
	}
	if _, err := svc.ReportProgress(context.Background(), 1, 42, 1, 40); !errors.Is(err, ErrInvalidUploadProgress) {
		t.Fatalf("regression err=%v", err)
	}
	if _, err := svc.ReportProgress(context.Background(), 1, 99, 3, 75); !errors.Is(err, ErrUploadOwnership) {
		t.Fatalf("ownership err=%v", err)
	}
	if _, err := svc.ReportProgress(context.Background(), 1, 42, 5, 101); !errors.Is(err, ErrInvalidUploadProgress) {
		t.Fatalf("overflow err=%v", err)
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
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "classroom/video/x.mp4", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 5, MaxParts: 2, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	store := &fakeMultipartStorage{parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e1", Size: 5}, {PartNumber: 2, ETag: "e2", Size: 5}}, head: storage.ObjectMetadata{ObjectKey: "classroom/video/x.mp4", ETag: "final-etag", Size: 10, Checksum: "crc64:123", ContentType: "video/mp4"}}
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
		{"size", storage.ObjectMetadata{ETag: "x", Size: 9, Checksum: "crc64:123", ContentType: "video/mp4"}, fakeProbe{}},
		{"checksum", storage.ObjectMetadata{ETag: "x", Size: 10, Checksum: "sha256:wrong", ContentType: "video/mp4"}, fakeProbe{}},
		{"codec", storage.ObjectMetadata{ETag: "x", Size: 10, Checksum: "crc64:123", ContentType: "video/mp4"}, fakeProbe{result: MediaProbeResult{Container: "mp4", VideoCodec: "hevc", AudioCodec: "aac"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 2}}
			st := &fakeMultipartStorage{parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: tc.head}
			svc := NewUploadService(repo, st, tc.probe, UploadConfig{Bucket: "b", PartSize: 10, MaxParts: 1, TaskTTL: time.Hour, MaxVideoBytes: 100, MaxAudioBytes: 100, MaxAttempts: 3}, func() time.Time { return now })
			if _, err := svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}}); err == nil {
				t.Fatal("expected mismatch")
			}
			if repo.task.Status != UploadFailed || repo.task.AttemptCount != 2 || repo.task.FailureReason == "" {
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
	if err != nil || got.Status != UploadAborted || got.CleanupStatus != "cleaned" {
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

func TestClassroomUploadAbortUsesSchemaCleanupStatusCleaned(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, PartSize: 5, MaxParts: 2, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	got, err := newUploadService(repo, &fakeMultipartStorage{}, now).Abort(context.Background(), 1, 42)
	if err != nil {
		t.Fatal(err)
	}
	if got.CleanupStatus != "cleaned" {
		t.Fatalf("cleanup status = %q", got.CleanupStatus)
	}
}

func TestClassroomUploadMaxAttemptsCountsActualAttemptsAcrossFailureRetryFailure(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}}
	st := &fakeMultipartStorage{parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: storage.ObjectMetadata{ETag: "e", Size: 9, Checksum: "crc64:123", ContentType: "video/mp4"}}
	svc := NewUploadService(repo, st, fakeProbe{result: MediaProbeResult{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", DurationSeconds: 1}}, UploadConfig{Bucket: "b", PartSize: 10, MaxParts: 1, TaskTTL: time.Hour, MaxVideoBytes: 100, MaxAudioBytes: 100, MaxAttempts: 2}, func() time.Time { return now })
	repo.task = UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}
	_, _ = svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
	if repo.task.AttemptCount != 1 || repo.task.Status != UploadFailed {
		t.Fatalf("first failure task=%+v", repo.task)
	}
	_, err := svc.Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "a.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"})
	if err != nil {
		t.Fatal(err)
	}
	if repo.task.AttemptCount != 2 {
		t.Fatalf("retry attempt=%d", repo.task.AttemptCount)
	}
	repo.task.Status = UploadUploading
	repo.task.ExpiresAt = now.Add(time.Hour)
	_, _ = svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
	if repo.task.AttemptCount != 2 {
		t.Fatalf("second failure attempt=%d", repo.task.AttemptCount)
	}
	if _, err := svc.Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "a.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"}); !errors.Is(err, ErrUploadAttempts) {
		t.Fatalf("expected retry limit, got %v", err)
	}
}

func TestClassroomUploadPartsRequireBidirectionalUniqueSet(t *testing.T) {
	task := UploadTask{ExpectedSize: 10, MaxParts: 2}
	listed := []storage.MultipartPart{{PartNumber: 1, ETag: "a", Size: 5}, {PartNumber: 2, ETag: "b", Size: 5}}
	if err := validateCompletedParts(task, []storage.CompletedPart{{PartNumber: 1, ETag: "a"}, {PartNumber: 1, ETag: "a"}}, listed); err == nil {
		t.Fatal("expected duplicate part rejection")
	}
}

func TestClassroomUploadRequiresChecksumAndAAC(t *testing.T) {
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}}
	_, err := newUploadService(repo, &fakeMultipartStorage{}, time.Now()).Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "a.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "sha256:x"})
	if err == nil {
		t.Fatal("expected crc64 checksum requirement")
	}
	if err := ValidateMediaProbe(ContentVideo, MediaProbeResult{Container: "mp4", VideoCodec: "h264", AudioCodec: "", DurationSeconds: 1}); err == nil {
		t.Fatal("expected AAC requirement")
	}
}

type fakeCoverExtractor struct {
	key   string
	calls int
	ratio CoverAspectRatio
}

func (f *fakeCoverExtractor) Extract(_ context.Context, _ string, _ int64, ratio CoverAspectRatio) (string, error) {
	f.calls++
	f.ratio = ratio
	return f.key, nil
}

func TestClassroomUploadTransitionsMediaProcessingThenReadyWithCover(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic, CoverAspectRatio: CoverAspectRatio9x16}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	st := &fakeMultipartStorage{parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: storage.ObjectMetadata{ETag: "final-etag", Size: 10, Checksum: "crc64:123", ContentType: "video/mp4"}}
	cover := &fakeCoverExtractor{key: "classroom/covers/7.jpg"}
	svc := NewUploadService(repo, st, fakeProbe{result: MediaProbeResult{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", DurationSeconds: 2}}, UploadConfig{Bucket: "b", PartSize: 10, MaxParts: 1, TaskTTL: time.Hour, MaxVideoBytes: 100, MaxAudioBytes: 100}, func() time.Time { return now }).WithCoverExtractor(cover)
	got, err := svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.mediaStatuses) < 2 || repo.mediaStatuses[0] != MediaProcessing || repo.mediaStatuses[len(repo.mediaStatuses)-1] != MediaReady {
		t.Fatalf("media states=%v", repo.mediaStatuses)
	}
	if len(repo.contentStatuses) < 2 || repo.contentStatuses[0] != ContentProcessing || repo.contentStatuses[len(repo.contentStatuses)-1] != ContentReady {
		t.Fatalf("content states=%v", repo.contentStatuses)
	}
	if got.Media.CoverObjectKey != "classroom/covers/7.jpg" || cover.calls != 1 || cover.ratio != CoverAspectRatio9x16 {
		t.Fatalf("cover=%q calls=%d ratio=%q", got.Media.CoverObjectKey, cover.calls, cover.ratio)
	}
	if repo.finalizeCalls != 1 {
		t.Fatalf("atomic finalize calls=%d", repo.finalizeCalls)
	}
}

func TestClassroomUploadFailureWritesFailedMediaAndContent(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	st := &fakeMultipartStorage{parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: storage.ObjectMetadata{ETag: "bad", Size: 9, Checksum: "crc64:123", ContentType: "video/mp4"}}
	_, err := newUploadService(repo, st, now).Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
	if err == nil {
		t.Fatal("expected failure")
	}
	if repo.media.StorageStatus != MediaFailed || repo.content.Status != ContentFailed || repo.task.MediaAssetID == nil {
		t.Fatalf("task=%+v media=%+v content=%+v", repo.task, repo.media, repo.content)
	}
	if len(repo.mediaStatuses) < 2 || repo.mediaStatuses[0] != MediaProcessing || repo.mediaStatuses[len(repo.mediaStatuses)-1] != MediaFailed {
		t.Fatalf("media failure states=%v", repo.mediaStatuses)
	}
	if len(repo.contentStatuses) < 2 || repo.contentStatuses[0] != ContentProcessing || repo.contentStatuses[len(repo.contentStatuses)-1] != ContentFailed {
		t.Fatalf("content failure states=%v", repo.contentStatuses)
	}
}

func TestClassroomUploadCleanupExpiredAbortsOrphans(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{expired: []UploadTask{{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(-time.Minute), AttemptCount: 1, CleanupStatus: "pending"}}}
	repo.task = repo.expired[0]
	st := &fakeMultipartStorage{}
	svc := newUploadService(repo, st, now)
	count, err := svc.CleanupExpired(context.Background(), 100)
	if err != nil || count != 1 || st.abortCalls != 1 || repo.task.Status != UploadExpired || repo.task.CleanupStatus != "cleaned" {
		t.Fatalf("count=%d err=%v calls=%d task=%+v", count, err, st.abortCalls, repo.task)
	}
}

func TestClassroomUploadConcurrentCompleteCallsStorageOnce(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	st := &fakeMultipartStorage{completeDelay: 20 * time.Millisecond, parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: storage.ObjectMetadata{ETag: "final-etag", Size: 10, Checksum: "crc64:123", ContentType: "video/mp4"}}
	svc := newUploadService(repo, st, now)
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			defer wg.Done()
			_, err := svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if st.completeCalls != 1 {
		t.Fatalf("complete calls=%d", st.completeCalls)
	}
}

type fakeCoverSigner struct{}

func (fakeCoverSigner) PresignGetURL(context.Context, string, time.Duration) (string, error) {
	return "https://media.test/video.mp4", nil
}

type fakeCoverUploader struct{ data []byte }

func (f *fakeCoverUploader) Upload(_ context.Context, in storage.UploadInput) (storage.UploadResult, error) {
	f.data, _ = io.ReadAll(in.Reader)
	return storage.UploadResult{Key: "classroom/covers/generated.jpg"}, nil
}
func TestFFmpegCoverExtractorUsesFirstDecodableFrameAndLandscapeCrop(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nfor arg do output=$arg; done\nprintf '%s\\n' \"$@\" > \"$0.args\"\nprintf jpeg > \"$output\"\n"
	if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	uploader := &fakeCoverUploader{}
	extractor := FFmpegCoverExtractor{Signer: fakeCoverSigner{}, Uploader: uploader, Binary: binary}
	key, err := extractor.Extract(context.Background(), "classroom/video/a.mp4", 7, CoverAspectRatio16x9)
	if err != nil {
		t.Fatal(err)
	}
	if key != "classroom/covers/generated.jpg" || string(uploader.data) != "jpeg" {
		t.Fatalf("key=%q data=%q", key, uploader.data)
	}
	args, err := os.ReadFile(binary + ".args")
	if err != nil {
		t.Fatal(err)
	}
	command := string(args)
	for _, want := range []string{"-map\n0:v:0", "scale=1280:720:force_original_aspect_ratio=increase", "crop=1280:720", "-frames:v\n1"} {
		if !strings.Contains(command, want) {
			t.Errorf("ffmpeg command missing %q: %s", want, command)
		}
	}
	if strings.Contains(command, "-ss\n00:00:01") {
		t.Fatalf("ffmpeg must use the first decodable frame: %s", command)
	}
}

func TestClassroomUploadTwoServiceInstancesCompleteIdempotently(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	st := &fakeMultipartStorage{completeDelay: 30 * time.Millisecond, parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: storage.ObjectMetadata{ETag: "final-etag", Size: 10, Checksum: "crc64:123", ContentType: "video/mp4"}}
	a := newUploadService(repo, st, now)
	b := newUploadService(repo, st, now)
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, svc := range []*UploadService{a, b} {
		go func(svc *UploadService) {
			<-start
			_, err := svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
			errs <- err
		}(svc)
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if st.completeCalls != 1 {
		t.Fatalf("complete calls=%d", st.completeCalls)
	}
}

func TestClassroomUploadTwoServiceInstancesAbortIdempotently(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	st := &fakeMultipartStorage{}
	a := newUploadService(repo, st, now)
	b := newUploadService(repo, st, now)
	errs := make(chan error, 2)
	for _, svc := range []*UploadService{a, b} {
		go func(svc *UploadService) { _, err := svc.Abort(context.Background(), 1, 42); errs <- err }(svc)
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if st.abortCalls != 1 {
		t.Fatalf("abort calls=%d", st.abortCalls)
	}
}

func TestClassroomUploadCleanupClaimIsMutuallyExclusive(t *testing.T) {
	now := time.Now()
	task := UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadFailed, ExpiresAt: now.Add(-time.Hour), AttemptCount: 1, CleanupStatus: "pending"}
	repo := &fakeUploadRepo{task: task, expired: []UploadTask{task}}
	st := &fakeMultipartStorage{}
	a := newUploadService(repo, st, now)
	b := newUploadService(repo, st, now)
	start := make(chan struct{})
	results := make(chan int, 2)
	errs := make(chan error, 2)
	for _, svc := range []*UploadService{a, b} {
		go func(svc *UploadService) {
			<-start
			n, err := svc.CleanupPending(context.Background(), 1)
			results <- n
			errs <- err
		}(svc)
	}
	close(start)
	cleaned := 0
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		cleaned += <-results
	}
	if cleaned != 1 || st.abortCalls != 1 {
		t.Fatalf("cleaned=%d abort calls=%d task=%+v", cleaned, st.abortCalls, repo.task)
	}
}

func TestClassroomUploadCleanupPendingFailedDeletesObjectAndCoverBeforeRetry(t *testing.T) {
	now := time.Now()
	mid := int64(91)
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentFailed, AccessLevel: AccessPublic}, media: MediaAsset{ID: mid, ObjectKey: "old.mp4", CoverObjectKey: "old.jpg", ContentType: ContentVideo, StorageStatus: MediaFailed}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "old-u", ObjectKey: "old.mp4", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadFailed, ExpiresAt: now.Add(time.Hour), AttemptCount: 1, CleanupStatus: "pending", MediaAssetID: &mid}}
	repo.expired = []UploadTask{repo.task}
	st := &fakeMultipartStorage{}
	svc := newUploadService(repo, st, now)
	count, err := svc.CleanupPending(context.Background(), 100)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if repo.task.CleanupStatus != "cleaned" || st.abortCalls != 1 || len(st.deleteCalls) != 2 {
		t.Fatalf("task=%+v abort=%d deletes=%v", repo.task, st.abortCalls, st.deleteCalls)
	}
	if _, err := svc.Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "new.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"}); err != nil {
		t.Fatal(err)
	}
	if repo.task.ObjectKey == "old.mp4" {
		t.Fatal("retry did not replace cleaned reference")
	}
}

func TestClassroomUploadCleanupRecoversStaleCleaningAfterFinishError(t *testing.T) {
	now := time.Now()
	task := UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "old-u", ObjectKey: "old.mp4", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadCleaning, ExpiresAt: now.Add(time.Hour), AttemptCount: 1, CleanupStatus: "pending", UpdatedAt: now.Add(-time.Hour)}
	repo := &fakeUploadRepo{task: task, expired: []UploadTask{task}, finishErr: errors.New("temporary finish failure")}
	svc := newUploadService(repo, &fakeMultipartStorage{}, now)
	if _, err := svc.CleanupPending(context.Background(), 1); err == nil {
		t.Fatal("expected temporary finish failure")
	}
	if repo.task.Status != UploadCleaning {
		t.Fatalf("failed finish must retain cleaning lease state: %+v", repo.task)
	}
	repo.mu.Lock()
	repo.finishErr = nil
	repo.task.UpdatedAt = now.Add(-time.Hour)
	repo.expired = []UploadTask{repo.task}
	repo.mu.Unlock()
	n, err := svc.CleanupPending(context.Background(), 1)
	if err != nil || n != 1 {
		t.Fatalf("stale cleanup retry n=%d err=%v", n, err)
	}
	if repo.task.Status != UploadExpired || repo.task.CleanupStatus != "cleaned" {
		t.Fatalf("stale cleaning task not finalized: %+v", repo.task)
	}
}

func TestClassroomUploadCleanupMarksInterruptedProcessingFailed(t *testing.T) {
	now := time.Now()
	mediaID := int64(91)
	task := UploadTask{
		ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "old-u", ObjectKey: "old.mp4",
		ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1,
		Status: UploadCompleting, ExpiresAt: now.Add(-time.Minute), AttemptCount: 1,
		CleanupStatus: "pending", MediaAssetID: &mediaID, UpdatedAt: now.Add(-time.Hour),
	}
	repo := &fakeUploadRepo{
		content: Content{ID: 7, ContentType: ContentVideo, Status: ContentProcessing, AccessLevel: AccessPublic, MediaAssetID: &mediaID},
		media:   MediaAsset{ID: mediaID, ObjectKey: "old.mp4", ContentType: ContentVideo, StorageStatus: MediaProcessing},
		task:    task,
		expired: []UploadTask{task},
	}

	n, err := newUploadService(repo, &fakeMultipartStorage{}, now).CleanupPending(context.Background(), 1)

	if err != nil || n != 1 {
		t.Fatalf("cleanup n=%d err=%v", n, err)
	}
	if repo.media.StorageStatus != MediaFailed || repo.content.Status != ContentFailed {
		t.Fatalf("interrupted processing state not failed media=%+v content=%+v", repo.media, repo.content)
	}
}

func TestClassroomUploadRetryPreservesFailedReferenceWhenCleanupFails(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentFailed, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "old-u", ObjectKey: "old.mp4", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadFailed, ExpiresAt: now.Add(time.Hour), AttemptCount: 1, CleanupStatus: "pending"}}
	st := &fakeMultipartStorage{deleteErr: errors.New("delete failed")}
	svc := newUploadService(repo, st, now)
	_, err := svc.Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "new.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"})
	if err == nil {
		t.Fatal("expected cleanup failure")
	}
	if repo.task.ObjectKey != "old.mp4" || repo.task.CleanupStatus != "failed" {
		t.Fatalf("reference overwritten: %+v", repo.task)
	}
}

func TestValidateMediaProbeRequiresMP3CodecForMP3Container(t *testing.T) {
	if err := ValidateMediaProbe(ContentAudio, MediaProbeResult{Container: "mp3", AudioCodec: "aac", DurationSeconds: 1}); err == nil {
		t.Fatal("MP3 container with AAC accepted")
	}
}

func TestClassroomUploadRetryRequiresCleanedForExpiredAndAborted(t *testing.T) {
	now := time.Now()
	for _, status := range []UploadStatus{UploadExpired, UploadAborted} {
		t.Run(string(status), func(t *testing.T) {
			repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentFailed, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "old", ObjectKey: "old.mp4", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: status, ExpiresAt: now.Add(time.Hour), AttemptCount: 1, CleanupStatus: "pending"}}
			st := &fakeMultipartStorage{abortErr: errors.New("abort failed")}
			_, err := newUploadService(repo, st, now).Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "new.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"})
			if err == nil || repo.task.ObjectKey != "old.mp4" {
				t.Fatalf("expected preserved retry refusal err=%v task=%+v", err, repo.task)
			}
		})
	}
}

type failingFinalizeRepo struct{ *fakeUploadRepo }

func (r *failingFinalizeRepo) FinalizeUpload(context.Context, UploadTask, MediaAsset) (UploadTask, MediaAsset, Content, error) {
	return UploadTask{}, MediaAsset{}, Content{}, errors.New("finalize failed")
}

func TestClassroomUploadFinalizeFailureCleansPersistedCover(t *testing.T) {
	now := time.Now()
	repo := &failingFinalizeRepo{&fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "video.mp4", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}}
	st := &fakeMultipartStorage{parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: storage.ObjectMetadata{ObjectKey: "video.mp4", ETag: "final-etag", Size: 10, Checksum: "crc64:123", ContentType: "video/mp4"}}
	svc := NewUploadService(repo, st, fakeProbe{result: MediaProbeResult{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", DurationSeconds: 1}}, UploadConfig{Bucket: "b", PartSize: 10, MaxParts: 1, TaskTTL: time.Hour, MaxVideoBytes: 100, MaxAudioBytes: 100}, func() time.Time { return now }).WithCoverExtractor(&fakeCoverExtractor{key: "cover.jpg"})
	_, err := svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
	if err == nil {
		t.Fatal("expected finalize failure")
	}
	found := false
	for _, key := range st.deleteCalls {
		if key == "cover.jpg" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cover not cleaned: %v", st.deleteCalls)
	}
	if repo.media.CoverObjectKey != "cover.jpg" {
		t.Fatalf("cover reference not persisted before failure: %+v", repo.media)
	}
}

func TestClassroomUploadFinalizeFailureMarksCleanupPending(t *testing.T) {
	now := time.Now()
	repo := &failingFinalizeRepo{&fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "video.mp4", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1, CleanupStatus: "retained"}}}
	st := &fakeMultipartStorage{parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: storage.ObjectMetadata{ObjectKey: "video.mp4", ETag: "final-etag", Size: 10, Checksum: "crc64:123", ContentType: "video/mp4"}}
	svc := NewUploadService(repo, st, fakeProbe{result: MediaProbeResult{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", DurationSeconds: 1}}, UploadConfig{Bucket: "b", PartSize: 10, MaxParts: 1, TaskTTL: time.Hour, MaxVideoBytes: 100, MaxAudioBytes: 100}, func() time.Time { return now }).WithCoverExtractor(&fakeCoverExtractor{key: "cover.jpg"})
	_, err := svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
	if err == nil {
		t.Fatal("expected finalize failure")
	}
	if repo.task.Status != UploadFailed || repo.task.CleanupStatus != "pending" {
		t.Fatalf("task=%+v", repo.task)
	}
}

func TestClassroomUploadCleanupTreatsAlreadyGoneAsSuccess(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{expired: []UploadTask{{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "old", ExpectedSize: 1, Checksum: "crc64:1", PartSize: 1, MaxParts: 1, Status: UploadFailed, ExpiresAt: now.Add(time.Hour), AttemptCount: 1, CleanupStatus: "pending"}}}
	repo.task = repo.expired[0]
	st := &fakeMultipartStorage{abortErr: storage.ErrAlreadyGone, deleteErr: storage.ErrAlreadyGone}
	n, err := newUploadService(repo, st, now).CleanupPending(context.Background(), 1)
	if err != nil || n != 1 || repo.task.CleanupStatus != "cleaned" {
		t.Fatalf("n=%d err=%v task=%+v", n, err, repo.task)
	}
}

func TestClassroomUploadInitiateRejectsActiveCompletingRegardlessCleanup(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentFailed, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "old", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadCompleting, ExpiresAt: now.Add(time.Hour), AttemptCount: 1, CleanupStatus: "cleaned"}}
	_, err := newUploadService(repo, &fakeMultipartStorage{}, now).Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "new.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"})
	if !errors.Is(err, ErrUploadConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestClassroomUploadDuplicateCompleteReturnsInProgressAfterConfiguredWait(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentProcessing, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "video.mp4", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadCompleting, ExpiresAt: now.Add(time.Hour), AttemptCount: 1, CleanupStatus: "pending"}}
	svc := NewUploadService(repo, &fakeMultipartStorage{}, fakeProbe{}, UploadConfig{Bucket: "b", PartSize: 10, MaxParts: 1, TaskTTL: time.Hour, CompletionWait: 20 * time.Millisecond, MaxVideoBytes: 100, MaxAudioBytes: 100}, func() time.Time { return now })
	started := time.Now()
	_, err := svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
	if !errors.Is(err, ErrUploadInProgress) {
		t.Fatalf("err=%v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("configured wait ignored: %s", elapsed)
	}
}

func TestClassroomUploadInitiateRejectsCleaningTask(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentFailed, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "old", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadCleaning, ExpiresAt: now.Add(time.Hour), AttemptCount: 1, CleanupStatus: "pending"}}
	_, err := newUploadService(repo, &fakeMultipartStorage{}, now).Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "new.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"})
	if !errors.Is(err, ErrUploadConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestClassroomUploadAbortRejectsCompletingTask(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "old", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadCompleting, ExpiresAt: now.Add(time.Hour), AttemptCount: 1, CleanupStatus: "pending"}}
	_, err := newUploadService(repo, &fakeMultipartStorage{}, now).Abort(context.Background(), 1, 42)
	if !errors.Is(err, ErrUploadConflict) {
		t.Fatalf("err=%v", err)
	}
	if repo.task.Status != UploadCompleting {
		t.Fatalf("completion claim overwritten: %+v", repo.task)
	}
}

func TestClassroomUploadAbortRejectsActiveInitiatingTask(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "initiating:token", ObjectKey: "pending", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadInitiating, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	st := &fakeMultipartStorage{}
	_, err := newUploadService(repo, st, now).Abort(context.Background(), 1, 42)
	if !errors.Is(err, ErrUploadInProgress) && !errors.Is(err, ErrUploadConflict) {
		t.Fatalf("err=%v, want in-progress/conflict", err)
	}
	if repo.task.Status != UploadInitiating || st.abortCalls != 0 {
		t.Fatalf("active initiation was reclaimed: task=%+v aborts=%d", repo.task, st.abortCalls)
	}
}

func TestClassroomUploadCoverPersistenceFailureDeletesLocalCover(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "video", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}, updateErr: errors.New("persist cover failed")}
	st := &fakeMultipartStorage{parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: storage.ObjectMetadata{ObjectKey: "video", ETag: "final-etag", Size: 10, Checksum: "crc64:123", ContentType: "video/mp4"}}
	svc := NewUploadService(repo, st, fakeProbe{result: MediaProbeResult{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", DurationSeconds: 1}}, UploadConfig{Bucket: "b", PartSize: 10, MaxParts: 1, TaskTTL: time.Hour, MaxVideoBytes: 100, MaxAudioBytes: 100}, func() time.Time { return now }).WithCoverExtractor(&fakeCoverExtractor{key: "local-cover.jpg"})
	_, err := svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
	if err == nil {
		t.Fatal("expected persistence failure")
	}
	for _, key := range st.deleteCalls {
		if key == "local-cover.jpg" {
			return
		}
	}
	t.Fatalf("cover not deleted: %v", st.deleteCalls)
}

func TestClassroomUploadFailurePersistsAfterCompletionContextExpires(t *testing.T) {
	now := time.Now()
	mediaID := int64(91)
	repo := &fakeUploadRepo{
		rejectCanceled: true,
		content:        Content{ID: 7, ContentType: ContentVideo, Status: ContentProcessing, AccessLevel: AccessPublic, MediaAssetID: &mediaID},
		media:          MediaAsset{ID: mediaID, ContentType: ContentVideo, StorageStatus: MediaProcessing},
		task:           UploadTask{ID: 1, ContentID: 7, CreatorID: 42, MediaAssetID: &mediaID, Status: UploadCompleting, ExpiresAt: now.Add(time.Hour)},
	}
	svc := newUploadService(repo, &fakeMultipartStorage{}, now)
	completionCtx, cancel := context.WithCancel(context.Background())
	cancel()
	cause := context.DeadlineExceeded

	err := svc.fail(completionCtx, repo.task, cause)

	if !errors.Is(err, cause) {
		t.Fatalf("err=%v, want deadline cause", err)
	}
	if repo.task.Status != UploadFailed || repo.media.StorageStatus != MediaFailed || repo.content.Status != ContentFailed {
		t.Fatalf("failure state was not persisted task=%+v media=%+v content=%+v", repo.task, repo.media, repo.content)
	}
}

func TestClassroomUploadFailurePersistsBeforeRemoteCoverCleanup(t *testing.T) {
	now := time.Now()
	mediaID := int64(91)
	repo := &fakeUploadRepo{
		content: Content{ID: 7, ContentType: ContentVideo, Status: ContentProcessing, AccessLevel: AccessPublic, MediaAssetID: &mediaID},
		media:   MediaAsset{ID: mediaID, ContentType: ContentVideo, CoverObjectKey: "cover.jpg", StorageStatus: MediaProcessing},
		task:    UploadTask{ID: 1, ContentID: 7, CreatorID: 42, MediaAssetID: &mediaID, Status: UploadCompleting, ExpiresAt: now.Add(time.Hour)},
	}
	persistedBeforeCleanup := false
	st := &fakeMultipartStorage{deleteHook: func(context.Context, string) error {
		persistedBeforeCleanup = repo.task.Status == UploadFailed && repo.media.StorageStatus == MediaFailed && repo.content.Status == ContentFailed
		return nil
	}}

	err := newUploadService(repo, st, now).fail(context.Background(), repo.task, errors.New("processing failed"))

	if err == nil {
		t.Fatal("expected original processing failure")
	}
	if !persistedBeforeCleanup {
		t.Fatalf("remote cleanup ran before failure state persisted task=%+v media=%+v content=%+v", repo.task, repo.media, repo.content)
	}
}

func TestClassroomUploadCoverDeleteFailureLeavesTraceForMaintenance(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "video", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}, updateErr: errors.New("persist cover failed")}
	st := &fakeMultipartStorage{deleteErr: errors.New("cover delete failed"), parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: storage.ObjectMetadata{ObjectKey: "video", ETag: "final-etag", Size: 10, Checksum: "crc64:123", ContentType: "video/mp4"}}
	svc := NewUploadService(repo, st, fakeProbe{result: MediaProbeResult{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", DurationSeconds: 1}}, UploadConfig{Bucket: "b", PartSize: 10, MaxParts: 1, TaskTTL: time.Hour, MaxVideoBytes: 100, MaxAudioBytes: 100}, func() time.Time { return now }).WithCoverExtractor(&fakeCoverExtractor{key: "trace-cover.jpg"})
	_, err := svc.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
	if err == nil || !strings.Contains(repo.task.FailureReason, "trace-cover.jpg") {
		t.Fatalf("trace missing err=%v task=%+v", err, repo.task)
	}
	repo.expired = []UploadTask{repo.task}
	repo.updateErr = nil
	st.deleteErr = nil
	if _, err := svc.CleanupPending(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, key := range st.deleteCalls {
		if key == "trace-cover.jpg" {
			found = true
		}
	}
	if !found {
		t.Fatalf("maintenance did not see trace: %v", st.deleteCalls)
	}
}

func TestClassroomUploadSecondServiceCannotRetryWhileFirstCompleting(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "video", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadUploading, ExpiresAt: now.Add(time.Hour), AttemptCount: 1}}
	st := &fakeMultipartStorage{completeDelay: 100 * time.Millisecond, parts: []storage.MultipartPart{{PartNumber: 1, ETag: "e", Size: 10}}, head: storage.ObjectMetadata{ObjectKey: "video", ETag: "final-etag", Size: 10, Checksum: "crc64:123", ContentType: "video/mp4"}}
	first := newUploadService(repo, st, now)
	second := newUploadService(repo, st, now)
	done := make(chan error, 1)
	go func() {
		_, err := first.Complete(context.Background(), 1, 42, []storage.CompletedPart{{PartNumber: 1, ETag: "e"}})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		task, _ := repo.GetUploadTask(context.Background(), 1)
		if task.Status == UploadCompleting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first service never claimed completing")
		}
		time.Sleep(time.Millisecond)
	}
	_, err := second.Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "new.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"})
	if !errors.Is(err, ErrUploadConflict) {
		t.Fatalf("retry err=%v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestClassroomUploadMaintenanceCleansExpiredCompletingBeforeRetry(t *testing.T) {
	now := time.Now()
	repo := &fakeUploadRepo{content: Content{ID: 7, ContentType: ContentVideo, Status: ContentFailed, AccessLevel: AccessPublic}, task: UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "old", ObjectKey: "old", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadCompleting, ExpiresAt: now.Add(-time.Minute), AttemptCount: 1, CleanupStatus: "pending"}}
	repo.expired = []UploadTask{repo.task}
	svc := newUploadService(repo, &fakeMultipartStorage{}, now)
	if _, err := svc.CleanupPending(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if repo.task.Status != UploadExpired || repo.task.CleanupStatus != "cleaned" {
		t.Fatalf("task=%+v", repo.task)
	}
	if _, err := svc.Initiate(context.Background(), InitiateUploadInput{ContentID: 7, CreatorID: 42, Filename: "new.mp4", ContentType: "video/mp4", SizeBytes: 10, Checksum: "crc64:123"}); err != nil {
		t.Fatal(err)
	}
}

func TestFFmpegCoverExtractorCropsAllSupportedAspectRatios(t *testing.T) {
	for _, tc := range []struct {
		ratio CoverAspectRatio
		scale string
		crop  string
	}{
		{CoverAspectRatio16x9, "scale=1280:720:force_original_aspect_ratio=increase", "crop=1280:720"},
		{CoverAspectRatio9x16, "scale=720:1280:force_original_aspect_ratio=increase", "crop=720:1280"},
		{CoverAspectRatio1x1, "scale=1080:1080:force_original_aspect_ratio=increase", "crop=1080:1080"},
	} {
		t.Run(string(tc.ratio), func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "ffmpeg")
			script := "#!/bin/sh\nfor arg do output=$arg; done\nprintf '%s\\n' \"$@\" > \"$0.args\"\nprintf jpeg > \"$output\"\n"
			if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			_, err := (FFmpegCoverExtractor{Signer: fakeCoverSigner{}, Uploader: &fakeCoverUploader{}, Binary: binary}).Extract(context.Background(), "classroom/video/a.mp4", 7, tc.ratio)
			if err != nil {
				t.Fatal(err)
			}
			args, err := os.ReadFile(binary + ".args")
			if err != nil {
				t.Fatal(err)
			}
			command := string(args)
			for _, want := range []string{"-map\n0:v:0", tc.scale, tc.crop, "-frames:v\n1"} {
				if !strings.Contains(command, want) {
					t.Errorf("command missing %q: %s", want, command)
				}
			}
			if strings.Contains(command, "-ss\n00:00:01") {
				t.Fatalf("command unexpectedly seeks: %s", command)
			}
		})
	}
}
