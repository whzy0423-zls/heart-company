package appnotification

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Notification struct {
	ID         int64  `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	DeepLink   string `json:"deepLink"`
	Read       bool   `json:"read"`
	ReadTime   string `json:"readTime,omitempty"`
	CreateTime string `json:"createTime"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CreateForUser(ctx context.Context, userID int64, kind, title, content, deepLink, sourceKey string) (int64, error) {
	if userID <= 0 {
		return 0, fmt.Errorf("invalid app user id")
	}
	kind, title, content, deepLink, sourceKey = normalizePayload(kind, title, content, deepLink, sourceKey)
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO app_notifications (app_user_id, kind, title, content, deep_link, source_key)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (app_user_id, source_key) WHERE source_key <> ''
		DO UPDATE SET source_key = EXCLUDED.source_key
		RETURNING id
	`, userID, kind, title, content, deepLink, sourceKey).Scan(&id)
	return id, err
}

func (s *Store) CreateForAudience(ctx context.Context, targetType, targetValue, kind, title, content, deepLink, sourceKey string) (int64, error) {
	targetType = strings.TrimSpace(targetType)
	targetValue = strings.TrimSpace(targetValue)
	kind, title, content, deepLink, sourceKey = normalizePayload(kind, title, content, deepLink, sourceKey)
	var (
		result sql.Result
		err    error
	)
	switch targetType {
	case "", "all":
		result, err = s.db.ExecContext(ctx, `
			INSERT INTO app_notifications (app_user_id, kind, title, content, deep_link, source_key)
			SELECT id, $1, $2, $3, $4, $5 FROM app_users WHERE status = 'active'
			ON CONFLICT (app_user_id, source_key) WHERE source_key <> '' DO NOTHING
		`, kind, title, content, deepLink, sourceKey)
	case "level":
		result, err = s.db.ExecContext(ctx, `
			INSERT INTO app_notifications (app_user_id, kind, title, content, deep_link, source_key)
			SELECT id, $1, $2, $3, $4, $5 FROM app_users
			WHERE status = 'active' AND member_level = $6
			ON CONFLICT (app_user_id, source_key) WHERE source_key <> '' DO NOTHING
		`, kind, title, content, deepLink, sourceKey, targetValue)
	default:
		return 0, fmt.Errorf("unsupported notification target type: %s", targetType)
	}
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) List(ctx context.Context, userID int64, page, pageSize int) ([]Notification, int, int, error) {
	page, pageSize = normalizePage(page, pageSize)
	offset := (page - 1) * pageSize
	var total, unread int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE read_time IS NULL)
		FROM app_notifications WHERE app_user_id = $1
	`, userID).Scan(&total, &unread); err != nil {
		return nil, 0, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, title, content, deep_link, read_time, create_time
		FROM app_notifications
		WHERE app_user_id = $1
		ORDER BY create_time DESC, id DESC
		LIMIT $2 OFFSET $3
	`, userID, pageSize, offset)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()
	items := make([]Notification, 0, pageSize)
	for rows.Next() {
		var item Notification
		var readTime sql.NullTime
		var createTime time.Time
		if err := rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Content, &item.DeepLink, &readTime, &createTime); err != nil {
			return nil, 0, 0, err
		}
		item.Read = readTime.Valid
		if readTime.Valid {
			item.ReadTime = formatTime(readTime.Time)
		}
		item.CreateTime = formatTime(createTime)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}
	return items, total, unread, nil
}

func (s *Store) UnreadCount(ctx context.Context, userID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM app_notifications
		WHERE app_user_id = $1 AND read_time IS NULL
	`, userID).Scan(&count)
	return count, err
}

func (s *Store) MarkRead(ctx context.Context, userID, notificationID int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE app_notifications SET read_time = COALESCE(read_time, now())
		WHERE id = $1 AND app_user_id = $2
	`, notificationID, userID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (s *Store) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE app_notifications SET read_time = now()
		WHERE app_user_id = $1 AND read_time IS NULL
	`, userID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

func normalizePayload(kind, title, content, deepLink, sourceKey string) (string, string, string, string, string) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "system"
	}
	return kind, strings.TrimSpace(title), strings.TrimSpace(content), strings.TrimSpace(deepLink), strings.TrimSpace(sourceKey)
}

func formatTime(value time.Time) string {
	return value.Local().Format("2006/01/02 15:04:05")
}
