package server

import (
	"context"
	"net/http"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

func (s *Server) appHealth(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	pingContext, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	defer cancel()
	if err := s.db.PingContext(pingContext); err != nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	database := map[string]any{}
	stats := s.db.Stats()
	database = map[string]any{"maxOpen": stats.MaxOpenConnections, "open": stats.OpenConnections, "inUse": stats.InUse, "idle": stats.Idle, "waitCount": stats.WaitCount}
	httpx.OK(w, map[string]any{
		"service":     "nine-xing-app",
		"status":      "ok",
		"version":     s.env.AppVersion,
		"environment": s.env.AppEnv,
		"time":        time.Now().Format("2006/01/02 15:04:05"),
		"metrics":     s.metrics.Snapshot(),
		"database":    database,
	})
}
