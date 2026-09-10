package appuser

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

// queryMap 把 URL 查询参数转成 map（每个 key 取首个值）。
func queryMap(r *http.Request) map[string]string {
	out := map[string]string{}
	for k, vs := range r.URL.Query() {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

// HandleAppUsers 处理 GET /api/app-users/list —— App 客户分页列表（后台只读）。
func (s *Store) HandleAppUsers(w http.ResponseWriter, r *http.Request) {
	result, err := s.List(r.Context(), queryMap(r))
	if err != nil {
		if strings.Contains(err.Error(), "invalid userId") || strings.Contains(err.Error(), "invalid careLevel") {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	for index := range result.Items {
		if care, careErr := s.CareSnapshot(r.Context(), result.Items[index].ID); careErr == nil {
			result.Items[index].CareLevel = care.CareLevel
			result.Items[index].CareLabel = care.CareLabel
			result.Items[index].CareSummary = care.CareSummary
			result.Items[index].CareTrend = care.CareTrend
			result.Items[index].CareDataStatus = care.CareDataStatus
			result.Items[index].CareEvaluatedAt = care.CareEvaluatedAt
		}
	}
	httpx.OK(w, result)
}

// HandleAppUserInsights 处理 GET /api/app-users/insights —— 后台用户提炼数据聚合列表。
func (s *Store) HandleAppUserInsights(w http.ResponseWriter, r *http.Request) {
	result, err := s.ListInsights(r.Context(), queryMap(r))
	if err != nil {
		if strings.Contains(err.Error(), "invalid userId") || strings.Contains(err.Error(), "invalid careLevel") {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	for index := range result.Items {
		s.populateInsightCare(r.Context(), result.Items[index].ID, &result.Items[index])
	}
	httpx.OK(w, result)
}

// HandleAppUserByID 处理 /api/app-users/{id} —— 单个 App 客户详情与后台字段更新。
func (s *Store) HandleAppUserByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/app-users/")
	idStr = strings.TrimSpace(idStr)
	if idStr == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleAppUserDetail(w, r, id)
	case http.MethodPut, http.MethodPatch:
		s.handleAppUserUpdate(w, r, id)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Store) handleAppUserDetail(w http.ResponseWriter, r *http.Request, id int64) {
	user, err := s.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(w, http.StatusNotFound, "app user not found")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if care, careErr := s.CareSnapshot(r.Context(), id); careErr == nil {
		user.CareLevel, user.CareLabel, user.CareSummary, user.CareTrend, user.CareDataStatus, user.CareEvaluatedAt = care.CareLevel, care.CareLabel, care.CareSummary, care.CareTrend, care.CareDataStatus, care.CareEvaluatedAt
	}
	httpx.OK(w, user)
}

func (s *Store) handleAppUserUpdate(w http.ResponseWriter, r *http.Request, id int64) {
	var input UpdateAdminFieldsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, err := s.UpdateAdminFields(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(w, http.StatusNotFound, "app user not found")
			return
		}
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, user)
}
