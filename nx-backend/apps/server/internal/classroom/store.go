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
	if item.Status != SeriesDraft {
		return Series{}, errors.New("new series must start as draft")
	}
	if err := item.Validate(); err != nil {
		return Series{}, err
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO classroom_series
		(title,summary,cover_url,cover_asset_id,teacher_key,teacher_name_snapshot,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13)
		RETURNING id,created_at,updated_at`, item.Title, item.Summary, item.CoverURL, item.CoverAssetID, item.TeacherKey, item.TeacherNameSnapshot, item.SortOrder, item.Status, item.PlaybackBlocked, item.AccessLevel, item.PriceCents, item.PublishedAt, item.CreatedBy).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Series{}, fmt.Errorf("create classroom series: %w", err)
	}
	return item, nil
}

func (s *Store) GetSeries(ctx context.Context, id int64) (Series, error) {
	item, err := getSeries(ctx, s.db, id, false)
	if err != nil {
		return Series{}, fmt.Errorf("get classroom series: %w", err)
	}
	return item, nil
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
	if err != nil {
		return Series{}, fmt.Errorf("update classroom series: %w", err)
	}
	return item, nil
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
		return nil, fmt.Errorf("list classroom series: %w", err)
	}
	defer rows.Close()
	items := make([]Series, 0)
	for rows.Next() {
		item, err := scanSeries(rows)
		if err != nil {
			return nil, fmt.Errorf("scan classroom series: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate classroom series: %w", err)
	}
	return items, nil
}

func (s *Store) CreateContent(ctx context.Context, item Content) (Content, error) {
	if item.Status != ContentDraft {
		return Content{}, errors.New("new content must start as draft")
	}
	var err error
	item.CoverAspectRatio, err = NormalizeCoverAspectRatio(item.CoverAspectRatio)
	if err != nil {
		return Content{}, err
	}
	if err := item.Validate(); err != nil {
		return Content{}, err
	}
	if item.Tags == nil {
		item.Tags = []string{}
	}
	tags, err := json.Marshal(item.Tags)
	if err != nil {
		return Content{}, err
	}
	err = s.db.QueryRowContext(ctx, `INSERT INTO classroom_contents
		(series_id,show_as_standalone,title,description,content_type,media_asset_id,cover_url,manual_cover_object_key,cover_aspect_ratio,duration_seconds,teacher_key,teacher_name_snapshot,recorded_at,badge,tags,episode_no,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,$16,$17,$18,$19,$20,$21,$22,$23,$23)
		RETURNING id,created_at,updated_at`, item.SeriesID, item.ShowAsStandalone, item.Title, item.Description, item.ContentType, item.MediaAssetID, item.CoverURL, item.ManualCoverObjectKey, item.CoverAspectRatio, item.DurationSeconds, item.TeacherKey, item.TeacherNameSnapshot, item.RecordedAt, item.Badge, string(tags), item.EpisodeNo, item.SortOrder, item.Status, item.PlaybackBlocked, item.AccessLevel, item.PriceCents, item.PublishedAt, item.CreatedBy).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Content{}, fmt.Errorf("create classroom content: %w", err)
	}
	return item, nil
}

func (s *Store) GetContent(ctx context.Context, id int64) (Content, error) {
	item, err := getContent(ctx, s.db, id, false)
	if err != nil {
		return Content{}, fmt.Errorf("get classroom content: %w", err)
	}
	return item, nil
}

func (s *Store) SetContentManualCover(ctx context.Context, id int64, objectKey string, expectedUpdatedAt time.Time, updatedBy *int64) (Content, error) {
	if expectedUpdatedAt.IsZero() {
		return Content{}, ErrConflict
	}
	row := s.db.QueryRowContext(ctx, `UPDATE classroom_contents SET manual_cover_object_key=$1,updated_by=$2,updated_at=now() WHERE id=$3 AND updated_at=$4 RETURNING id,series_id,show_as_standalone,title,description,content_type,media_asset_id,cover_url,manual_cover_object_key,cover_aspect_ratio,duration_seconds,teacher_key,teacher_name_snapshot,recorded_at,badge,tags,episode_no,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by,created_at,updated_at`, strings.TrimSpace(objectKey), updatedBy, id, expectedUpdatedAt)
	item, err := scanContent(row)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound) {
		return Content{}, ErrConflict
	}
	if err != nil {
		return Content{}, fmt.Errorf("set classroom content manual cover: %w", err)
	}
	return item, nil
}

func (s *Store) SetContentCoverSettings(ctx context.Context, id int64, ratio CoverAspectRatio, expectedUpdatedAt time.Time, updatedBy *int64, mediaID *int64, expectedGeneratedKey, generatedKey string) (Content, error) {
	if expectedUpdatedAt.IsZero() {
		return Content{}, ErrConflict
	}
	normalized, err := NormalizeCoverAspectRatio(ratio)
	if err != nil {
		return Content{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Content{}, fmt.Errorf("begin classroom cover settings update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := getContent(ctx, tx, id, true)
	if err != nil {
		return Content{}, err
	}
	if !current.UpdatedAt.Equal(expectedUpdatedAt) {
		return Content{}, ErrConflict
	}
	if mediaID != nil {
		if current.MediaAssetID == nil || *current.MediaAssetID != *mediaID {
			return Content{}, ErrConflict
		}
		media, err := getMediaAsset(ctx, tx, *mediaID, true)
		if err != nil {
			return Content{}, err
		}
		if media.CoverObjectKey != expectedGeneratedKey {
			return Content{}, ErrConflict
		}
		var updatedMediaID int64
		err = tx.QueryRowContext(ctx, `UPDATE classroom_media_assets SET cover_object_key=$1,updated_at=now() WHERE id=$2 AND cover_object_key=$3 RETURNING id`, strings.TrimSpace(generatedKey), *mediaID, expectedGeneratedKey).Scan(&updatedMediaID)
		if errors.Is(err, sql.ErrNoRows) {
			return Content{}, ErrConflict
		}
		if err != nil {
			return Content{}, fmt.Errorf("set classroom generated cover: %w", err)
		}
	}
	row := tx.QueryRowContext(ctx, `UPDATE classroom_contents SET cover_aspect_ratio=$1,updated_by=$2,updated_at=now() WHERE id=$3 AND updated_at=$4 RETURNING id,series_id,show_as_standalone,title,description,content_type,media_asset_id,cover_url,manual_cover_object_key,cover_aspect_ratio,duration_seconds,teacher_key,teacher_name_snapshot,recorded_at,badge,tags,episode_no,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by,created_at,updated_at`, normalized, updatedBy, id, expectedUpdatedAt)
	updated, err := scanContent(row)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound) {
		return Content{}, ErrConflict
	}
	if err != nil {
		return Content{}, fmt.Errorf("set classroom cover settings: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return Content{}, fmt.Errorf("commit classroom cover settings: %w", err)
	}
	return updated, nil
}

func (s *Store) UpdateContent(ctx context.Context, item Content, expectedUpdatedAt time.Time) (Content, error) {
	if expectedUpdatedAt.IsZero() {
		return Content{}, ErrConflict
	}
	var err error
	item.CoverAspectRatio, err = NormalizeCoverAspectRatio(item.CoverAspectRatio)
	if err != nil {
		return Content{}, err
	}
	if err := item.Validate(); err != nil {
		return Content{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Content{}, fmt.Errorf("begin classroom content update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	current, err := getContent(ctx, tx, item.ID, true)
	if err != nil {
		return Content{}, fmt.Errorf("lock classroom content: %w", err)
	}
	if !CanTransitionContent(current.Status, item.Status) {
		return Content{}, fmt.Errorf("invalid content transition %s -> %s", current.Status, item.Status)
	}
	if item.Status == ContentPublished {
		if item.MediaAssetID == nil {
			return Content{}, errors.New("published content requires media")
		}
		media, err := getMediaAsset(ctx, tx, *item.MediaAssetID, true)
		if err != nil {
			return Content{}, fmt.Errorf("lock classroom media asset: %w", err)
		}
		var parent *Series
		if item.SeriesID != nil {
			value, err := getSeries(ctx, tx, *item.SeriesID, true)
			if err != nil {
				return Content{}, fmt.Errorf("lock classroom parent series: %w", err)
			}
			parent = &value
		}
		ready := item
		ready.Status = ContentReady
		if err := ValidateContentPublish(ready, media, parent); err != nil {
			return Content{}, err
		}
	}
	if item.Tags == nil {
		item.Tags = []string{}
	}
	tags, err := json.Marshal(item.Tags)
	if err != nil {
		return Content{}, err
	}
	err = tx.QueryRowContext(ctx, `UPDATE classroom_contents SET series_id=$1,show_as_standalone=$2,title=$3,description=$4,content_type=$5,media_asset_id=$6,cover_url=$7,manual_cover_object_key=$8,cover_aspect_ratio=$9,duration_seconds=$10,teacher_key=$11,teacher_name_snapshot=$12,recorded_at=$13,badge=$14,tags=$15::jsonb,episode_no=$16,sort_order=$17,status=$18,playback_blocked=$19,access_level=$20,price_cents=$21,published_at=$22,updated_by=$23,updated_at=now() WHERE id=$24 AND updated_at=$25 RETURNING created_at,updated_at`, item.SeriesID, item.ShowAsStandalone, item.Title, item.Description, item.ContentType, item.MediaAssetID, item.CoverURL, item.ManualCoverObjectKey, item.CoverAspectRatio, item.DurationSeconds, item.TeacherKey, item.TeacherNameSnapshot, item.RecordedAt, item.Badge, string(tags), item.EpisodeNo, item.SortOrder, item.Status, item.PlaybackBlocked, item.AccessLevel, item.PriceCents, item.PublishedAt, item.UpdatedBy, item.ID, expectedUpdatedAt).Scan(&item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Content{}, ErrConflict
	}
	if err != nil {
		return Content{}, fmt.Errorf("update classroom content: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Content{}, fmt.Errorf("commit classroom content update: %w", err)
	}
	return item, nil
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
	query := `SELECT id,series_id,show_as_standalone,title,description,content_type,media_asset_id,cover_url,manual_cover_object_key,cover_aspect_ratio,duration_seconds,teacher_key,teacher_name_snapshot,recorded_at,badge,tags,episode_no,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by,created_at,updated_at FROM classroom_contents WHERE ` + strings.Join(clauses, " AND ") + fmt.Sprintf(" ORDER BY sort_order,id LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list classroom contents: %w", err)
	}
	defer rows.Close()
	items := make([]Content, 0)
	for rows.Next() {
		item, err := scanContent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan classroom content: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate classroom contents: %w", err)
	}
	return items, nil
}

func (s *Store) CreateMediaAsset(ctx context.Context, item MediaAsset) (MediaAsset, error) {
	if err := item.Validate(); err != nil {
		return MediaAsset{}, err
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO classroom_media_assets (bucket,object_key,etag,checksum,content_type,size_bytes,duration_seconds,width,height,cover_object_key,storage_status,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id,created_at,updated_at`, item.Bucket, item.ObjectKey, item.ETag, item.Checksum, item.ContentType, item.SizeBytes, item.DurationSeconds, item.Width, item.Height, item.CoverObjectKey, item.StorageStatus, item.CreatedBy).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return MediaAsset{}, fmt.Errorf("create classroom media asset: %w", err)
	}
	return item, nil
}

func (s *Store) GetMediaAsset(ctx context.Context, id int64) (MediaAsset, error) {
	item, err := getMediaAsset(ctx, s.db, id, false)
	if err != nil {
		return MediaAsset{}, fmt.Errorf("get classroom media asset: %w", err)
	}
	return item, nil
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
	err = s.db.QueryRowContext(ctx, `INSERT INTO classroom_upload_tasks (content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) RETURNING id,created_at,updated_at`, item.ContentID, item.CreatorID, item.OriginalFilename, item.OSSUploadID, item.ObjectKey, item.ExpectedSize, item.Checksum, item.CompletedParts, item.CompletedBytes, item.PartSize, item.MaxParts, item.Status, item.ExpiresAt, item.AttemptCount, defaultCleanup(item.CleanupStatus), item.MediaAssetID, item.FailureReason).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return UploadTask{}, fmt.Errorf("create classroom upload task: %w", err)
	}
	return item, nil
}

func (s *Store) GetUploadTask(ctx context.Context, id int64) (UploadTask, error) {
	var item UploadTask
	err := s.db.QueryRowContext(ctx, `SELECT id,content_id,creator_id,original_filename,oss_upload_id,object_key,expected_size,checksum,completed_parts,completed_bytes,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at FROM classroom_upload_tasks WHERE id=$1`, id).Scan(&item.ID, &item.ContentID, &item.CreatorID, &item.OriginalFilename, &item.OSSUploadID, &item.ObjectKey, &item.ExpectedSize, &item.Checksum, &item.CompletedParts, &item.CompletedBytes, &item.PartSize, &item.MaxParts, &item.Status, &item.ExpiresAt, &item.AttemptCount, &item.CleanupStatus, &item.MediaAssetID, &item.FailureReason, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		return UploadTask{}, fmt.Errorf("get classroom upload task: %w", err)
	}
	return item, nil
}

func (s *Store) CreateEntitlement(ctx context.Context, item Entitlement) (Entitlement, error) {
	if err := item.Validate(); err != nil {
		return Entitlement{}, err
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO classroom_entitlements (wx_user_id,series_id,content_id,order_id,source,expires_at,revoked_at,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,created_at,updated_at`, item.WXUserID, item.SeriesID, item.ContentID, item.OrderID, item.Source, item.ExpiresAt, item.RevokedAt, item.CreatedBy).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Entitlement{}, fmt.Errorf("create classroom entitlement: %w", err)
	}
	return item, nil
}

func (s *Store) UpsertProgress(ctx context.Context, item Progress) (Progress, error) {
	if item.WXUserID <= 0 || item.ContentID <= 0 || item.PositionSeconds < 0 {
		return Progress{}, errors.New("invalid classroom progress")
	}
	if item.LastPlayedAt.IsZero() {
		item.LastPlayedAt = time.Now()
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO classroom_progress (wx_user_id,content_id,position_seconds,completed,last_played_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (wx_user_id,content_id) DO UPDATE SET
			position_seconds=GREATEST(classroom_progress.position_seconds,EXCLUDED.position_seconds),
			completed=classroom_progress.completed OR EXCLUDED.completed,
			last_played_at=GREATEST(classroom_progress.last_played_at,EXCLUDED.last_played_at),
			updated_at=CASE WHEN EXCLUDED.last_played_at > classroom_progress.last_played_at OR EXCLUDED.position_seconds > classroom_progress.position_seconds OR (EXCLUDED.completed AND NOT classroom_progress.completed) THEN now() ELSE classroom_progress.updated_at END
		RETURNING position_seconds,completed,last_played_at,created_at,updated_at`, item.WXUserID, item.ContentID, item.PositionSeconds, item.Completed, item.LastPlayedAt).Scan(&item.PositionSeconds, &item.Completed, &item.LastPlayedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Progress{}, fmt.Errorf("upsert classroom progress: %w", err)
	}
	return item, nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const seriesSelect = `SELECT id,title,summary,cover_url,cover_asset_id,teacher_key,teacher_name_snapshot,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by,created_at,updated_at FROM classroom_series WHERE id=$1`
const contentSelect = `SELECT id,series_id,show_as_standalone,title,description,content_type,media_asset_id,cover_url,manual_cover_object_key,cover_aspect_ratio,duration_seconds,teacher_key,teacher_name_snapshot,recorded_at,badge,tags,episode_no,sort_order,status,playback_blocked,access_level,price_cents,published_at,created_by,updated_by,created_at,updated_at FROM classroom_contents WHERE id=$1`
const mediaAssetSelect = `SELECT id,bucket,object_key,etag,checksum,content_type,size_bytes,duration_seconds,width,height,cover_object_key,storage_status,created_by,created_at,updated_at FROM classroom_media_assets WHERE id=$1`

func getSeries(ctx context.Context, q queryRower, id int64, forUpdate bool) (Series, error) {
	query := seriesSelect
	if forUpdate {
		query += " FOR UPDATE"
	}
	return scanSeries(q.QueryRowContext(ctx, query, id))
}
func getContent(ctx context.Context, q queryRower, id int64, forUpdate bool) (Content, error) {
	query := contentSelect
	if forUpdate {
		query += " FOR UPDATE"
	}
	return scanContent(q.QueryRowContext(ctx, query, id))
}
func getMediaAsset(ctx context.Context, q queryRower, id int64, forUpdate bool) (MediaAsset, error) {
	query := mediaAssetSelect
	if forUpdate {
		query += " FOR UPDATE"
	}
	var item MediaAsset
	err := q.QueryRowContext(ctx, query, id).Scan(&item.ID, &item.Bucket, &item.ObjectKey, &item.ETag, &item.Checksum, &item.ContentType, &item.SizeBytes, &item.DurationSeconds, &item.Width, &item.Height, &item.CoverObjectKey, &item.StorageStatus, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
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
	err := row.Scan(&item.ID, &item.SeriesID, &item.ShowAsStandalone, &item.Title, &item.Description, &item.ContentType, &item.MediaAssetID, &item.CoverURL, &item.ManualCoverObjectKey, &item.CoverAspectRatio, &item.DurationSeconds, &item.TeacherKey, &item.TeacherNameSnapshot, &item.RecordedAt, &item.Badge, &tags, &item.EpisodeNo, &item.SortOrder, &item.Status, &item.PlaybackBlocked, &item.AccessLevel, &item.PriceCents, &item.PublishedAt, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt)
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
	item.CoverAspectRatio, err = NormalizeCoverAspectRatio(item.CoverAspectRatio)
	if err != nil {
		return Content{}, err
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
