package appuser

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type User struct {
	ID             int64  `json:"id"`
	Phone          string `json:"phone"`
	Nickname       string `json:"nickname"`
	Avatar         string `json:"avatar"`
	Status         string `json:"status"`
	MemberLevel    string `json:"memberLevel"`
	RegisterSource string `json:"registerSource"`
	LastLoginAt    string `json:"lastLoginAt"`
	CreateTime     string `json:"createTime"`
	UpdateTime     string `json:"updateTime"`
}

type RefreshToken struct {
	ID         int64     `json:"id"`
	AppUserID  int64     `json:"appUserId"`
	TokenHash  string    `json:"-"`
	DeviceInfo string    `json:"deviceInfo"`
	ExpiresAt  string    `json:"expiresAt"`
	expiresAt  time.Time `json:"-"`
	Revoked    bool      `json:"revoked"`
	CreateTime string    `json:"createTime"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

func (s *Store) FindOrCreateByPhone(ctx context.Context, phone string) (User, error) {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO app_users (phone) VALUES ($1) ON CONFLICT (phone) DO NOTHING`, phone)
	if err != nil {
		return User{}, fmt.Errorf("appuser insert: %w", err)
	}
	var u User
	var lastLogin sql.NullTime
	var createTime, updateTime time.Time
	err = s.db.QueryRowContext(ctx,
		`UPDATE app_users SET last_login_at = now(), update_time = now()
		 WHERE phone = $1
		 RETURNING id, phone, nickname, avatar, status, member_level, register_source, last_login_at, create_time, update_time`,
		phone).Scan(&u.ID, &u.Phone, &u.Nickname, &u.Avatar, &u.Status, &u.MemberLevel, &u.RegisterSource, &lastLogin, &createTime, &updateTime)
	if err != nil {
		return User{}, fmt.Errorf("appuser find: %w", err)
	}
	if lastLogin.Valid {
		u.LastLoginAt = formatTime(lastLogin.Time)
	}
	u.CreateTime = formatTime(createTime)
	u.UpdateTime = formatTime(updateTime)
	return u, nil
}

func (s *Store) FindByID(ctx context.Context, id int64) (User, error) {
	var u User
	var lastLogin sql.NullTime
	var createTime, updateTime time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT id, phone, nickname, avatar, status, member_level, register_source, last_login_at, create_time, update_time
		 FROM app_users WHERE id = $1`, id).
		Scan(&u.ID, &u.Phone, &u.Nickname, &u.Avatar, &u.Status, &u.MemberLevel, &u.RegisterSource, &lastLogin, &createTime, &updateTime)
	if err != nil {
		return User{}, err
	}
	if lastLogin.Valid {
		u.LastLoginAt = formatTime(lastLogin.Time)
	}
	u.CreateTime = formatTime(createTime)
	u.UpdateTime = formatTime(updateTime)
	return u, nil
}

// PageResult 是分页查询的通用返回结构。
type PageResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// pageParams 从查询参数解析分页，默认 page=1, pageSize=20（上限 100）。
func pageParams(query map[string]string) (int, int) {
	page := 1
	if n, err := strconv.Atoi(strings.TrimSpace(query["page"])); err == nil && n > 0 {
		page = n
	}
	pageSize := 20
	if n, err := strconv.Atoi(strings.TrimSpace(query["pageSize"])); err == nil && n > 0 {
		pageSize = n
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

// List 分页查询 App 客户，支持按手机号/昵称模糊搜索及状态、会员等级过滤。
func (s *Store) List(ctx context.Context, query map[string]string) (PageResult[User], error) {
	where := []string{"1=1"}
	args := []any{}

	if kw := strings.TrimSpace(query["keyword"]); kw != "" {
		args = append(args, "%"+strings.ToLower(kw)+"%")
		p := "$" + strconv.Itoa(len(args))
		where = append(where, "(lower(phone) LIKE "+p+" OR lower(nickname) LIKE "+p+")")
	}
	if st := strings.TrimSpace(query["status"]); st != "" {
		args = append(args, st)
		where = append(where, "status = $"+strconv.Itoa(len(args)))
	}
	if ml := strings.TrimSpace(query["memberLevel"]); ml != "" {
		args = append(args, ml)
		where = append(where, "member_level = $"+strconv.Itoa(len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT count(*) FROM app_users WHERE "+cond, args...).Scan(&total); err != nil {
		return PageResult[User]{}, fmt.Errorf("appuser count: %w", err)
	}

	page, pageSize := pageParams(query)
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, phone, nickname, avatar, status, member_level, register_source, last_login_at, create_time, update_time"+
			" FROM app_users WHERE "+cond+
			" ORDER BY create_time DESC, id DESC"+
			" LIMIT $"+strconv.Itoa(len(args)-1)+" OFFSET $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return PageResult[User]{}, fmt.Errorf("appuser list: %w", err)
	}
	defer rows.Close()

	items := []User{}
	for rows.Next() {
		var u User
		var lastLogin sql.NullTime
		var createTime, updateTime time.Time
		if err := rows.Scan(&u.ID, &u.Phone, &u.Nickname, &u.Avatar, &u.Status, &u.MemberLevel, &u.RegisterSource, &lastLogin, &createTime, &updateTime); err != nil {
			return PageResult[User]{}, fmt.Errorf("appuser scan: %w", err)
		}
		if lastLogin.Valid {
			u.LastLoginAt = formatTime(lastLogin.Time)
		}
		u.CreateTime = formatTime(createTime)
		u.UpdateTime = formatTime(updateTime)
		items = append(items, u)
	}
	if err := rows.Err(); err != nil {
		return PageResult[User]{}, err
	}
	return PageResult[User]{Items: items, Total: total}, nil
}

func (s *Store) StoreSMSCode(ctx context.Context, phone, codeHash, sendIP string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO app_sms_codes (phone, code_hash, expires_at, send_ip) VALUES ($1, $2, $3, $4)`,
		phone, codeHash, expiresAt, sendIP)
	return err
}

func (s *Store) VerifyAndUseSMSCode(ctx context.Context, phone, codeHash string) (bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`UPDATE app_sms_codes SET used = true
		 WHERE id = (
		   SELECT id FROM app_sms_codes
		   WHERE phone = $1 AND code_hash = $2 AND used = false AND expires_at > now()
		   ORDER BY create_time DESC LIMIT 1
		 )
		 RETURNING id`, phone, codeHash).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) CreateRefreshToken(ctx context.Context, userID int64, tokenHash, deviceInfo string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO app_refresh_tokens (app_user_id, token_hash, device_info, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, tokenHash, deviceInfo, expiresAt)
	return err
}

func (s *Store) FindRefreshToken(ctx context.Context, tokenHash string) (RefreshToken, error) {
	var rt RefreshToken
	var createTime time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT id, app_user_id, token_hash, device_info, expires_at, revoked, create_time
		 FROM app_refresh_tokens WHERE token_hash = $1`, tokenHash).
		Scan(&rt.ID, &rt.AppUserID, &rt.TokenHash, &rt.DeviceInfo, &rt.expiresAt, &rt.Revoked, &createTime)
	if err != nil {
		return RefreshToken{}, err
	}
	rt.ExpiresAt = formatTime(rt.expiresAt)
	rt.CreateTime = formatTime(createTime)
	return rt, nil
}

func (rt RefreshToken) IsExpired(now time.Time) bool {
	return now.After(rt.expiresAt)
}

func (s *Store) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_refresh_tokens SET revoked = true WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *Store) RevokeAllUserTokens(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_refresh_tokens SET revoked = true WHERE app_user_id = $1 AND revoked = false`, userID)
	return err
}
