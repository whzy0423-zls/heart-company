package server

import (
	"net/http"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

func (s *Server) appHealth(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, map[string]any{
		"service":     "nine-xing-app",
		"status":      "ok",
		"version":     s.env.AppVersion,
		"environment": s.env.AppEnv,
		"time":        time.Now().Format("2006/01/02 15:04:05"),
	})
}
