package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/teacher"
)

// classroomUploadAuthorizer is intentionally separate from the admin upload
// handler interface. Existing admin fakes and integrations only need the
// multipart operations; teacher-scoped routes additionally bind a task to the
// content id present in the URL.
type classroomUploadAuthorizer interface {
	AuthorizeTask(context.Context, int64, int64, int64) error
}

type teacherUploadPath struct {
	ContentID  int64
	TaskID     int64
	PartNumber int
	Action     string
	OK         bool
}

func parseTeacherUploadPath(path string) teacherUploadPath {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 || parts[1] != "uploads" {
		return teacherUploadPath{}
	}
	contentID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || contentID <= 0 {
		return teacherUploadPath{}
	}
	if len(parts) == 3 && parts[2] == "initiate" {
		return teacherUploadPath{ContentID: contentID, Action: "initiate", OK: true}
	}
	if len(parts) != 4 && len(parts) != 6 {
		return teacherUploadPath{}
	}
	taskID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || taskID <= 0 {
		return teacherUploadPath{}
	}
	if len(parts) == 4 && (parts[3] == "complete" || parts[3] == "progress" || parts[3] == "abort") {
		return teacherUploadPath{ContentID: contentID, TaskID: taskID, Action: parts[3], OK: true}
	}
	if len(parts) == 6 && parts[3] == "parts" && parts[5] == "sign" {
		part, err := strconv.Atoi(parts[4])
		if err == nil && part > 0 {
			return teacherUploadPath{ContentID: contentID, TaskID: taskID, PartNumber: part, Action: "sign", OK: true}
		}
	}
	return teacherUploadPath{}
}

func teacherUploadMethodAllowed(method string) bool { return method == http.MethodPost }

func (s *Server) appTeacherUploadRouter(w http.ResponseWriter, r *http.Request, roles teacher.RoleSet, path string) {
	parsed := parseTeacherUploadPath(path)
	if !parsed.OK {
		httpx.Fail(w, http.StatusNotFound, "not found")
		return
	}
	if !teacherUploadMethodAllowed(r.Method) {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	if s.teachers == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "teacher service unavailable")
		return
	}
	content, err := s.teachers.GetContent(r.Context(), parsed.ContentID)
	if err != nil || content.TeacherKey != roles.TeacherKey {
		httpx.Fail(w, http.StatusNotFound, "content not found")
		return
	}
	if content.ContentType != "" && content.ContentType != "video" {
		httpx.Fail(w, http.StatusBadRequest, "teacher uploads require video content")
		return
	}
	if parsed.Action == "initiate" {
		if s.classroomUploads == nil {
			httpx.Fail(w, http.StatusServiceUnavailable, "classroom uploads unavailable")
			return
		}
		if content.ReviewStatus != teacher.ReviewDraft && content.ReviewStatus != teacher.ReviewRejected {
			httpx.Fail(w, http.StatusConflict, "content is not editable")
			return
		}
		if content.Status != "" && content.Status != "draft" && content.Status != "failed" {
			httpx.Fail(w, http.StatusConflict, "content is not ready for upload")
			return
		}
		s.teacherUploadInitiate(w, r, parsed.ContentID)
		return
	}

	if s.classroomUploads == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "classroom uploads unavailable")
		return
	}
	authorizer, ok := s.classroomUploads.(classroomUploadAuthorizer)
	if !ok {
		httpx.Fail(w, http.StatusServiceUnavailable, "teacher uploads unavailable")
		return
	}
	if err := authorizer.AuthorizeTask(r.Context(), parsed.TaskID, appUserID(r), parsed.ContentID); err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	switch parsed.Action {
	case "sign":
		s.teacherUploadPart(w, r, parsed)
	case "complete":
		s.teacherUploadComplete(w, r, parsed.TaskID)
	case "progress":
		s.teacherUploadProgress(w, r, parsed.TaskID)
	case "abort":
		s.teacherUploadAbort(w, r, parsed.TaskID)
	}
}

func (s *Server) teacherUploadInitiate(w http.ResponseWriter, r *http.Request, contentID int64) {
	var body struct {
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
		SizeBytes   int64  `json:"sizeBytes"`
		Checksum    string `json:"checksum"`
	}
	if !decodeTeacherUploadJSON(w, r, &body, 64<<10) {
		return
	}
	result, err := s.classroomUploads.Initiate(r.Context(), classroom.InitiateUploadInput{
		ContentID: contentID, CreatorID: appUserID(r), Filename: body.Filename,
		ContentType: body.ContentType, SizeBytes: body.SizeBytes, Checksum: body.Checksum,
	})
	if err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	httpx.OK(w, classroomUploadInitiateDTO{Task: toClassroomUploadTaskDTO(result.Task)})
}

func (s *Server) teacherUploadPart(w http.ResponseWriter, r *http.Request, path teacherUploadPath) {
	result, err := s.classroomUploads.SignPart(r.Context(), path.TaskID, appUserID(r), path.PartNumber)
	if err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	httpx.OK(w, classroomSignedPartDTO{URL: result.URL, PartNumber: result.PartNumber, ExpiresAt: result.ExpiresAt})
}

func (s *Server) teacherUploadComplete(w http.ResponseWriter, r *http.Request, taskID int64) {
	var body struct {
		Parts []storage.CompletedPart `json:"parts"`
	}
	if !decodeTeacherUploadJSON(w, r, &body, 2<<20) {
		return
	}
	if len(body.Parts) == 0 || len(body.Parts) > 10000 {
		httpx.Fail(w, http.StatusBadRequest, "invalid parts")
		return
	}
	completionCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Minute)
	defer cancel()
	result, err := s.classroomUploads.Complete(completionCtx, taskID, appUserID(r), body.Parts)
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

func (s *Server) teacherUploadProgress(w http.ResponseWriter, r *http.Request, taskID int64) {
	var body struct {
		CompletedParts int   `json:"completedParts"`
		CompletedBytes int64 `json:"completedBytes"`
	}
	if !decodeTeacherUploadJSON(w, r, &body, 16<<10) {
		return
	}
	result, err := s.classroomUploads.ReportProgress(r.Context(), taskID, appUserID(r), body.CompletedParts, body.CompletedBytes)
	if err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	httpx.OK(w, toClassroomUploadTaskDTO(result))
}

func (s *Server) teacherUploadAbort(w http.ResponseWriter, r *http.Request, taskID int64) {
	result, err := s.classroomUploads.Abort(r.Context(), taskID, appUserID(r))
	if err != nil {
		writeClassroomUploadError(w, err)
		return
	}
	httpx.OK(w, toClassroomUploadTaskDTO(result))
}

func decodeTeacherUploadJSON(w http.ResponseWriter, r *http.Request, value any, maxBytes int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil || ensureJSONEOF(decoder) != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid upload request")
		return false
	}
	return true
}
