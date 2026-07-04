package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

func (s *Server) adminAppUserByID(w http.ResponseWriter, r *http.Request) {
	permission := "Customer:App:List"
	if r.Method == http.MethodPut || r.Method == http.MethodPatch {
		permission = "Customer:App:Write"
	}
	s.requirePermission(permission, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut || r.Method == http.MethodPatch {
			s.adminAppUserUpdate(w, r)
			return
		}
		s.appUsers.HandleAppUserByID(w, r)
	})(w, r)
}

func (s *Server) adminAppUserUpdate(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/app-users/"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid id")
		return
	}

	var input appuser.UpdateAdminFieldsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}

	before, beforeErr := s.appUsers.FindByID(r.Context(), id)
	updated, err := s.appUsers.UpdateAdminFields(r.Context(), id, input)
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

	beforeData := any(map[string]any{"lookupError": beforeErrString(beforeErr)})
	if beforeErr == nil {
		beforeData = appUserAuditSnapshot(before)
	}
	s.recordAdminAudit(r, auditlog.Entry{
		Action:     "app_user.update",
		TargetType: "app_user",
		TargetID:   strconv.FormatInt(id, 10),
		Before:     beforeData,
		After:      appUserAuditSnapshot(updated),
		Summary:    "更新 App 客户状态或会员等级",
	})

	httpx.OK(w, updated)
}

func appUserAuditSnapshot(user appuser.User) map[string]any {
	return map[string]any{
		"id":          user.ID,
		"phone":       user.Phone,
		"nickname":    user.Nickname,
		"status":      user.Status,
		"memberLevel": user.MemberLevel,
	}
}

func beforeErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
