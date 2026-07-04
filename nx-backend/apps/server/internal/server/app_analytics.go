package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type appAnalyticsEventInput struct {
	Event  string          `json:"event"`
	Params json.RawMessage `json:"params"`
	TS     json.RawMessage `json:"ts"`
}

type appAnalyticsOverviewResponse struct {
	TotalUsers           int64                        `json:"totalUsers"`
	NewUsersToday        int64                        `json:"newUsersToday"`
	ActiveUsers          int64                        `json:"activeUsers"`
	MemberUsers          int64                        `json:"memberUsers"`
	DisabledUsers        int64                        `json:"disabledUsers"`
	ExtractedUsers       int64                        `json:"extractedUsers"`
	QuizSubmissions      int64                        `json:"quizSubmissions"`
	Cards                int64                        `json:"cards"`
	Memories             int64                        `json:"memories"`
	ChatSessions         int64                        `json:"chatSessions"`
	ChatMessages         int64                        `json:"chatMessages"`
	CompatibilityReports int64                        `json:"compatibilityReports"`
	RecentUsers          []appAnalyticsUserItem       `json:"recentUsers"`
	RecentMemoryUsers    []appAnalyticsMemoryUserItem `json:"recentMemoryUsers"`
	MemberDistribution   map[string]int64             `json:"memberDistribution"`
	StatusDistribution   map[string]int64             `json:"statusDistribution"`
}

type appAnalyticsUserItem struct {
	ID          int64  `json:"id"`
	Phone       string `json:"phone"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	Status      string `json:"status"`
	MemberLevel string `json:"memberLevel"`
	CreateTime  string `json:"createTime"`
}

type appAnalyticsMemoryUserItem struct {
	appAnalyticsUserItem
	LastMemoryAt string `json:"lastMemoryAt"`
	MemoryCount  int64  `json:"memoryCount"`
}

func (s *Server) appAnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	overview := appAnalyticsOverviewResponse{
		RecentUsers:        []appAnalyticsUserItem{},
		RecentMemoryUsers:  []appAnalyticsMemoryUserItem{},
		MemberDistribution: map[string]int64{},
		StatusDistribution: map[string]int64{},
	}

	if err := s.db.QueryRowContext(r.Context(), `
		SELECT
			COUNT(*) AS total_users,
			COUNT(*) FILTER (WHERE create_time >= date_trunc('day', now())) AS new_users_today,
			COUNT(*) FILTER (WHERE status = 'active') AS active_users,
			COUNT(*) FILTER (WHERE member_level <> 'free') AS member_users,
			COUNT(*) FILTER (WHERE status <> 'active') AS disabled_users
		FROM app_users
	`).Scan(&overview.TotalUsers, &overview.NewUsersToday, &overview.ActiveUsers, &overview.MemberUsers, &overview.DisabledUsers); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}

	if err := s.db.QueryRowContext(r.Context(), `
		WITH extracted AS (
			SELECT app_user_id FROM app_user_cards WHERE status = 'active'
			UNION
			SELECT app_user_id FROM app_memories WHERE status = 'active'
			UNION
			SELECT app_user_id FROM app_quiz_submissions
		)
		SELECT COUNT(DISTINCT app_user_id) FROM extracted
	`).Scan(&overview.ExtractedUsers); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}

	counts := []struct {
		query string
		dest  *int64
	}{
		{`SELECT COUNT(*) FROM app_quiz_submissions`, &overview.QuizSubmissions},
		{`SELECT COUNT(*) FROM app_user_cards`, &overview.Cards},
		{`SELECT COUNT(*) FROM app_memories`, &overview.Memories},
		{`SELECT COUNT(*) FROM app_chat_sessions`, &overview.ChatSessions},
		{`SELECT COUNT(*) FROM app_chat_messages`, &overview.ChatMessages},
		{`SELECT COUNT(*) FROM app_compatibility_reports`, &overview.CompatibilityReports},
	}
	for _, item := range counts {
		if err := s.db.QueryRowContext(r.Context(), item.query).Scan(item.dest); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "查询失败")
			return
		}
	}

	var err error
	if overview.MemberDistribution, err = queryAppAnalyticsDistribution(r.Context(), s.db, `
		SELECT member_level, COUNT(*) FROM app_users GROUP BY member_level ORDER BY member_level
	`); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if overview.StatusDistribution, err = queryAppAnalyticsDistribution(r.Context(), s.db, `
		SELECT status, COUNT(*) FROM app_users GROUP BY status ORDER BY status
	`); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if overview.RecentUsers, err = queryRecentAppAnalyticsUsers(r.Context(), s.db); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if overview.RecentMemoryUsers, err = queryRecentAppAnalyticsMemoryUsers(r.Context(), s.db); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}

	httpx.OK(w, overview)
}

func (s *Server) appAnalyticsEvent(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var body appAnalyticsEventInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	event := strings.TrimSpace(body.Event)
	if event == "" {
		httpx.Fail(w, http.StatusBadRequest, "event required")
		return
	}
	if len(event) > 128 {
		event = event[:128]
	}
	params := body.Params
	if len(params) == 0 || strings.TrimSpace(string(params)) == "null" {
		params = json.RawMessage(`{}`)
	}
	clientTS, err := parseAppAnalyticsTS(body.TS)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid ts")
		return
	}

	_, err = s.db.ExecContext(r.Context(),
		`INSERT INTO app_analytics_events (app_user_id, event, params, client_ts, ip, user_agent)
		 VALUES ($1, $2, $3::jsonb, $4, $5, $6)`,
		userInfo.ID,
		event,
		params,
		clientTS,
		s.clientIP(r),
		truncateHeader(r.UserAgent(), 512),
	)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, map[string]bool{"stored": true})
}

func queryAppAnalyticsDistribution(ctx context.Context, db *sql.DB, query string) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var key string
		var count int64
		if err := rows.Scan(&key, &count); err != nil {
			return nil, err
		}
		out[key] = count
	}
	return out, rows.Err()
}

func queryRecentAppAnalyticsUsers(ctx context.Context, db *sql.DB) ([]appAnalyticsUserItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, phone, nickname, avatar, status, member_level, create_time
		FROM app_users
		ORDER BY create_time DESC, id DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []appAnalyticsUserItem{}
	for rows.Next() {
		var item appAnalyticsUserItem
		var createTime time.Time
		if err := rows.Scan(&item.ID, &item.Phone, &item.Nickname, &item.Avatar, &item.Status, &item.MemberLevel, &createTime); err != nil {
			return nil, err
		}
		item.CreateTime = formatAppAnalyticsTime(createTime)
		items = append(items, item)
	}
	return items, rows.Err()
}

func queryRecentAppAnalyticsMemoryUsers(ctx context.Context, db *sql.DB) ([]appAnalyticsMemoryUserItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT u.id, u.phone, u.nickname, u.avatar, u.status, u.member_level,
		       MAX(m.update_time) AS last_memory_at,
		       COUNT(*) AS memory_count
		FROM app_memories m
		JOIN app_users u ON u.id = m.app_user_id
		WHERE m.status = 'active'
		GROUP BY u.id, u.phone, u.nickname, u.avatar, u.status, u.member_level
		ORDER BY last_memory_at DESC, u.id DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []appAnalyticsMemoryUserItem{}
	for rows.Next() {
		var item appAnalyticsMemoryUserItem
		var lastMemoryAt time.Time
		if err := rows.Scan(&item.ID, &item.Phone, &item.Nickname, &item.Avatar, &item.Status, &item.MemberLevel, &lastMemoryAt, &item.MemoryCount); err != nil {
			return nil, err
		}
		item.LastMemoryAt = formatAppAnalyticsTime(lastMemoryAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func formatAppAnalyticsTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

func parseAppAnalyticsTS(raw json.RawMessage) (sql.NullTime, error) {
	var out sql.NullTime
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return out, nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		asString = strings.TrimSpace(asString)
		if asString == "" {
			return out, nil
		}
		t, err := time.Parse(time.RFC3339Nano, asString)
		if err != nil {
			return out, err
		}
		out.Time = t
		out.Valid = true
		return out, nil
	}
	var asNumber float64
	if err := json.Unmarshal(raw, &asNumber); err != nil {
		return out, err
	}
	if math.IsNaN(asNumber) || math.IsInf(asNumber, 0) || asNumber <= 0 {
		return out, nil
	}
	seconds := int64(asNumber)
	nanos := int64((asNumber - float64(seconds)) * 1e9)
	if asNumber > 1e12 {
		seconds = int64(asNumber) / 1000
		nanos = (int64(asNumber) % 1000) * int64(time.Millisecond)
	}
	out.Time = time.Unix(seconds, nanos).UTC()
	out.Valid = true
	return out, nil
}

func truncateHeader(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}
