package apprelease

import (
	"errors"
	"time"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

type Release struct {
	ID            int64      `json:"id"`
	Platform      string     `json:"platform"`
	VersionName   string     `json:"versionName"`
	VersionCode   int64      `json:"versionCode"`
	ReleaseNotes  string     `json:"releaseNotes"`
	FileName      string     `json:"fileName"`
	FilePath      string     `json:"-"`
	FileSize      int64      `json:"fileSize"`
	SHA256        string     `json:"sha256"`
	Status        Status     `json:"status"`
	FileAvailable bool       `json:"fileAvailable"`
	CreatedAt     time.Time  `json:"createdAt"`
	PublishedAt   *time.Time `json:"publishedAt"`
}

type StagedFile struct {
	TempPath     string
	OriginalName string
	Size         int64
	SHA256       string
}

type SavedFile struct {
	Key          string
	Path         string
	OriginalName string
	Size         int64
	SHA256       string
}

var (
	ErrFileTooLarge                  = errors.New("apprelease: file too large")
	ErrInvalidExtension              = errors.New("apprelease: invalid file extension")
	ErrUnsafePath                    = errors.New("apprelease: unsafe file path")
	ErrUnsupportedPlatform           = errors.New("apprelease: unsupported platform")
	ErrInvalidVersion                = errors.New("apprelease: invalid version")
	ErrInvalidAPK                    = errors.New("apprelease: invalid APK")
	ErrUnsignedAPK                   = errors.New("apprelease: unsigned APK")
	ErrPackageMismatch               = errors.New("apprelease: package name mismatch")
	ErrCertificateMismatch           = errors.New("apprelease: signing certificate mismatch")
	ErrPublishCertificateUnavailable = errors.New("apprelease: publish certificate unavailable")
	ErrNotFound                      = errors.New("apprelease: release not found")
	ErrConflict                      = errors.New("apprelease: release conflict")
)
