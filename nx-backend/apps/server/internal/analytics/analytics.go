package analytics

import (
	"context"
	"database/sql"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Store struct {
	db *sql.DB
}

type VisitInput struct {
	Path      string `json:"path"`
	Referrer  string `json:"referrer"`
	Title     string `json:"title"`
	VisitorID string `json:"visitorId"`
}

type Overview struct {
	RangeLeads  int           `json:"rangeLeads"`
	RangeVisits int           `json:"rangeVisits"`
	Series      []SeriesPoint `json:"series"`
	TodayLeads  int           `json:"todayLeads"`
	TodayVisits int           `json:"todayVisits"`
	TotalLeads  int           `json:"totalLeads"`
	TotalVisits int           `json:"totalVisits"`
}

type SeriesPoint struct {
	Date   string `json:"date"`
	Leads  int    `json:"leads"`
	Visits int    `json:"visits"`
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

func (s *Store) TrackVisit(ctx context.Context, input VisitInput, r *http.Request) error {
	path := truncate(strings.TrimSpace(input.Path), 512)
	if path == "" {
		path = "/"
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(c,
		`INSERT INTO site_visits (visitor_id, path, title, referrer, ip, user_agent)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		truncate(strings.TrimSpace(input.VisitorID), 128),
		path,
		truncate(strings.TrimSpace(input.Title), 256),
		truncate(strings.TrimSpace(input.Referrer), 1024),
		clientIP(r),
		truncate(strings.TrimSpace(r.UserAgent()), 512),
	)
	return err
}

func (s *Store) Overview(ctx context.Context, values url.Values) (Overview, error) {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start, end := dateRange(values)
	var result Overview
	err := s.db.QueryRowContext(c, `
		WITH boundary AS (
			SELECT (date_trunc('day', now() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai') AS today_start
		)
		SELECT
			(SELECT count(DISTINCT COALESCE(NULLIF(visitor_id, ''), ip || '|' || user_agent)) FROM site_visits),
			(SELECT count(DISTINCT COALESCE(NULLIF(visitor_id, ''), ip || '|' || user_agent)) FROM site_visits, boundary WHERE create_time >= boundary.today_start),
			(SELECT count(*) FROM signups),
			(SELECT count(*) FROM signups, boundary WHERE create_time >= boundary.today_start)
	`).Scan(
		&result.TotalVisits,
		&result.TodayVisits,
		&result.TotalLeads,
		&result.TodayLeads,
	)
	if err != nil {
		return result, err
	}

	result.Series, err = s.series(c, start, end)
	if err != nil {
		return result, err
	}
	result.RangeVisits, result.RangeLeads, err = s.rangeTotals(c, start, end)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) rangeTotals(ctx context.Context, start time.Time, end time.Time) (int, int, error) {
	var visits int
	var leads int
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(DISTINCT COALESCE(NULLIF(visitor_id, ''), ip || '|' || user_agent))
			 FROM site_visits
			 WHERE create_time >= ($1::date AT TIME ZONE 'Asia/Shanghai')
			   AND create_time < (($2::date + 1) AT TIME ZONE 'Asia/Shanghai')),
			(SELECT count(*)
			 FROM signups
			 WHERE create_time >= ($1::date AT TIME ZONE 'Asia/Shanghai')
			   AND create_time < (($2::date + 1) AT TIME ZONE 'Asia/Shanghai'))
	`, start.Format("2006-01-02"), end.Format("2006-01-02")).Scan(&visits, &leads)
	return visits, leads, err
}

func (s *Store) series(ctx context.Context, start time.Time, end time.Time) ([]SeriesPoint, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH days AS (
			SELECT generate_series($1::date, $2::date, interval '1 day')::date AS day
		),
		visit_daily AS (
			SELECT
				(create_time AT TIME ZONE 'Asia/Shanghai')::date AS day,
				count(DISTINCT COALESCE(NULLIF(visitor_id, ''), ip || '|' || user_agent)) AS visits
			FROM site_visits
			WHERE create_time >= ($1::date AT TIME ZONE 'Asia/Shanghai')
			  AND create_time < (($2::date + 1) AT TIME ZONE 'Asia/Shanghai')
			GROUP BY 1
		),
		lead_daily AS (
			SELECT
				(create_time AT TIME ZONE 'Asia/Shanghai')::date AS day,
				count(*) AS leads
			FROM signups
			WHERE create_time >= ($1::date AT TIME ZONE 'Asia/Shanghai')
			  AND create_time < (($2::date + 1) AT TIME ZONE 'Asia/Shanghai')
			GROUP BY 1
		)
		SELECT
			to_char(days.day, 'YYYY-MM-DD'),
			COALESCE(visit_daily.visits, 0),
			COALESCE(lead_daily.leads, 0)
		FROM days
		LEFT JOIN visit_daily ON visit_daily.day = days.day
		LEFT JOIN lead_daily ON lead_daily.day = days.day
		ORDER BY days.day ASC
	`, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []SeriesPoint{}
	for rows.Next() {
		var item SeriesPoint
		if err := rows.Scan(&item.Date, &item.Visits, &item.Leads); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func dateRange(values url.Values) (time.Time, time.Time) {
	location, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(location)
	end := parseDate(values.Get("endDate"), location, now)
	start := parseDate(values.Get("startDate"), location, end.AddDate(0, 0, -6))
	if start.After(end) {
		start = end
	}
	if end.Sub(start) > 89*24*time.Hour {
		start = end.AddDate(0, 0, -89)
	}
	return start, end
}

func parseDate(value string, location *time.Location, fallback time.Time) time.Time {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), location)
	if err != nil {
		return time.Date(fallback.Year(), fallback.Month(), fallback.Day(), 0, 0, 0, 0, location)
	}
	return t
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func clientIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		parts := strings.Split(value, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
