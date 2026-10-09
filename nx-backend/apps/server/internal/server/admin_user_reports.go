package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/growthinsight"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

func (s *Server) adminUserReports(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if s.growthInsights == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "成长分析暂未开放")
		return
	}
	idText := strings.TrimPrefix(r.URL.Path, "/api/app-user-reports")
	if idText != "" {
		id, err := strconv.ParseInt(strings.TrimPrefix(idText, "/"), 10, 64)
		if err != nil || id <= 0 {
			httpx.Fail(w, http.StatusBadRequest, "invalid report id")
			return
		}
		report, err := s.growthInsights.Report(r.Context(), id)
		if err != nil {
			growthLoopError(w, err)
			return
		}
		s.recordAdminAudit(r, auditlog.Entry{Action: "growth_report.read", TargetType: "growth_report", TargetID: strconv.FormatInt(id, 10), Summary: "查看用户成长分析报告"})
		httpx.OK(w, report)
		return
	}
	if r.Method == http.MethodGet {
		userID, err := strconv.ParseInt(r.URL.Query().Get("appUserId"), 10, 64)
		if err != nil || userID <= 0 {
			httpx.Fail(w, http.StatusBadRequest, "invalid user id")
			return
		}
		list, err := s.growthInsights.AdminList(r.Context(), userID)
		if err != nil {
			growthLoopError(w, err)
			return
		}
		httpx.OK(w, list)
		return
	}
	if r.Method != http.MethodPost {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		AppUserID int64 `json:"appUserId"`
	}
	if !readGrowthJSON(w, r, &body) {
		return
	}
	if body.AppUserID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := s.growthInsights.Enqueue(r.Context(), body.AppUserID); err != nil {
		if errors.Is(err, growthinsight.ErrNoNewEvidence) {
			httpx.OK(w, map[string]any{"queued": false, "reason": "no_new_evidence"})
			return
		}
		growthLoopError(w, err)
		return
	}
	s.recordAdminAudit(r, auditlog.Entry{Action: "growth_report.enqueue", TargetType: "app_user", TargetID: strconv.FormatInt(body.AppUserID, 10), Summary: "请求用户成长分析"})
	httpx.OK(w, map[string]bool{"queued": true})
}
