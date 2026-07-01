package quiz

import (
	"context"
	"database/sql"
	"time"
)

// DailyCheckin 一条每日打卡记录。
type DailyCheckin struct {
	Date       string    `json:"date"`     // YYYY-MM-DD
	MainType   int       `json:"mainType"` // 打卡时主型
	CreateTime time.Time `json:"-"`
}

// GetDailyCheckin 查询某用户在指定日期是否已打卡。未打卡返回 (nil, nil)。
func (s *Store) GetDailyCheckin(ctx context.Context, appUserID int64, date string) (*DailyCheckin, error) {
	const q = `SELECT main_type, create_time FROM app_daily_checkins
	           WHERE app_user_id = $1 AND checkin_date = $2`
	var c DailyCheckin
	c.Date = date
	err := s.db.QueryRowContext(ctx, q, appUserID, date).Scan(&c.MainType, &c.CreateTime)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpsertDailyCheckin 记录某用户在指定日期的打卡（幂等，重复打卡保持首次记录不变）。
func (s *Store) UpsertDailyCheckin(ctx context.Context, appUserID int64, date string, mainType int) error {
	const q = `INSERT INTO app_daily_checkins (app_user_id, checkin_date, main_type)
	           VALUES ($1, $2, $3)
	           ON CONFLICT (app_user_id, checkin_date) DO NOTHING`
	_, err := s.db.ExecContext(ctx, q, appUserID, date, mainType)
	return err
}

// CountDailyCheckins 返回某用户累计打卡天数，用于展示坚持记录。
func (s *Store) CountDailyCheckins(ctx context.Context, appUserID int64) (int, error) {
	const q = `SELECT COUNT(*) FROM app_daily_checkins WHERE app_user_id = $1`
	var n int
	if err := s.db.QueryRowContext(ctx, q, appUserID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
