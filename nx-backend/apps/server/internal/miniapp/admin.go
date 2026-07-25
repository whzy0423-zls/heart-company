package miniapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/privacy"
)

var ErrInvalidAdminPagination = errors.New("miniapp: invalid admin pagination")

type AdminPagination struct {
	Page     int
	PageSize int
}

func NormalizeAdminPagination(page, pageSize int) (AdminPagination, error) {
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 20
	}
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return AdminPagination{}, ErrInvalidAdminPagination
	}
	return AdminPagination{Page: page, PageSize: pageSize}, nil
}

type AdminListOptions struct {
	Page     int
	PageSize int
	Keyword  string
	Channel  string
}

type AdminDetailOptions struct {
	TestPage        int
	TestPageSize    int
	BookingPage     int
	BookingPageSize int
}

type AdminUser struct {
	ID          string `json:"id"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	Phone       string `json:"phone"`
	Gender      string `json:"gender"`
	MainType    int    `json:"mainType"`
	MemberLevel int    `json:"memberLevel"`
	Channel     string `json:"channel"`
	Scene       string `json:"scene"`
	CreateTime  string `json:"createTime"`
	LastLoginAt string `json:"lastLoginAt"`
	OpenID      string `json:"-"`
	UnionID     string `json:"-"`
}

type AdminUserPage struct {
	Items []AdminUser `json:"items"`
	Total int         `json:"total"`
}

type AdminTestRecordPage struct {
	Items []TestRecord `json:"items"`
	Total int          `json:"total"`
}

type AdminBookingPage struct {
	Items []Booking `json:"items"`
	Total int       `json:"total"`
}

type AdminUserDetail struct {
	User        AdminUser           `json:"user"`
	TestRecords AdminTestRecordPage `json:"testRecords"`
	Bookings    AdminBookingPage    `json:"bookings"`
}

type AdminStore struct {
	db *sql.DB
}

func NewAdminStore(database *sql.DB) *AdminStore {
	return &AdminStore{db: database}
}

func (s *AdminStore) ListUsers(ctx context.Context, opts AdminListOptions) (AdminUserPage, error) {
	if s == nil || s.db == nil {
		return AdminUserPage{}, ErrNilDBTX
	}
	pagination, err := NormalizeAdminPagination(opts.Page, opts.PageSize)
	if err != nil {
		return AdminUserPage{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	where := []string{"1=1"}
	args := []any{}
	if keyword := strings.TrimSpace(opts.Keyword); keyword != "" {
		args = append(args, "%"+keyword+"%")
		placeholder := "$" + strconv.Itoa(len(args))
		where = append(where, "(nickname ILIKE "+placeholder+" OR phone ILIKE "+placeholder+" OR channel ILIKE "+placeholder+" OR scene ILIKE "+placeholder+")")
	}
	if channel := strings.TrimSpace(opts.Channel); channel != "" {
		args = append(args, channel)
		where = append(where, "channel = $"+strconv.Itoa(len(args)))
	}
	whereSQL := strings.Join(where, " AND ")

	var result AdminUserPage
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM wx_users WHERE "+whereSQL, args...).Scan(&result.Total); err != nil {
		return AdminUserPage{}, fmt.Errorf("count miniapp admin users: %w", err)
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT id,nickname,avatar,phone,gender,main_type,member_level,channel,scene,create_time,last_login_at
		FROM wx_users WHERE `+whereSQL+` ORDER BY create_time DESC,id DESC LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2), listArgs...)
	if err != nil {
		return AdminUserPage{}, fmt.Errorf("list miniapp admin users: %w", err)
	}
	defer rows.Close()
	result.Items = []AdminUser{}
	for rows.Next() {
		user, err := scanAdminUser(rows)
		if err != nil {
			return AdminUserPage{}, fmt.Errorf("scan miniapp admin user: %w", err)
		}
		result.Items = append(result.Items, user)
	}
	if err := rows.Err(); err != nil {
		return AdminUserPage{}, fmt.Errorf("iterate miniapp admin users: %w", err)
	}
	return result, nil
}

func (s *AdminStore) GetUserDetail(ctx context.Context, id int64, opts AdminDetailOptions) (AdminUserDetail, error) {
	if s == nil || s.db == nil {
		return AdminUserDetail{}, ErrNilDBTX
	}
	if id <= 0 {
		return AdminUserDetail{}, ErrInvalidAdminPagination
	}
	testPagination, err := NormalizeAdminPagination(opts.TestPage, opts.TestPageSize)
	if err != nil {
		return AdminUserDetail{}, err
	}
	bookingPagination, err := NormalizeAdminPagination(opts.BookingPage, opts.BookingPageSize)
	if err != nil {
		return AdminUserDetail{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	user, err := scanAdminUser(s.db.QueryRowContext(ctx, `SELECT id,nickname,avatar,phone,gender,main_type,member_level,channel,scene,create_time,last_login_at FROM wx_users WHERE id=$1`, id))
	if err != nil {
		return AdminUserDetail{}, err
	}
	detail := AdminUserDetail{User: user}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM test_records WHERE wx_user_id=$1`, id).Scan(&detail.TestRecords.Total); err != nil {
		return AdminUserDetail{}, fmt.Errorf("count miniapp test records: %w", err)
	}
	detail.TestRecords.Items, err = s.listTestRecords(ctx, id, testPagination)
	if err != nil {
		return AdminUserDetail{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM bookings WHERE wx_user_id=$1`, id).Scan(&detail.Bookings.Total); err != nil {
		return AdminUserDetail{}, fmt.Errorf("count miniapp bookings: %w", err)
	}
	detail.Bookings.Items, err = s.listBookings(ctx, id, bookingPagination)
	if err != nil {
		return AdminUserDetail{}, err
	}
	return detail, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanAdminUser(row rowScanner) (AdminUser, error) {
	var user AdminUser
	var id int64
	var createdAt, lastLoginAt time.Time
	if err := row.Scan(&id, &user.Nickname, &user.Avatar, &user.Phone, &user.Gender, &user.MainType, &user.MemberLevel, &user.Channel, &user.Scene, &createdAt, &lastLoginAt); err != nil {
		return AdminUser{}, err
	}
	user.ID = strconv.FormatInt(id, 10)
	user.Phone = privacy.MaskPhone(user.Phone)
	user.CreateTime = fmtTime(createdAt)
	user.LastLoginAt = fmtTime(lastLoginAt)
	return user, nil
}

func (s *AdminStore) listTestRecords(ctx context.Context, userID int64, page AdminPagination) ([]TestRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,gender,result_type,second_type,scores,centers,create_time FROM test_records WHERE wx_user_id=$1 ORDER BY create_time DESC,id DESC LIMIT $2 OFFSET $3`, userID, page.PageSize, (page.Page-1)*page.PageSize)
	if err != nil {
		return nil, fmt.Errorf("list miniapp test records: %w", err)
	}
	defer rows.Close()
	items := []TestRecord{}
	for rows.Next() {
		var item TestRecord
		var id int64
		var createdAt time.Time
		var scores, centers []byte
		if err := rows.Scan(&id, &item.Gender, &item.ResultType, &item.SecondType, &scores, &centers, &createdAt); err != nil {
			return nil, fmt.Errorf("scan miniapp test record: %w", err)
		}
		item.ID, item.Scores, item.Centers, item.CreateTime = strconv.FormatInt(id, 10), scores, centers, fmtTime(createdAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AdminStore) listBookings(ctx context.Context, userID int64, page AdminPagination) ([]Booking, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(signup_id,0),kind,contact_name,phone,intent,preferred_time,message,status,create_time FROM bookings WHERE wx_user_id=$1 ORDER BY create_time DESC,id DESC LIMIT $2 OFFSET $3`, userID, page.PageSize, (page.Page-1)*page.PageSize)
	if err != nil {
		return nil, fmt.Errorf("list miniapp bookings: %w", err)
	}
	defer rows.Close()
	items := []Booking{}
	for rows.Next() {
		var item Booking
		var id, signupID int64
		var createdAt time.Time
		if err := rows.Scan(&id, &signupID, &item.Kind, &item.ContactName, &item.Phone, &item.Intent, &item.PreferredTime, &item.Message, &item.Status, &createdAt); err != nil {
			return nil, fmt.Errorf("scan miniapp booking: %w", err)
		}
		item.ID = strconv.FormatInt(id, 10)
		if signupID > 0 {
			item.SignupID = strconv.FormatInt(signupID, 10)
		}
		item.Phone = privacy.MaskPhone(item.Phone)
		item.CreateTime = fmtTime(createdAt)
		items = append(items, item)
	}
	return items, rows.Err()
}
