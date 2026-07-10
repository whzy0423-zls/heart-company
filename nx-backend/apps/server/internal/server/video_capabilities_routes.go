package server

import (
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/video"
)

func (s *Server) videoCapabilities(w http.ResponseWriter, r *http.Request) {
	effective := s.effectiveVideoConfig()
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	profile := ""
	if model == "" {
		model = strings.TrimSpace(effective.Model)
		profile = strings.TrimSpace(effective.ModelProfile)
	} else if model == strings.TrimSpace(effective.Model) {
		profile = strings.TrimSpace(effective.ModelProfile)
	}

	httpx.OK(w, video.ResolveCapabilities(video.CapabilityConfig{
		Model:           model,
		ModelProfile:    profile,
		GatewayContract: effective.GatewayContract,
	}))
}
