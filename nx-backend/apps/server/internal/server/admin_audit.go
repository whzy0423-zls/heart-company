package server

import (
	"net/http"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

func (s *Server) adminAuditLogs(w http.ResponseWriter, r *http.Request) {
	result, err := s.auditLogs.List(r.Context(), queryMap(r))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}
