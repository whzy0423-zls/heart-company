package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestAppClassroomContentAliasRequiresAppAuth(t *testing.T) {
	s := &Server{env: config.Env{JWTSecret: "test"}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/app/classroom/content/", s.method(http.MethodGet, s.requireAppAuth(s.classroomAppContent)))
	req := httptest.NewRequest(http.MethodGet, "/api/app/classroom/content/12", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAppClassroomPlaybackAliasRequiresAppAuth(t *testing.T) {
	s := &Server{env: config.Env{JWTSecret: "test"}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/app/classroom/content/", s.requireAppAuth(s.classroomAppContentRouter))
	req := httptest.NewRequest(http.MethodPost, "/api/app/classroom/content/12/play", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
