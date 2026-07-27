package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/storage"
)

type classroomUploadHandlerService interface {
	Initiate(context.Context, classroom.InitiateUploadInput) (classroom.InitiateUploadResult, error)
	SignPart(context.Context, int64, int64, int) (storage.SignPartResult, error)
	Complete(context.Context, int64, int64, []storage.CompletedPart) (classroom.CompleteUploadResult, error)
	Abort(context.Context, int64, int64) (classroom.UploadTask, error)
	ReportProgress(context.Context, int64, int64, int, int64) (classroom.UploadTask, error)
}

func registerClassroomUploadRoutes(mux *http.ServeMux, permission func(string, http.HandlerFunc) http.HandlerFunc, initiate, part, complete, abort, progress http.HandlerFunc) {
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
		if strings.HasSuffix(path, "/progress") {
			progress(w, r)
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
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid upload request")
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid upload request")
		return
	}
	result, err := s.classroomUploads.Initiate(r.Context(), classroom.InitiateUploadInput{ContentID: body.ContentID, CreatorID: userFromRequest(r).ID, Filename: body.Filename, ContentType: body.ContentType, SizeBytes: body.SizeBytes, Checksum: body.Checksum})
	if err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	httpx.OK(w, classroomUploadInitiateDTO{Task: toClassroomUploadTaskDTO(result.Task)})
}
func (s *Server) classroomUploadPart(w http.ResponseWriter, r *http.Request) {
	if s.classroomUploads == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "classroom uploads unavailable")
		return
	}
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
	httpx.OK(w, classroomSignedPartDTO{URL: result.URL, PartNumber: result.PartNumber, ExpiresAt: result.ExpiresAt})
}
func (s *Server) classroomUploadComplete(w http.ResponseWriter, r *http.Request) {
	if s.classroomUploads == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "classroom uploads unavailable")
		return
	}
	id, ok := parseClassroomUploadActionPath(r.URL.Path, "complete")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid upload id")
		return
	}
	var body struct {
		Parts []storage.CompletedPart `json:"parts"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid parts")
		return
	}
	if err := ensureJSONEOF(decoder); err != nil || len(body.Parts) == 0 || len(body.Parts) > 10000 {
		httpx.Fail(w, http.StatusBadRequest, "invalid parts")
		return
	}
	result, err := s.classroomUploads.Complete(r.Context(), id, userFromRequest(r).ID, body.Parts)
	if err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	httpx.OK(w, classroomUploadCompleteDTO{
		Task:    toClassroomUploadTaskDTO(result.Task),
		Media:   classroomUploadMediaDTO{ID: result.Media.ID, ContentType: result.Media.ContentType, SizeBytes: result.Media.SizeBytes, DurationSeconds: result.Media.DurationSeconds, Width: result.Media.Width, Height: result.Media.Height, Status: result.Media.StorageStatus},
		Content: classroomUploadContentDTO{ID: result.Content.ID, Status: result.Content.Status, MediaAssetID: result.Content.MediaAssetID, DurationSeconds: result.Content.DurationSeconds},
	})
}
func (s *Server) classroomUploadAbort(w http.ResponseWriter, r *http.Request) {
	if s.classroomUploads == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "classroom uploads unavailable")
		return
	}
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
	httpx.OK(w, toClassroomUploadTaskDTO(result))
}

func (s *Server) classroomUploadProgress(w http.ResponseWriter, r *http.Request) {
	if s.classroomUploads == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "classroom uploads unavailable")
		return
	}
	id, ok := parseClassroomUploadActionPath(r.URL.Path, "progress")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid upload id")
		return
	}
	var body struct {
		CompletedParts int   `json:"completedParts"`
		CompletedBytes int64 `json:"completedBytes"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || ensureJSONEOF(decoder) != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid upload progress")
		return
	}
	result, err := s.classroomUploads.ReportProgress(r.Context(), id, userFromRequest(r).ID, body.CompletedParts, body.CompletedBytes)
	if err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	httpx.OK(w, toClassroomUploadTaskDTO(result))
}

type classroomUploadInitiateDTO struct {
	Task classroomUploadTaskDTO `json:"task"`
}
type classroomSignedPartDTO struct {
	URL        string    `json:"url"`
	PartNumber int       `json:"partNumber"`
	ExpiresAt  time.Time `json:"expiresAt"`
}
type classroomUploadCompleteDTO struct {
	Task    classroomUploadTaskDTO    `json:"task"`
	Media   classroomUploadMediaDTO   `json:"media"`
	Content classroomUploadContentDTO `json:"content"`
}
type classroomUploadMediaDTO struct {
	ID              int64                 `json:"id"`
	ContentType     classroom.ContentType `json:"contentType"`
	SizeBytes       int64                 `json:"sizeBytes"`
	DurationSeconds int                   `json:"durationSeconds"`
	Width           int                   `json:"width"`
	Height          int                   `json:"height"`
	Status          classroom.MediaStatus `json:"status"`
}
type classroomUploadContentDTO struct {
	ID              int64                   `json:"id"`
	Status          classroom.ContentStatus `json:"status"`
	MediaAssetID    *int64                  `json:"mediaAssetId,omitempty"`
	DurationSeconds int                     `json:"durationSeconds"`
}

func toClassroomUploadTaskDTO(v classroom.UploadTask) classroomUploadTaskDTO {
	if v.Status == classroom.UploadCompleted {
		v.CompletedParts = v.MaxParts
		v.CompletedBytes = v.ExpectedSize
	}
	progress := float64(0)
	if v.ExpectedSize > 0 {
		progress = float64(v.CompletedBytes) * 100 / float64(v.ExpectedSize)
	}
	if progress > 100 {
		progress = 100
	}
	return classroomUploadTaskDTO{ID: v.ID, ContentID: v.ContentID, OriginalFilename: v.OriginalFilename, ExpectedSize: v.ExpectedSize, ExpectedChecksum: v.Checksum, CompletedParts: v.CompletedParts, CompletedBytes: v.CompletedBytes, TotalBytes: v.ExpectedSize, ProgressPercent: progress, PartSize: v.PartSize, MaxParts: v.MaxParts, Status: v.Status, ExpiresAt: v.ExpiresAt, AttemptCount: v.AttemptCount, CleanupStatus: v.CleanupStatus, MediaAssetID: v.MediaAssetID, FailureReason: v.FailureReason, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
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
	message := "classroom upload failed"
	switch {
	case errors.Is(err, classroom.ErrUploadInProgress):
		status = http.StatusAccepted
		message = "upload in progress"
	case errors.Is(err, classroom.ErrUploadOwnership):
		status = http.StatusForbidden
		message = "forbidden"
	case errors.Is(err, classroom.ErrUploadExpired), errors.Is(err, classroom.ErrUploadAttempts):
		status = http.StatusGone
		message = "upload unavailable"
	case errors.Is(err, classroom.ErrUploadConflict), errors.Is(err, classroom.ErrInvalidUploadPart):
		status = http.StatusConflict
		message = "upload conflict"
	case errors.Is(err, classroom.ErrInvalidUploadProgress):
		status = http.StatusConflict
		message = "invalid upload progress"
	case errors.Is(err, classroom.ErrNotFound):
		status = http.StatusNotFound
		message = "upload not found"
	}
	httpx.Fail(w, status, message)
}
func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("trailing json")
	}
	return err
}

type classroomUploadMaintenance interface {
	CleanupPending(context.Context, int) (int, error)
}

func startClassroomUploadMaintenance(ctx context.Context, svc classroomUploadMaintenance, interval time.Duration) {
	if svc == nil {
		return
	}
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	run := func() {
		if _, err := svc.CleanupPending(ctx, 100); err != nil {
			log.Printf("classroom upload cleanup: %v", err)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
