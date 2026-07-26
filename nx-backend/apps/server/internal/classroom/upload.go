package classroom

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/storage"
)

var (
	ErrUploadOwnership   = errors.New("classroom upload is owned by another user")
	ErrUploadExpired     = errors.New("classroom upload expired")
	ErrInvalidUploadPart = errors.New("invalid classroom upload part")
	ErrUploadConflict    = errors.New("classroom content already has an active upload")
	ErrUploadAttempts    = errors.New("classroom upload retry limit reached")
)

type UploadConfig struct {
	Bucket        string
	Prefix        string
	PartSize      int64
	MaxParts      int
	CredentialTTL time.Duration
	TaskTTL       time.Duration
	MaxVideoBytes int64
	MaxAudioBytes int64
	MaxAttempts   int
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

type UploadRepository interface {
	GetContent(context.Context, int64) (Content, error)
	FindUploadTaskByContent(context.Context, int64) (UploadTask, error)
	CreateUploadTask(context.Context, UploadTask) (UploadTask, error)
	GetUploadTask(context.Context, int64) (UploadTask, error)
	SaveUploadTask(context.Context, UploadTask) (UploadTask, error)
	CreateMediaAsset(context.Context, MediaAsset) (MediaAsset, error)
	AttachMediaToContent(context.Context, int64, MediaAsset) (Content, error)
}

type UploadService struct {
	repo   UploadRepository
	store  storage.MultipartStorage
	probe  MediaProbe
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

func (s *UploadService) Initiate(ctx context.Context, input InitiateUploadInput) (InitiateUploadResult, error) {
	if input.ContentID <= 0 || input.CreatorID <= 0 || input.SizeBytes <= 0 || strings.TrimSpace(input.Checksum) == "" {
		return InitiateUploadResult{}, errors.New("invalid upload request")
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
	if findErr == nil && (previous.Status == UploadInitiated || previous.Status == UploadUploading) {
		return InitiateUploadResult{}, ErrUploadConflict
	}
	attempt := 1
	if findErr == nil {
		attempt = previous.AttemptCount + 1
		if attempt > s.config.MaxAttempts {
			return InitiateUploadResult{}, ErrUploadAttempts
		}
	}
	objectKey := s.objectKey(content, input.Filename)
	initiated, err := s.store.InitiateMultipart(ctx, storage.InitiateMultipartInput{ObjectKey: objectKey, ContentType: input.ContentType, Checksum: input.Checksum})
	if err != nil {
		return InitiateUploadResult{}, err
	}
	task := UploadTask{ContentID: input.ContentID, CreatorID: input.CreatorID, OSSUploadID: initiated.UploadID, ObjectKey: objectKey, ExpectedSize: input.SizeBytes, Checksum: strings.TrimSpace(input.Checksum), PartSize: s.config.PartSize, MaxParts: parts, Status: UploadInitiated, ExpiresAt: s.now().Add(s.config.TaskTTL), AttemptCount: attempt, CleanupStatus: "pending"}
	if findErr == nil {
		task.ID = previous.ID
		task.CreatedAt = previous.CreatedAt
		task, err = s.repo.SaveUploadTask(ctx, task)
	} else if errors.Is(findErr, ErrNotFound) || errors.Is(findErr, sql.ErrNoRows) {
		task, err = s.repo.CreateUploadTask(ctx, task)
	} else {
		return InitiateUploadResult{}, findErr
	}
	if err != nil {
		_ = s.store.AbortMultipart(ctx, storage.AbortMultipartInput{ObjectKey: objectKey, UploadID: initiated.UploadID})
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
		task.Status = UploadUploading
		if _, err = s.repo.SaveUploadTask(ctx, task); err != nil {
			return storage.SignPartResult{}, err
		}
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
	if err := s.ensureActive(ctx, task); err != nil {
		return CompleteUploadResult{}, err
	}
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
	media, err := s.repo.CreateMediaAsset(ctx, MediaAsset{Bucket: s.config.Bucket, ObjectKey: task.ObjectKey, ETag: head.ETag, Checksum: head.Checksum, ContentType: content.ContentType, SizeBytes: head.Size, DurationSeconds: probe.DurationSeconds, Width: probe.Width, Height: probe.Height, CoverObjectKey: probe.CoverObjectKey, StorageStatus: MediaReady, CreatedBy: &creatorID})
	if err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	content, err = s.repo.AttachMediaToContent(ctx, task.ContentID, media)
	if err != nil {
		return CompleteUploadResult{}, s.fail(ctx, task, err)
	}
	task.Status = UploadCompleted
	task.MediaAssetID = &media.ID
	task.FailureReason = ""
	task.CleanupStatus = "retained"
	task, err = s.repo.SaveUploadTask(ctx, task)
	if err != nil {
		return CompleteUploadResult{}, err
	}
	return CompleteUploadResult{Task: task, Media: media, Content: content}, nil
}

func (s *UploadService) Abort(ctx context.Context, taskID, creatorID int64) (UploadTask, error) {
	task, err := s.ownedTask(ctx, taskID, creatorID)
	if err != nil {
		return UploadTask{}, err
	}
	if task.Status == UploadAborted {
		return task, nil
	}
	if task.Status == UploadCompleted {
		return task, nil
	}
	if err = s.store.AbortMultipart(ctx, storage.AbortMultipartInput{ObjectKey: task.ObjectKey, UploadID: task.OSSUploadID}); err != nil {
		task.CleanupStatus = "failed"
		task.FailureReason = err.Error()
		_, _ = s.repo.SaveUploadTask(ctx, task)
		return UploadTask{}, err
	}
	task.Status = UploadAborted
	task.CleanupStatus = "clean"
	task.FailureReason = ""
	return s.repo.SaveUploadTask(ctx, task)
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
		task.Status = UploadExpired
		_, _ = s.repo.SaveUploadTask(ctx, task)
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
	task.Status = UploadFailed
	task.AttemptCount++
	task.FailureReason = cause.Error()
	_, _ = s.repo.SaveUploadTask(ctx, task)
	return cause
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
	byNo := map[int]storage.MultipartPart{}
	var total int64
	for _, p := range listed {
		byNo[p.PartNumber] = p
		total += p.Size
	}
	for _, p := range provided {
		stored, ok := byNo[p.PartNumber]
		if !ok || strings.Trim(stored.ETag, `"`) != strings.Trim(p.ETag, `"`) {
			return ErrInvalidUploadPart
		}
	}
	if total != task.ExpectedSize {
		return fmt.Errorf("uploaded size mismatch")
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
		if container != "mp4" || video != "h264" || (audio != "" && audio != "aac") {
			return errors.New("video must be MP4 with H.264 and AAC")
		}
	case ContentAudio:
		if !((container == "mp3" && (audio == "mp3" || audio == "aac")) || (container == "m4a" && audio == "aac")) {
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
	err := s.db.QueryRowContext(ctx, `SELECT id,content_id,creator_id,oss_upload_id,object_key,expected_size,checksum,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at FROM classroom_upload_tasks WHERE content_id=$1`, contentID).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return UploadTask{}, ErrNotFound
	}
	return item, err
}
func (s *Store) SaveUploadTask(ctx context.Context, item UploadTask) (UploadTask, error) {
	err := s.db.QueryRowContext(ctx, `UPDATE classroom_upload_tasks SET creator_id=$1,oss_upload_id=$2,object_key=$3,expected_size=$4,checksum=$5,part_size=$6,max_parts=$7,status=$8,expires_at=$9,attempt_count=$10,cleanup_status=$11,media_asset_id=$12,failure_reason=$13,updated_at=now() WHERE id=$14 RETURNING created_at,updated_at`, item.CreatorID, item.OSSUploadID, item.ObjectKey, item.ExpectedSize, item.Checksum, item.PartSize, item.MaxParts, item.Status, item.ExpiresAt, item.AttemptCount, item.CleanupStatus, item.MediaAssetID, item.FailureReason, item.ID).Scan(&item.CreatedAt, &item.UpdatedAt)
	return item, err
}
func (s *Store) AttachMediaToContent(ctx context.Context, contentID int64, media MediaAsset) (Content, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE classroom_contents SET media_asset_id=$1,duration_seconds=$2,status='ready',updated_at=now() WHERE id=$3 AND status IN ('draft','failed','processing')`, media.ID, media.DurationSeconds, contentID)
	if err != nil {
		return Content{}, err
	}
	return s.GetContent(ctx, contentID)
}
func (s *UploadService) repoMedia(ctx context.Context, id int64) (MediaAsset, error) {
	if store, ok := s.repo.(interface {
		GetMediaAsset(context.Context, int64) (MediaAsset, error)
	}); ok {
		return store.GetMediaAsset(ctx, id)
	}
	return MediaAsset{ID: id, StorageStatus: MediaReady}, nil
}
