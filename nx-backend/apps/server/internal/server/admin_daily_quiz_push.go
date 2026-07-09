package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/profilecalibration"
)

// appDailyQuizPushAdminService is the admin-facing read model for daily quiz
// push records. It is intentionally separate from the App-facing quiz contract.
type appDailyQuizPushAdminService interface {
	DailyQuizPushStats(ctx context.Context, date string) (profilecalibration.DailyQuizPushStats, error)
	ListDailyQuizPushRecords(ctx context.Context, date string, page, pageSize int) ([]profilecalibration.DailyQuizPushRecord, int, error)
}

func newAppDailyQuizPushAdminService(database *sql.DB) appDailyQuizPushAdminService {
	if database == nil {
		return nil
	}
	return profilecalibration.NewStore(database)
}

func (s *Server) adminDailyQuizPushStats(w http.ResponseWriter, r *http.Request) {
	date, ok := adminDailyQuizPushDateParam(w, r)
	if !ok {
		return
	}
	if s == nil || s.appDailyQuizPushAdmin == nil {
		httpx.Fail(w, http.StatusInternalServerError, "每日题推送服务不可用")
		return
	}
	stats, err := s.appDailyQuizPushAdmin.DailyQuizPushStats(r.Context(), date)
	if err != nil {
		if errors.Is(err, profilecalibration.ErrInvalidInput) {
			httpx.Fail(w, http.StatusBadRequest, "日期格式错误")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}
	stats.Pushed = stats.Pushed || stats.PushedUsers > 0
	httpx.OK(w, stats)
}

func (s *Server) adminDailyQuizPushRecords(w http.ResponseWriter, r *http.Request) {
	date, ok := adminDailyQuizPushDateParam(w, r)
	if !ok {
		return
	}
	if s == nil || s.appDailyQuizPushAdmin == nil {
		httpx.Fail(w, http.StatusInternalServerError, "每日题推送服务不可用")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	items, total, err := s.appDailyQuizPushAdmin.ListDailyQuizPushRecords(r.Context(), date, page, pageSize)
	if err != nil {
		if errors.Is(err, profilecalibration.ErrInvalidInput) {
			httpx.Fail(w, http.StatusBadRequest, "日期格式错误")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}
	httpx.OK(w, map[string]any{
		"items": items,
		"total": total,
	})
}

func adminDailyQuizPushDateParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		return appCalibrationBusinessDate(time.Now()), true
	}
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Format("2006-01-02") != date {
		httpx.Fail(w, http.StatusBadRequest, "日期格式错误")
		return "", false
	}
	return date, true
}
