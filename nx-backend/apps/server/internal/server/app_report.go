package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

// appReportRouter 路由周报详情请求
func (s *Server) appReportRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idText := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/app/reports/"), "/")
	s.appReportDetail(w, r, idText)
}

// appReportList 返回成长周报列表。
// GET /api/app/reports
func (s *Server) appReportList(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// 查询用户的主卡片
	var cardID int64
	err := s.db.QueryRowContext(r.Context(), `
		SELECT id FROM app_user_cards
		WHERE app_user_id = $1 AND card_type = 'primary' AND status = 'active'
		LIMIT 1
	`, userInfo.ID).Scan(&cardID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// 没有主卡片，返回空列表
			httpx.OK(w, []interface{}{})
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}

	// 获取该卡片的会话创建时间范围
	var firstSession sql.NullTime
	err = s.db.QueryRowContext(r.Context(), `
		SELECT MIN(create_time) FROM app_chat_sessions
		WHERE card_id = $1
	`, cardID).Scan(&firstSession)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	if !firstSession.Valid || firstSession.Time.IsZero() {
		// 没有会话记录，返回空列表
		httpx.OK(w, []interface{}{})
		return
	}
	firstSessionTime := firstSession.Time

	// 生成周报列表（从首次会话开始，每周一份）
	type weeklyReport struct {
		ID        int64  `json:"id"`
		WeekLabel string `json:"weekLabel"`
		Summary   string `json:"summary"`
		StartDate string `json:"startDate"`
		EndDate   string `json:"endDate"`
	}

	var reports []weeklyReport
	now := time.Now()

	// 计算第一周的周一
	firstWeekStart := getWeekStart(firstSessionTime)
	weeklyCounts := map[time.Time]int{}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT date_trunc('week', m.create_time)::timestamptz AS week_start, COUNT(*) AS msg_count
		FROM app_chat_messages m
		JOIN app_chat_sessions s ON m.session_id = s.id
		WHERE s.card_id = $1
		  AND m.create_time >= $2
		  AND m.create_time <= $3
		GROUP BY week_start
	`, cardID, firstWeekStart, now)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var weekStart time.Time
		var msgCount int
		if err := rows.Scan(&weekStart, &msgCount); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "query failed")
			return
		}
		weeklyCounts[getWeekStart(weekStart)] = msgCount
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}

	for weekStart, weekID := firstWeekStart, int64(1); weekStart.Before(now); weekStart, weekID = weekStart.AddDate(0, 0, 7), weekID+1 {
		weekEnd := capReportEndDate(weekStart.AddDate(0, 0, 6), now)
		msgCount := weeklyCounts[weekStart]

		if msgCount > 0 {
			reports = append(reports, weeklyReport{
				ID:        weekID,
				WeekLabel: weekStart.Format("2006年第") + getWeekOfYear(weekStart) + "周",
				Summary:   "本周共进行 " + strconv.Itoa(msgCount) + " 次对话",
				StartDate: weekStart.Format("2006-01-02"),
				EndDate:   weekEnd.Format("2006-01-02"),
			})
		}
	}

	// 倒序返回（最新的在前）
	for i, j := 0, len(reports)-1; i < j; i, j = i+1, j-1 {
		reports[i], reports[j] = reports[j], reports[i]
	}

	httpx.OK(w, reports)
}

// appReportDetail 返回指定周报的详情。
// GET /api/app/reports/:id
func (s *Server) appReportDetail(w http.ResponseWriter, r *http.Request, reportIDText string) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	reportID, err := strconv.ParseInt(reportIDText, 10, 64)
	if err != nil || reportID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid report id")
		return
	}

	// 查询用户的主卡片
	var cardID int64
	err = s.db.QueryRowContext(r.Context(), `
		SELECT id FROM app_user_cards
		WHERE app_user_id = $1 AND card_type = 'primary' AND status = 'active'
		LIMIT 1
	`, userInfo.ID).Scan(&cardID)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "card not found")
		return
	}

	// 获取首次会话时间
	var firstSession sql.NullTime
	err = s.db.QueryRowContext(r.Context(), `
		SELECT MIN(create_time) FROM app_chat_sessions
		WHERE card_id = $1
	`, cardID).Scan(&firstSession)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	if !firstSession.Valid || firstSession.Time.IsZero() {
		httpx.Fail(w, http.StatusNotFound, "no sessions found")
		return
	}
	firstSessionTime := firstSession.Time

	// 计算目标周的时间范围
	firstWeekStart := getWeekStart(firstSessionTime)
	targetWeekStart := firstWeekStart.AddDate(0, 0, int((reportID-1)*7))
	now := time.Now()
	targetWeekEnd := capReportEndDate(targetWeekStart.AddDate(0, 0, 6), now)
	if targetWeekStart.After(now) {
		httpx.Fail(w, http.StatusNotFound, "report not found")
		return
	}

	// 查询该周的对话内容
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT m.content, m.role, m.create_time
		FROM app_chat_messages m
		JOIN app_chat_sessions s ON m.session_id = s.id
		WHERE s.card_id = $1
		  AND m.create_time >= $2
		  AND m.create_time < $3
		ORDER BY m.create_time
	`, cardID, targetWeekStart, targetWeekEnd.AddDate(0, 0, 1))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	var userMsgCount, aiMsgCount int
	for rows.Next() {
		var content, role string
		var createTime time.Time
		if err := rows.Scan(&content, &role, &createTime); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "query failed")
			return
		}
		if role == "user" {
			userMsgCount++
		} else {
			aiMsgCount++
		}
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	if userMsgCount+aiMsgCount == 0 {
		httpx.Fail(w, http.StatusNotFound, "report not found")
		return
	}

	type reportDetail struct {
		ID          int64  `json:"id"`
		WeekLabel   string `json:"weekLabel"`
		Summary     string `json:"summary"`
		StartDate   string `json:"startDate"`
		EndDate     string `json:"endDate"`
		Insights    string `json:"insights"`
		Suggestions string `json:"suggestions"`
	}

	detail := reportDetail{
		ID:          reportID,
		WeekLabel:   targetWeekStart.Format("2006年第") + getWeekOfYear(targetWeekStart) + "周",
		Summary:     "本周您共发起 " + strconv.Itoa(userMsgCount) + " 次提问，AI 回复 " + strconv.Itoa(aiMsgCount) + " 次",
		StartDate:   targetWeekStart.Format("2006-01-02"),
		EndDate:     targetWeekEnd.Format("2006-01-02"),
		Insights:    "本周您在九型人格的探索中展现出积极的思考态度。持续的对话有助于深化自我认知。",
		Suggestions: "建议下周尝试更多情境化的提问，例如在具体场景中如何运用九型知识。",
	}

	httpx.OK(w, detail)
}

// getWeekStart 返回给定日期所在周的周一（凌晨）
func getWeekStart(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // 周日视为第7天
	}
	offset := weekday - 1
	monday := t.AddDate(0, 0, -offset)
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location())
}

// getWeekOfYear 返回给定日期是该年的第几周
func getWeekOfYear(t time.Time) string {
	_, week := t.ISOWeek()
	return strconv.Itoa(week)
}

func capReportEndDate(end time.Time, now time.Time) time.Time {
	if end.After(now) {
		return now
	}
	return end
}
