package appuser

import (
	"database/sql"
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
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// HandleAppUserByID 处理 GET /api/app-users/{id} —— 单个 App 客户详情。
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
	user, err := s.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(w, http.StatusNotFound, "app user not found")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, user)
}
