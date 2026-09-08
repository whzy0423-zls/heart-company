package server

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/directmedia"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

// Keep image/voice uploads lightweight while allowing short phone videos to
// complete without being truncated by the multipart reader.
const directMediaMaxBytes = 50 << 20

func (s *Server) appDirectMedia(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method == http.MethodGet {
		s.appDirectMediaDownload(w, r, user.ID)
		return
	}
	if r.Method != http.MethodPost {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/app/direct/conversations/"), "/")
	if !strings.HasSuffix(path, "/media") {
		httpx.Fail(w, http.StatusNotFound, "direct_media.not_found")
		return
	}
	conversationID, ok := parseDirectPathID(path, "", "/media")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "direct_media.invalid")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, directMediaMaxBytes+(1<<20))
	if err := r.ParseMultipartForm(directMediaMaxBytes); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "direct_media.invalid")
		return
	}
	mediaType := strings.TrimSpace(r.FormValue("mediaType"))
	durationMs, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("durationMs")))
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "direct_media.file_required")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, directMediaMaxBytes+1))
	if err != nil || len(content) == 0 || len(content) > directMediaMaxBytes || !validDirectMedia(mediaType, header, content) {
		httpx.Fail(w, http.StatusBadRequest, "direct_media.invalid")
		return
	}
	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	item, err := s.directMedia.Create(r.Context(), user.ID, conversationID, mediaType, durationMs, uploadasset.CreateInput{
		ContentType: contentType,
		Data:        content,
		Dir:         "app/direct/" + mediaType,
		Name:        header.Filename,
		Size:        int64(len(content)),
	})
	if errors.Is(err, directmedia.ErrNotParticipant) {
		httpx.Fail(w, http.StatusForbidden, err.Error())
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "direct_media.save_failed")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"code": 0, "data": item, "error": nil, "message": "ok"})
}

func (s *Server) appDirectMediaDownload(w http.ResponseWriter, r *http.Request, userID int64) {
	mediaID, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/app/direct/media/"), "/"), 10, 64)
	if err != nil || mediaID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "direct_media.invalid")
		return
	}
	asset, err := s.directMedia.FindAsset(r.Context(), userID, mediaID)
	if errors.Is(err, directmedia.ErrNotParticipant) {
		httpx.Fail(w, http.StatusForbidden, err.Error())
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "direct_media.not_found")
		return
	}
	writeUploadAsset(w, asset)
}

func validDirectMedia(mediaType string, header *multipart.FileHeader, content []byte) bool {
	switch mediaType {
	case "image":
		return isImageUpload(header.Header.Get("Content-Type"), content)
	case "voice":
		return isAllowedASRAudioUpload(header)
	case "video":
		return isAllowedDirectVideoUpload(header, content)
	default:
		return false
	}
}

func isAllowedDirectVideoUpload(header *multipart.FileHeader, content []byte) bool {
	if header == nil {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "video/") {
		return true
	}
	detected := strings.ToLower(http.DetectContentType(content))
	if strings.HasPrefix(detected, "video/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(header.Filename)) {
	case ".mp4", ".mov", ".m4v", ".webm", ".3gp", ".mkv":
		return contentType == "" || contentType == "application/octet-stream" || contentType == "binary/octet-stream"
	default:
		return false
	}
}
