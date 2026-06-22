package uploadasset

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Store struct {
	db *sql.DB
}

type CreateInput struct {
	ContentType string
	Data        []byte
	Dir         string
	Name        string
	ObjectKey   string
	ObjectURL   string
	Size        int64
}

type Asset struct {
	ContentType string
	Data        []byte
	ID          int64
	Key         string
	Name        string
	ObjectKey   string
	ObjectURL   string
	Size        int64
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

func (s *Store) Create(ctx context.Context, input CreateInput) (Asset, error) {
	if s == nil || s.db == nil {
		return Asset{}, fmt.Errorf("upload asset database is not configured")
	}
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var id int64
	if err := s.db.QueryRowContext(c, `SELECT nextval(pg_get_serial_sequence('upload_assets','id'))`).Scan(&id); err != nil {
		return Asset{}, err
	}
	key := "upload-assets/" + strconv.FormatInt(id, 10)
	var asset Asset
	err := s.db.QueryRowContext(c,
		`INSERT INTO upload_assets (id, key, name, dir, content_type, size, data, object_key, object_url)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 RETURNING id, key, name, content_type, size, data, object_key, object_url`,
		id,
		key,
		strings.TrimSpace(input.Name),
		strings.TrimSpace(input.Dir),
		contentType,
		input.Size,
		input.Data,
		strings.TrimSpace(input.ObjectKey),
		strings.TrimSpace(input.ObjectURL),
	).Scan(&asset.ID, &asset.Key, &asset.Name, &asset.ContentType, &asset.Size, &asset.Data, &asset.ObjectKey, &asset.ObjectURL)
	if err != nil {
		return Asset{}, err
	}
	return asset, nil
}

// CreateAudio stores generated audio bytes and returns the new asset id. Thin
// convenience used by the article 听书 pipeline.
func (s *Store) CreateAudio(ctx context.Context, name string, contentType string, data []byte) (int64, error) {
	asset, err := s.Create(ctx, CreateInput{
		ContentType: contentType,
		Data:        data,
		Dir:         "article/audio",
		Name:        name,
		Size:        int64(len(data)),
	})
	if err != nil {
		return 0, err
	}
	return asset.ID, nil
}

func (s *Store) Find(ctx context.Context, id int64) (Asset, error) {
	if s == nil || s.db == nil {
		return Asset{}, sql.ErrNoRows
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var asset Asset
	err := s.db.QueryRowContext(c,
		`SELECT id, key, name, content_type, size, data, object_key, object_url
		 FROM upload_assets
		 WHERE id=$1`,
		id,
	).Scan(&asset.ID, &asset.Key, &asset.Name, &asset.ContentType, &asset.Size, &asset.Data, &asset.ObjectKey, &asset.ObjectURL)
	if err != nil {
		return Asset{}, err
	}
	return asset, nil
}
