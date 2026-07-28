package server

import (
	_ "embed"
	"net/http"
	"time"
)

const classroomAudioCoverPath = "/api/public/classroom/audio-cover.svg"

//go:embed classroom_audio_cover.svg
var classroomAudioCoverSVG []byte

func (s *Server) classroomCoverTTL() time.Duration {
	seconds := s.env.ClassroomMedia.CoverURLTTLSeconds
	if seconds <= 0 || seconds > 86400 {
		seconds = 1800
	}
	return time.Duration(seconds) * time.Second
}

func classroomAudioCover(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(classroomAudioCoverSVG)
}
