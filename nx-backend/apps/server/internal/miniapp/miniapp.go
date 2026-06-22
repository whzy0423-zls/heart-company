// Package miniapp 提供小程序业务存储：微信用户、测试存档、预约。
package miniapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type Store struct {
	db *sql.DB
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

const queryTimeout = 10 * time.Second

func (s *Store) ctx(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, queryTimeout)
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

// ---------------- 用户 ----------------

type User struct {
	ID          string `json:"id"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	Phone       string `json:"phone"`
	Gender      string `json:"gender"`
	MainType    int    `json:"mainType"`
	MemberLevel int    `json:"memberLevel"`
	CreateTime  string `json:"createTime"`
}

// UpsertByOpenID 按 openid 查找或创建用户，返回用户 id。
func (s *Store) UpsertByOpenID(ctx context.Context, openid, unionid, channel, scene string) (int64, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	var id int64
	err := s.db.QueryRowContext(c,
		`INSERT INTO wx_users (openid, unionid, channel, scene)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (openid) DO UPDATE SET last_login_at = now()
		 RETURNING id`,
		openid, unionid, channel, scene,
	).Scan(&id)
	return id, err
}

func (s *Store) GetUser(ctx context.Context, id int64) (User, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	var u User
	var uid int64
	var ct time.Time
	err := s.db.QueryRowContext(c,
		`SELECT id, nickname, avatar, phone, gender, main_type, member_level, create_time
		 FROM wx_users WHERE id=$1`, id,
	).Scan(&uid, &u.Nickname, &u.Avatar, &u.Phone, &u.Gender, &u.MainType, &u.MemberLevel, &ct)
	if err != nil {
		return User{}, err
	}
	u.ID = strconv.FormatInt(uid, 10)
	u.CreateTime = fmtTime(ct)
	return u, nil
}

type ProfileUpdate struct {
	Nickname *string `json:"nickname"`
	Avatar   *string `json:"avatar"`
	Phone    *string `json:"phone"`
	Gender   *string `json:"gender"`
}

func (s *Store) UpdateUser(ctx context.Context, id int64, in ProfileUpdate) (User, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	sets := []string{}
	args := []any{}
	add := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, col+"=$"+strconv.Itoa(len(args)))
	}
	if in.Nickname != nil {
		add("nickname", *in.Nickname)
	}
	if in.Avatar != nil {
		add("avatar", *in.Avatar)
	}
	if in.Phone != nil {
		add("phone", *in.Phone)
	}
	if in.Gender != nil {
		add("gender", *in.Gender)
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := s.db.ExecContext(c,
			"UPDATE wx_users SET "+strings.Join(sets, ", ")+" WHERE id=$"+strconv.Itoa(len(args)), args...); err != nil {
			return User{}, err
		}
	}
	return s.GetUser(ctx, id)
}

// ---------------- 测试存档 ----------------

type TestRecord struct {
	ID         string          `json:"id"`
	Gender     string          `json:"gender"`
	ResultType int             `json:"resultType"`
	SecondType int             `json:"secondType"`
	Scores     json.RawMessage `json:"scores"`
	Centers    json.RawMessage `json:"centers"`
	CreateTime string          `json:"createTime"`
}

type TestRecordInput struct {
	Gender     string          `json:"gender"`
	ResultType int             `json:"resultType"`
	SecondType int             `json:"secondType"`
	Scores     json.RawMessage `json:"scores"`
	Centers    json.RawMessage `json:"centers"`
}

func (s *Store) SaveTestRecord(ctx context.Context, userID int64, in TestRecordInput) (TestRecord, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	scores := string(in.Scores)
	if scores == "" {
		scores = "{}"
	}
	centers := string(in.Centers)
	if centers == "" {
		centers = "[]"
	}

	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return TestRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var id int64
	var ct time.Time
	if err := tx.QueryRowContext(c,
		`INSERT INTO test_records (wx_user_id, gender, result_type, second_type, scores, centers)
		 VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb) RETURNING id, create_time`,
		userID, in.Gender, in.ResultType, in.SecondType, scores, centers,
	).Scan(&id, &ct); err != nil {
		return TestRecord{}, err
	}
	// 同步用户最近主型
	if _, err := tx.ExecContext(c, `UPDATE wx_users SET main_type=$1 WHERE id=$2`, in.ResultType, userID); err != nil {
		return TestRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return TestRecord{}, err
	}

	return TestRecord{
		ID:         strconv.FormatInt(id, 10),
		Gender:     in.Gender,
		ResultType: in.ResultType,
		SecondType: in.SecondType,
		Scores:     json.RawMessage(scores),
		Centers:    json.RawMessage(centers),
		CreateTime: fmtTime(ct),
	}, nil
}

func (s *Store) ListTestRecords(ctx context.Context, userID int64) ([]TestRecord, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(c,
		`SELECT id, gender, result_type, second_type, scores, centers, create_time
		 FROM test_records WHERE wx_user_id=$1 ORDER BY create_time DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []TestRecord{}
	for rows.Next() {
		var t TestRecord
		var id int64
		var ct time.Time
		var scores, centers []byte
		if err := rows.Scan(&id, &t.Gender, &t.ResultType, &t.SecondType, &scores, &centers, &ct); err != nil {
			return nil, err
		}
		t.ID = strconv.FormatInt(id, 10)
		t.Scores = json.RawMessage(scores)
		t.Centers = json.RawMessage(centers)
		t.CreateTime = fmtTime(ct)
		items = append(items, t)
	}
	return items, rows.Err()
}

// ---------------- 预约 ----------------

type Booking struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	ContactName   string `json:"contactName"`
	Phone         string `json:"phone"`
	Intent        string `json:"intent"`
	PreferredTime string `json:"preferredTime"`
	Message       string `json:"message"`
	Status        string `json:"status"`
	CreateTime    string `json:"createTime"`
}

type BookingInput struct {
	Kind          string `json:"kind"`
	ContactName   string `json:"contactName"`
	Phone         string `json:"phone"`
	Intent        string `json:"intent"`
	PreferredTime string `json:"preferredTime"`
	Message       string `json:"message"`
}

// CreateBooking 落库预约，并返回新预约 id。signupID 为关联的后台线索 id（0 表示未关联）。
func (s *Store) CreateBooking(ctx context.Context, userID int64, in BookingInput, signupID int64) (Booking, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	kind := in.Kind
	if kind == "" {
		kind = "consult"
	}
	var sid sql.NullInt64
	if signupID > 0 {
		sid = sql.NullInt64{Int64: signupID, Valid: true}
	}
	var id int64
	var ct time.Time
	err := s.db.QueryRowContext(c,
		`INSERT INTO bookings (wx_user_id, kind, contact_name, phone, intent, preferred_time, message, signup_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, create_time`,
		userID, kind, in.ContactName, in.Phone, in.Intent, in.PreferredTime, in.Message, sid,
	).Scan(&id, &ct)
	if err != nil {
		return Booking{}, err
	}
	return Booking{
		ID:            strconv.FormatInt(id, 10),
		Kind:          kind,
		ContactName:   in.ContactName,
		Phone:         in.Phone,
		Intent:        in.Intent,
		PreferredTime: in.PreferredTime,
		Message:       in.Message,
		Status:        "pending",
		CreateTime:    fmtTime(ct),
	}, nil
}

func (s *Store) ListBookings(ctx context.Context, userID int64) ([]Booking, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(c,
		`SELECT id, kind, contact_name, phone, intent, preferred_time, message, status, create_time
		 FROM bookings WHERE wx_user_id=$1 ORDER BY create_time DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Booking{}
	for rows.Next() {
		var b Booking
		var id int64
		var ct time.Time
		if err := rows.Scan(&id, &b.Kind, &b.ContactName, &b.Phone, &b.Intent, &b.PreferredTime, &b.Message, &b.Status, &ct); err != nil {
			return nil, err
		}
		b.ID = strconv.FormatInt(id, 10)
		b.CreateTime = fmtTime(ct)
		items = append(items, b)
	}
	return items, rows.Err()
}
