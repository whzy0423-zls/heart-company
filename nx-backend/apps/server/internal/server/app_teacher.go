package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/teacher"
)

func (s *Server) appTeacherCollection(w http.ResponseWriter, r *http.Request) {
	if s.teachers == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "teacher service unavailable")
		return
	}
	items, err := s.teachers.List(r.Context(), false)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "list teachers failed")
		return
	}
	for i := range items {
		rewritePublicTeacherAssets(&items[i])
	}
	httpx.OK(w, map[string]any{"items": items})
}

func (s *Server) appTeacherRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/app/teachers/"), "/")
	if path == "" {
		s.appTeacherCollection(w, r)
		return
	}
	parts := strings.Split(path, "/")
	key := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		s.appTeacherDetail(w, r, key)
		return
	}
	if len(parts) == 2 && r.Method == http.MethodGet {
		if parts[1] == "videos" {
			s.appTeacherVideoList(w, r, key)
			return
		}
		if parts[1] != "courses" && parts[1] != "updates" {
			httpx.Fail(w, http.StatusNotFound, "not found")
			return
		}
		s.appTeacherContentList(w, r, key, parts[1] == "updates")
		return
	}
	if len(parts) == 3 && r.Method == http.MethodPost && (parts[2] == "like" || parts[2] == "favorite") {
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			httpx.Fail(w, 400, "invalid content id")
			return
		}
		u, ok := appUserFromContext(r)
		if !ok {
			httpx.Fail(w, 401, "Unauthorized Exception")
			return
		}
		item, active, err := s.teachers.ToggleEngagement(r.Context(), id, u.ID, parts[2])
		if err != nil {
			httpx.Fail(w, 400, "engagement failed")
			return
		}
		httpx.OK(w, map[string]any{"content": item, "active": active, "likeCount": item.LikeCount, "favoriteCount": item.FavoriteCount})
		return
	}
	httpx.Fail(w, http.StatusNotFound, "not found")
}

func (s *Server) appTeacherVideoList(w http.ResponseWriter, r *http.Request, key string) {
	if s.teachers == nil || s.classroomPublic == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "teacher video service unavailable")
		return
	}
	profile, err := s.teachers.Get(r.Context(), key)
	if errors.Is(err, teacher.ErrNotFound) || errors.Is(err, sql.ErrNoRows) || (err == nil && !profile.Enabled) {
		httpx.Fail(w, http.StatusNotFound, "teacher not found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "get teacher failed")
		return
	}
	drafts, err := s.teachers.ListPublishedVideos(r.Context(), key)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "list teacher videos failed")
		return
	}
	items := make([]classroomPublicContent, 0, len(drafts))
	for _, draft := range drafts {
		item, err := s.classroomPublic.GetContent(r.Context(), draft.ID, appUserID(r))
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "list teacher videos failed")
			return
		}
		if item.PublishedAt == nil {
			publishedAt := draft.CreatedAt
			item.PublishedAt = &publishedAt
		}
		item.LikeCount = draft.LikeCount
		item.FavoriteCount = draft.FavoriteCount
		items = append(items, item)
	}
	httpx.OK(w, map[string]any{"items": items})
}

func (s *Server) appTeacherDetail(w http.ResponseWriter, r *http.Request, key string) {
	item, err := s.teachers.Get(r.Context(), key)
	if errors.Is(err, teacher.ErrNotFound) || errors.Is(err, errors.New("record not found")) {
		httpx.Fail(w, http.StatusNotFound, "teacher not found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "get teacher failed")
		return
	}
	if !item.Enabled {
		httpx.Fail(w, http.StatusNotFound, "teacher not found")
		return
	}
	rewritePublicTeacherAssets(&item)
	httpx.OK(w, item)
}

func rewritePublicTeacherAssets(item *teacher.Teacher) {
	if item == nil {
		return
	}
	item.Avatar = publicTeacherAssetURL(item.Avatar)
	item.Cover = publicTeacherAssetURL(item.Cover)
}

func publicTeacherAssetURL(raw string) string {
	return publicConfigAssetURL(raw, "/api/public/teacher-assets/", "/api/public/teacher-uploads/")
}

func teachersReferenceUploadAsset(items []teacher.Teacher, id int64) bool {
	for _, item := range items {
		if item.Enabled && (valueReferencesUploadAsset(item.Avatar, id) || valueReferencesUploadAsset(item.Cover, id)) {
			return true
		}
	}
	return false
}

func teachersReferenceLocalUpload(items []teacher.Teacher, privateURL string) bool {
	for _, item := range items {
		if item.Enabled && (valueReferencesLocalUpload(item.Avatar, privateURL) || valueReferencesLocalUpload(item.Cover, privateURL)) {
			return true
		}
	}
	return false
}

func (s *Server) appTeacherContentList(w http.ResponseWriter, r *http.Request, key string, daily bool) {
	if _, err := s.teachers.Get(r.Context(), key); err != nil {
		httpx.Fail(w, http.StatusNotFound, "teacher not found")
		return
	}
	feed := "course"
	if daily {
		feed = "daily"
	}
	items, err := s.teachers.ListContent(r.Context(), key, teacher.ReviewPublished, feed, true)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "list teacher content failed")
		return
	}
	httpx.OK(w, map[string]any{"items": items, "feedType": feed})
}

func (s *Server) teacherRole(r *http.Request) (teacher.RoleSet, bool) {
	u, ok := appUserFromContext(r)
	if !ok || u.ID <= 0 || s.teachers == nil {
		return teacher.RoleSet{}, false
	}
	roles, err := s.teachers.Roles(r.Context(), u.ID)
	if err != nil || !roles.IsTeacher() {
		return teacher.RoleSet{}, false
	}
	return roles, true
}

func (s *Server) appTeacherMe(w http.ResponseWriter, r *http.Request) {
	roles, ok := s.teacherRole(r)
	if !ok {
		httpx.Fail(w, http.StatusForbidden, "teacher role required")
		return
	}
	item, err := s.teachers.Get(r.Context(), roles.TeacherKey)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "teacher profile not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, _ := s.teachers.ListContent(r.Context(), roles.TeacherKey, "", "", false)
		s.decorateTeacherContentCovers(r.Context(), items)
		httpx.OK(w, map[string]any{"teacher": item, "contents": items})
		return
	case http.MethodPut:
		var in struct {
			Avatar, Cover, Title, ShortIntro, DetailIntro string
			Expertise                                     []string
			IntroVideoURL                                 string
			CustomerService, OfflineService               map[string]any
		}
		if !decodeTeacherJSON(w, r, &in) {
			return
		}
		item.Avatar = in.Avatar
		item.Cover = in.Cover
		item.Title = in.Title
		item.ShortIntro = in.ShortIntro
		item.DetailIntro = in.DetailIntro
		item.Expertise = in.Expertise
		item.IntroVideoURL = in.IntroVideoURL
		item.CustomerService = in.CustomerService
		item.OfflineService = in.OfflineService
		draftID, err := s.teachers.SaveProfileDraft(r.Context(), roles.TeacherKey, appUserID(r), item)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, map[string]any{"teacher": item, "draftId": draftID, "reviewStatus": teacher.ReviewPending})
		return
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Server) appTeacherContentMe(w http.ResponseWriter, r *http.Request) {
	roles, ok := s.teacherRole(r)
	if !ok {
		httpx.Fail(w, http.StatusForbidden, "teacher role required")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/app/teacher/me/content"), "/")
	if parts := strings.Split(path, "/"); len(parts) >= 2 && parts[1] == "uploads" {
		s.appTeacherUploadRouter(w, r, roles, path)
		return
	}
	if path == "" && r.Method == http.MethodGet {
		items, err := s.teachers.ListContent(r.Context(), roles.TeacherKey, "", "", false)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "list teacher content failed")
			return
		}
		s.decorateTeacherContentCovers(r.Context(), items)
		httpx.OK(w, map[string]any{"items": items})
		return
	}
	if path == "" && r.Method == http.MethodPost {
		var in teacher.ContentDraft
		if !decodeTeacherJSON(w, r, &in) {
			return
		}
		in.TeacherKey = roles.TeacherKey
		item, err := s.teachers.CreateContent(r.Context(), in, appUserID(r))
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		s.decorateTeacherContentCover(r.Context(), &item)
		httpx.OK(w, item)
		return
	}
	parts := strings.Split(path, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid content id")
		return
	}
	if len(parts) == 2 && (parts[1] == "submit" || parts[1] == "submit-review") && r.Method == http.MethodPost {
		item, err := s.teachers.SubmitContent(r.Context(), id, roles.TeacherKey)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, "content cannot be submitted")
			return
		}
		s.decorateTeacherContentCover(r.Context(), &item)
		httpx.OK(w, item)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		item, err := s.teachers.GetContent(r.Context(), id)
		if err != nil || item.TeacherKey != roles.TeacherKey {
			httpx.Fail(w, http.StatusNotFound, "content not found")
			return
		}
		s.decorateTeacherContentCover(r.Context(), &item)
		httpx.OK(w, item)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodPut {
		item, err := s.teachers.GetContent(r.Context(), id)
		if err != nil || item.TeacherKey != roles.TeacherKey {
			httpx.Fail(w, http.StatusNotFound, "content not found")
			return
		}
		var in teacher.ContentDraft
		if !decodeTeacherJSON(w, r, &in) {
			return
		}
		in.ID = id
		updated, err := s.teachers.UpdateContent(r.Context(), in, roles.TeacherKey)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		s.decorateTeacherContentCover(r.Context(), &updated)
		httpx.OK(w, updated)
		return
	}
	httpx.Fail(w, http.StatusNotFound, "not found")
}

// decorateTeacherContentCovers resolves generated video covers for the private
// teacher workspace. The object key stays server-side; clients receive only a
// short-lived signed URL, matching the public classroom cover behavior.
func (s *Server) decorateTeacherContentCovers(ctx context.Context, items []teacher.ContentDraft) {
	for i := range items {
		s.decorateTeacherContentCover(ctx, &items[i])
	}
}

func (s *Server) decorateTeacherContentCover(ctx context.Context, item *teacher.ContentDraft) {
	if item == nil || strings.TrimSpace(item.CoverURL) != "" || item.MediaAssetID == nil || *item.MediaAssetID <= 0 || s.db == nil || s.classroomPlaybackSigner == nil {
		return
	}
	var generatedKey string
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(cover_object_key,'') FROM classroom_media_assets WHERE id=$1`, *item.MediaAssetID).Scan(&generatedKey); err != nil {
		return
	}
	_ = signTeacherGeneratedCover(ctx, item, generatedKey, s.classroomPlaybackSigner, s.classroomCoverTTL())
}

func signTeacherGeneratedCover(ctx context.Context, item *teacher.ContentDraft, objectKey string, signer storage.ObjectSigner, ttl time.Duration) error {
	if item == nil || signer == nil || strings.TrimSpace(objectKey) == "" {
		return nil
	}
	url, err := signer.PresignGetURL(ctx, strings.TrimSpace(objectKey), ttl)
	if err != nil {
		return err
	}
	item.CoverURL = url
	return nil
}

func appUserID(r *http.Request) int64 { u, _ := appUserFromContext(r); return u.ID }
func decodeTeacherJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	if err := d.Decode(v); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
