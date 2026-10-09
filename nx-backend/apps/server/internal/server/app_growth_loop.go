package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/growthinsight"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type growthInsightStore interface {
	ConsentMetadata(context.Context, int64) (growthinsight.ConsentState, error)
	SetConsent(context.Context, int64, bool) (bool, error)
	View(context.Context, int64, int64) (growthinsight.UserView, error)
	FeedbackAction(context.Context, int64, int64, growthinsight.Feedback) (growthinsight.Action, error)
	Correct(context.Context, int64, int64, string, string) error
	AdminList(context.Context, int64) (growthinsight.AdminList, error)
	Report(context.Context, int64) (growthinsight.Report, error)
	Enqueue(context.Context, int64) error
}

func (s *Server) appGrowthLoop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if s.growthInsights == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "成长分析暂未开放")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/app/growth-loop")
	switch {
	case path == "/consent":
		if r.Method == http.MethodGet {
			state, err := s.growthInsights.ConsentMetadata(r.Context(), user.ID)
			if err != nil {
				growthLoopError(w, err)
				return
			}
			httpx.OK(w, state)
			return
		}
		if r.Method != http.MethodPut {
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if !readGrowthJSON(w, r, &body) {
			return
		}
		if body.Enabled == nil {
			httpx.Fail(w, http.StatusBadRequest, "请选择是否开启")
			return
		}
		enabled, err := s.growthInsights.SetConsent(r.Context(), user.ID, *body.Enabled)
		if err != nil {
			growthLoopError(w, err)
			return
		}
		httpx.OK(w, map[string]bool{"enabled": enabled})
	case path == "":
		if r.Method != http.MethodGet {
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		cardID, err := strconv.ParseInt(r.URL.Query().Get("cardId"), 10, 64)
		if err != nil || cardID <= 0 {
			httpx.Fail(w, http.StatusBadRequest, "invalid card id")
			return
		}
		access, err := s.portraitTrendMembershipResourceMetadata(r.Context(), user.ID, cardID, "请升级 VIP 后查看成长分析")
		if err != nil {
			httpx.Fail(w, http.StatusServiceUnavailable, "会员状态查询失败，请稍后重试")
			return
		}
		if membershipContentLocked(access) {
			httpx.Fail(w, http.StatusForbidden, "请升级 VIP 后查看成长分析")
			return
		}
		view, err := s.growthInsights.View(r.Context(), user.ID, cardID)
		if err != nil {
			growthLoopError(w, err)
			return
		}
		httpx.OK(w, view)
	case path == "/feedback":
		if r.Method != http.MethodPost {
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body struct {
			ReportID int64  `json:"reportId"`
			Kind     string `json:"kind"`
			Note     string `json:"note"`
		}
		if !readGrowthJSON(w, r, &body) {
			return
		}
		body.Note = strings.TrimSpace(body.Note)
		if body.ReportID <= 0 || body.Kind != "inaccurate" || body.Note == "" || utf8.RuneCountInString(body.Note) > 500 {
			httpx.Fail(w, http.StatusBadRequest, "请填写 1 至 500 字的纠正内容")
			return
		}
		if err := s.growthInsights.Correct(r.Context(), user.ID, body.ReportID, body.Kind, body.Note); err != nil {
			growthLoopError(w, err)
			return
		}
		httpx.OK(w, map[string]bool{"saved": true})
	case strings.HasPrefix(path, "/actions/") && strings.HasSuffix(path, "/feedback"):
		if r.Method != http.MethodPut {
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		id, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(path, "/actions/"), "/feedback"), 10, 64)
		if err != nil || id <= 0 {
			httpx.Fail(w, http.StatusBadRequest, "invalid action id")
			return
		}
		var body growthinsight.Feedback
		if !readGrowthJSON(w, r, &body) {
			return
		}
		plan, err := s.currentAppMembershipPlanWithError(r.Context(), user.ID)
		if err != nil {
			httpx.Fail(w, http.StatusServiceUnavailable, "会员状态查询失败，请稍后重试")
			return
		}
		if membershipLevelRank(plan.PlanLevel) < membershipLevelRank("vip") {
			httpx.Fail(w, http.StatusForbidden, "请升级 VIP 后记录成长行动")
			return
		}
		action, err := s.growthInsights.FeedbackAction(r.Context(), user.ID, id, body)
		if err != nil {
			growthLoopError(w, err)
			return
		}
		httpx.OK(w, action)
	default:
		httpx.Fail(w, http.StatusNotFound, "not found")
	}
}

func readGrowthJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func growthLoopError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, growthinsight.ErrNotFound):
		httpx.Fail(w, http.StatusNotFound, "记录不存在或已删除")
	case errors.Is(err, growthinsight.ErrDisabled):
		httpx.Fail(w, http.StatusConflict, "用户尚未开启个性化成长分析")
	case errors.Is(err, growthinsight.ErrInvalid):
		httpx.Fail(w, http.StatusBadRequest, "请检查提交的内容")
	case errors.Is(err, growthinsight.ErrStale):
		httpx.Fail(w, http.StatusConflict, "资料已更新，请刷新后重试")
	default:
		httpx.Fail(w, http.StatusServiceUnavailable, "成长分析暂时不可用，请稍后重试")
	}
}

func (s *Server) growthViewForMember(ctx context.Context, userID, cardID int64) (growthinsight.UserView, bool) {
	if s.growthInsights == nil {
		return growthinsight.UserView{}, false
	}
	access, err := s.portraitTrendMembershipResourceMetadata(ctx, userID, cardID, "")
	if err != nil || membershipContentLocked(access) {
		return growthinsight.UserView{}, false
	}
	v, err := s.growthInsights.View(ctx, userID, cardID)
	return v, err == nil && v.Enabled && v.Status == "ready" && v.CardID == cardID
}

// Keep the baseline snapshot separate so consent withdrawal cannot leave a
// personalized payload cached for another seven days.
func applyGrowthPortrait(p *portraitResp, v growthinsight.UserView) {
	if p == nil || !v.Enabled || v.Status != "ready" || v.Summary == "" {
		return
	}
	p.HasEnoughData = true
	p.Summary, p.StateLabel = v.Summary, "近期成长观察"
	p.Strengths, p.StressPoints, p.GrowthAdvice, p.AwarenessPrompts = v.Strengths, v.StressPoints, v.GrowthAdvice, v.AwarenessPrompts
	p.PotentialDirections, p.SupportResources, p.PositivePatterns = nil, nil, nil
	p.StressSolutions, p.EmotionSupport, p.RelationshipPatterns, p.GuidingQuestions = nil, nil, nil, nil
	if at, err := time.Parse(time.RFC3339, v.GeneratedAt); err == nil {
		p.UpdatedAt = portraitTimestamp(at)
	}
	if at, err := time.Parse(time.RFC3339, v.NextUpdateAt); err == nil {
		p.NextUpdateAt = portraitTimestamp(at)
	}
}

func growthWeeklyReview(v growthinsight.UserView, weekStart time.Time) string {
	if !v.Enabled || v.Status != "ready" || v.Timezone != "Asia/Shanghai" {
		return ""
	}
	start, err := time.Parse(time.RFC3339, v.PeriodStart)
	if err != nil {
		return ""
	}
	end, err := time.Parse(time.RFC3339, v.PeriodEnd)
	if err != nil || !end.After(start) || end.After(start.Add(7*24*time.Hour)) {
		return ""
	}
	if start.In(shanghaiLoc).Format("2006-01-02") != weekStart.Format("2006-01-02") {
		return ""
	}
	return v.WeeklyReview
}
