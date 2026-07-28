package classroom

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/webp"

	"nine-xing/nx-backend/apps/server/internal/storage"
)

var (
	ErrInvalidCoverImage       = errors.New("invalid classroom cover image")
	ErrCoverStorageUnavailable = errors.New("classroom cover storage unavailable")
)

const DefaultCoverImageMaxBytes int64 = 10 << 20

const (
	maxCoverImageDimension = 16_384
	maxCoverImagePixels    = 40_000_000
	coverCleanupTimeout    = 3 * time.Second
)

type classroomCoverStorage interface {
	storage.ObjectUploader
	storage.ObjectSigner
	DeleteObject(context.Context, string) error
}

type coverContentStore interface {
	GetContent(context.Context, int64) (Content, error)
	SetContentManualCover(context.Context, int64, string, time.Time, *int64) (Content, error)
}

type coverSettingsStore interface {
	GetMediaAsset(context.Context, int64) (MediaAsset, error)
	SetContentCoverSettings(context.Context, int64, CoverAspectRatio, time.Time, *int64, *int64, string, string) (Content, error)
}

type CoverService struct {
	store     coverContentStore
	objects   classroomCoverStorage
	extractor CoverExtractor
	maxBytes  int64
}

func NewCoverService(store coverContentStore, objects classroomCoverStorage, maxBytes int64) *CoverService {
	if maxBytes <= 0 {
		maxBytes = DefaultCoverImageMaxBytes
	}
	svc := &CoverService{store: store, objects: objects, maxBytes: maxBytes}
	if objects != nil {
		svc.extractor = FFmpegCoverExtractor{Signer: objects, Uploader: objects}
	}
	return svc
}

func (s *CoverService) Upload(ctx context.Context, contentID int64, expected time.Time, updatedBy *int64, _ string, reader io.Reader) (Content, error) {
	if s == nil || s.store == nil {
		return Content{}, errors.New("classroom cover service unavailable")
	}
	current, err := s.store.GetContent(ctx, contentID)
	if err != nil {
		return Content{}, err
	}
	if expected.IsZero() || !current.UpdatedAt.Equal(expected) {
		return Content{}, ErrConflict
	}
	if s.objects == nil {
		return Content{}, ErrCoverStorageUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(reader, s.maxBytes+1))
	if err != nil {
		return Content{}, ErrInvalidCoverImage
	}
	if len(data) == 0 || int64(len(data)) > s.maxBytes {
		return Content{}, ErrInvalidCoverImage
	}
	mime, ext := coverImageType(data)
	if mime == "" {
		return Content{}, ErrInvalidCoverImage
	}
	name, err := randomCoverName(ext)
	if err != nil {
		return Content{}, fmt.Errorf("create classroom cover name: %w", err)
	}
	dir := fmt.Sprintf("classroom/covers/manual/%d", contentID)
	result, err := s.objects.Upload(ctx, storage.UploadInput{ContentType: mime, Dir: dir, Filename: name, Reader: bytes.NewReader(data), Size: int64(len(data))})
	if err != nil {
		return Content{}, ErrCoverStorageUnavailable
	}
	newKey := strings.TrimSpace(result.ObjectKey)
	if newKey == "" {
		newKey = strings.TrimSpace(result.Key)
	}
	if newKey == "" {
		return Content{}, ErrCoverStorageUnavailable
	}
	updated, err := s.store.SetContentManualCover(ctx, contentID, newKey, expected, updatedBy)
	if err != nil {
		s.cleanupObject(ctx, contentID, "upload_rollback", newKey)
		return Content{}, err
	}
	oldKey := strings.TrimSpace(current.ManualCoverObjectKey)
	if oldKey != "" && oldKey != newKey {
		s.cleanupObject(ctx, contentID, "replace_old", oldKey)
	}
	return updated, nil
}

func (s *CoverService) UpdateSettings(ctx context.Context, contentID int64, ratio CoverAspectRatio, expected time.Time, updatedBy *int64) (Content, error) {
	if s == nil || s.store == nil {
		return Content{}, errors.New("classroom cover service unavailable")
	}
	normalized, err := NormalizeCoverAspectRatio(ratio)
	if err != nil {
		return Content{}, err
	}
	current, err := s.store.GetContent(ctx, contentID)
	if err != nil {
		return Content{}, err
	}
	if expected.IsZero() || !current.UpdatedAt.Equal(expected) {
		return Content{}, ErrConflict
	}
	settings, ok := s.store.(coverSettingsStore)
	if !ok {
		return Content{}, errors.New("classroom cover settings unavailable")
	}
	if current.ContentType != ContentVideo || current.MediaAssetID == nil {
		return settings.SetContentCoverSettings(ctx, contentID, normalized, expected, updatedBy, nil, "", "")
	}
	if s.extractor == nil {
		return Content{}, ErrCoverStorageUnavailable
	}
	media, err := settings.GetMediaAsset(ctx, *current.MediaAssetID)
	if err != nil {
		return Content{}, err
	}
	newKey, err := s.extractor.Extract(ctx, media.ObjectKey, contentID, normalized)
	if err != nil {
		return Content{}, err
	}
	newKey = strings.TrimSpace(newKey)
	if newKey == "" {
		return Content{}, ErrCoverStorageUnavailable
	}
	updated, err := settings.SetContentCoverSettings(ctx, contentID, normalized, expected, updatedBy, current.MediaAssetID, media.CoverObjectKey, newKey)
	if err != nil {
		s.cleanupObject(ctx, contentID, "settings_rollback", newKey)
		return Content{}, err
	}
	if oldKey := strings.TrimSpace(media.CoverObjectKey); oldKey != "" && oldKey != newKey {
		s.cleanupObject(ctx, contentID, "settings_replace_old", oldKey)
	}
	return updated, nil
}

func (s *CoverService) Delete(ctx context.Context, contentID int64, expected time.Time, updatedBy *int64) (Content, error) {
	if s == nil || s.store == nil {
		return Content{}, errors.New("classroom cover service unavailable")
	}
	current, err := s.store.GetContent(ctx, contentID)
	if err != nil {
		return Content{}, err
	}
	if expected.IsZero() || !current.UpdatedAt.Equal(expected) {
		return Content{}, ErrConflict
	}
	oldKey := strings.TrimSpace(current.ManualCoverObjectKey)
	updated, err := s.store.SetContentManualCover(ctx, contentID, "", expected, updatedBy)
	if err != nil {
		return Content{}, err
	}
	if oldKey != "" {
		s.cleanupObject(ctx, contentID, "delete_manual", oldKey)
	}
	return updated, nil
}

func (s *CoverService) cleanupObject(ctx context.Context, contentID int64, stage, key string) {
	if s.objects == nil {
		log.Printf("classroom cover cleanup skipped content_id=%d stage=%s: deleter unavailable", contentID, stage)
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), coverCleanupTimeout)
	defer cancel()
	if err := s.objects.DeleteObject(cleanupCtx, key); err != nil && !storage.IsAlreadyGone(err) {
		log.Printf("classroom cover cleanup failed content_id=%d stage=%s: %v", contentID, stage, err)
	}
}

func coverImageType(data []byte) (string, string) {
	switch http.DetectContentType(data) {
	case "image/jpeg":
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || !validCoverDimensions(config.Width, config.Height) {
			return "", ""
		}
		if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
			return "", ""
		}
		return "image/jpeg", ".jpg"
	case "image/png":
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || !validCoverDimensions(config.Width, config.Height) {
			return "", ""
		}
		if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
			return "", ""
		}
		return "image/png", ".png"
	case "image/webp":
		config, err := webp.DecodeConfig(bytes.NewReader(data))
		if err != nil || !validCoverDimensions(config.Width, config.Height) {
			return "", ""
		}
		if _, err := webp.Decode(bytes.NewReader(data)); err != nil {
			return "", ""
		}
		return "image/webp", ".webp"
	default:
		return "", ""
	}
}

func validCoverDimensions(width, height int) bool {
	return width > 0 && height > 0 && width <= maxCoverImageDimension && height <= maxCoverImageDimension && int64(width)*int64(height) <= maxCoverImagePixels
}

func randomCoverName(ext string) (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]) + strings.ToLower(filepath.Ext(ext)), nil
}
