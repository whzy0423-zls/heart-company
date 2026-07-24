package analytics

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var ErrInvalidDays = errors.New("invalid days: must be 7 or 30")

type PlatformOverview struct {
	Website          WebsiteOverview       `json:"website"`
	Miniapp          MiniappOverview       `json:"miniapp"`
	Series           []PlatformSeriesPoint `json:"series"`
	RecentActivities []RecentActivity      `json:"recentActivities"`
}
type WebsiteOverview struct {
	TotalUsers       int     `json:"totalUsers"`
	NewUsersToday    int     `json:"newUsersToday"`
	ActiveUsersToday int     `json:"activeUsersToday"`
	TotalPV          int     `json:"totalPV"`
	TodayPV          int     `json:"todayPV"`
	TotalSubmissions int     `json:"totalSubmissions"`
	SubmissionsToday int     `json:"submissionsToday"`
	ConversionRate   float64 `json:"conversionRate"`
}
type MiniappOverview struct {
	TotalUsers    int `json:"totalUsers"`
	NewUsersToday int `json:"newUsersToday"`
	TotalTests    int `json:"totalTests"`
	TestsToday    int `json:"testsToday"`
	TotalBookings int `json:"totalBookings"`
	BookingsToday int `json:"bookingsToday"`
}
type PlatformSeriesPoint struct {
	Date               string `json:"date"`
	WebsiteActiveUsers int    `json:"websiteActiveUsers"`
	MiniappNewUsers    int    `json:"miniappNewUsers"`
	WebsiteSubmissions int    `json:"websiteSubmissions"`
	MiniappTests       int    `json:"miniappTests"`
	MiniappBookings    int    `json:"miniappBookings"`
}
type RecentActivity struct {
	ID         string `json:"id"`
	EventKey   string `json:"eventKey"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	TargetPath string `json:"targetPath"`
	CreateTime string `json:"createTime"`
	Platform   string `json:"platform"`
}

func (s *Store) PlatformOverview(ctx context.Context, days int, now time.Time) (PlatformOverview, error) {
	if days != 7 && days != 30 {
		return PlatformOverview{}, ErrInvalidDays
	}
	if now.IsZero() {
		now = time.Now()
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now = now.In(loc)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	start := midnight.AddDate(0, 0, -(days - 1))
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var out PlatformOverview
	err := s.db.QueryRowContext(c, `
WITH bounds AS (SELECT $1::timestamptz AS start_at, $2::timestamptz AS end_at, ($2::date AT TIME ZONE 'Asia/Shanghai') AS today_start),
first_visits AS (SELECT COALESCE(NULLIF(visitor_id,''), ip||'|'||user_agent) AS visitor, min(create_time) AS first_at FROM site_visits GROUP BY 1)
SELECT
 (SELECT count(*) FROM first_visits),
 (SELECT count(*) FROM first_visits,bounds WHERE first_at >= bounds.today_start),
 (SELECT count(DISTINCT COALESCE(NULLIF(visitor_id,''),ip||'|'||user_agent)) FROM site_visits,bounds WHERE create_time >= bounds.today_start AND create_time < bounds.end_at),
 (SELECT count(*) FROM site_visits),
 (SELECT count(*) FROM site_visits,bounds WHERE create_time >= bounds.today_start AND create_time < bounds.end_at),
 (SELECT count(*) FROM signups WHERE source_platform='website'),
 (SELECT count(*) FROM signups,bounds WHERE source_platform='website' AND create_time >= bounds.today_start AND create_time < bounds.end_at),
 (SELECT count(*) FROM wx_users),
 (SELECT count(*) FROM wx_users,bounds WHERE create_time >= bounds.today_start AND create_time < bounds.end_at),
 (SELECT count(*) FROM test_records),
 (SELECT count(*) FROM test_records,bounds WHERE create_time >= bounds.today_start AND create_time < bounds.end_at),
 (SELECT count(*) FROM bookings),
 (SELECT count(*) FROM bookings,bounds WHERE create_time >= bounds.today_start AND create_time < bounds.end_at)
`, start, now.Add(24*time.Hour)).Scan(&out.Website.TotalUsers, &out.Website.NewUsersToday, &out.Website.ActiveUsersToday, &out.Website.TotalPV, &out.Website.TodayPV, &out.Website.TotalSubmissions, &out.Website.SubmissionsToday, &out.Miniapp.TotalUsers, &out.Miniapp.NewUsersToday, &out.Miniapp.TotalTests, &out.Miniapp.TestsToday, &out.Miniapp.TotalBookings, &out.Miniapp.BookingsToday)
	if err != nil {
		return out, err
	}
	if out.Website.TotalUsers > 0 {
		out.Website.ConversionRate = float64(out.Website.TotalSubmissions) / float64(out.Website.TotalUsers) * 100
		out.Website.ConversionRate = float64(int(out.Website.ConversionRate*10+0.5)) / 10
	}
	rows, err := s.db.QueryContext(c, `WITH days AS (SELECT generate_series($1::date,$2::date,'1 day')::date AS series_day), w AS (SELECT (create_time AT TIME ZONE 'Asia/Shanghai')::date AS series_day,count(DISTINCT COALESCE(NULLIF(visitor_id,''),ip||'|'||user_agent)) n FROM site_visits WHERE create_time >= $1::timestamptz AND create_time < $2::timestamptz GROUP BY 1), u AS (SELECT (create_time AT TIME ZONE 'Asia/Shanghai')::date AS series_day,count(*) n FROM wx_users WHERE create_time >= $1::timestamptz AND create_time < $2::timestamptz GROUP BY 1), s AS (SELECT (create_time AT TIME ZONE 'Asia/Shanghai')::date AS series_day,count(*) n FROM signups WHERE source_platform='website' AND create_time >= $1::timestamptz AND create_time < $2::timestamptz GROUP BY 1), t AS (SELECT (create_time AT TIME ZONE 'Asia/Shanghai')::date AS series_day,count(*) n FROM test_records WHERE create_time >= $1::timestamptz AND create_time < $2::timestamptz GROUP BY 1), b AS (SELECT (create_time AT TIME ZONE 'Asia/Shanghai')::date AS series_day,count(*) n FROM bookings WHERE create_time >= $1::timestamptz AND create_time < $2::timestamptz GROUP BY 1) SELECT to_char(days.series_day,'YYYY-MM-DD'),coalesce(w.n,0),coalesce(u.n,0),coalesce(s.n,0),coalesce(t.n,0),coalesce(b.n,0) FROM days LEFT JOIN w USING(series_day) LEFT JOIN u USING(series_day) LEFT JOIN s USING(series_day) LEFT JOIN t USING(series_day) LEFT JOIN b USING(series_day) ORDER BY days.series_day`, start, now.Add(24*time.Hour))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.Series = []PlatformSeriesPoint{}
	for rows.Next() {
		var p PlatformSeriesPoint
		if err := rows.Scan(&p.Date, &p.WebsiteActiveUsers, &p.MiniappNewUsers, &p.WebsiteSubmissions, &p.MiniappTests, &p.MiniappBookings); err != nil {
			return out, err
		}
		out.Series = append(out.Series, p)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	rows, err = s.db.QueryContext(c, `SELECT id::text,coalesce(event_key,''),title,content,target_path,create_time,coalesce(platform,'') FROM messages ORDER BY create_time DESC,id DESC LIMIT 10`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.RecentActivities = []RecentActivity{}
	phoneRE := regexp.MustCompile(`(1\d{2})\d{4}(\d{4})`)
	for rows.Next() {
		var a RecentActivity
		var tm time.Time
		if err := rows.Scan(&a.ID, &a.EventKey, &a.Title, &a.Summary, &a.TargetPath, &tm, &a.Platform); err != nil {
			return out, err
		}
		a.Summary = phoneRE.ReplaceAllString(a.Summary, `${1}****${2}`)
		a.CreateTime = tm.In(loc).Format(time.RFC3339)
		out.RecentActivities = append(out.RecentActivities, a)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
}
