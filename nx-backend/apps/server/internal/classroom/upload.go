package classroom

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/storage"
)

var (
	ErrUploadOwnership       = errors.New("classroom upload is owned by another user")
	ErrUploadExpired         = errors.New("classroom upload expired")
	ErrInvalidUploadPart     = errors.New("invalid classroom upload part")
	ErrUploadConflict        = errors.New("classroom content already has an active upload")
	ErrUploadInProgress      = errors.New("classroom upload is in progress")
	ErrUploadAttempts        = errors.New("classroom upload retry limit reached")
	ErrInvalidUploadProgress = errors.New("invalid classroom upload progress")
)

const uploadFailurePersistenceTimeout = 30 * time.Second

type UploadConfig struct {
	Bucket         string
	Prefix         string
	PartSize       int64
	MaxParts       int
	CredentialTTL  time.Duration
	TaskTTL        time.Duration
	MaxVideoBytes  int64
	MaxAudioBytes  int64
	MaxAttempts    int
	CompletionWait time.Duration
}

type InitiateUploadInput struct {
	ContentID   int64
	CreatorID   int64
	Filename    string
	ContentType string
	SizeBytes   int64
	Checksum    string
}

type InitiateUploadResult struct {
	Task UploadTask `json:"task"`
}
type CompleteUploadResult struct {
	Task    UploadTask `json:"task"`
	Media   MediaAsset `json:"media"`
	Content Content    `json:"content"`
}

type ProbeInput struct {
	ObjectKey   string
	ContentType string
}
type MediaProbeResult struct {
	Container       string
	VideoCodec      string
	AudioCodec      string
	DurationSeconds int
	Width           int
	Height          int
	CoverObjectKey  string
}
type MediaProbe interface {
	Probe(context.Context, ProbeInput) (MediaProbeResult, error)
}
type CoverExtractor interface {
	Extract(context.Context, string, int64, CoverAspectRatio) (string, error)
}

type UploadRepository interface {
	GetContent(context.Context, int64) (Content, error)
	FindUploadTaskByContent(context.Context, int64) (UploadTask, error)
	CreateUploadTask(context.Context, UploadTask) (UploadTask, error)
	GetUploadTask(context.Context, int64) (UploadTask, error)
	SaveUploadTask(context.Context, UploadTask) (UploadTask, error)
	CreateMediaAsset(context.Context, MediaAsset) (MediaAsset, error)
	UpdateMediaAsset(context.Context, MediaAsset) (MediaAsset, error)
	SetContentMediaState(context.Context, int64, *int64, ContentStatus, int) (Content, error)
	ListExpiredUploadTasks(context.Context, int) ([]UploadTask, error)
	FinalizeUpload(context.Context, UploadTask, MediaAsset) (UploadTask, MediaAsset, Content, error)
	ClaimUploadCompletion(context.Context, int64) (UploadTask, bool, error)
	ReserveUploadInitiation(context.Context, UploadTask, *UploadTask) (UploadTask, bool, error)
	ConfirmUploadInitiation(context.Context, UploadTask, string) (UploadTask, bool, error)
	FailUploadInitiation(context.Context, UploadTask, string, string, string) (UploadTask, bool, error)
	MarkUploadUploading(context.Context, int64) (UploadTask, error)
	ClaimUploadCleanup(context.Context, UploadTask, UploadStatus) (UploadTask, bool, error)
	FinishUploadCleanup(context.Context, UploadTask, UploadStatus, string, string) (UploadTask, bool, error)
	UpdateUploadProgress(context.Context, int64, int64, int, int64) (UploadTask, error)
}

type UploadService struct {
	repo   UploadRepository
	store  storage.MultipartStorage
	probe  MediaProbe
	cover  CoverExtractor
	config UploadConfig
	now    func() time.Time
}

func NewUploadService(repo UploadRepository, store storage.MultipartStorage, probe MediaProbe, config UploadConfig, now func() time.Time) *UploadService {
	if now == nil {
		now = time.Now
	}
	if config.PartSize <= 0 {
		config.PartSize = 8 << 20
	}
	if config.MaxParts <= 0 || config.MaxParts > 10000 {
		config.MaxParts = 10000
	}
	if config.CompletionWait <= 0 {
		config.CompletionWait = 3 * time.Second
	}
	if config.CredentialTTL <= 0 {
		config.CredentialTTL = 15 * time.Minute
	}
	if config.TaskTTL <= 0 {
		config.TaskTTL = 24 * time.Hour
	}
	if config.MaxVideoBytes <= 0 {
		config.MaxVideoBytes = 4 << 30
	}
	if config.MaxAudioBytes <= 0 {
		config.MaxAudioBytes = 512 << 20
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if strings.TrimSpace(config.Prefix) == "" {
		config.Prefix = "classroom"
	}
	return &UploadService{repo: repo, store: store, probe: probe, config: config, now: now}
}

func (s *UploadService) WithCoverExtractor(cover CoverExtractor) *UploadService {
	s.cover = cover
	return s
}

func (s *UploadService) Initiate(ctx context.Context, input InitiateUploadInput) (InitiateUploadResult, error) {
	if input.ContentID <= 0 || input.CreatorID <= 0 || input.SizeBytes <= 0 || !validCRC64(input.Checksum) {
		return InitiateUploadResult{}, errors.New("checksum must use crc64:<value>")
	}
	content, err := s.repo.GetContent(ctx, input.ContentID)
	if err != nil {
		return InitiateUploadResult{}, err
	}
	if content.Status != ContentDraft && content.Status != ContentFailed {
		return InitiateUploadResult{}, errors.New("media upload requires a draft content")
	}
	if !mimeMatchesContent(content.ContentType, input.ContentType, input.Filename) {
		return InitiateUploadResult{}, errors.New("media type does not match content")
	}
	limit := s.config.MaxVideoBytes
	if content.ContentType == ContentAudio {
		limit = s.config.MaxAudioBytes
	}
	if input.SizeBytes > limit {
		return InitiateUploadResult{}, errors.New("media exceeds configured size limit")
	}
	parts := int((input.SizeBytes + s.config.PartSize - 1) / s.config.PartSize)
	if parts > s.config.MaxParts {
		return InitiateUploadResult{}, errors.New("media exceeds multipart limit")
	}
	previous, findErr := s.repo.FindUploadTaskByContent(ctx, input.ContentID)
	if findErr == nil {
		switch previous.Status {
		case UploadInitiating, UploadInitiated, UploadUploading:
			return InitiateUploadResult{}, ErrUploadConflict
		case UploadCompleting, UploadCleaning:
			return InitiateUploadResult{}, ErrUploadConflict
		case UploadFailed, UploadExpired, UploadAborted:
			if previous.CleanupStatus != "cleaned" {
				terminalStatus := previous.Status
				claimed, ok, claimErr := s.repo.ClaimUploadCleanup(ctx, previous, terminalStatus)
				if claimErr != nil {
					return InitiateUploadResult{}, claimErr
				}
				if !ok {
					return InitiateUploadResult{}, ErrUploadConflict
				}
				previous = claimed
				if err := s.cleanupTask(ctx, &previous, terminalStatus); err != nil {
					return InitiateUploadResult{}, fmt.Errorf("cleanup previous upload: %w", err)
				}
			}
		case UploadCompleted:
			return InitiateUploadResult{}, ErrUploadConflict
		}
	}
	attempt := 1
	if findErr == nil {
		if previous.AttemptCount >= s.config.MaxAttempts {
			return InitiateUploadResult{}, ErrUploadAttempts
		}
		attempt = previous.AttemptCount + 1
	}
	objectKey := s.objectKey(content, input.Filename)
	reservation := UploadTask{ContentID: input.ContentID, CreatorID: input.CreatorID, OriginalFilename: filepath.Base(input.Filename), OSSUploadID: "initiating:" + randomUploadToken(), ObjectKey: objectKey, ExpectedSize: input.SizeBytes, Checksum: strings.TrimSpace(input.Checksum), PartSize: s.config.PartSize, MaxParts: parts, Status: UploadInitiating, ExpiresAt: s.now().Add(s.config.TaskTTL), AttemptCount: attempt, CleanupStatus: "pending"}
	var expected *UploadTask
	if findErr == nil {
		expected = &previous
	} else if !errors.Is(findErr, ErrNotFound) && !errors.Is(findErr, sql.ErrNoRows) {
		return InitiateUploadResult{}, findErr
	}
	reserved, ok, err := s.repo.ReserveUploadInitiation(ctx, reservation, expected)
	if err != nil {
		return InitiateUploadResult{}, err
	}
	if !ok {
		return InitiateUploadResult{}, ErrUploadConflict
	}
	initiated, err := s.store.InitiateMultipart(ctx, storage.InitiateMultipartInput{ObjectKey: reserved.ObjectKey, ContentType: input.ContentType, Checksum: input.Checksum})
	if err != nil {
		_, _, _ = s.repo.FailUploadInitiation(ctx, reserved, "", "cleaned", err.Error())
		return InitiateUploadResult{}, err
	}
	task, confirmed, err := s.repo.ConfirmUploadInitiation(ctx, reserved, initiated.UploadID)
	if err != nil || !confirmed {
		abortErr := s.store.AbortMultipart(ctx, storage.AbortMultipartInput{ObjectKey: reserved.ObjectKey, UploadID: initiated.UploadID})
		cleanupStatus := "cleaned"
		if abortErr != nil && !storage.IsAlreadyGone(abortErr) {
			cleanupStatus = "pending"
		}
		_, _, _ = s.repo.FailUploadInitiation(ctx, reserved, initiated.UploadID, cleanupStatus, "confirm upload initiation failed")
		if err == nil {
			err = ErrUploadConflict
		}
		return InitiateUploadResult{}, err
	}
	return InitiateUploadResult{Task: task}, nil
}

func (s *UploadService) SignPart(ctx context.Context, taskID, creatorID int64, partNumber int) (storage.SignPartResult, error) {
	task, err := s.ownedTask(ctx, taskID, creatorID)
	if err != nil {
		return storage.SignPartResult{}, err
	}
	if err := s.ensureActive(ctx, task); err != nil {
		return storage.SignPartResult{}, err
	}
	if partNumber < 1 || partNumber > task.MaxParts {
		return storage.SignPartResult{}, ErrInvalidUploadPart
	}
	if task.Status == UploadInitiated {
		updated, markErr := s.repo.MarkUploadUploading(ctx, task.ID)
		if markErr != nil {
			return storage.SignPartResult{}, markErr
		}
		if updated.Status != UploadUploading {
			return storage.SignPartResult{}, ErrUploadConflict
		}
		task = updated
	}
	signed, err := s.store.SignMultipartPart(ctx, storage.SignPartInput{ObjectKey: task.ObjectKey, UploadID: task.OSSUploadID, PartNumber: partNumber, Expires: s.config.CredentialTTL})
	if err != nil {
		return storage.SignPartResult{}, err
	}
	signed.PartNumber = partNumber
	return signed, nil
}

func (s *UploadService) Complete(ctx context.Context, taskID, creatorID int64, provided []storage.CompletedPart) (CompleteUploadResult, error) {
	task, err := s.ownedTask(ctx, taskID, creatorID)
	if err != nil {
		return CompleteUploadResult{}, err
	}
	if task.Status == UploadCompleted {
		media := MediaAsset{}
		if task.MediaAssetID != nil {
			media, _ = s.repoMedia(ctx, *task.MediaAssetID)
		}
		content, _ := s.repo.GetContent(ctx, task.ContentID)
		return CompleteUploadResult{Task: task, Media: media, Content: content}, nil
	}
	if task.Status == UploadCompleting {
		return s.waitForCompleted(ctx, task.ID)
	}
	if err := s.ensureActive(ctx, task); err != nil {
		return CompleteUploadResult{}, err
	}
	claimed, ok, err := s.repo.ClaimUploadCompletion(ctx, task.ID)
	if err != nil {
		return CompleteUploadResult{}, err
	}
	if !ok {
		return s.waitForCompleted(ctx, task.ID)
	}
	task = claimed
	listed, err := s.store.ListMultipartParts(ctx, storage.ListPartsInput{ObjectKey: task.ObjectKey, UploadID: task.OSSUploadID, MaxParts: task.MaxParts})
	if err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	if err = validateCompletedParts(task, provided, listed); err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	completed, err := s.store.CompleteMultipart(ctx, storage.CompleteMultipartInput{ObjectKey: task.ObjectKey, UploadID: task.OSSUploadID, Parts: provided})
	if err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	head, err := s.store.HeadObject(ctx, task.ObjectKey)
	if err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	if err = validateHead(task, completed, head); err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	content, err := s.repo.GetContent(ctx, task.ContentID)
	if err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	media, err := s.repo.CreateMediaAsset(ctx, MediaAsset{Bucket: s.config.Bucket, ObjectKey: task.ObjectKey, ETag: head.ETag, Checksum: head.Checksum, ContentType: content.ContentType, SizeBytes: head.Size, StorageStatus: MediaProcessing, CreatedBy: &creatorID})
	if err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	task.MediaAssetID = &media.ID
	_, _ = s.repo.SaveUploadTask(ctx, task)
	content, err = s.repo.SetContentMediaState(ctx, task.ContentID, &media.ID, ContentProcessing, 0)
	if err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	if s.probe == nil {
		return CompleteUploadResult{}, s.fail(ctx, task, errors.New("media probe is unavailable"))
	}
	probe, err := s.probe.Probe(ctx, ProbeInput{ObjectKey: task.ObjectKey, ContentType: head.ContentType})
	if err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	if err = ValidateMediaProbe(content.ContentType, probe); err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	if content.ContentType == ContentVideo {
		if s.cover == nil {
			return CompleteUploadResult{}, s.fail(ctx, task, errors.New("cover extractor is unavailable"))
		}
		ratio, ratioErr := NormalizeCoverAspectRatio(content.CoverAspectRatio)
		if ratioErr != nil {
			return CompleteUploadResult{}, s.fail(ctx, task, ratioErr)
		}
		probe.CoverObjectKey, err = s.cover.Extract(ctx, task.ObjectKey, task.ContentID, ratio)
		if err != nil {
			return CompleteUploadResult{}, s.fail(ctx, task, err)
		}
		media.CoverObjectKey = probe.CoverObjectKey
		media, err = s.repo.UpdateMediaAsset(ctx, media)
		if err != nil {
			cause := err
			if deleteErr := s.store.DeleteObject(ctx, probe.CoverObjectKey); deleteErr != nil && !storage.IsAlreadyGone(deleteErr) {
				cause = fmt.Errorf("%w; cover_object_key=%s; cover cleanup: %v", err, probe.CoverObjectKey, deleteErr)
			}
			return CompleteUploadResult{}, s.fail(ctx, task, cause)
		}
	}
	media.ETag = head.ETag
	media.Checksum = head.Checksum
	media.SizeBytes = head.Size
	media.DurationSeconds = probe.DurationSeconds
	media.Width = probe.Width
	media.Height = probe.Height
	media.CoverObjectKey = probe.CoverObjectKey
	media.StorageStatus = MediaReady
	task.Status = UploadCompleted
	mediaID := media.ID
	task.MediaAssetID = &mediaID
	task.FailureReason = ""
	task.CleanupStatus = "retained"
	finalTask, finalMedia, finalContent, finalErr := s.repo.FinalizeUpload(ctx, task, media)
	if finalErr != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, finalErr)
	}
	return CompleteUploadResult{Task: finalTask, Media: finalMedia, Content: finalContent}, nil
}

func (s *UploadService) Abort(ctx context.Context, taskID, creatorID int64) (UploadTask, error) {
	task, err := s.ownedTask(ctx, taskID, creatorID)
	if err != nil {
		return UploadTask{}, err
	}
	if task.Status == UploadCompleted {
		return task, nil
	}
	if task.Status == UploadAborted && task.CleanupStatus == "cleaned" {
		return task, nil
	}
	if task.Status == UploadCleaning {
		return s.waitForCleaned(ctx, task.ID)
	}
	if task.Status == UploadInitiating {
		return UploadTask{}, ErrUploadInProgress
	}
	if task.Status == UploadCompleting {
		return UploadTask{}, ErrUploadConflict
	}
	claimed, ok, err := s.repo.ClaimUploadCleanup(ctx, task, UploadAborted)
	if err != nil {
		return UploadTask{}, err
	}
	if !ok {
		return s.waitForCleaned(ctx, task.ID)
	}
	task = claimed
	if err = s.cleanupTask(ctx, &task, UploadAborted); err != nil {
		return UploadTask{}, err
	}
	return task, nil
}

// ReportProgress persists client-confirmed multipart progress. It is deliberately
// separate from signing: a signed URL does not prove that the browser PUT finished.
func (s *UploadService) ReportProgress(ctx context.Context, taskID, creatorID int64, completedParts int, completedBytes int64) (UploadTask, error) {
	task, err := s.ownedTask(ctx, taskID, creatorID)
	if err != nil {
		return UploadTask{}, err
	}
	if completedParts < task.CompletedParts || completedBytes < task.CompletedBytes || completedParts < 0 || completedBytes < 0 || completedParts > task.MaxParts || completedBytes > task.ExpectedSize {
		return UploadTask{}, ErrInvalidUploadProgress
	}
	updated, err := s.repo.UpdateUploadProgress(ctx, taskID, creatorID, completedParts, completedBytes)
	if err != nil {
		return UploadTask{}, err
	}
	if updated.CreatorID != creatorID {
		return UploadTask{}, ErrUploadOwnership
	}
	return updated, nil
}

func (s *UploadService) ownedTask(ctx context.Context, id, creator int64) (UploadTask, error) {
	task, err := s.repo.GetUploadTask(ctx, id)
	if err != nil {
		return UploadTask{}, err
	}
	if task.CreatorID != creator {
		return UploadTask{}, ErrUploadOwnership
	}
	return task, nil
}
func (s *UploadService) ensureActive(ctx context.Context, task UploadTask) error {
	if !task.ExpiresAt.After(s.now()) {
		if claimed, ok, _ := s.repo.ClaimUploadCleanup(ctx, task, UploadExpired); ok {
			_ = s.cleanupTask(ctx, &claimed, UploadExpired)
		}
		return ErrUploadExpired
	}
	if task.Status != UploadInitiated && task.Status != UploadUploading {
		return fmt.Errorf("upload is %s", task.Status)
	}
	if task.AttemptCount > s.config.MaxAttempts {
		return ErrUploadAttempts
	}
	return nil
}
func (s *UploadService) fail(ctx context.Context, task UploadTask, cause error) error {
	baseCtx := context.WithoutCancel(ctx)
	failureCtx, cancel := context.WithTimeout(baseCtx, uploadFailurePersistenceTimeout)
	defer cancel()
	ctx = failureCtx
	var errs []error
	var coverObjectKey string
	task.Status = UploadFailed
	task.CleanupStatus = "pending"
	task.FailureReason = cause.Error()
	content, contentErr := s.repo.GetContent(ctx, task.ContentID)
	if contentErr != nil {
		errs = append(errs, contentErr)
	} else {
		var media MediaAsset
		if task.MediaAssetID != nil {
			media, _ = s.repoMedia(ctx, *task.MediaAssetID)
		} else {
			var createErr error
			media, createErr = s.repo.CreateMediaAsset(ctx, MediaAsset{Bucket: s.config.Bucket, ObjectKey: task.ObjectKey, Checksum: task.Checksum, ContentType: content.ContentType, SizeBytes: task.ExpectedSize, StorageStatus: MediaProcessing, CreatedBy: &task.CreatorID})
			if createErr != nil {
				errs = append(errs, createErr)
			}
			if media.ID > 0 {
				task.MediaAssetID = &media.ID
				if _, saveErr := s.repo.SaveUploadTask(ctx, task); saveErr != nil {
					errs = append(errs, saveErr)
				}
			}
		}
		if _, stateErr := s.repo.SetContentMediaState(ctx, task.ContentID, task.MediaAssetID, ContentProcessing, 0); stateErr != nil {
			errs = append(errs, stateErr)
		}
		coverObjectKey = media.CoverObjectKey
		media.StorageStatus = MediaFailed
		if _, updateErr := s.repo.UpdateMediaAsset(ctx, media); updateErr != nil {
			errs = append(errs, updateErr)
		}
		if _, stateErr := s.repo.SetContentMediaState(ctx, task.ContentID, task.MediaAssetID, ContentFailed, 0); stateErr != nil {
			errs = append(errs, stateErr)
		}
	}
	if _, saveErr := s.repo.SaveUploadTask(ctx, task); saveErr != nil {
		errs = append(errs, saveErr)
	}
	if coverObjectKey != "" {
		cleanupCtx, cleanupCancel := context.WithTimeout(baseCtx, uploadFailurePersistenceTimeout)
		if deleteErr := s.store.DeleteObject(cleanupCtx, coverObjectKey); deleteErr != nil && !storage.IsAlreadyGone(deleteErr) {
			errs = append(errs, deleteErr)
		}
		cleanupCancel()
	}
	if len(errs) > 0 {
		return errors.Join(cause, errors.Join(errs...))
	}
	return cause
}

func coverKeyFromFailure(reason string) string {
	const marker = "cover_object_key="
	index := strings.Index(reason, marker)
	if index < 0 {
		return ""
	}
	value := reason[index+len(marker):]
	if end := strings.IndexAny(value, "; \n\r\t"); end >= 0 {
		value = value[:end]
	}
	return strings.TrimSpace(value)
}
func validCRC64(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if !strings.HasPrefix(value, "crc64:") || len(value) == 6 {
		return false
	}
	for _, r := range strings.TrimPrefix(value, "crc64:") {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func (s *UploadService) cleanupTask(ctx context.Context, task *UploadTask, status UploadStatus) error {
	var cleanupErr error
	if task.MediaAssetID != nil {
		stateCtx, stateCancel := context.WithTimeout(context.WithoutCancel(ctx), uploadFailurePersistenceTimeout)
		media, mediaErr := s.repoMedia(stateCtx, *task.MediaAssetID)
		if mediaErr != nil {
			cleanupErr = mediaErr
		} else {
			media.StorageStatus = MediaFailed
			if _, err := s.repo.UpdateMediaAsset(stateCtx, media); err != nil {
				cleanupErr = err
			}
		}
		if _, err := s.repo.SetContentMediaState(stateCtx, task.ContentID, task.MediaAssetID, ContentFailed, 0); cleanupErr == nil && err != nil {
			cleanupErr = err
		}
		stateCancel()
	}
	head, headErr := s.store.HeadObject(ctx, task.ObjectKey)
	if headErr == nil && (head.ObjectKey != "" || head.Size > 0) {
		if err := s.store.DeleteObject(ctx, task.ObjectKey); cleanupErr == nil && err != nil && !storage.IsAlreadyGone(err) {
			cleanupErr = err
		}
	} else {
		if err := s.store.AbortMultipart(ctx, storage.AbortMultipartInput{ObjectKey: task.ObjectKey, UploadID: task.OSSUploadID}); err != nil && !storage.IsAlreadyGone(err) {
			cleanupErr = err
		}
		if err := s.store.DeleteObject(ctx, task.ObjectKey); cleanupErr == nil && err != nil && !storage.IsAlreadyGone(err) {
			cleanupErr = err
		}
	}
	if task.MediaAssetID != nil {
		if media, err := s.repoMedia(ctx, *task.MediaAssetID); err == nil && media.CoverObjectKey != "" {
			if err := s.store.DeleteObject(ctx, media.CoverObjectKey); cleanupErr == nil && err != nil && !storage.IsAlreadyGone(err) {
				cleanupErr = err
			}
		}
	}
	if traced := coverKeyFromFailure(task.FailureReason); traced != "" {
		if err := s.store.DeleteObject(ctx, traced); cleanupErr == nil && err != nil && !storage.IsAlreadyGone(err) {
			cleanupErr = err
		}
	}
	cleanupStatus := "cleaned"
	failureReason := ""
	if cleanupErr != nil {
		cleanupStatus = "failed"
		failureReason = cleanupErr.Error()
	}
	finished, ok, finishErr := s.repo.FinishUploadCleanup(ctx, *task, status, cleanupStatus, failureReason)
	if ok {
		*task = finished
	}
	if cleanupErr != nil {
		return cleanupErr
	}
	if finishErr != nil {
		return finishErr
	}
	if !ok {
		return ErrUploadConflict
	}
	return nil
}
func (s *UploadService) CleanupPending(ctx context.Context, limit int) (int, error) {
	tasks, err := s.repo.ListExpiredUploadTasks(ctx, limit)
	if err != nil {
		return 0, err
	}
	cleaned := 0
	for i := range tasks {
		status := tasks[i].Status
		if status == UploadInitiating || status == UploadInitiated || status == UploadUploading || status == UploadCompleting || status == UploadCleaning {
			status = UploadExpired
		}
		claimed, ok, claimErr := s.repo.ClaimUploadCleanup(ctx, tasks[i], status)
		if claimErr != nil {
			return cleaned, claimErr
		}
		if !ok {
			continue
		}
		if err := s.cleanupTask(ctx, &claimed, status); err != nil {
			return cleaned, err
		}
		cleaned++
	}
	return cleaned, nil
}
func (s *UploadService) CleanupExpired(ctx context.Context, limit int) (int, error) {
	return s.CleanupPending(ctx, limit)
}
func (s *UploadService) waitForCompleted(ctx context.Context, id int64) (CompleteUploadResult, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(s.config.CompletionWait)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return CompleteUploadResult{}, ctx.Err()
		case <-timeout.C:
			return CompleteUploadResult{}, ErrUploadInProgress
		case <-ticker.C:
			task, err := s.repo.GetUploadTask(ctx, id)
			if err != nil {
				return CompleteUploadResult{}, err
			}
			if task.Status == UploadCompleted {
				media := MediaAsset{}
				if task.MediaAssetID != nil {
					media, _ = s.repoMedia(ctx, *task.MediaAssetID)
				}
				content, _ := s.repo.GetContent(ctx, task.ContentID)
				return CompleteUploadResult{Task: task, Media: media, Content: content}, nil
			}
			if task.Status == UploadFailed {
				return CompleteUploadResult{}, errors.New(task.FailureReason)
			}
			if task.Status == UploadAborted || task.Status == UploadExpired {
				return CompleteUploadResult{}, ErrUploadConflict
			}
		}
	}
}
func (s *UploadService) waitForCleaned(ctx context.Context, id int64) (UploadTask, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return UploadTask{}, ctx.Err()
		case <-timeout.C:
			return UploadTask{}, ErrUploadConflict
		case <-ticker.C:
			task, err := s.repo.GetUploadTask(ctx, id)
			if err != nil {
				return UploadTask{}, err
			}
			if task.CleanupStatus == "cleaned" {
				return task, nil
			}
			if task.CleanupStatus == "failed" {
				return UploadTask{}, errors.New(task.FailureReason)
			}
		}
	}
}

func (s *UploadService) objectKey(content Content, filename string) string {
	now := s.now().UTC()
	ext := strings.ToLower(filepath.Ext(filename))
	kind := string(content.ContentType)
	return fmt.Sprintf("%s/%s/%04d/%02d/%02d/content-%d/%s%s", strings.Trim(s.config.Prefix, "/"), kind, now.Year(), now.Month(), now.Day(), content.ID, randomUploadToken(), safeMediaExtension(content.ContentType, ext))
}
func randomUploadToken() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
func safeMediaExtension(kind ContentType, ext string) string {
	if kind == ContentVideo {
		return ".mp4"
	}
	if ext == ".m4a" || ext == ".mp3" {
		return ext
	}
	return ".m4a"
}
func mimeMatchesContent(kind ContentType, mime, name string) bool {
	mime = strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	ext := strings.ToLower(filepath.Ext(name))
	if kind == ContentVideo {
		return mime == "video/mp4" && (ext == ".mp4" || ext == "")
	}
	return (mime == "audio/mpeg" && (ext == ".mp3" || ext == "")) || ((mime == "audio/mp4" || mime == "audio/x-m4a") && (ext == ".m4a" || ext == ""))
}
func validateCompletedParts(task UploadTask, provided []storage.CompletedPart, listed []storage.MultipartPart) error {
	if len(provided) == 0 || len(provided) != len(listed) || len(provided) > task.MaxParts {
		return ErrInvalidUploadPart
	}
	listedByNo := map[int]storage.MultipartPart{}
	var total int64
	for _, p := range listed {
		if p.PartNumber < 1 {
			return ErrInvalidUploadPart
		}
		if _, exists := listedByNo[p.PartNumber]; exists {
			return ErrInvalidUploadPart
		}
		listedByNo[p.PartNumber] = p
		total += p.Size
	}
	seen := map[int]struct{}{}
	for _, p := range provided {
		if _, duplicate := seen[p.PartNumber]; duplicate {
			return ErrInvalidUploadPart
		}
		seen[p.PartNumber] = struct{}{}
		stored, ok := listedByNo[p.PartNumber]
		if !ok || strings.Trim(stored.ETag, `"`) != strings.Trim(p.ETag, `"`) || strings.TrimSpace(p.ETag) == "" {
			return ErrInvalidUploadPart
		}
	}
	if len(seen) != len(listedByNo) || total != task.ExpectedSize {
		return ErrInvalidUploadPart
	}
	sort.Slice(provided, func(i, j int) bool { return provided[i].PartNumber < provided[j].PartNumber })
	return nil
}

func validateHead(task UploadTask, completed storage.CompleteMultipartResult, head storage.ObjectMetadata) error {
	if head.Size != task.ExpectedSize {
		return fmt.Errorf("object size mismatch")
	}
	if completed.Checksum != "" && !strings.EqualFold(strings.TrimSpace(completed.Checksum), strings.TrimSpace(task.Checksum)) {
		return fmt.Errorf("completed checksum mismatch")
	}
	if strings.TrimSpace(head.Checksum) == "" || !strings.EqualFold(strings.TrimSpace(head.Checksum), strings.TrimSpace(task.Checksum)) {
		return fmt.Errorf("object checksum mismatch")
	}
	if strings.Trim(head.ETag, `"`) == "" || strings.Trim(completed.ETag, `"`) != "" && strings.Trim(head.ETag, `"`) != strings.Trim(completed.ETag, `"`) {
		return fmt.Errorf("object etag mismatch")
	}
	return nil
}

func ValidateMediaProbe(kind ContentType, p MediaProbeResult) error {
	container := strings.ToLower(p.Container)
	video := strings.ToLower(p.VideoCodec)
	audio := strings.ToLower(p.AudioCodec)
	if p.DurationSeconds <= 0 {
		return errors.New("media duration is invalid")
	}
	switch kind {
	case ContentVideo:
		if container != "mp4" || video != "h264" || audio != "aac" {
			return errors.New("video must be MP4 with H.264 and AAC")
		}
	case ContentAudio:
		if !((container == "mp3" && audio == "mp3") || (container == "m4a" && audio == "aac")) {
			return errors.New("audio must be MP3 or M4A/AAC")
		}
	default:
		return errors.New("unsupported classroom media type")
	}
	return nil
}

// FFProbe probes the private object through a short-lived signed URL.
type FFProbe struct {
	Signer storage.ObjectSigner
	Binary string
}

func (p FFProbe) Probe(ctx context.Context, input ProbeInput) (MediaProbeResult, error) {
	if p.Signer == nil {
		return MediaProbeResult{}, errors.New("media signer is unavailable")
	}
	url, err := p.Signer.PresignGetURL(ctx, input.ObjectKey, 5*time.Minute)
	if err != nil {
		return MediaProbeResult{}, err
	}
	binary := p.Binary
	if binary == "" {
		binary = "ffprobe"
	}
	out, err := exec.CommandContext(ctx, binary, "-v", "error", "-show_entries", "format=format_name,duration:stream=codec_type,codec_name,width,height", "-of", "json", url).Output()
	if err != nil {
		return MediaProbeResult{}, fmt.Errorf("ffprobe: %w", err)
	}
	var raw struct {
		Format struct {
			FormatName string `json:"format_name"`
			Duration   string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if err = json.Unmarshal(out, &raw); err != nil {
		return MediaProbeResult{}, err
	}
	result := MediaProbeResult{}
	names := strings.Split(raw.Format.FormatName, ",")
	for _, name := range names {
		if name == "mp4" || name == "mov" || name == "m4a" {
			result.Container = "mp4"
			if strings.Contains(strings.ToLower(input.ContentType), "audio") {
				result.Container = "m4a"
			}
			break
		}
		if name == "mp3" {
			result.Container = "mp3"
		}
	}
	var duration float64
	_, _ = fmt.Sscanf(raw.Format.Duration, "%f", &duration)
	result.DurationSeconds = int(duration + 0.5)
	for _, stream := range raw.Streams {
		if stream.CodecType == "video" {
			result.VideoCodec = stream.CodecName
			result.Width = stream.Width
			result.Height = stream.Height
		}
		if stream.CodecType == "audio" {
			result.AudioCodec = stream.CodecName
		}
	}
	return result, nil
}

// Store upload persistence methods live here to keep the classroom store API cohesive.
func (s *Store) FindUploadTaskByContent(ctx context.Context, contentID int64) (UploadTask, error) {
	var item UploadTask
	err := s.db.QueryRowContext(ctx, `SELECT id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at FROM classroom_upload_tasks WHERE content_id=$1`, contentID).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return UploadTask{}, ErrNotFound
	}
	return item, err
}
func (s *Store) ReserveUploadInitiation(ctx context.Context, item UploadTask, expected *UploadTask) (UploadTask, bool, error) {
	if item.Status != UploadInitiating {
		return UploadTask{}, false, errors.New("upload reservation must be initiating")
	}
	if err := item.Validate(); err != nil {
		return UploadTask{}, false, err
	}
	var err error
	if expected == nil {
		err = s.db.QueryRowContext(ctx, `INSERT INTO classroom_upload_tasks (content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason) VALUES ($1,$2,$3,$4,$5,$6,$7,0,0,$8,$9,'initiating',$10,$11,'pending',NULL,'') ON CONFLICT (content_id) DO NOTHING RETURNING id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at`, item.ContentID, item.CreatorID, item.OriginalFilename, item.OSSUploadID, item.ObjectKey, item.ExpectedSize, item.Checksum, item.PartSize, item.MaxParts, item.ExpiresAt, item.AttemptCount).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	} else {
		err = s.db.QueryRowContext(ctx, `UPDATE classroom_upload_tasks SET creator_id=$1,original_filename=$2,oss_upload_id=$3,object_key=$4,expected_size=$5,checksum=$6,completed_parts=0,completed_bytes=0,part_size=$7,max_parts=$8,status='initiating',expires_at=$9,attempt_count=$10,cleanup_status='pending',media_asset_id=NULL,failure_reason='',updated_at=now() WHERE id=$11 AND status=$12 AND updated_at=$13 RETURNING id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at`, item.CreatorID, item.OriginalFilename, item.OSSUploadID, item.ObjectKey, item.ExpectedSize, item.Checksum, item.PartSize, item.MaxParts, item.ExpiresAt, item.AttemptCount, expected.ID, expected.Status, expected.UpdatedAt).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	}
	if errors.Is(err, sql.ErrNoRows) {
		current, getErr := s.FindUploadTaskByContent(ctx, item.ContentID)
		return current, false, getErr
	}
	return item, err == nil, err
}
func (s *Store) ConfirmUploadInitiation(ctx context.Context, expected UploadTask, uploadID string) (UploadTask, bool, error) {
	var item UploadTask
	err := s.db.QueryRowContext(ctx, `UPDATE classroom_upload_tasks SET oss_upload_id=$1,status='initiated',updated_at=now() WHERE id=$2 AND status='initiating' AND updated_at=$3 RETURNING id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at`, uploadID, expected.ID, expected.UpdatedAt).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		current, getErr := s.GetUploadTask(ctx, expected.ID)
		return current, false, getErr
	}
	return item, err == nil, err
}
func (s *Store) FailUploadInitiation(ctx context.Context, expected UploadTask, uploadID, cleanupStatus, reason string) (UploadTask, bool, error) {
	if cleanupStatus != "cleaned" && cleanupStatus != "pending" {
		return UploadTask{}, false, errors.New("invalid initiation cleanup status")
	}
	var item UploadTask
	err := s.db.QueryRowContext(ctx, `UPDATE classroom_upload_tasks SET oss_upload_id=CASE WHEN $1='' THEN oss_upload_id ELSE $1 END,status='failed',cleanup_status=$2,failure_reason=$3,updated_at=now() WHERE id=$4 AND status='initiating' AND updated_at=$5 RETURNING id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at`, uploadID, cleanupStatus, reason, expected.ID, expected.UpdatedAt).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		current, getErr := s.GetUploadTask(ctx, expected.ID)
		return current, false, getErr
	}
	return item, err == nil, err
}
func (s *Store) SaveUploadTask(ctx context.Context, item UploadTask) (UploadTask, error) {
	if err := item.Validate(); err != nil {
		return UploadTask{}, err
	}
	err := s.db.QueryRowContext(ctx, `UPDATE classroom_upload_tasks SET creator_id=$1,original_filename=$2,oss_upload_id=$3,object_key=$4,expected_size=$5,checksum=$6,completed_parts=$7,completed_bytes=$8,part_size=$9,max_parts=$10,status=$11,expires_at=$12,attempt_count=$13,cleanup_status=$14,media_asset_id=$15,failure_reason=$16,updated_at=now() WHERE id=$17 RETURNING created_at,updated_at`, item.CreatorID, item.OriginalFilename, item.OSSUploadID, item.ObjectKey, item.ExpectedSize, item.Checksum, item.CompletedParts, item.CompletedBytes, item.PartSize, item.MaxParts, item.Status, item.ExpiresAt, item.AttemptCount, item.CleanupStatus, item.MediaAssetID, item.FailureReason, item.ID).Scan(&item.CreatedAt, &item.UpdatedAt)
	return item, err
}
func (s *Store) UpdateMediaAsset(ctx context.Context, item MediaAsset) (MediaAsset, error) {
	if err := item.Validate(); err != nil {
		return MediaAsset{}, err
	}
	err := s.db.QueryRowContext(ctx, `UPDATE classroom_media_assets SET bucket=$1,object_key=$2,etag=$3,checksum=$4,content_type=$5,size_bytes=$6,duration_seconds=$7,width=$8,height=$9,cover_object_key=$10,storage_status=$11,updated_at=now() WHERE id=$12 RETURNING created_at,updated_at`, item.Bucket, item.ObjectKey, item.ETag, item.Checksum, item.ContentType, item.SizeBytes, item.DurationSeconds, item.Width, item.Height, item.CoverObjectKey, item.StorageStatus, item.ID).Scan(&item.CreatedAt, &item.UpdatedAt)
	return item, err
}
func (s *Store) SetContentMediaState(ctx context.Context, contentID int64, mediaID *int64, status ContentStatus, duration int) (Content, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE classroom_contents SET media_asset_id=$1,duration_seconds=$2,status=$3,updated_at=now() WHERE id=$4 AND status IN ('draft','failed','processing')`, mediaID, duration, status, contentID)
	if err != nil {
		return Content{}, err
	}
	return s.GetContent(ctx, contentID)
}
func (s *Store) ListExpiredUploadTasks(ctx context.Context, limit int) ([]UploadTask, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at FROM classroom_upload_tasks WHERE ((((status IN ('initiating','initiated','uploading','completing') AND expires_at<=now()) OR status IN ('failed','expired','aborted')) AND cleanup_status IN ('pending','failed')) OR (status='cleaning' AND cleanup_status IN ('pending','failed') AND updated_at <= now()-interval '15 minutes')) ORDER BY expires_at,id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []UploadTask{}
	for rows.Next() {
		var item UploadTask
		if err := rows.Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) MarkUploadUploading(ctx context.Context, id int64) (UploadTask, error) {
	var item UploadTask
	err := s.db.QueryRowContext(ctx, `UPDATE classroom_upload_tasks SET status='uploading',updated_at=now() WHERE id=$1 AND status='initiated' RETURNING id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at`, id).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return s.GetUploadTask(ctx, id)
	}
	return item, err
}

func (s *Store) UpdateUploadProgress(ctx context.Context, id, creatorID int64, completedParts int, completedBytes int64) (UploadTask, error) {
	var item UploadTask
	err := s.db.QueryRowContext(ctx, `UPDATE classroom_upload_tasks SET completed_parts=$1,completed_bytes=$2,status=CASE WHEN status='initiated' THEN 'uploading' ELSE status END,updated_at=now() WHERE id=$3 AND creator_id=$4 AND status IN ('initiated','uploading') AND completed_parts<=$1 AND completed_bytes<=$2 AND max_parts>=$1 AND expected_size>=$2 RETURNING id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at`, completedParts, completedBytes, id, creatorID).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return UploadTask{}, ErrInvalidUploadProgress
	}
	return item, err
}

func (s *Store) ClaimUploadCompletion(ctx context.Context, id int64) (UploadTask, bool, error) {
	return s.claimUploadStatus(ctx, id, []UploadStatus{UploadInitiated, UploadUploading}, UploadCompleting, "pending")
}
func (s *Store) ClaimUploadAbort(ctx context.Context, id int64) (UploadTask, bool, error) {
	return s.claimUploadStatus(ctx, id, []UploadStatus{UploadInitiated, UploadUploading, UploadFailed}, UploadAborted, "pending")
}
func (s *Store) claimUploadStatus(ctx context.Context, id int64, from []UploadStatus, to UploadStatus, cleanup string) (UploadTask, bool, error) {
	values := make([]string, len(from))
	args := []any{to, cleanup, id}
	for i, status := range from {
		values[i] = fmt.Sprintf("$%d", i+4)
		args = append(args, status)
	}
	query := `UPDATE classroom_upload_tasks SET status=$1,cleanup_status=$2,expires_at=CASE WHEN $1='completing' THEN GREATEST(expires_at,now()+interval '30 minutes') ELSE expires_at END,updated_at=now() WHERE id=$3 AND status IN (` + strings.Join(values, ",") + `) RETURNING id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at`
	var item UploadTask
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		current, getErr := s.GetUploadTask(ctx, id)
		return current, false, getErr
	}
	return item, err == nil, err
}

func (s *Store) ClaimUploadCleanup(ctx context.Context, expected UploadTask, status UploadStatus) (UploadTask, bool, error) {
	var item UploadTask
	err := s.db.QueryRowContext(ctx, `UPDATE classroom_upload_tasks SET status='cleaning',cleanup_status='pending',updated_at=now() WHERE id=$1 AND updated_at=$3 AND ((status=$2 AND status<>'cleaning') OR (status='cleaning' AND updated_at <= now()-interval '15 minutes')) RETURNING id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at`, expected.ID, expected.Status, expected.UpdatedAt).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return UploadTask{}, false, nil
	}
	return item, err == nil, err
}

func (s *Store) FinishUploadCleanup(ctx context.Context, expected UploadTask, status UploadStatus, cleanupStatus, failureReason string) (UploadTask, bool, error) {
	var item UploadTask
	err := s.db.QueryRowContext(ctx, `UPDATE classroom_upload_tasks SET status=$1,cleanup_status=$2,failure_reason=$3,updated_at=now() WHERE id=$4 AND status='cleaning' AND updated_at=$5 RETURNING id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at`, status, cleanupStatus, failureReason, expected.ID, expected.UpdatedAt).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return UploadTask{}, false, nil
	}
	return item, err == nil, err
}

func (s *Store) FinalizeUpload(ctx context.Context, task UploadTask, media MediaAsset) (UploadTask, MediaAsset, Content, error) {
	if err := task.Validate(); err != nil {
		return UploadTask{}, MediaAsset{}, Content{}, err
	}
	if err := media.Validate(); err != nil {
		return UploadTask{}, MediaAsset{}, Content{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UploadTask{}, MediaAsset{}, Content{}, err
	}
	defer func() { _ = tx.Rollback() }()
	err = tx.QueryRowContext(ctx, `UPDATE classroom_media_assets SET bucket=$1,object_key=$2,etag=$3,checksum=$4,content_type=$5,size_bytes=$6,duration_seconds=$7,width=$8,height=$9,cover_object_key=$10,storage_status='ready',updated_at=now() WHERE id=$11 AND storage_status='processing' RETURNING created_at,updated_at`, media.Bucket, media.ObjectKey, media.ETag, media.Checksum, media.ContentType, media.SizeBytes, media.DurationSeconds, media.Width, media.Height, media.CoverObjectKey, media.ID).Scan(&media.CreatedAt, &media.UpdatedAt)
	if err != nil {
		return UploadTask{}, MediaAsset{}, Content{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE classroom_contents SET media_asset_id=$1,duration_seconds=$2,status='ready',updated_at=now() WHERE id=$3 AND status='processing'`, media.ID, media.DurationSeconds, task.ContentID)
	if err != nil {
		return UploadTask{}, MediaAsset{}, Content{}, err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return UploadTask{}, MediaAsset{}, Content{}, ErrConflict
	}
	err = tx.QueryRowContext(ctx, `UPDATE classroom_upload_tasks SET status='completed',cleanup_status='retained',completed_parts=max_parts,completed_bytes=expected_size,media_asset_id=$1,failure_reason='',updated_at=now() WHERE id=$2 AND status='completing' RETURNING created_at,updated_at`, media.ID, task.ID).Scan(&task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return UploadTask{}, MediaAsset{}, Content{}, err
	}
	task.CompletedParts = task.MaxParts
	task.CompletedBytes = task.ExpectedSize
	content, err := getContent(ctx, tx, task.ContentID, false)
	if err != nil {
		return UploadTask{}, MediaAsset{}, Content{}, err
	}
	if err = tx.Commit(); err != nil {
		return UploadTask{}, MediaAsset{}, Content{}, err
	}
	return task, media, content, nil
}

func (s *UploadService) repoMedia(ctx context.Context, id int64) (MediaAsset, error) {
	if store, ok := s.repo.(interface {
		GetMediaAsset(context.Context, int64) (MediaAsset, error)
	}); ok {
		return store.GetMediaAsset(ctx, id)
	}
	return MediaAsset{ID: id, StorageStatus: MediaReady}, nil
}

// FFmpegCoverExtractor extracts the first decodable video frame and persists it through ObjectUploader.
type FFmpegCoverExtractor struct {
	Signer   storage.ObjectSigner
	Uploader storage.ObjectUploader
	Binary   string
}

func (e FFmpegCoverExtractor) Extract(ctx context.Context, objectKey string, contentID int64, ratio CoverAspectRatio) (string, error) {
	if e.Signer == nil || e.Uploader == nil {
		return "", errors.New("cover extractor storage is unavailable")
	}
	url, err := e.Signer.PresignGetURL(ctx, objectKey, 5*time.Minute)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "classroom-cover-*.jpg")
	if err != nil {
		return "", err
	}
	path := file.Name()
	_ = file.Close()
	defer os.Remove(path)
	width, height, err := coverDimensions(ratio)
	if err != nil {
		return "", err
	}
	binary := e.Binary
	if binary == "" {
		binary = "ffmpeg"
	}
	filter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", width, height, width, height)
	if output, runErr := exec.CommandContext(ctx, binary, "-y", "-i", url, "-map", "0:v:0", "-vf", filter, "-frames:v", "1", path).CombinedOutput(); runErr != nil {
		return "", fmt.Errorf("ffmpeg snapshot: %w: %s", runErr, strings.TrimSpace(string(output)))
	}
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	reader, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	filename, err := randomCoverName(".jpg")
	if err != nil {
		return "", fmt.Errorf("create generated classroom cover name: %w", err)
	}
	result, err := e.Uploader.Upload(ctx, storage.UploadInput{ContentType: "image/jpeg", Dir: fmt.Sprintf("classroom/covers/generated/%d", contentID), Filename: filename, Reader: reader, Size: stat.Size()})
	if err != nil {
		return "", err
	}
	if result.Key != "" {
		return result.Key, nil
	}
	return result.ObjectKey, nil
}

func coverDimensions(ratio CoverAspectRatio) (int, int, error) {
	normalized, err := NormalizeCoverAspectRatio(ratio)
	if err != nil {
		return 0, 0, err
	}
	switch normalized {
	case CoverAspectRatio16x9:
		return 1280, 720, nil
	case CoverAspectRatio9x16:
		return 720, 1280, nil
	case CoverAspectRatio1x1:
		return 1080, 1080, nil
	default:
		return 0, 0, fmt.Errorf("invalid cover aspect ratio %q", ratio)
	}
}
