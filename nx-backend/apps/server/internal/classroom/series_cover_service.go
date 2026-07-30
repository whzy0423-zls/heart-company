package classroom

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/storage"
)

type seriesCoverStore interface {
	GetSeries(context.Context, int64) (Series, error)
	SetSeriesManualCover(context.Context, int64, string, time.Time, *int64) (Series, error)
	SetSeriesCoverSettings(context.Context, int64, CoverAspectRatio, time.Time, *int64) (Series, error)
}

type SeriesCoverService struct {
	store    seriesCoverStore
	objects  classroomCoverStorage
	maxBytes int64
}

func NewSeriesCoverService(store seriesCoverStore, objects classroomCoverStorage, maxBytes int64) *SeriesCoverService {
	if maxBytes <= 0 {
		maxBytes = DefaultCoverImageMaxBytes
	}
	return &SeriesCoverService{store: store, objects: objects, maxBytes: maxBytes}
}

func (s *SeriesCoverService) Upload(ctx context.Context, seriesID int64, expected time.Time, updatedBy *int64, _ string, reader io.Reader) (Series, error) {
	if s == nil || s.store == nil {
		return Series{}, errors.New("classroom series cover service unavailable")
	}
	current, err := s.store.GetSeries(ctx, seriesID)
	if err != nil {
		return Series{}, err
	}
	if expected.IsZero() || !current.UpdatedAt.Equal(expected) {
		return Series{}, ErrConflict
	}
	if s.objects == nil {
		return Series{}, ErrCoverStorageUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(reader, s.maxBytes+1))
	if err != nil || len(data) == 0 || int64(len(data)) > s.maxBytes {
		return Series{}, ErrInvalidCoverImage
	}
	mime, ext := coverImageType(data)
	if mime == "" {
		return Series{}, ErrInvalidCoverImage
	}
	name, err := randomCoverName(ext)
	if err != nil {
		return Series{}, fmt.Errorf("create classroom series cover name: %w", err)
	}
	result, err := s.objects.Upload(ctx, storage.UploadInput{
		ContentType: mime,
		Dir:         fmt.Sprintf("classroom/covers/series/%d", seriesID),
		Filename:    name,
		Reader:      bytes.NewReader(data),
		Size:        int64(len(data)),
	})
	if err != nil {
		return Series{}, ErrCoverStorageUnavailable
	}
	newKey := strings.TrimSpace(result.ObjectKey)
	if newKey == "" {
		newKey = strings.TrimSpace(result.Key)
	}
	if newKey == "" {
		return Series{}, ErrCoverStorageUnavailable
	}
	updated, err := s.store.SetSeriesManualCover(ctx, seriesID, newKey, expected, updatedBy)
	if err != nil {
		s.cleanupObject(ctx, seriesID, "upload_rollback", newKey)
		return Series{}, err
	}
	if oldKey := strings.TrimSpace(current.ManualCoverObjectKey); oldKey != "" && oldKey != newKey {
		s.cleanupObject(ctx, seriesID, "replace_old", oldKey)
	}
	return updated, nil
}

func (s *SeriesCoverService) Delete(ctx context.Context, seriesID int64, expected time.Time, updatedBy *int64) (Series, error) {
	if s == nil || s.store == nil {
		return Series{}, errors.New("classroom series cover service unavailable")
	}
	current, err := s.store.GetSeries(ctx, seriesID)
	if err != nil {
		return Series{}, err
	}
	if expected.IsZero() || !current.UpdatedAt.Equal(expected) {
		return Series{}, ErrConflict
	}
	updated, err := s.store.SetSeriesManualCover(ctx, seriesID, "", expected, updatedBy)
	if err != nil {
		return Series{}, err
	}
	if oldKey := strings.TrimSpace(current.ManualCoverObjectKey); oldKey != "" {
		s.cleanupObject(ctx, seriesID, "delete_manual", oldKey)
	}
	return updated, nil
}

func (s *SeriesCoverService) UpdateSettings(ctx context.Context, seriesID int64, ratio CoverAspectRatio, expected time.Time, updatedBy *int64) (Series, error) {
	if s == nil || s.store == nil {
		return Series{}, errors.New("classroom series cover service unavailable")
	}
	current, err := s.store.GetSeries(ctx, seriesID)
	if err != nil {
		return Series{}, err
	}
	if expected.IsZero() || !current.UpdatedAt.Equal(expected) {
		return Series{}, ErrConflict
	}
	normalized, err := NormalizeCoverAspectRatio(ratio)
	if err != nil {
		return Series{}, err
	}
	return s.store.SetSeriesCoverSettings(ctx, seriesID, normalized, expected, updatedBy)
}

func (s *SeriesCoverService) cleanupObject(ctx context.Context, seriesID int64, stage, key string) {
	if s.objects == nil {
		log.Printf("classroom series cover cleanup skipped series_id=%d stage=%s: deleter unavailable", seriesID, stage)
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), coverCleanupTimeout)
	defer cancel()
	if err := s.objects.DeleteObject(cleanupCtx, key); err != nil && !storage.IsAlreadyGone(err) {
		log.Printf("classroom series cover cleanup failed series_id=%d stage=%s: %v", seriesID, stage, err)
	}
}
