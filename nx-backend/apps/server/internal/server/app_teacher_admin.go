package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/teacher"
)

func teacherAdminPermission(next http.HandlerFunc) http.HandlerFunc {
	return next
}

func (s *Server) adminTeacherCollection(w http.ResponseWriter, r *http.Request) {
	if s.teachers == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "teacher service unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.teachers.List(r.Context(), true)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "list teachers failed")
			return
		}
		items = filterAdminTeachers(items, r)
		page, pageSize := parseTeacherAdminPage(r)
		total := len(items)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		httpx.OK(w, map[string]any{"items": items[start:end], "total": total, "page": page, "pageSize": pageSize})
		return
	case http.MethodPost:
		var item teacher.Teacher
		if !decodeTeacherJSON(w, r, &item) {
			return
		}
		saved, err := s.teachers.Save(r.Context(), item)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, saved)
		return
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func filterAdminTeachers(items []teacher.Teacher, r *http.Request) []teacher.Teacher {
	keyword := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("keyword")))
	enabled := strings.TrimSpace(r.URL.Query().Get("enabled"))
	filtered := make([]teacher.Teacher, 0, len(items))
	for _, item := range items {
		if enabled == "true" && !item.Enabled || enabled == "false" && item.Enabled {
			continue
		}
		if keyword != "" {
			text := strings.ToLower(strings.Join([]string{item.Key, item.Name, item.Title, item.ShortIntro}, " "))
			if !strings.Contains(text, keyword) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func parseTeacherAdminPage(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func (s *Server) adminTeacherRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/teachers/"), "/")
	if path == "" {
		s.adminTeacherCollection(w, r)
		return
	}
	parts := strings.Split(path, "/")
	key := parts[0]
	if len(parts) == 1 && (r.Method == http.MethodGet || r.Method == http.MethodPut) {
		item, err := s.teachers.Get(r.Context(), key)
		if errors.Is(err, teacher.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "teacher not found")
			return
		}
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "get teacher failed")
			return
		}
		if r.Method == http.MethodGet {
			httpx.OK(w, item)
			return
		}
		var in teacher.Teacher
		if !decodeTeacherJSON(w, r, &in) {
			return
		}
		in.Key = key
		saved, err := s.teachers.Save(r.Context(), in)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, saved)
		return
	}
	if len(parts) == 2 && (parts[1] == "enabled" || parts[1] == "status") && r.Method == http.MethodPut {
		var in struct {
			Enabled *bool `json:"enabled"`
		}
		if !decodeTeacherJSON(w, r, &in) || in.Enabled == nil {
			httpx.Fail(w, http.StatusBadRequest, "enabled is required")
			return
		}
		item, err := s.teachers.SetEnabled(r.Context(), key, *in.Enabled)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, "update teacher failed")
			return
		}
		httpx.OK(w, item)
		return
	}
	if len(parts) == 2 && parts[1] == "review" && r.Method == http.MethodPost {
		var in struct {
			Status teacher.ReviewState `json:"status"`
			Reason string              `json:"reason"`
		}
		if !decodeTeacherJSON(w, r, &in) {
			return
		}
		item, err := s.teachers.ReviewProfileDraft(r.Context(), key, in.Status, in.Reason, userFromRequest(r).ID)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, item)
		return
	}
	if len(parts) == 2 && parts[1] == "binding" && r.Method == http.MethodPut {
		var in struct {
			AppUserID int64 `json:"appUserId"`
			Enabled   *bool `json:"enabled"`
		}
		if !decodeTeacherJSON(w, r, &in) {
			return
		}
		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		actor := userFromRequest(r).ID
		roles, err := s.teachers.BindTeacher(r.Context(), in.AppUserID, key, enabled, &actor)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, roles)
		return
	}
	if len(parts) >= 2 && parts[0] == "content" {
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || id <= 0 {
			httpx.Fail(w, http.StatusBadRequest, "invalid content id")
			return
		}
		if len(parts) == 3 && parts[2] == "review" && r.Method == http.MethodPost {
			var in struct {
				Status teacher.ReviewState `json:"status"`
				Reason string              `json:"reason"`
			}
			if !decodeTeacherJSON(w, r, &in) {
				return
			}
			item, err := s.teachers.ReviewContent(r.Context(), id, in.Status, in.Reason, userFromRequest(r).ID)
			if err != nil {
				httpx.Fail(w, http.StatusBadRequest, err.Error())
				return
			}
			httpx.OK(w, item)
			return
		}
	}
	httpx.Fail(w, http.StatusNotFound, "not found")
}

// teacherReviewItem is the compact row consumed by the admin review table.
// A classroom content id is also exposed as contentId for clients that keep
// separate review and content identifiers.
type teacherReviewItem struct {
	ID                int64               `json:"id"`
	ContentID         int64               `json:"contentId"`
	ContentType       string              `json:"contentType,omitempty"`
	FeedType          string              `json:"feedType"`
	ReplacedContentID *int64              `json:"replacedContentId,omitempty"`
	ReviewReason      string              `json:"reviewReason"`
	ReviewStatus      teacher.ReviewState `json:"reviewStatus"`
	SeriesID          *int64              `json:"seriesId,omitempty"`
	TeacherKey        string              `json:"teacherKey"`
	TeacherName       string              `json:"teacherName"`
	Title             string              `json:"title"`
	UpdatedAt         time.Time           `json:"updatedAt"`
}

// adminTeacherReviews serves the web console review queue. The default queue
// is pending_review; passing reviewStatus/status allows audit views as well.
func (s *Server) adminTeacherReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	if s.db == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "teacher service unavailable")
		return
	}
	page, pageSize := parseTeacherReviewPage(r)
	status := strings.TrimSpace(r.URL.Query().Get("reviewStatus"))
	if status == "" {
		status = strings.TrimSpace(r.URL.Query().Get("status"))
	}
	if status == "" {
		status = string(teacher.ReviewPending)
	}
	feedType := strings.TrimSpace(r.URL.Query().Get("feedType"))
	teacherKey := strings.TrimSpace(r.URL.Query().Get("teacherKey"))
	// The teacher queue must not include legacy classroom rows that have no
	// teacher ownership metadata.
	where := []string{"c.review_status=$1", "c.teacher_key <> ''"}
	args := []any{status}
	if feedType != "" {
		args = append(args, feedType)
		where = append(where, "c.feed_type=$"+strconv.Itoa(len(args)))
	}
	if teacherKey != "" {
		args = append(args, teacherKey)
		where = append(where, "c.teacher_key=$"+strconv.Itoa(len(args)))
	}
	var total int
	if err := s.db.QueryRowContext(r.Context(), "SELECT count(*) FROM classroom_contents c WHERE "+strings.Join(where, " AND "), args...).Scan(&total); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "list teacher reviews failed")
		return
	}
	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(r.Context(), `SELECT c.id,c.series_id,c.title,c.content_type,c.teacher_key,COALESCE(t.name,''),c.feed_type,c.review_status,c.review_reason,c.replaces_content_id,c.updated_at
		FROM classroom_contents c LEFT JOIN teacher_profiles t ON t.teacher_key=c.teacher_key WHERE `+strings.Join(where, " AND ")+` ORDER BY c.updated_at DESC,c.id DESC LIMIT $`+strconv.Itoa(limitPos)+` OFFSET $`+strconv.Itoa(offsetPos), args...)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "list teacher reviews failed")
		return
	}
	defer rows.Close()
	items := make([]teacherReviewItem, 0)
	for rows.Next() {
		var item teacherReviewItem
		if err := rows.Scan(&item.ID, &item.SeriesID, &item.Title, &item.ContentType, &item.TeacherKey, &item.TeacherName, &item.FeedType, &item.ReviewStatus, &item.ReviewReason, &item.ReplacedContentID, &item.UpdatedAt); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "list teacher reviews failed")
			return
		}
		item.ContentID = item.ID
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "list teacher reviews failed")
		return
	}
	httpx.OK(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func parseTeacherReviewPage(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

// adminTeacherReviewAction maps the web console's semantic actions to the
// existing content review state machine.
func (s *Server) adminTeacherReviewAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || s.teachers == nil {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/teacher-reviews/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		httpx.Fail(w, http.StatusNotFound, "not found")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid review id")
		return
	}
	state := teacher.ReviewState("")
	switch parts[1] {
	case "approve":
		state = teacher.ReviewPublished
	case "reject":
		state = teacher.ReviewRejected
	case "offline":
		state = teacher.ReviewOffline
	default:
		httpx.Fail(w, http.StatusNotFound, "not found")
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
			httpx.Fail(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	item, err := s.teachers.ReviewContent(r.Context(), id, state, in.Reason, userFromRequest(r).ID)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, teacherReviewItem{
		ID: item.ID, ContentID: item.ID, FeedType: item.FeedType,
		ReviewReason: item.ReviewReason, ReviewStatus: item.ReviewStatus,
		SeriesID: item.SeriesID, ReplacedContentID: item.ReplacesContentID,
		TeacherKey: item.TeacherKey, Title: item.Title, UpdatedAt: item.UpdatedAt,
	})
}
