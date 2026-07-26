package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/storage"
)

type classroomUploadHandlerService interface {
	Initiate(context.Context, classroom.InitiateUploadInput) (classroom.InitiateUploadResult, error)
	SignPart(context.Context, int64, int64, int) (storage.SignPartResult, error)
	Complete(context.Context, int64, int64, []storage.CompletedPart) (classroom.CompleteUploadResult, error)
	Abort(context.Context, int64, int64) (classroom.UploadTask, error)
}

func registerClassroomUploadRoutes(mux *http.ServeMux, permission func(string, http.HandlerFunc) http.HandlerFunc, initiate, part, complete, abort http.HandlerFunc) {
	mux.HandleFunc("/api/admin/classroom/uploads/initiate", permission("Miniapp:Classroom:Upload", classroomMethod(http.MethodPost, initiate)))
	mux.HandleFunc("/api/admin/classroom/uploads/", permission("Miniapp:Classroom:Upload", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/admin/classroom/uploads/")
		if r.Method != http.MethodPost {
			httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		if strings.HasSuffix(path, "/sign") {
			part(w, r)
			return
		}
		if strings.HasSuffix(path, "/complete") {
			complete(w, r)
			return
		}
		if strings.HasSuffix(path, "/abort") {
			abort(w, r)
			return
		}
		httpx.Fail(w, http.StatusNotFound, "upload route not found")
	}))
}

func classroomMethod(expected string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != expected {
			httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		next(w, r)
	}
}

func (s *Server) classroomUploadInit(w http.ResponseWriter, r *http.Request) {
	if s.classroomUploads == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "classroom uploads unavailable")
		return
	}
	var body struct {
		ContentID   int64  `json:"contentId"`
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
		SizeBytes   int64  `json:"sizeBytes"`
		Checksum    string `json:"checksum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid upload request")
		return
	}
	result, err := s.classroomUploads.Initiate(r.Context(), classroom.InitiateUploadInput{ContentID: body.ContentID, CreatorID: userFromRequest(r).ID, Filename: body.Filename, ContentType: body.ContentType, SizeBytes: body.SizeBytes, Checksum: body.Checksum})
	if err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	httpx.OK(w, result)
}
func (s *Server) classroomUploadPart(w http.ResponseWriter, r *http.Request) {
	id, partNo, ok := parseClassroomUploadPartPath(r.URL.Path)
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid upload part")
		return
	}
	result, err := s.classroomUploads.SignPart(r.Context(), id, userFromRequest(r).ID, partNo)
	if err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	httpx.OK(w, result)
}
func (s *Server) classroomUploadComplete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseClassroomUploadActionPath(r.URL.Path, "complete")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid upload id")
		return
	}
	var body struct {
		Parts []storage.CompletedPart `json:"parts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid parts")
		return
	}
	result, err := s.classroomUploads.Complete(r.Context(), id, userFromRequest(r).ID, body.Parts)
	if err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	httpx.OK(w, result)
}
func (s *Server) classroomUploadAbort(w http.ResponseWriter, r *http.Request) {
	id, ok := parseClassroomUploadActionPath(r.URL.Path, "abort")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid upload id")
		return
	}
	result, err := s.classroomUploads.Abort(r.Context(), id, userFromRequest(r).ID)
	if err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	httpx.OK(w, result)
}

func parseClassroomUploadPartPath(path string) (int64, int, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 6 {
		return 0, 0, false
	}
	id, err := strconv.ParseInt(parts[len(parts)-4], 10, 64)
	if err != nil || id <= 0 {
		return 0, 0, false
	}
	part, err := strconv.Atoi(parts[len(parts)-2])
	return id, part, err == nil && part > 0
}
func parseClassroomUploadActionPath(path, action string) (int64, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 5 || parts[len(parts)-1] != action {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
	return id, err == nil && id > 0
}
func writeClassroomUploadError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, classroom.ErrUploadOwnership):
		status = http.StatusForbidden
	case errors.Is(err, classroom.ErrUploadExpired), errors.Is(err, classroom.ErrUploadAttempts):
		status = http.StatusGone
	case errors.Is(err, classroom.ErrUploadConflict), errors.Is(err, classroom.ErrInvalidUploadPart):
		status = http.StatusConflict
	case errors.Is(err, classroom.ErrNotFound):
		status = http.StatusNotFound
	}
	httpx.Fail(w, status, err.Error())
}
