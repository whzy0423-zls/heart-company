package server

import (
	"errors"
	"net/http"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/quiz"
)

// shanghaiLoc 统一以 Asia/Shanghai 计算"今天"，避免 UTC 跨日误差。
var shanghaiLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}()

// dailyPracticeResp 今日成长练习的响应体。
type dailyPracticeResp struct {
	HasCard      bool   `json:"hasCard"`             // 是否已有主卡（决定是否展示空状态引导）
	MainType     int    `json:"mainType,omitempty"`  // 主型 id
	Practice     string `json:"practice,omitempty"`  // 今日练习
	MindWord     string `json:"mindWord,omitempty"`  // 今日心语
	Question     string `json:"question,omitempty"`  // 今日适合问的问题
	Date         string `json:"date"`                // 今日日期 YYYY-MM-DD
	CheckedIn    bool   `json:"checkedIn"`           // 今日是否已打卡
	CheckinCount int    `json:"checkinCount"`        // 累计打卡天数
}

// appDailyPractice 返回当前用户今日的成长练习内容与打卡状态。
func (s *Server) appDailyPractice(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	now := time.Now().In(shanghaiLoc)
	date := now.Format("2006-01-02")
	resp := dailyPracticeResp{Date: date}

	card, err := s.quiz.PrimaryCard(r.Context(), userInfo.ID)
	if errors.Is(err, quiz.ErrNotFound) {
		// 无主卡：返回空状态，由 App 引导用户先完成测评。
		httpx.OK(w, resp)
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}

	resp.HasCard = true
	resp.MainType = card.MainType

	// 按一年中的第几天轮换当日内容，保证同一天稳定、跨天变化。
	if item, ok := quiz.DailyPracticeOf(card.MainType, now.YearDay()); ok {
		resp.Practice = item.Practice
		resp.MindWord = item.MindWord
		resp.Question = item.Question
	}

	if c, err := s.quiz.GetDailyCheckin(r.Context(), userInfo.ID, date); err == nil && c != nil {
		resp.CheckedIn = true
	}
	if n, err := s.quiz.CountDailyCheckins(r.Context(), userInfo.ID); err == nil {
		resp.CheckinCount = n
	}

	httpx.OK(w, resp)
}

// appDailyCheckin 记录当前用户今日的成长打卡（幂等）。
func (s *Server) appDailyCheckin(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	now := time.Now().In(shanghaiLoc)
	date := now.Format("2006-01-02")

	mainType := 0
	if card, err := s.quiz.PrimaryCard(r.Context(), userInfo.ID); err == nil {
		mainType = card.MainType
	} else if !errors.Is(err, quiz.ErrNotFound) {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}

	if err := s.quiz.UpsertDailyCheckin(r.Context(), userInfo.ID, date, mainType); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "checkin failed")
		return
	}

	count, _ := s.quiz.CountDailyCheckins(r.Context(), userInfo.ID)
	httpx.OK(w, map[string]any{
		"date":         date,
		"checkedIn":    true,
		"checkinCount": count,
	})
}
