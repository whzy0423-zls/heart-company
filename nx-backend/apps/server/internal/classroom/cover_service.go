package classroom

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
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

	"nine-xing/nx-backend/apps/server/internal/storage"
)

var (
	ErrInvalidCoverImage       = errors.New("invalid classroom cover image")
	ErrCoverStorageUnavailable = errors.New("classroom cover storage unavailable")
)

const DefaultCoverImageMaxBytes int64 = 10 << 20

type classroomCoverStorage interface {
	storage.ObjectUploader
	storage.ObjectSigner
	DeleteObject(context.Context, string) error
}

type coverContentStore interface {
	GetContent(context.Context, int64) (Content, error)
	SetContentManualCover(context.Context, int64, string, time.Time, *int64) (Content, error)
}

type CoverService struct {
	store    coverContentStore
	objects  classroomCoverStorage
	maxBytes int64
}

func NewCoverService(store coverContentStore, objects classroomCoverStorage, maxBytes int64) *CoverService {
	if maxBytes <= 0 {
		maxBytes = DefaultCoverImageMaxBytes
	}
	return &CoverService{store: store, objects: objects, maxBytes: maxBytes}
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
		if cleanupErr := s.objects.DeleteObject(ctx, newKey); cleanupErr != nil && !storage.IsAlreadyGone(cleanupErr) {
			log.Printf("classroom manual cover orphan cleanup failed key=%s: %v", newKey, cleanupErr)
		}
		return Content{}, err
	}
	oldKey := strings.TrimSpace(current.ManualCoverObjectKey)
	if oldKey != "" && oldKey != newKey {
		if cleanupErr := s.objects.DeleteObject(ctx, oldKey); cleanupErr != nil && !storage.IsAlreadyGone(cleanupErr) {
			log.Printf("classroom replaced cover cleanup failed key=%s: %v", oldKey, cleanupErr)
		}
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
	if oldKey == "" {
		return current, nil
	}
	updated, err := s.store.SetContentManualCover(ctx, contentID, "", expected, updatedBy)
	if err != nil {
		return Content{}, err
	}
	if s.objects != nil {
		if cleanupErr := s.objects.DeleteObject(ctx, oldKey); cleanupErr != nil && !storage.IsAlreadyGone(cleanupErr) {
			log.Printf("classroom deleted cover cleanup failed key=%s: %v", oldKey, cleanupErr)
		}
	}
	return updated, nil
}

func coverImageType(data []byte) (string, string) {
	switch http.DetectContentType(data) {
	case "image/jpeg":
		if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
			return "", ""
		}
		return "image/jpeg", ".jpg"
	case "image/png":
		if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
			return "", ""
		}
		return "image/png", ".png"
	case "image/webp":
		if !validWebP(data) {
			return "", ""
		}
		return "image/webp", ".webp"
	default:
		return "", ""
	}
}

func validWebP(data []byte) bool {
	if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" || int(binary.LittleEndian.Uint32(data[4:8]))+8 > len(data) {
		return false
	}
	switch string(data[12:16]) {
	case "VP8X":
		return len(data) >= 30 && (data[24] != 0 || data[25] != 0 || data[26] != 0) && (data[27] != 0 || data[28] != 0 || data[29] != 0)
	case "VP8L":
		return len(data) >= 25 && data[20] == 0x2f
	case "VP8 ":
		return len(data) >= 30 && data[23] == 0x9d && data[24] == 0x01 && data[25] == 0x2a
	default:
		return false
	}
}

func randomCoverName(ext string) (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]) + strings.ToLower(filepath.Ext(ext)), nil
}
