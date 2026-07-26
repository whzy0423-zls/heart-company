package classroom

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(database *sql.DB) *Store { return &Store{db: database} }

type SeriesFilter struct {
	Status      SeriesStatus
	AccessLevel AccessLevel
	Limit       int
	Offset      int
}

type ContentFilter struct {
	SeriesID       *int64
	Status         ContentStatus
	ContentType    ContentType
	StandaloneOnly bool
	Limit          int
	Offset         int
}

func (s *Store) CreateSeries(ctx context.Context, item Series) (Series, error) {
	if err := item.Validate(); err != nil {
		return Series{}, err
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO classroom_series
		(title,summary,cover_url,cover_asset_id,teacher_key,teacher_name_snapshot,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13)
		RETURNING id,created_at,updated_at`, item.Title, item.Summary, item.CoverURL, item.CoverAssetID, item.TeacherKey, item.TeacherNameSnapshot, item.SortOrder, item.Status, item.PlaybackBlocked, item.AccessLevel, item.PriceCents, item.PublishedAt, item.CreatedBy).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Store) GetSeries(ctx context.Context, id int64) (Series, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,title,summary,cover_url,cover_asset_id,teacher_key,teacher_name_snapshot,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by,created_at,updated_at FROM classroom_series WHERE id=$1`, id)
	return scanSeries(row)
}

func (s *Store) UpdateSeries(ctx context.Context, item Series, expectedUpdatedAt time.Time) (Series, error) {
	if expectedUpdatedAt.IsZero() {
		return Series{}, ErrConflict
	}
	if err := item.Validate(); err != nil {
		return Series{}, err
	}
	current, err := s.GetSeries(ctx, item.ID)
	if err != nil {
		return Series{}, err
	}
	if !CanTransitionSeries(current.Status, item.Status) {
		return Series{}, fmt.Errorf("invalid series transition %s -> %s", current.Status, item.Status)
	}
	err = s.db.QueryRowContext(ctx, `UPDATE classroom_series SET title=$1,summary=$2,cover_url=$3,cover_asset_id=$4,teacher_key=$5,teacher_name_snapshot=$6,sort_order=$7,status=$8,playback_blocked=$9,access_level=$10,price_cents=$11,published_at=$12,updated_by=$13,updated_at=now() WHERE id=$14 AND updated_at=$15 RETURNING created_at,updated_at`, item.Title, item.Summary, item.CoverURL, item.CoverAssetID, item.TeacherKey, item.TeacherNameSnapshot, item.SortOrder, item.Status, item.PlaybackBlocked, item.AccessLevel, item.PriceCents, item.PublishedAt, item.UpdatedBy, item.ID, expectedUpdatedAt).Scan(&item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Series{}, ErrConflict
	}
	return item, err
}

func (s *Store) ListSeries(ctx context.Context, filter SeriesFilter) ([]Series, error) {
	clauses, args := []string{"1=1"}, []any{}
	if filter.Status != "" {
		args = append(args, filter.Status)
		clauses = append(clauses, fmt.Sprintf("status=$%d", len(args)))
	}
	if filter.AccessLevel != "" {
		args = append(args, filter.AccessLevel)
		clauses = append(clauses, fmt.Sprintf("access_level=$%d", len(args)))
	}
	limit := boundedLimit(filter.Limit)
	args = append(args, limit, max(filter.Offset, 0))
	query := `SELECT id,title,summary,cover_url,cover_asset_id,teacher_key,teacher_name_snapshot,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by,created_at,updated_at FROM classroom_series WHERE ` + strings.Join(clauses, " AND ") + fmt.Sprintf(" ORDER BY sort_order,id LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Series, 0)
	for rows.Next() {
		item, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateContent(ctx context.Context, item Content) (Content, error) {
	if item.Status == ContentPublished {
		return Content{}, errors.New("content must be ready before publication")
	}
	if err := item.Validate(); err != nil {
		return Content{}, err
	}
	tags, err := json.Marshal(item.Tags)
	if err != nil {
		return Content{}, err
	}
	err = s.db.QueryRowContext(ctx, `INSERT INTO classroom_contents
		(series_id,show_as_standalone,title,description,content_type,media_asset_id,cover_url,duration_seconds,teacher_key,teacher_name_snapshot,recorded_at,badge,tags,episode_no,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14,$15,$16,$17,$18,$19,$20,$21,$21)
		RETURNING id,created_at,updated_at`, item.SeriesID, item.ShowAsStandalone, item.Title, item.Description, item.ContentType, item.MediaAssetID, item.CoverURL, item.DurationSeconds, item.TeacherKey, item.TeacherNameSnapshot, item.RecordedAt, item.Badge, string(tags), item.EpisodeNo, item.SortOrder, item.Status, item.PlaybackBlocked, item.AccessLevel, item.PriceCents, item.PublishedAt, item.CreatedBy).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Store) GetContent(ctx context.Context, id int64) (Content, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,series_id,show_as_standalone,title,description,content_type,media_asset_id,cover_url,duration_seconds,teacher_key,teacher_name_snapshot,recorded_at,badge,tags,episode_no,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by,created_at,updated_at FROM classroom_contents WHERE id=$1`, id)
	return scanContent(row)
}

func (s *Store) UpdateContent(ctx context.Context, item Content, expectedUpdatedAt time.Time) (Content, error) {
	if expectedUpdatedAt.IsZero() {
		return Content{}, ErrConflict
	}
	if err := item.Validate(); err != nil {
		return Content{}, err
	}
	current, err := s.GetContent(ctx, item.ID)
	if err != nil {
		return Content{}, err
	}
	if !CanTransitionContent(current.Status, item.Status) {
		return Content{}, fmt.Errorf("invalid content transition %s -> %s", current.Status, item.Status)
	}
	if item.Status == ContentPublished {
		if item.MediaAssetID == nil {
			return Content{}, errors.New("published content requires media")
		}
		media, err := s.GetMediaAsset(ctx, *item.MediaAssetID)
		if err != nil {
			return Content{}, err
		}
		var parent *Series
		if item.SeriesID != nil {
			value, err := s.GetSeries(ctx, *item.SeriesID)
			if err != nil {
				return Content{}, err
			}
			parent = &value
		}
		ready := item
		ready.Status = ContentReady
		if err := ValidateContentPublish(ready, media, parent); err != nil {
			return Content{}, err
		}
	}
	tags, err := json.Marshal(item.Tags)
	if err != nil {
		return Content{}, err
	}
	err = s.db.QueryRowContext(ctx, `UPDATE classroom_contents SET series_id=$1,show_as_standalone=$2,title=$3,description=$4,content_type=$5,media_asset_id=$6,cover_url=$7,duration_seconds=$8,teacher_key=$9,teacher_name_snapshot=$10,recorded_at=$11,badge=$12,tags=$13::jsonb,episode_no=$14,sort_order=$15,status=$16,playback_blocked=$17,access_level=$18,price_cents=$19,published_at=$20,updated_by=$21,updated_at=now() WHERE id=$22 AND updated_at=$23 RETURNING created_at,updated_at`, item.SeriesID, item.ShowAsStandalone, item.Title, item.Description, item.ContentType, item.MediaAssetID, item.CoverURL, item.DurationSeconds, item.TeacherKey, item.TeacherNameSnapshot, item.RecordedAt, item.Badge, string(tags), item.EpisodeNo, item.SortOrder, item.Status, item.PlaybackBlocked, item.AccessLevel, item.PriceCents, item.PublishedAt, item.UpdatedBy, item.ID, expectedUpdatedAt).Scan(&item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Content{}, ErrConflict
	}
	return item, err
}

func (s *Store) ListContents(ctx context.Context, filter ContentFilter) ([]Content, error) {
	clauses, args := []string{"1=1"}, []any{}
	if filter.SeriesID != nil {
		args = append(args, *filter.SeriesID)
		clauses = append(clauses, fmt.Sprintf("series_id=$%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		clauses = append(clauses, fmt.Sprintf("status=$%d", len(args)))
	}
	if filter.ContentType != "" {
		args = append(args, filter.ContentType)
		clauses = append(clauses, fmt.Sprintf("content_type=$%d", len(args)))
	}
	if filter.StandaloneOnly {
		clauses = append(clauses, "(series_id IS NULL OR show_as_standalone=true)")
	}
	args = append(args, boundedLimit(filter.Limit), max(filter.Offset, 0))
	query := `SELECT id,series_id,show_as_standalone,title,description,content_type,media_asset_id,cover_url,duration_seconds,teacher_key,teacher_name_snapshot,recorded_at,badge,tags,episode_no,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by,created_at,updated_at FROM classroom_contents WHERE ` + strings.Join(clauses, " AND ") + fmt.Sprintf(" ORDER BY sort_order,id LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Content, 0)
	for rows.Next() {
		item, err := scanContent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateMediaAsset(ctx context.Context, item MediaAsset) (MediaAsset, error) {
	if err := item.Validate(); err != nil {
		return MediaAsset{}, err
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO classroom_media_assets (bucket,object_key,etag,checksum,content_type,size_bytes,duration_seconds,width,height,cover_object_key,storage_status,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id,created_at,updated_at`, item.Bucket, item.ObjectKey, item.ETag, item.Checksum, item.ContentType, item.SizeBytes, item.DurationSeconds, item.Width, item.Height, item.CoverObjectKey, item.StorageStatus, item.CreatedBy).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Store) GetMediaAsset(ctx context.Context, id int64) (MediaAsset, error) {
	var item MediaAsset
	err := s.db.QueryRowContext(ctx, `SELECT id,bucket,object_key,etag,checksum,content_type,size_bytes,duration_seconds,width,height,cover_object_key,storage_status,created_by,created_at,updated_at FROM classroom_media_assets WHERE id=$1`, id).Scan(&item.ID, &item.Bucket, &item.ObjectKey, &item.ETag, &item.Checksum, &item.ContentType, &item.SizeBytes, &item.DurationSeconds, &item.Width, &item.Height, &item.CoverObjectKey, &item.StorageStatus, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return item, err
}

func (s *Store) CreateUploadTask(ctx context.Context, item UploadTask) (UploadTask, error) {
	if err := item.Validate(); err != nil {
		return UploadTask{}, err
	}
	content, err := s.GetContent(ctx, item.ContentID)
	if err != nil {
		return UploadTask{}, err
	}
	if err := ValidateUploadDraftBinding(item, content); err != nil {
		return UploadTask{}, err
	}
	err = s.db.QueryRowContext(ctx, `INSERT INTO classroom_upload_tasks (content_id,creator_id,oss_upload_id,object_key,expected_size,checksum,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id,created_at,updated_at`, item.ContentID, item.CreatorID, item.OSSUploadID, item.ObjectKey, item.ExpectedSize, item.Checksum, item.PartSize, item.MaxParts, item.Status, item.ExpiresAt, item.AttemptCount, defaultCleanup(item.CleanupStatus), item.MediaAssetID, item.FailureReason).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Store) GetUploadTask(ctx context.Context, id int64) (UploadTask, error) {
	var item UploadTask
	err := s.db.QueryRowContext(ctx, `SELECT id,content_id,creator_id,oss_upload_id,object_key,expected_size,checksum,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at FROM classroom_upload_tasks WHERE id=$1`, id).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return item, err
}

func (s *Store) CreateEntitlement(ctx context.Context, item Entitlement) (Entitlement, error) {
	if err := item.Validate(); err != nil {
		return Entitlement{}, err
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO classroom_entitlements (wx_user_id,series_id,content_id,order_id,source,expires_at,revoked_at,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,created_at,updated_at`, item.WXUserID, item.SeriesID, item.ContentID, item.OrderID, item.Source, item.ExpiresAt, item.RevokedAt, item.CreatedBy).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Store) UpsertProgress(ctx context.Context, item Progress) (Progress, error) {
	if item.WXUserID <= 0 || item.ContentID <= 0 || item.PositionSeconds < 0 {
		return Progress{}, errors.New("invalid classroom progress")
	}
	if item.LastPlayedAt.IsZero() {
		item.LastPlayedAt = time.Now()
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO classroom_progress (wx_user_id,content_id,position_seconds,completed,last_played_at) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (wx_user_id,content_id) DO UPDATE SET position_seconds=EXCLUDED.position_seconds,completed=EXCLUDED.completed,last_played_at=EXCLUDED.last_played_at,updated_at=now() RETURNING created_at,updated_at`, item.WXUserID, item.ContentID, item.PositionSeconds, item.Completed, item.LastPlayedAt).Scan(&item.CreatedAt, &item.UpdatedAt)
	return item, err
}

type scanner interface{ Scan(...any) error }

func scanSeries(row scanner) (Series, error) {
	var item Series
	err := row.Scan(&item.ID, &item.Title, &item.Summary, &item.CoverURL, &item.CoverAssetID, &item.TeacherKey, &item.TeacherNameSnapshot, &item.SortOrder, &item.Status, &item.PlaybackBlocked, &item.AccessLevel, &item.PriceCents, &item.PublishedAt, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return item, err
}

func scanContent(row scanner) (Content, error) {
	var item Content
	var tags []byte
	err := row.Scan(&item.ID, &item.SeriesID, &item.ShowAsStandalone, &item.Title, &item.Description, &item.ContentType, &item.MediaAssetID, &item.CoverURL, &item.DurationSeconds, &item.TeacherKey, &item.TeacherNameSnapshot, &item.RecordedAt, &item.Badge, &tags, &item.EpisodeNo, &item.SortOrder, &item.Status, &item.PlaybackBlocked, &item.AccessLevel, &item.PriceCents, &item.PublishedAt, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Content{}, ErrNotFound
	}
	if err != nil {
		return Content{}, err
	}
	if len(tags) > 0 {
		if err := json.Unmarshal(tags, &item.Tags); err != nil {
			return Content{}, err
		}
	}
	return item, nil
}

func boundedLimit(value int) int {
	if value <= 0 {
		return 50
	}
	if value > 200 {
		return 200
	}
	return value
}
func defaultCleanup(value string) string {
	if strings.TrimSpace(value) == "" {
		return "pending"
	}
	return value
}
