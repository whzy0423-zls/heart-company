package classroom

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type SeriesStatus string
type ContentStatus string
type AccessLevel string
type ContentType string
type MediaStatus string
type UploadStatus string
type EntitlementSource string

const (
	SeriesDraft     SeriesStatus = "draft"
	SeriesPublished SeriesStatus = "published"
	SeriesOffline   SeriesStatus = "offline"

	ContentDraft      ContentStatus = "draft"
	ContentProcessing ContentStatus = "processing"
	ContentReady      ContentStatus = "ready"
	ContentPublished  ContentStatus = "published"
	ContentOffline    ContentStatus = "offline"
	ContentFailed     ContentStatus = "failed"

	AccessInherit AccessLevel = "inherit"
	AccessPublic  AccessLevel = "public"
	AccessLogin   AccessLevel = "login"
	AccessMember  AccessLevel = "member"
	AccessPaid    AccessLevel = "paid"

	ContentVideo ContentType = "video"
	ContentAudio ContentType = "audio"

	MediaPending    MediaStatus = "pending"
	MediaUploaded   MediaStatus = "uploaded"
	MediaProcessing MediaStatus = "processing"
	MediaReady      MediaStatus = "ready"
	MediaFailed     MediaStatus = "failed"
	MediaDeleted    MediaStatus = "deleted"

	UploadInitiated UploadStatus = "initiated"
	UploadUploading UploadStatus = "uploading"
	UploadCompleted UploadStatus = "completed"
	UploadAborted   UploadStatus = "aborted"
	UploadExpired   UploadStatus = "expired"
	UploadFailed    UploadStatus = "failed"

	EntitlementPurchase EntitlementSource = "purchase"
	EntitlementManual   EntitlementSource = "manual"
)

var (
	ErrNotFound = errors.New("classroom record not found")
	ErrConflict = errors.New("classroom record was modified")
)

type Series struct {
	ID                  int64
	Title               string
	Summary             string
	CoverURL            string
	CoverAssetID        *int64
	TeacherKey          string
	TeacherNameSnapshot string
	SortOrder           int
	Status              SeriesStatus
	PlaybackBlocked     bool
	AccessLevel         AccessLevel
	PriceCents          int
	PublishedAt         *time.Time
	CreatedBy           *int64
	UpdatedBy           *int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (s Series) Validate() error {
	if strings.TrimSpace(s.Title) == "" {
		return errors.New("series title is required")
	}
	if !oneOf(string(s.Status), string(SeriesDraft), string(SeriesPublished), string(SeriesOffline)) {
		return fmt.Errorf("invalid series status %q", s.Status)
	}
	if !oneOf(string(s.AccessLevel), string(AccessPublic), string(AccessLogin), string(AccessMember), string(AccessPaid)) {
		return fmt.Errorf("invalid series access %q", s.AccessLevel)
	}
	if s.SortOrder < 0 {
		return errors.New("sort order must not be negative")
	}
	return validatePrice(s.AccessLevel, s.PriceCents)
}

func CanTransitionSeries(from, to SeriesStatus) bool {
	if from == to {
		return true
	}
	return (from == SeriesDraft && to == SeriesPublished) || (from == SeriesPublished && to == SeriesOffline)
}

type Content struct {
	ID                  int64
	SeriesID            *int64
	ShowAsStandalone    bool
	Title               string
	Description         string
	ContentType         ContentType
	MediaAssetID        *int64
	CoverURL            string
	DurationSeconds     int
	TeacherKey          string
	TeacherNameSnapshot string
	RecordedAt          *time.Time
	Badge               string
	Tags                []string
	EpisodeNo           int
	SortOrder           int
	Status              ContentStatus
	PlaybackBlocked     bool
	AccessLevel         AccessLevel
	PriceCents          int
	PublishedAt         *time.Time
	CreatedBy           *int64
	UpdatedBy           *int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (c Content) Validate() error {
	if strings.TrimSpace(c.Title) == "" {
		return errors.New("content title is required")
	}
	if !oneOf(string(c.ContentType), string(ContentVideo), string(ContentAudio)) {
		return fmt.Errorf("invalid content type %q", c.ContentType)
	}
	if !oneOf(string(c.Status), string(ContentDraft), string(ContentProcessing), string(ContentReady), string(ContentPublished), string(ContentOffline), string(ContentFailed)) {
		return fmt.Errorf("invalid content status %q", c.Status)
	}
	if !oneOf(string(c.AccessLevel), string(AccessInherit), string(AccessPublic), string(AccessLogin), string(AccessMember), string(AccessPaid)) {
		return fmt.Errorf("invalid content access %q", c.AccessLevel)
	}
	if c.SeriesID == nil && c.AccessLevel == AccessInherit {
		return errors.New("standalone content cannot inherit access")
	}
	if c.SeriesID == nil && c.ShowAsStandalone {
		return errors.New("show_as_standalone requires a series")
	}
	if c.EpisodeNo < 0 || c.SortOrder < 0 || c.DurationSeconds < 0 {
		return errors.New("numeric metadata must not be negative")
	}
	return validatePrice(c.AccessLevel, c.PriceCents)
}

func CanTransitionContent(from, to ContentStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case ContentDraft:
		return to == ContentProcessing
	case ContentProcessing:
		return to == ContentReady || to == ContentFailed
	case ContentReady:
		return to == ContentPublished || to == ContentProcessing
	case ContentPublished:
		return to == ContentOffline
	case ContentFailed:
		return to == ContentDraft || to == ContentProcessing
	default:
		return false
	}
}

func ValidateContentPublish(content Content, media MediaAsset, parent *Series) error {
	if err := content.Validate(); err != nil {
		return err
	}
	if content.Status != ContentReady {
		return errors.New("content must be ready before publication")
	}
	if content.MediaAssetID == nil || *content.MediaAssetID != media.ID {
		return errors.New("content media asset does not match")
	}
	if err := media.Validate(); err != nil {
		return err
	}
	if media.StorageStatus != MediaReady {
		return errors.New("media asset is not ready")
	}
	if media.ContentType != content.ContentType {
		return errors.New("media type does not match content")
	}
	if content.SeriesID != nil {
		if parent == nil || parent.ID != *content.SeriesID {
			return errors.New("parent series is required")
		}
		if parent.Status != SeriesPublished {
			return errors.New("parent series must be published")
		}
	}
	return nil
}

type MediaAsset struct {
	ID              int64
	Bucket          string
	ObjectKey       string
	ETag            string
	Checksum        string
	ContentType     ContentType
	SizeBytes       int64
	DurationSeconds int
	Width           int
	Height          int
	CoverObjectKey  string
	StorageStatus   MediaStatus
	CreatedBy       *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (m MediaAsset) Validate() error {
	if !oneOf(string(m.ContentType), string(ContentVideo), string(ContentAudio)) {
		return fmt.Errorf("invalid media type %q", m.ContentType)
	}
	if !oneOf(string(m.StorageStatus), string(MediaPending), string(MediaUploaded), string(MediaProcessing), string(MediaReady), string(MediaFailed), string(MediaDeleted)) {
		return fmt.Errorf("invalid media status %q", m.StorageStatus)
	}
	if m.SizeBytes < 0 || m.DurationSeconds < 0 || m.Width < 0 || m.Height < 0 {
		return errors.New("media metadata must not be negative")
	}
	if m.StorageStatus == MediaReady && (strings.TrimSpace(m.Bucket) == "" || strings.TrimSpace(m.ObjectKey) == "" || strings.TrimSpace(m.ETag) == "" || strings.TrimSpace(m.Checksum) == "" || m.SizeBytes == 0) {
		return errors.New("ready media requires complete object metadata")
	}
	return nil
}

type UploadTask struct {
	ID            int64
	ContentID     int64
	CreatorID     int64
	OSSUploadID   string
	ObjectKey     string
	ExpectedSize  int64
	Checksum      string
	PartSize      int64
	MaxParts      int
	Status        UploadStatus
	ExpiresAt     time.Time
	AttemptCount  int
	CleanupStatus string
	MediaAssetID  *int64
	FailureReason string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (u UploadTask) Validate() error {
	if u.ContentID <= 0 || u.CreatorID <= 0 {
		return errors.New("upload task requires content and creator")
	}
	if strings.TrimSpace(u.OSSUploadID) == "" || strings.TrimSpace(u.ObjectKey) == "" {
		return errors.New("upload task requires OSS metadata")
	}
	if u.ExpectedSize <= 0 || u.PartSize <= 0 || u.MaxParts <= 0 || u.AttemptCount < 0 {
		return errors.New("invalid upload task size or attempts")
	}
	if !oneOf(string(u.Status), string(UploadInitiated), string(UploadUploading), string(UploadCompleted), string(UploadAborted), string(UploadExpired), string(UploadFailed)) {
		return fmt.Errorf("invalid upload status %q", u.Status)
	}
	if u.ExpiresAt.IsZero() {
		return errors.New("upload task expiry is required")
	}
	return nil
}

type Entitlement struct {
	ID        int64
	WXUserID  int64
	SeriesID  *int64
	ContentID *int64
	OrderID   *int64
	Source    EntitlementSource
	ExpiresAt *time.Time
	RevokedAt *time.Time
	CreatedBy *int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (e Entitlement) Validate() error {
	if e.WXUserID <= 0 {
		return errors.New("entitlement user is required")
	}
	if (e.SeriesID == nil) == (e.ContentID == nil) {
		return errors.New("entitlement must target exactly one series or content")
	}
	if !oneOf(string(e.Source), string(EntitlementPurchase), string(EntitlementManual)) {
		return fmt.Errorf("invalid entitlement source %q", e.Source)
	}
	return nil
}

type Progress struct {
	WXUserID        int64
	ContentID       int64
	PositionSeconds int
	Completed       bool
	LastPlayedAt    time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func validatePrice(access AccessLevel, price int) error {
	if price < 0 {
		return errors.New("price must not be negative")
	}
	if access == AccessPaid && price <= 0 {
		return errors.New("paid access requires a positive price")
	}
	if access != AccessPaid && price != 0 {
		return errors.New("non-paid access must have zero price")
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func ValidateUploadDraftBinding(task UploadTask, content Content) error {
	if err := task.Validate(); err != nil {
		return err
	}
	if content.ID != task.ContentID {
		return errors.New("upload task content binding does not match")
	}
	if content.Status != ContentDraft && content.Status != ContentFailed {
		return errors.New("upload task requires a draft or failed content")
	}
	return nil
}
