package push

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Pusher 推送发送接口。AppKey 为空时返回 noop 实现（仅写日志）。
type Pusher interface {
	Push(ctx context.Context, registrationIDs []string, msg Message) (PushResult, error)
}

type Message struct {
	Title    string
	Content  string
	DeepLink string
}

type PushResult struct {
	MsgID string
	Sent  int
}

// DeviceToken 设备推送令牌记录。
type DeviceToken struct {
	ID             int64  `json:"id"`
	AppUserID      int64  `json:"appUserId"`
	RegistrationID string `json:"registrationId"`
	Platform       string `json:"platform"`
	DeviceInfo     string `json:"deviceInfo"`
	CreateTime     string `json:"createTime"`
}

// PushNotification 推送记录。
type PushNotification struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	TargetType   string `json:"targetType"`
	TargetValue  string `json:"targetValue"`
	DeepLink     string `json:"deepLink"`
	SentCount    int    `json:"sentCount"`
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	Operator     string `json:"operator"`
	CreateTime   string `json:"createTime"`
}

// Store 推送相关的数据库操作。
type Store struct {
	db     *sql.DB
	pusher Pusher
}

func NewStore(db *sql.DB, pusher Pusher) *Store {
	return &Store{db: db, pusher: pusher}
}

func (s *Store) Pusher() Pusher { return s.pusher }

// RegisterDevice 注册或更新设备推送令牌。
func (s *Store) RegisterDevice(ctx context.Context, userID int64, regID, platform, deviceInfo string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_device_tokens (app_user_id, registration_id, platform, device_info, update_time)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (registration_id) DO UPDATE SET
			app_user_id = EXCLUDED.app_user_id,
			platform = EXCLUDED.platform,
			device_info = EXCLUDED.device_info,
			update_time = now()
	`, userID, regID, platform, deviceInfo)
	return err
}

// UnregisterDevice 删除指定设备令牌。
func (s *Store) UnregisterDevice(ctx context.Context, userID int64, regID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM app_device_tokens WHERE app_user_id = $1 AND registration_id = $2
	`, userID, regID)
	return err
}

// UnregisterAllDevices 删除用户所有设备令牌（登出时调用）。
func (s *Store) UnregisterAllDevices(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_device_tokens WHERE app_user_id = $1`, userID)
	return err
}

// GetAllRegistrationIDs 获取所有活跃设备令牌。
func (s *Store) GetAllRegistrationIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT dt.registration_id FROM app_device_tokens dt
		JOIN app_users u ON u.id = dt.app_user_id
		WHERE u.status = 'active'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetRegistrationIDsByUserIDs 根据用户 ID 列表获取令牌。
func (s *Store) GetRegistrationIDsByUserIDs(ctx context.Context, userIDs []int64) ([]string, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	query := `SELECT registration_id FROM app_device_tokens WHERE app_user_id = ANY($1)`
	rows, err := s.db.QueryContext(ctx, query, int64SliceToArray(userIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetRegistrationIDsByLevel 根据会员等级获取令牌。
func (s *Store) GetRegistrationIDsByLevel(ctx context.Context, level string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT dt.registration_id FROM app_device_tokens dt
		JOIN app_users u ON u.id = dt.app_user_id
		WHERE u.member_level = $1 AND u.status = 'active'
	`, level)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ForEachRegistrationIDBatch streams active registration IDs in bounded batches.
func (s *Store) ForEachRegistrationIDBatch(ctx context.Context, targetType, targetValue string, batchSize int, fn func([]string) error) error {
	if batchSize <= 0 || batchSize > 1000 {
		batchSize = 1000
	}
	if fn == nil {
		return nil
	}

	var lastID int64
	for {
		var (
			rows *sql.Rows
			err  error
		)
		switch targetType {
		case "", "all":
			rows, err = s.db.QueryContext(ctx, `
				SELECT dt.id, dt.registration_id
				FROM app_device_tokens dt
				JOIN app_users u ON u.id = dt.app_user_id
				WHERE u.status = 'active' AND dt.id > $1
				ORDER BY dt.id ASC
				LIMIT $2
			`, lastID, batchSize)
		case "level":
			rows, err = s.db.QueryContext(ctx, `
				SELECT dt.id, dt.registration_id
				FROM app_device_tokens dt
				JOIN app_users u ON u.id = dt.app_user_id
				WHERE u.status = 'active' AND u.member_level = $1 AND dt.id > $2
				ORDER BY dt.id ASC
				LIMIT $3
			`, targetValue, lastID, batchSize)
		default:
			return fmt.Errorf("unsupported push target type: %s", targetType)
		}
		if err != nil {
			return err
		}

		ids := make([]string, 0, batchSize)
		for rows.Next() {
			var id int64
			var registrationID string
			if err := rows.Scan(&id, &registrationID); err != nil {
				_ = rows.Close()
				return err
			}
			lastID = id
			ids = append(ids, registrationID)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := fn(ids); err != nil {
			return err
		}
		if len(ids) < batchSize {
			return nil
		}
	}
}

// CountAudience counts active users and distinct device tokens matching a push target.
func (s *Store) CountAudience(ctx context.Context, targetType, targetValue string) (int64, int64, error) {
	var (
		deviceCount int64
		userCount   int64
		err         error
	)
	switch targetType {
	case "", "all":
		err = s.db.QueryRowContext(ctx, `
			SELECT COUNT(DISTINCT dt.registration_id), COUNT(DISTINCT u.id)
			FROM app_device_tokens dt
			JOIN app_users u ON u.id = dt.app_user_id
			WHERE u.status = 'active'
		`).Scan(&deviceCount, &userCount)
	case "level":
		err = s.db.QueryRowContext(ctx, `
			SELECT COUNT(DISTINCT dt.registration_id), COUNT(DISTINCT u.id)
			FROM app_device_tokens dt
			JOIN app_users u ON u.id = dt.app_user_id
			WHERE u.status = 'active' AND u.member_level = $1
		`, targetValue).Scan(&deviceCount, &userCount)
	default:
		err = fmt.Errorf("unsupported push target type: %s", targetType)
	}
	if err != nil {
		return 0, 0, err
	}
	return deviceCount, userCount, nil
}

// CreatePushRecord 创建推送记录。
func (s *Store) CreatePushRecord(ctx context.Context, title, content, targetType, targetValue, deepLink, operator string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO push_notifications (title, content, target_type, target_value, deep_link, operator)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
	`, title, content, targetType, targetValue, deepLink, operator).Scan(&id)
	return id, err
}

// UpdatePushStatus 更新推送状态。
func (s *Store) UpdatePushStatus(ctx context.Context, id int64, status string, sentCount int, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE push_notifications SET status = $2, sent_count = $3, error_message = $4
		WHERE id = $1
	`, id, status, sentCount, errMsg)
	return err
}

// ClaimPendingPushTask atomically moves a pending task into sending state.
// It returns false when another worker already claimed or finished the task.
func (s *Store) ClaimPendingPushTask(ctx context.Context, id int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE push_notifications SET status = $2, sent_count = $3, error_message = $4
		WHERE id = $1 AND status = 'pending'
	`, id, "sending", 0, "")
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// ListPushHistory 分页查询推送历史。
func (s *Store) ListPushHistory(ctx context.Context, page, pageSize int) ([]PushNotification, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM push_notifications`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, content, target_type, target_value, deep_link, sent_count, status, error_message, operator, create_time
		FROM push_notifications ORDER BY create_time DESC LIMIT $1 OFFSET $2
	`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []PushNotification
	for rows.Next() {
		var n PushNotification
		var ct time.Time
		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &n.TargetType, &n.TargetValue,
			&n.DeepLink, &n.SentCount, &n.Status, &n.ErrorMessage, &n.Operator, &ct); err != nil {
			return nil, 0, err
		}
		n.CreateTime = ct.Format("2006/01/02 15:04:05")
		items = append(items, n)
	}
	return items, total, rows.Err()
}

// ListRecoverablePushTasks returns pending push tasks after process restart.
func (s *Store) ListRecoverablePushTasks(ctx context.Context, limit int) ([]PushNotification, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, content, target_type, target_value, deep_link, sent_count, status, error_message, operator, create_time
		FROM push_notifications
		WHERE status = 'pending'
		ORDER BY create_time ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PushNotification
	for rows.Next() {
		var n PushNotification
		var ct time.Time
		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &n.TargetType, &n.TargetValue,
			&n.DeepLink, &n.SentCount, &n.Status, &n.ErrorMessage, &n.Operator, &ct); err != nil {
			return nil, err
		}
		n.CreateTime = ct.Format("2006/01/02 15:04:05")
		items = append(items, n)
	}
	return items, rows.Err()
}

// MarkInterruptedPushTasks marks tasks that were in-flight during a prior process as failed.
func (s *Store) MarkInterruptedPushTasks(ctx context.Context, reason string) error {
	return s.MarkInterruptedPushTasksBefore(ctx, reason, time.Time{})
}

// MarkInterruptedPushTasksBefore marks in-flight tasks older than cutoff as failed.
// A zero cutoff preserves the legacy behavior and marks all sending tasks.
func (s *Store) MarkInterruptedPushTasksBefore(ctx context.Context, reason string, cutoff time.Time) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "推送发送已中断，请重新发送"
	}
	if cutoff.IsZero() {
		_, err := s.db.ExecContext(ctx, `
			UPDATE push_notifications
			SET status = 'failed', error_message = $1
			WHERE status = 'sending'
		`, reason)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE push_notifications
		SET status = 'failed', error_message = $1
		WHERE status = 'sending' AND create_time < $2
	`, reason, cutoff)
	return err
}

// int64SliceToArray 将 int64 切片转为 PostgreSQL array 兼容类型。
func int64SliceToArray(ids []int64) interface{} {
	// pgx 驱动直接支持 []int64 → int8[]
	return ids
}
