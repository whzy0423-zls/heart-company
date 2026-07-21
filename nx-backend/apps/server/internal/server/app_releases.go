package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/apprelease"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

func (s *Server) appReleaseList(w http.ResponseWriter, r *http.Request) {
	if s.appReleases == nil {
		httpx.Fail(w, 503, "App release service unavailable")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	result, err := s.appReleases.List(r.Context(), page, size)
	if err != nil {
		httpx.Fail(w, 500, "Failed to load app releases")
		return
	}
	httpx.OK(w, result)
}

func (s *Server) appReleaseUpload(w http.ResponseWriter, r *http.Request) {
	if s.appReleases == nil {
		httpx.Fail(w, 503, "App release service unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, apprelease.MaxMultipartBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		httpx.Fail(w, 400, "Invalid multipart request")
		return
	}
	var staged apprelease.StagedFile
	var hasFile bool
	var notes string
	defer func() {
		if hasFile {
			_ = s.appReleases.DiscardStaged(staged)
		}
	}()
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			mapAppReleaseError(w, nextErr)
			return
		}
		if part.FormName() == "file" {
			if hasFile || part.FileName() == "" {
				httpx.Fail(w, 400, "Exactly one APK file is required")
				return
			}
			staged, err = s.appReleases.StageAPK(filepath.Base(part.FileName()), part)
			if err != nil {
				mapAppReleaseError(w, err)
				return
			}
			hasFile = true
		} else if part.FormName() == "release_notes" {
			raw, readErr := io.ReadAll(io.LimitReader(part, 64*1024+1))
			if readErr != nil || len(raw) > 64*1024 {
				httpx.Fail(w, 400, "Release notes are too large")
				return
			}
			notes = string(raw)
		} else {
			if _, err := io.Copy(io.Discard, io.LimitReader(part, 64*1024+1)); err != nil {
				httpx.Fail(w, 400, "Invalid multipart field")
				return
			}
		}
		_ = part.Close()
	}
	if !hasFile {
		httpx.Fail(w, 400, "APK file is required")
		return
	}
	created, err := s.appReleases.CreateDraftFromStaged(r.Context(), staged, notes)
	hasFile = false
	if err != nil {
		mapAppReleaseError(w, err)
		return
	}
	httpx.OK(w, created)
}

func (s *Server) appReleaseMutation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Fail(w, 405, "MethodNotAllowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/app-releases/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, 400, "Invalid release ID")
		return
	}
	var release apprelease.Release
	switch parts[1] {
	case "publish":
		release, err = s.appReleases.Publish(r.Context(), id)
	case "archive":
		release, err = s.appReleases.Archive(r.Context(), id)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		mapAppReleaseError(w, err)
		return
	}
	httpx.OK(w, release)
}

func (s *Server) publicAppReleaseLatest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	if s.appReleases == nil {
		httpx.Fail(w, 503, "App release service unavailable")
		return
	}
	release, err := s.appReleases.Latest(r.Context(), "android")
	if errors.Is(err, apprelease.ErrNotFound) {
		httpx.OK(w, map[string]any{"available": false})
		return
	}
	if err != nil {
		httpx.Fail(w, 500, "Failed to load app release")
		return
	}
	if !release.FileAvailable {
		httpx.Fail(w, 503, "安装包暂时不可用")
		return
	}
	httpx.OK(w, map[string]any{"available": true, "platform": release.Platform, "versionName": release.VersionName, "versionCode": release.VersionCode, "publishedAt": release.PublishedAt, "fileSize": release.FileSize, "sha256": release.SHA256, "releaseNotes": release.ReleaseNotes, "downloadUrl": fmt.Sprintf("/api/public/app-releases/%d/download", release.ID)})
}

func (s *Server) publicAppReleaseLatestDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	release, err := s.appReleases.Latest(r.Context(), "android")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !release.FileAvailable {
		httpx.Fail(w, 503, "安装包暂时不可用")
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/api/public/app-releases/%d/download", release.ID), http.StatusFound)
}

func (s *Server) publicAppReleaseDownload(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/public/app-releases/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) != 2 || parts[1] != "download" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	release, file, err := s.appReleases.Open(r.Context(), id)
	if errors.Is(err, apprelease.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.Fail(w, 503, "安装包暂时不可用")
		return
	}
	defer file.Close()
	if release.Status == apprelease.StatusDraft {
		http.NotFound(w, r)
		return
	}
	if release.Status == apprelease.StatusArchived {
		httpx.Fail(w, http.StatusGone, "Release archived")
		return
	}
	etag := `"` + release.SHA256 + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeDownloadName(release.FileName)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300, must-revalidate")
	http.ServeContent(w, r, release.FileName, release.CreatedAt, file)
}

func safeDownloadName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\"", ""))
	if !strings.EqualFold(filepath.Ext(name), ".apk") {
		return "nine-xing.apk"
	}
	return name
}

func mapAppReleaseError(w http.ResponseWriter, err error) {
	status, message := 500, "App release operation failed"
	switch {
	case errors.Is(err, apprelease.ErrFileTooLarge):
		status, message = 413, "APK exceeds 300 MiB"
	case errors.Is(err, apprelease.ErrConflict):
		status, message = 409, "Release version already exists or state conflicts"
	case errors.Is(err, apprelease.ErrPublishCertificateUnavailable):
		status, message = 503, "Signing certificate is not configured"
	case errors.Is(err, apprelease.ErrCertificateMismatch), errors.Is(err, apprelease.ErrPackageMismatch):
		status, message = 409, "APK identity does not match configuration"
	case errors.Is(err, apprelease.ErrNotFound):
		status, message = 404, "Release not found"
	case errors.Is(err, apprelease.ErrInvalidAPK), errors.Is(err, apprelease.ErrUnsignedAPK), errors.Is(err, apprelease.ErrInvalidExtension), errors.Is(err, apprelease.ErrInvalidVersion):
		status, message = 400, "Invalid APK"
	}
	httpx.Fail(w, status, message)
}
