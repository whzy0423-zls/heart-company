package server

import (
	"net/http"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

func (s *Server) appXinzhiliRealtimeCapabilities(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, xinzhili.DefaultRealtimeCapabilities())
}
