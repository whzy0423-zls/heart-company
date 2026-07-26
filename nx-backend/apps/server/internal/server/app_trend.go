package server

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type appTrendPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

type appTrendSeries struct {
	Label     string          `json:"label"`
	Dimension string          `json:"dimension"`
	Points    []appTrendPoint `json:"points"`
}

type appTrendDaySignals struct {
	UserMessages      int
	AssistantMessages int
	UserChars         int
	StressHits        int
	EnergyHits        int
	RelationshipHits  int
	AwarenessHits     int
	FavoriteHelpful   int
	Checkins          int
	Memories          int
}

// appCardTrend 返回指定卡片在某时间范围内的状态趋势数据。
// GET /api/app/cards/:id/trend?days=7
func (s *Server) appCardTrend(w http.ResponseWriter, r *http.Request, userID int64, idText string) {
	if r.Method != http.MethodGet {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cardID, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || cardID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid card id")
		return
	}

	daysStr := r.URL.Query().Get("days")
	days := 7
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && (d == 7 || d == 30) {
			days = d
		}
	}

	// 验证卡片归属
	var ownerID int64
	err = s.db.QueryRowContext(r.Context(),
		`SELECT app_user_id FROM app_user_cards WHERE id = $1 AND status = 'active'`,
		cardID,
	).Scan(&ownerID)
	if err == sql.ErrNoRows {
		httpx.Fail(w, http.StatusNotFound, "card not found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	if ownerID != userID {
		httpx.Fail(w, http.StatusForbidden, "access denied")
		return
	}

	// 计算自然日范围：days=7 返回今天及之前 6 天，共 7 个点。
	now := time.Now().In(appTrendLocation())
	endExclusive := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	startTime := endExclusive.AddDate(0, 0, -days)

	signals, err := s.appTrendSignals(r.Context(), userID, cardID, startTime, endExclusive)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}

	httpx.OK(w, buildAppTrendSeries(startTime, endExclusive, signals))
}

func appTrendLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return location
}

func buildAppTrendSeries(startTime, endExclusive time.Time, signals map[string]appTrendDaySignals) []appTrendSeries {
	dimensions := []struct {
		Label     string
		Dimension string
		Baseline  float64
	}{
		{"压力", "stress", 48},
		{"活力", "energy", 54},
		{"人际", "relationship", 52},
		{"觉察", "awareness", 50},
	}

	var result []appTrendSeries
	for _, dim := range dimensions {
		var points []appTrendPoint
		for d := startTime; d.Before(endExclusive); d = d.AddDate(0, 0, 1) {
			date := d.Format("2006-01-02")
			points = append(points, appTrendPoint{
				Date:  date,
				Value: scoreAppTrendDimension(dim.Dimension, dim.Baseline, signals[date]),
			})
		}

		result = append(result, appTrendSeries{
			Label:     dim.Label,
			Dimension: dim.Dimension,
			Points:    points,
		})
	}
	return result
}

func scoreAppTrendDimension(dimension string, baseline float64, signals appTrendDaySignals) float64 {
	value := baseline
	switch dimension {
	case "stress":
		value += float64(signals.StressHits * 8)
		value -= float64(signals.EnergyHits * 3)
		value -= float64(signals.Checkins * 4)
		value -= float64(signals.FavoriteHelpful * 2)
		value += float64(minInt(signals.UserMessages, 4))
	case "energy":
		value += float64(signals.Checkins * 10)
		value += float64(signals.EnergyHits * 5)
		value += float64(minInt(signals.UserMessages, 3) * 2)
		value -= float64(signals.StressHits * 4)
	case "relationship":
		value += float64(signals.RelationshipHits * 7)
		value += float64(minInt(signals.AssistantMessages, 3) * 2)
		value += float64(signals.FavoriteHelpful * 4)
	case "awareness":
		value += float64(signals.Checkins * 8)
		value += float64(signals.AwarenessHits * 6)
		value += float64(signals.Memories * 5)
		value += float64(signals.FavoriteHelpful * 4)
		value += float64(minInt(signals.UserChars/80, 5) * 2)
	}
	return clampAppTrendScore(value)
}

func (s *Server) appTrendSignals(ctx context.Context, userID, cardID int64, startTime, endExclusive time.Time) (map[string]appTrendDaySignals, error) {
	signals := make(map[string]appTrendDaySignals)
	if err := s.addAppTrendMessageSignals(ctx, signals, userID, cardID, startTime, endExclusive); err != nil {
		return nil, err
	}
	if err := s.addAppTrendCheckinSignals(ctx, signals, userID, startTime, endExclusive); err != nil {
		return nil, err
	}
	if err := s.addAppTrendMemorySignals(ctx, signals, userID, cardID, startTime, endExclusive); err != nil {
		return nil, err
	}
	return signals, nil
}

func (s *Server) addAppTrendMessageSignals(ctx context.Context, signals map[string]appTrendDaySignals, userID, cardID int64, startTime, endExclusive time.Time) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.role, m.content, m.favorite, m.feedback, m.create_time
		FROM app_chat_sessions s
		JOIN app_chat_messages m ON m.session_id = s.id
		WHERE s.app_user_id = $1
		  AND s.card_id = $2
		  AND s.scene = 'chat'
		  AND m.create_time >= $3
		  AND m.create_time < $4
		ORDER BY m.create_time
	`, userID, cardID, startTime, endExclusive)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var role, content, feedback string
		var favorite bool
		var createTime time.Time
		if err := rows.Scan(&role, &content, &favorite, &feedback, &createTime); err != nil {
			return err
		}
		date := createTime.In(startTime.Location()).Format("2006-01-02")
		signal := signals[date]
		normalizedRole := strings.TrimSpace(strings.ToLower(role))
		if normalizedRole == "user" {
			signal.UserMessages++
			signal.UserChars += len([]rune(content))
			signal.StressHits += countAppTrendKeywordHits(content, appTrendStressKeywords)
			signal.EnergyHits += countAppTrendKeywordHits(content, appTrendEnergyKeywords)
			signal.RelationshipHits += countAppTrendKeywordHits(content, appTrendRelationshipKeywords)
			signal.AwarenessHits += countAppTrendKeywordHits(content, appTrendAwarenessKeywords)
		}
		if normalizedRole == "assistant" {
			signal.AssistantMessages++
		}
		if favorite || strings.TrimSpace(strings.ToLower(feedback)) == "helpful" {
			signal.FavoriteHelpful++
		}
		signals[date] = signal
	}
	return rows.Err()
}

func (s *Server) addAppTrendCheckinSignals(ctx context.Context, signals map[string]appTrendDaySignals, userID int64, startTime, endExclusive time.Time) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT checkin_date
		FROM app_daily_checkins
		WHERE app_user_id = $1
		  AND checkin_date >= $2::date
		  AND checkin_date < $3::date
	`, userID, startTime, endExclusive)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var checkinDate time.Time
		if err := rows.Scan(&checkinDate); err != nil {
			return err
		}
		date := checkinDate.In(startTime.Location()).Format("2006-01-02")
		signal := signals[date]
		signal.Checkins++
		signals[date] = signal
	}
	return rows.Err()
}

func (s *Server) addAppTrendMemorySignals(ctx context.Context, signals map[string]appTrendDaySignals, userID, cardID int64, startTime, endExclusive time.Time) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(source_time, create_time)
		FROM app_memories
		WHERE app_user_id = $1
		  AND card_id = $2
		  AND status = 'active'
		  AND COALESCE(source_time, create_time) >= $3
		  AND COALESCE(source_time, create_time) < $4
	`, userID, cardID, startTime, endExclusive)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var sourceTime time.Time
		if err := rows.Scan(&sourceTime); err != nil {
			return err
		}
		date := sourceTime.In(startTime.Location()).Format("2006-01-02")
		signal := signals[date]
		signal.Memories++
		signals[date] = signal
	}
	return rows.Err()
}

var appTrendStressKeywords = []string{
	"压力", "焦虑", "烦", "累", "崩溃", "失眠", "害怕", "担心", "生气", "难受", "痛苦", "抑郁", "内耗", "委屈",
}

var appTrendEnergyKeywords = []string{
	"完成", "行动", "计划", "轻松", "开心", "稳定", "可以", "进步", "感谢", "有力量", "清晰", "睡得好",
}

var appTrendRelationshipKeywords = []string{
	"关系", "伴侣", "朋友", "同事", "妈妈", "爸爸", "孩子", "沟通", "冲突", "亲密", "边界", "家人",
}

var appTrendAwarenessKeywords = []string{
	"觉察", "模式", "感受", "需求", "边界", "价值", "成长", "反思", "原因", "触发", "情绪",
}

func countAppTrendKeywordHits(content string, keywords []string) int {
	normalized := strings.ToLower(content)
	hits := 0
	for _, keyword := range keywords {
		if strings.Contains(normalized, keyword) {
			hits++
		}
	}
	return hits
}

func clampAppTrendScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
