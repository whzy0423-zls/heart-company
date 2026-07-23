// Package miniapp 提供小程序业务存储：微信用户、测试存档、预约。
package miniapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/dbtx"
)

type Store struct {
	db *sql.DB
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

const queryTimeout = 10 * time.Second

const (
	maxTestRecordJSONBytes = 64 * 1024
	maxTestGenderRunes     = 32
)

const (
	maxOpenIDRunes  = 128
	maxUnionIDRunes = 128
	maxChannelRunes = 64
	maxSceneRunes   = 64
)

var (
	ErrNilDBTX           = errors.New("miniapp: query target is nil")
	ErrInvalidOpenID     = errors.New("miniapp: invalid openid")
	ErrInvalidUserSource = errors.New("miniapp: invalid user source")
	ErrInvalidTestRecord = errors.New("miniapp: invalid test record")
)

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
	if s == nil || s.db == nil {
		return 0, ErrNilDBTX
	}
	id, _, err := s.UpsertByOpenIDWithDBTX(c, s.db, openid, unionid, channel, scene)
	return id, err
}

// UpsertByOpenIDWithDBTX 在调用方事务中创建或更新登录用户，并明确返回是否首次创建。
func (s *Store) UpsertByOpenIDWithDBTX(ctx context.Context, q dbtx.DBTX, openid, unionid, channel, scene string) (int64, bool, error) {
	var err error
	openid, unionid, channel, scene, err = normalizeUserSource(openid, unionid, channel, scene)
	if err != nil {
		return 0, false, err
	}
	if q == nil {
		return 0, false, ErrNilDBTX
	}

	var id int64
	err = q.QueryRowContext(ctx,
		`INSERT INTO wx_users (openid, unionid, channel, scene)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (openid) DO NOTHING
		 RETURNING id`,
		openid, unionid, channel, scene,
	).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("insert miniapp user: %w", err)
	}
	err = q.QueryRowContext(ctx,
		`UPDATE wx_users
		 SET last_login_at=now()
		 WHERE openid=$1
		 RETURNING id`,
		openid,
	).Scan(&id)
	if err != nil {
		return 0, false, fmt.Errorf("update miniapp user login: %w", err)
	}
	return id, false, nil
}

func normalizeUserSource(openid, unionid, channel, scene string) (string, string, string, string, error) {
	if containsControl(openid) {
		return "", "", "", "", ErrInvalidOpenID
	}
	if containsControl(unionid) || containsControl(channel) || containsControl(scene) {
		return "", "", "", "", ErrInvalidUserSource
	}
	openid = strings.TrimSpace(openid)
	unionid = strings.TrimSpace(unionid)
	channel = strings.TrimSpace(channel)
	scene = strings.TrimSpace(scene)
	if openid == "" || utf8.RuneCountInString(openid) > maxOpenIDRunes || !isSafeWechatID(openid) {
		return "", "", "", "", ErrInvalidOpenID
	}
	if utf8.RuneCountInString(unionid) > maxUnionIDRunes || (unionid != "" && !isSafeWechatID(unionid)) ||
		utf8.RuneCountInString(channel) > maxChannelRunes || utf8.RuneCountInString(scene) > maxSceneRunes {
		return "", "", "", "", ErrInvalidUserSource
	}
	return openid, unionid, channel, scene, nil
}

func containsControl(value string) bool {
	return strings.ContainsFunc(value, unicode.IsControl)
}

func isSafeWechatID(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func (s *Store) GetUser(ctx context.Context, id int64) (User, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	if s == nil || s.db == nil {
		return User{}, ErrNilDBTX
	}
	return s.GetUserWithDBTX(c, s.db, id)
}

func (s *Store) GetUserWithDBTX(ctx context.Context, q dbtx.DBTX, id int64) (User, error) {
	if q == nil {
		return User{}, ErrNilDBTX
	}
	var u User
	var uid int64
	var ct time.Time
	err := q.QueryRowContext(ctx,
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

func normalizeTestRecordInput(userID int64, in TestRecordInput) (TestRecordInput, error) {
	if userID <= 0 || in.ResultType < 1 || in.ResultType > 9 || in.SecondType < 0 || in.SecondType > 9 {
		return TestRecordInput{}, ErrInvalidTestRecord
	}
	in.Gender = strings.TrimSpace(in.Gender)
	if utf8.RuneCountInString(in.Gender) > maxTestGenderRunes || containsControl(in.Gender) {
		return TestRecordInput{}, ErrInvalidTestRecord
	}
	if len(in.Scores) == 0 {
		in.Scores = json.RawMessage(`{}`)
	}
	if len(in.Centers) == 0 {
		in.Centers = json.RawMessage(`[]`)
	}
	if len(in.Scores) > maxTestRecordJSONBytes || len(in.Centers) > maxTestRecordJSONBytes ||
		!json.Valid(in.Scores) || !json.Valid(in.Centers) {
		return TestRecordInput{}, ErrInvalidTestRecord
	}
	in.Scores = append(json.RawMessage(nil), in.Scores...)
	in.Centers = append(json.RawMessage(nil), in.Centers...)
	return in, nil
}

func (s *Store) InsertTestRecord(ctx context.Context, q dbtx.DBTX, userID int64, in TestRecordInput) (TestRecord, error) {
	in, err := normalizeTestRecordInput(userID, in)
	if err != nil {
		return TestRecord{}, err
	}
	if q == nil {
		return TestRecord{}, ErrNilDBTX
	}

	var id int64
	var ct time.Time
	if err := q.QueryRowContext(ctx,
		`INSERT INTO test_records (wx_user_id, gender, result_type, second_type, scores, centers)
		 VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb) RETURNING id, create_time`,
		userID, in.Gender, in.ResultType, in.SecondType, string(in.Scores), string(in.Centers),
	).Scan(&id, &ct); err != nil {
		return TestRecord{}, err
	}

	return TestRecord{
		ID:         strconv.FormatInt(id, 10),
		Gender:     in.Gender,
		ResultType: in.ResultType,
		SecondType: in.SecondType,
		Scores:     in.Scores,
		Centers:    in.Centers,
		CreateTime: fmtTime(ct),
	}, nil
}

func (s *Store) UpdateMainType(ctx context.Context, q dbtx.DBTX, userID int64, resultType int) error {
	if userID <= 0 || resultType < 1 || resultType > 9 {
		return ErrInvalidTestRecord
	}
	if q == nil {
		return ErrNilDBTX
	}
	result, err := q.ExecContext(ctx, `UPDATE wx_users SET main_type=$1 WHERE id=$2`, resultType, userID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SaveTestRecord 已弃用：新调用方应通过 Service.SaveTestRecord 同时写入业务消息。
func (s *Store) SaveTestRecord(ctx context.Context, userID int64, in TestRecordInput) (TestRecord, error) {
	if s == nil || s.db == nil {
		return TestRecord{}, ErrNilDBTX
	}
	c, cancel := s.ctx(ctx)
	defer cancel()
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return TestRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := s.InsertTestRecord(c, tx, userID, in)
	if err != nil {
		return TestRecord{}, err
	}
	if err := s.UpdateMainType(c, tx, userID, record.ResultType); err != nil {
		return TestRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return TestRecord{}, err
	}
	return record, nil
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
