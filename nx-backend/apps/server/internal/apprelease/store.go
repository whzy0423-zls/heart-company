package apprelease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	defaultListPage     = 1
	defaultListPageSize = 20
	maxListPageSize     = 100
)

type Store struct {
	db *sql.DB
}

type ListResult struct {
	Current       *Release  `json:"current"`
	Items         []Release `json:"items"`
	Page          int       `json:"page"`
	PageSize      int       `json:"pageSize"`
	Total         int       `json:"total"`
	TotalFileSize int64     `json:"totalFileSize"`
}

type releaseScanner interface {
	Scan(dest ...any) error
}

const releaseColumns = `
	id, platform, version_name, version_code, release_notes, file_name, file_path,
	file_size, sha256, status, created_at, published_at`

func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

func (s *Store) CreateDraft(ctx context.Context, input Release) (Release, error) {
	if input.Platform != "android" {
		return Release{}, ErrUnsupportedPlatform
	}
	if input.VersionCode <= 0 || strings.TrimSpace(input.VersionName) == "" {
		return Release{}, ErrInvalidVersion
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO app_releases
		(platform, version_name, version_code, release_notes, file_name, file_path, file_size, sha256, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'draft')
		RETURNING `+releaseColumns,
		input.Platform,
		strings.TrimSpace(input.VersionName),
		input.VersionCode,
		strings.TrimSpace(input.ReleaseNotes),
		strings.TrimSpace(input.FileName),
		strings.TrimSpace(input.FilePath),
		input.FileSize,
		strings.ToLower(strings.TrimSpace(input.SHA256)),
	)
	release, err := scanRelease(row)
	if err != nil {
		return Release{}, translateStoreError(err)
	}
	return release, nil
}

func (s *Store) List(ctx context.Context, page, pageSize int) (ListResult, error) {
	page, pageSize = normalizeListPage(page, pageSize)
	result := ListResult{
		Items:    []Release{},
		Page:     page,
		PageSize: pageSize,
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*), COALESCE(sum(file_size), 0)
		FROM app_releases`,
	).Scan(&result.Total, &result.TotalFileSize); err != nil {
		return ListResult{}, fmt.Errorf("list app release totals: %w", err)
	}

	current, err := s.LatestPublished(ctx, "android")
	if err == nil {
		result.Current = &current
	} else if !errors.Is(err, ErrNotFound) {
		return ListResult{}, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+releaseColumns+`
		FROM app_releases
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return ListResult{}, fmt.Errorf("list app releases: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		release, err := scanRelease(rows)
		if err != nil {
			return ListResult{}, fmt.Errorf("scan app release list: %w", err)
		}
		result.Items = append(result.Items, release)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate app release list: %w", err)
	}
	return result, nil
}

func (s *Store) FindByID(ctx context.Context, id int64) (Release, error) {
	release, err := scanRelease(s.db.QueryRowContext(ctx, `
		SELECT `+releaseColumns+`
		FROM app_releases
		WHERE id=$1`, id))
	if err != nil {
		return Release{}, translateStoreError(err)
	}
	return release, nil
}

func (s *Store) LatestPublished(ctx context.Context, platform string) (Release, error) {
	if platform != "android" {
		return Release{}, ErrUnsupportedPlatform
	}
	release, err := scanRelease(s.db.QueryRowContext(ctx, `
		SELECT `+releaseColumns+`
		FROM app_releases
		WHERE platform=$1 AND status='published'
		ORDER BY published_at DESC NULLS LAST, id DESC
		LIMIT 1`, platform))
	if err != nil {
		return Release{}, translateStoreError(err)
	}
	return release, nil
}

func (s *Store) Archive(ctx context.Context, id int64, platform string) (Release, error) {
	if platform != "android" {
		return Release{}, ErrUnsupportedPlatform
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Release{}, fmt.Errorf("begin app release archive: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	target, err := scanRelease(tx.QueryRowContext(ctx, `
		SELECT `+releaseColumns+`
		FROM app_releases
		WHERE id=$1 AND platform=$2
		FOR UPDATE`, id, platform))
	if err != nil {
		return Release{}, translateStoreError(err)
	}
	if target.Status != StatusPublished {
		return Release{}, ErrConflict
	}

	archived, err := scanRelease(tx.QueryRowContext(ctx, `
		UPDATE app_releases
		SET status='archived'
		WHERE id=$1 AND platform=$2 AND status='published'
		RETURNING `+releaseColumns, id, platform))
	if err != nil {
		return Release{}, translateStoreError(err)
	}
	if err := tx.Commit(); err != nil {
		return Release{}, translateStoreError(fmt.Errorf("commit app release archive: %w", err))
	}
	return archived, nil
}

func (s *Store) Publish(ctx context.Context, id int64, platform string) (Release, error) {
	if platform != "android" {
		return Release{}, ErrUnsupportedPlatform
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Release{}, fmt.Errorf("begin app release publish: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, platform); err != nil {
		return Release{}, fmt.Errorf("lock app release platform: %w", err)
	}
	target, err := scanRelease(tx.QueryRowContext(ctx, `
		SELECT `+releaseColumns+`
		FROM app_releases
		WHERE id=$1 AND platform=$2
		FOR UPDATE`, id, platform))
	if err != nil {
		return Release{}, translateStoreError(err)
	}
	if target.Status == StatusPublished {
		if err := tx.Commit(); err != nil {
			return Release{}, translateStoreError(fmt.Errorf("commit idempotent app release publish: %w", err))
		}
		return target, nil
	}
	if target.Status != StatusDraft && target.Status != StatusArchived {
		return Release{}, ErrConflict
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE app_releases
		SET status='archived'
		WHERE platform=$1 AND status='published' AND id<>$2`, platform, id); err != nil {
		return Release{}, translateStoreError(err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE app_releases
		SET status='published', published_at=now()
		WHERE id=$1 AND platform=$2 AND status IN ('draft','archived')`, id, platform)
	if err != nil {
		return Release{}, translateStoreError(err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return Release{}, fmt.Errorf("read app release publish result: %w", err)
	}
	if updated != 1 {
		return Release{}, ErrConflict
	}
	published, err := scanRelease(tx.QueryRowContext(ctx, `
		SELECT `+releaseColumns+`
		FROM app_releases
		WHERE id=$1`, id))
	if err != nil {
		return Release{}, translateStoreError(err)
	}
	if err := tx.Commit(); err != nil {
		return Release{}, translateStoreError(fmt.Errorf("commit app release publish: %w", err))
	}
	return published, nil
}

func (s *Store) ReferencedKeys(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT file_path FROM app_releases`)
	if err != nil {
		return nil, fmt.Errorf("list app release file keys: %w", err)
	}
	defer rows.Close()
	keys := make(map[string]struct{})
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan app release file key: %w", err)
		}
		if key = strings.TrimSpace(key); key != "" {
			keys[key] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate app release file keys: %w", err)
	}
	return keys, nil
}

func scanRelease(scanner releaseScanner) (Release, error) {
	var (
		release     Release
		status      string
		publishedAt sql.NullTime
	)
	err := scanner.Scan(
		&release.ID,
		&release.Platform,
		&release.VersionName,
		&release.VersionCode,
		&release.ReleaseNotes,
		&release.FileName,
		&release.FilePath,
		&release.FileSize,
		&release.SHA256,
		&status,
		&release.CreatedAt,
		&publishedAt,
	)
	if err != nil {
		return Release{}, err
	}
	release.Status = Status(status)
	if publishedAt.Valid {
		value := publishedAt.Time
		release.PublishedAt = &value
	}
	return release, nil
}

func normalizeListPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = defaultListPage
	}
	if pageSize <= 0 || pageSize > maxListPageSize {
		pageSize = defaultListPageSize
	}
	return page, pageSize
}

func translateStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %s", ErrConflict, pgErr.ConstraintName)
	}
	return err
}
