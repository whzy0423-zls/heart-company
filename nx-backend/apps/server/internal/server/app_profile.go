package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

const appAvatarMaxBytes = 5 << 20

func (s *Server) appProfileUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input appuser.UpdateSelfProfileInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	updated, err := s.appUsers.UpdateSelfProfile(r.Context(), user.ID, input)
	if err != nil {
		status := http.StatusInternalServerError
		message := err.Error()
		switch {
		case errors.Is(err, appuser.ErrInvalidNickname), strings.Contains(message, "required"), strings.Contains(message, "invalid avatar"):
			status = http.StatusBadRequest
		case errors.Is(err, sql.ErrNoRows):
			status = http.StatusNotFound
		}
		httpx.Fail(w, status, message)
		return
	}
	httpx.OK(w, updated)
}

// appProfileAvatarUpload stores an image through the App-authenticated path.
// The generic /api/upload endpoint intentionally remains backend-admin-only.
func (s *Server) appProfileAvatarUpload(w http.ResponseWriter, r *http.Request) {
	uploader, err := s.objectUploader()
	if err != nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "avatar storage unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, appAvatarMaxBytes+(1<<10))
	if err := r.ParseMultipartForm(appAvatarMaxBytes); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid avatar upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "avatar file is required")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, appAvatarMaxBytes+1))
	if err != nil || len(content) == 0 || len(content) > appAvatarMaxBytes || !isImageUpload(header.Header.Get("Content-Type"), content) {
		httpx.Fail(w, http.StatusBadRequest, "avatar must be an image under 5MB")
		return
	}
	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	result, err := uploader.Upload(r.Context(), storage.UploadInput{
		ContentType: contentType,
		Dir:         "user-avatars",
		Filename:    filepath.Base(header.Filename),
		Reader:      bytes.NewReader(content),
		Size:        int64(len(content)),
	})
	if err != nil {
		httpx.Fail(w, http.StatusBadGateway, "avatar upload failed")
		return
	}
	if s.db == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "avatar storage unavailable")
		return
	}
	asset, err := s.uploads.Create(r.Context(), uploadasset.CreateInput{
		ContentType: result.ContentType,
		Data:        content,
		Dir:         "user-avatars",
		Name:        result.Name,
		ObjectKey:   result.ObjectKey,
		ObjectURL:   result.ObjectURL,
		Size:        int64(len(content)),
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "avatar upload failed")
		return
	}
	httpx.OK(w, map[string]any{
		"url":     fmt.Sprintf("/api/app/profile/avatar/%d", asset.ID),
		"assetId": asset.ID,
	})
}

func (s *Server) appProfileAvatarDownload(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/app/profile/avatar/"), "/"), 10, 64)
	if err != nil || id <= 0 || s.uploads == nil {
		http.NotFound(w, r)
		return
	}
	asset, err := s.uploads.Find(r.Context(), id)
	if err != nil || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(asset.ContentType)), "image/") {
		http.NotFound(w, r)
		return
	}
	writeUploadAsset(w, asset)
}
