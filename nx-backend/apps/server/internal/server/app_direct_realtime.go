package server

import (
	"net/http"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/realtime"
)

func (s *Server) appDirectRealtimeTicket(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	raw, err := realtime.NewRawTicket()
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "realtime.ticket_issue_failed")
		return
	}
	expires, err := s.realtimeTickets.Issue(r.Context(), user.ID, raw)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "realtime.ticket_issue_failed")
		return
	}
	httpx.OK(w, map[string]any{"ticket": raw, "expiresAt": expires.UTC().Format("2006-01-02T15:04:05Z")})
}
