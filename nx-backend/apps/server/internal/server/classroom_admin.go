package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const maxClassroomPriceCents = 99_999_900

type classroomAuditRecorder interface {
	Record(context.Context, auditlog.Entry) error
}

type classroomAdminService interface {
	ListSeries(context.Context, classroom.SeriesFilter) ([]classroom.Series, int, error)
	GetSeries(context.Context, int64) (classroom.Series, error)
	CreateSeries(context.Context, classroom.Series) (classroom.Series, error)
	UpdateSeries(context.Context, classroom.Series, time.Time) (classroom.Series, error)
	DeleteSeries(context.Context, int64, time.Time) error
	ListContents(context.Context, classroom.ContentFilter) ([]classroom.Content, int, error)
	GetContent(context.Context, int64) (classroom.Content, error)
	CreateContent(context.Context, classroom.Content) (classroom.Content, error)
	UpdateContent(context.Context, classroom.Content, time.Time) (classroom.Content, error)
	DeleteContent(context.Context, int64, time.Time) error
	ListUploadTasks(context.Context, int, int) ([]classroom.UploadTask, int, error)
}

type classroomAdminStore struct {
	db    *sql.DB
	store *classroom.Store
}

func newClassroomAdminStore(db *sql.DB) classroomAdminService {
	if db == nil {
		return nil
	}
	return &classroomAdminStore{db: db, store: classroom.NewStore(db)}
}
func (a *classroomAdminStore) GetSeries(ctx context.Context, id int64) (classroom.Series, error) {
	return a.store.GetSeries(ctx, id)
}
func (a *classroomAdminStore) CreateSeries(ctx context.Context, v classroom.Series) (classroom.Series, error) {
	return a.store.CreateSeries(ctx, v)
}
func (a *classroomAdminStore) UpdateSeries(ctx context.Context, v classroom.Series, at time.Time) (classroom.Series, error) {
	return a.store.UpdateSeries(ctx, v, at)
}
func (a *classroomAdminStore) DeleteSeries(ctx context.Context, id int64, expected time.Time) error {
	if expected.IsZero() {
		return classroom.ErrConflict
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var status classroom.SeriesStatus
	var updated time.Time
	if err = tx.QueryRowContext(ctx, "SELECT status,updated_at FROM classroom_series WHERE id=$1 FOR UPDATE", id).Scan(&status, &updated); errors.Is(err, sql.ErrNoRows) {
		return classroom.ErrNotFound
	} else if err != nil {
		return err
	}
	if !updated.Equal(expected) {
		return classroom.ErrConflict
	}
	if status != classroom.SeriesDraft {
		return errors.New("only draft series can be deleted")
	}
	var dependencies int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM classroom_contents WHERE series_id=$1", id).Scan(&dependencies); err != nil {
		return err
	}
	if dependencies > 0 {
		return errors.New("series has dependent contents")
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM classroom_series WHERE id=$1", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *classroomAdminStore) GetContent(ctx context.Context, id int64) (classroom.Content, error) {
	return a.store.GetContent(ctx, id)
}
func (a *classroomAdminStore) CreateContent(ctx context.Context, v classroom.Content) (classroom.Content, error) {
	return a.store.CreateContent(ctx, v)
}
func (a *classroomAdminStore) UpdateContent(ctx context.Context, v classroom.Content, at time.Time) (classroom.Content, error) {
	return a.store.UpdateContent(ctx, v, at)
}
func (a *classroomAdminStore) DeleteContent(ctx context.Context, id int64, expected time.Time) error {
	if expected.IsZero() {
		return classroom.ErrConflict
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var status classroom.ContentStatus
	var updated time.Time
	var mediaID *int64
	if err = tx.QueryRowContext(ctx, "SELECT status,updated_at,media_asset_id FROM classroom_contents WHERE id=$1 FOR UPDATE", id).Scan(&status, &updated, &mediaID); errors.Is(err, sql.ErrNoRows) {
		return classroom.ErrNotFound
	} else if err != nil {
		return err
	}
	if !updated.Equal(expected) {
		return classroom.ErrConflict
	}
	if status != classroom.ContentDraft {
		return errors.New("only draft content can be deleted")
	}
	if mediaID != nil {
		return errors.New("content has media dependency")
	}
	var uploads int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM classroom_upload_tasks WHERE content_id=$1", id).Scan(&uploads); err != nil {
		return err
	}
	if uploads > 0 {
		return errors.New("content has upload dependencies")
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM classroom_contents WHERE id=$1", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *classroomAdminStore) ListSeries(ctx context.Context, f classroom.SeriesFilter) ([]classroom.Series, int, error) {
	items, err := a.store.ListSeries(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	where, args := []string{"1=1"}, []any{}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	if f.AccessLevel != "" {
		args = append(args, f.AccessLevel)
		where = append(where, fmt.Sprintf("access_level=$%d", len(args)))
	}
	var total int
	err = a.db.QueryRowContext(ctx, "SELECT count(*) FROM classroom_series WHERE "+strings.Join(where, " AND "), args...).Scan(&total)
	return items, total, err
}
func (a *classroomAdminStore) ListContents(ctx context.Context, f classroom.ContentFilter) ([]classroom.Content, int, error) {
	items, err := a.store.ListContents(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	where, args := []string{"1=1"}, []any{}
	if f.SeriesID != nil {
		args = append(args, *f.SeriesID)
		where = append(where, fmt.Sprintf("series_id=$%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	if f.ContentType != "" {
		args = append(args, f.ContentType)
		where = append(where, fmt.Sprintf("content_type=$%d", len(args)))
	}
	if f.StandaloneOnly {
		where = append(where, "(series_id IS NULL OR show_as_standalone=true)")
	}
	var total int
	err = a.db.QueryRowContext(ctx, "SELECT count(*) FROM classroom_contents WHERE "+strings.Join(where, " AND "), args...).Scan(&total)
	return items, total, err
}
func (a *classroomAdminStore) ListUploadTasks(ctx context.Context, limit, offset int) ([]classroom.UploadTask, int, error) {
	limit = classroomAdminLimit(limit)
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := a.db.QueryRowContext(ctx, "SELECT count(*) FROM classroom_upload_tasks").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := a.db.QueryContext(ctx, `SELECT id,content_id,creator_id,oss_upload_id,object_key,expected_size,checksum,part_size,max_parts,status,expires_at,attempt_count,cleanup_status,media_asset_id,failure_reason,created_at,updated_at FROM classroom_upload_tasks ORDER BY created_at DESC,id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []classroom.UploadTask{}
	for rows.Next() {
		var v classroom.UploadTask
		if err := rows.Scan(&v.ID, &v.ContentID, &v.CreatorID, &v.OSSUploadID, &v.ObjectKey, &v.ExpectedSize, &v.Checksum, &v.PartSize, &v.MaxParts, &v.Status, &v.ExpiresAt, &v.AttemptCount, &v.CleanupStatus, &v.MediaAssetID, &v.FailureReason, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, v)
	}
	return items, total, rows.Err()
}

type classroomPage[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}
type classroomSeriesDTO struct {
	ID              int64                  `json:"id"`
	Title           string                 `json:"title"`
	Summary         string                 `json:"summary"`
	CoverURL        string                 `json:"coverUrl"`
	TeacherKey      string                 `json:"teacherKey"`
	TeacherName     string                 `json:"teacherName"`
	SortOrder       int                    `json:"sortOrder"`
	Status          classroom.SeriesStatus `json:"status"`
	PlaybackBlocked bool                   `json:"playbackBlocked"`
	AccessLevel     classroom.AccessLevel  `json:"accessLevel"`
	PriceCents      int                    `json:"priceCents"`
	PublishedAt     *time.Time             `json:"publishedAt,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
}
type classroomContentDTO struct {
	ID                   int64                   `json:"id"`
	SeriesID             *int64                  `json:"seriesId,omitempty"`
	ShowAsStandalone     bool                    `json:"showAsStandalone"`
	Title                string                  `json:"title"`
	Description          string                  `json:"description"`
	ContentType          classroom.ContentType   `json:"contentType"`
	MediaAssetID         *int64                  `json:"mediaAssetId,omitempty"`
	CoverURL             string                  `json:"coverUrl"`
	DurationSeconds      int                     `json:"durationSeconds"`
	TeacherKey           string                  `json:"teacherKey"`
	TeacherName          string                  `json:"teacherName"`
	RecordedAt           *time.Time              `json:"recordedAt,omitempty"`
	Badge                string                  `json:"badge"`
	Tags                 []string                `json:"tags"`
	EpisodeNo            int                     `json:"episodeNo"`
	SortOrder            int                     `json:"sortOrder"`
	Status               classroom.ContentStatus `json:"status"`
	PlaybackBlocked      bool                    `json:"playbackBlocked"`
	AccessLevel          classroom.AccessLevel   `json:"accessLevel"`
	EffectiveAccessLevel classroom.AccessLevel   `json:"effectiveAccessLevel"`
	PriceCents           int                     `json:"priceCents"`
	EffectivePriceCents  int                     `json:"effectivePriceCents"`
	PurchaseTarget       string                  `json:"purchaseTarget,omitempty"`
	PublishedAt          *time.Time              `json:"publishedAt,omitempty"`
	CreatedAt            time.Time               `json:"createdAt"`
	UpdatedAt            time.Time               `json:"updatedAt"`
}
type classroomUploadTaskDTO struct {
	ID            int64                  `json:"id"`
	ContentID     int64                  `json:"contentId"`
	ExpectedSize  int64                  `json:"expectedSize"`
	PartSize      int64                  `json:"partSize"`
	MaxParts      int                    `json:"maxParts"`
	Status        classroom.UploadStatus `json:"status"`
	ExpiresAt     time.Time              `json:"expiresAt"`
	AttemptCount  int                    `json:"attemptCount"`
	CleanupStatus string                 `json:"cleanupStatus"`
	MediaAssetID  *int64                 `json:"mediaAssetId,omitempty"`
	FailureReason string                 `json:"failureReason,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
}

func registerClassroomAdminRoutes(mux *http.ServeMux, permission func(string, http.HandlerFunc) http.HandlerFunc, s *Server) {
	mux.HandleFunc("/api/admin/classroom/series", func(w http.ResponseWriter, r *http.Request) {
		code := "Miniapp:Classroom:List"
		if r.Method == http.MethodPost {
			code = "Miniapp:Classroom:Write"
		}
		permission(code, s.classroomSeriesCollection)(w, r)
	})
	mux.HandleFunc("/api/admin/classroom/series/", func(w http.ResponseWriter, r *http.Request) {
		permission(classroomActionPermission(r), s.classroomSeriesItem)(w, r)
	})
	mux.HandleFunc("/api/admin/classroom/contents", func(w http.ResponseWriter, r *http.Request) {
		code := "Miniapp:Classroom:List"
		if r.Method == http.MethodPost {
			code = "Miniapp:Classroom:Write"
		}
		permission(code, s.classroomContentCollection)(w, r)
	})
	mux.HandleFunc("/api/admin/classroom/contents/", func(w http.ResponseWriter, r *http.Request) {
		permission(classroomActionPermission(r), s.classroomContentItem)(w, r)
	})
	mux.HandleFunc("/api/admin/classroom/upload-tasks", permission("Miniapp:Classroom:Upload", s.classroomUploadTasks))
	mux.HandleFunc("/api/admin/classroom/uploads/tasks", permission("Miniapp:Classroom:Upload", s.classroomUploadTasks))
}
func classroomActionPermission(r *http.Request) string {
	p := strings.Trim(r.URL.Path, "/")
	if strings.HasSuffix(p, "/publish") || strings.HasSuffix(p, "/offline") || strings.HasSuffix(p, "/playback-blocked") {
		return "Miniapp:Classroom:Publish"
	}
	if strings.HasSuffix(p, "/price") {
		return "Miniapp:Classroom:Price"
	}
	if r.Method == http.MethodGet {
		return "Miniapp:Classroom:List"
	}
	return "Miniapp:Classroom:Write"
}

func (s *Server) classroomSeriesCollection(w http.ResponseWriter, r *http.Request) {
	if !s.classroomReady(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.classroomSeriesList(w, r)
	case http.MethodPost:
		s.classroomSeriesCreate(w, r)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}
func (s *Server) classroomContentCollection(w http.ResponseWriter, r *http.Request) {
	if !s.classroomReady(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.classroomContentList(w, r)
	case http.MethodPost:
		s.classroomContentCreate(w, r)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}
func (s *Server) classroomReady(w http.ResponseWriter) bool {
	if s == nil || s.classroomAdmin == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "classroom admin unavailable")
		return false
	}
	return true
}
func adminClassroomPage(r *http.Request) (int, int, error) {
	p, err := parseAdminPage(r, "page", "pageSize")
	return p.Page, p.PageSize, err
}
func classroomAdminLimit(v int) int {
	if v <= 0 {
		return 20
	}
	if v > 200 {
		return 200
	}
	return v
}
func (s *Server) classroomSeriesList(w http.ResponseWriter, r *http.Request) {
	page, size, err := adminClassroomPage(r)
	if err != nil {
		httpx.Fail(w, 400, "invalid pagination")
		return
	}
	items, total, err := s.classroomAdmin.ListSeries(r.Context(), classroom.SeriesFilter{Status: classroom.SeriesStatus(strings.TrimSpace(r.URL.Query().Get("status"))), AccessLevel: classroom.AccessLevel(strings.TrimSpace(r.URL.Query().Get("accessLevel"))), Limit: size, Offset: (page - 1) * size})
	if err != nil {
		writeClassroomAdminError(w, err)
		return
	}
	out := make([]classroomSeriesDTO, 0, len(items))
	for _, v := range items {
		out = append(out, toSeriesDTO(v))
	}
	httpx.OK(w, classroomPage[classroomSeriesDTO]{out, total, page, size})
}
func (s *Server) classroomContentList(w http.ResponseWriter, r *http.Request) {
	page, size, err := adminClassroomPage(r)
	if err != nil {
		httpx.Fail(w, 400, "invalid pagination")
		return
	}
	var seriesID *int64
	if raw := strings.TrimSpace(r.URL.Query().Get("seriesId")); raw != "" {
		v, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || v <= 0 {
			httpx.Fail(w, 400, "invalid series id")
			return
		}
		seriesID = &v
	}
	standalone := r.URL.Query().Get("standaloneOnly") == "true"
	items, total, err := s.classroomAdmin.ListContents(r.Context(), classroom.ContentFilter{SeriesID: seriesID, Status: classroom.ContentStatus(strings.TrimSpace(r.URL.Query().Get("status"))), ContentType: classroom.ContentType(strings.TrimSpace(r.URL.Query().Get("contentType"))), StandaloneOnly: standalone, Limit: size, Offset: (page - 1) * size})
	if err != nil {
		writeClassroomAdminError(w, err)
		return
	}
	out := make([]classroomContentDTO, 0, len(items))
	for _, v := range items {
		dto, err := s.toContentDTO(r.Context(), v)
		if err != nil {
			writeClassroomAdminError(w, err)
			return
		}
		out = append(out, dto)
	}
	httpx.OK(w, classroomPage[classroomContentDTO]{out, total, page, size})
}

func (s *Server) classroomSeriesCreate(w http.ResponseWriter, r *http.Request) {
	var in seriesWriteInput
	if !decodeClassroomJSON(w, r, &in) {
		return
	}
	user := userFromRequest(r)
	item := in.series()
	item.Status = classroom.SeriesDraft
	item.CreatedBy = &user.ID
	created, err := s.classroomAdmin.CreateSeries(r.Context(), item)
	if err != nil {
		writeClassroomAdminError(w, err)
		return
	}
	if err := s.recordClassroomAudit(r, "create", "classroom_series", created.ID, nil, created, "创建课程系列"); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "record classroom audit failed")
		return
	}
	httpx.OK(w, toSeriesDTO(created))
}
func (s *Server) classroomContentCreate(w http.ResponseWriter, r *http.Request) {
	var in contentWriteInput
	if !decodeClassroomJSON(w, r, &in) {
		return
	}
	user := userFromRequest(r)
	item := in.content()
	item.Status = classroom.ContentDraft
	item.CreatedBy = &user.ID
	created, err := s.classroomAdmin.CreateContent(r.Context(), item)
	if err != nil {
		writeClassroomAdminError(w, err)
		return
	}
	if err := s.recordClassroomAudit(r, "create", "classroom_content", created.ID, nil, created, "创建课件草稿"); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "record classroom audit failed")
		return
	}
	dto, err := s.toContentDTO(r.Context(), created)
	if err != nil {
		writeClassroomAdminError(w, err)
		return
	}
	httpx.OK(w, dto)
}

func (s *Server) classroomSeriesItem(w http.ResponseWriter, r *http.Request) {
	if !s.classroomReady(w) {
		return
	}
	id, action, ok := parseClassroomItemPath(r.URL.Path, "series")
	if !ok {
		httpx.Fail(w, 400, "invalid series id")
		return
	}
	current, err := s.classroomAdmin.GetSeries(r.Context(), id)
	if err != nil {
		writeClassroomAdminError(w, err)
		return
	}
	if action == "" && r.Method == http.MethodGet {
		httpx.OK(w, toSeriesDTO(current))
		return
	}
	if action == "" && r.Method == http.MethodPut {
		if current.Status != classroom.SeriesDraft {
			httpx.Fail(w, http.StatusConflict, "only draft series can be edited")
			return
		}
		var in seriesWriteInput
		if !decodeClassroomJSON(w, r, &in) {
			return
		}
		next := in.series()
		next.ID = id
		next.Status = current.Status
		next.AccessLevel = current.AccessLevel
		next.PriceCents = current.PriceCents
		next.PlaybackBlocked = current.PlaybackBlocked
		next.PublishedAt = current.PublishedAt
		next.CreatedBy = current.CreatedBy
		next.CreatedAt = current.CreatedAt
		s.updateSeries(w, r, current, next, in.ExpectedUpdatedAt, "update", "更新课程系列")
		return
	}
	if action == "" && r.Method == http.MethodDelete {
		expected, ok := expectedUpdatedAtFromQuery(r)
		if !ok {
			httpx.Fail(w, http.StatusBadRequest, "expectedUpdatedAt is required")
			return
		}
		if err := s.classroomAdmin.DeleteSeries(r.Context(), id, expected); err != nil {
			writeClassroomAdminError(w, err)
			return
		}
		if err := s.recordClassroomAudit(r, "delete", "classroom_series", id, current, nil, classroomAuditSummary("删除课程系列", r.URL.Query().Get("reason"))); err != nil {
			httpx.Fail(w, 500, "record classroom audit failed")
			return
		}
		httpx.OK(w, map[string]any{"deleted": true})
		return
	}
	s.mutateSeries(w, r, current, action)
}
func (s *Server) mutateSeries(w http.ResponseWriter, r *http.Request, current classroom.Series, action string) {
	if r.Method != http.MethodPost {
		httpx.Fail(w, 405, "Method Not Allowed")
		return
	}
	next := current
	var expected time.Time
	var actionBody classroomActionInput
	if !decodeClassroomJSON(w, r, &actionBody) {
		return
	}
	if actionBody.ExpectedUpdatedAt.IsZero() {
		httpx.Fail(w, http.StatusBadRequest, "expectedUpdatedAt is required")
		return
	}
	expected = actionBody.ExpectedUpdatedAt
	var reason string
	switch action {
	case "publish":
		next.Status = classroom.SeriesPublished
		now := time.Now()
		next.PublishedAt = &now
		reason = classroomAuditSummary("发布课程系列", actionBody.Reason)
	case "offline":
		next.Status = classroom.SeriesOffline
		reason = classroomAuditSummary("下线课程系列", actionBody.Reason)
	case "price":
		var in priceInput
		in = priceInput{AccessLevel: actionBody.AccessLevel, PriceCents: actionBody.PriceCents, ExpectedUpdatedAt: expected}
		if !validClassroomPrice(w, in.AccessLevel, in.PriceCents) {
			return
		}
		next.AccessLevel = in.AccessLevel
		next.PriceCents = in.PriceCents
		reason = classroomAuditSummary("调整课程系列价格", actionBody.Reason)
	case "playback-blocked":
		next.PlaybackBlocked = actionBody.Blocked
		reason = classroomAuditSummary("调整课程系列停播状态", actionBody.Reason)
	default:
		httpx.Fail(w, 404, "Not Found")
		return
	}
	s.updateSeries(w, r, current, next, expected, action, reason)
}
func (s *Server) updateSeries(w http.ResponseWriter, r *http.Request, before, next classroom.Series, expected time.Time, action, summary string) {
	if expected.IsZero() {
		httpx.Fail(w, http.StatusBadRequest, "expectedUpdatedAt is required")
		return
	}
	uid := userFromRequest(r).ID
	next.UpdatedBy = &uid
	updated, err := s.classroomAdmin.UpdateSeries(r.Context(), next, expected)
	if err != nil {
		writeClassroomAdminError(w, err)
		return
	}
	if err := s.recordClassroomAudit(r, action, "classroom_series", updated.ID, before, updated, summary); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "record classroom audit failed")
		return
	}
	httpx.OK(w, toSeriesDTO(updated))
}

func (s *Server) classroomContentItem(w http.ResponseWriter, r *http.Request) {
	if !s.classroomReady(w) {
		return
	}
	id, action, ok := parseClassroomItemPath(r.URL.Path, "contents")
	if !ok {
		httpx.Fail(w, 400, "invalid content id")
		return
	}
	current, err := s.classroomAdmin.GetContent(r.Context(), id)
	if err != nil {
		writeClassroomAdminError(w, err)
		return
	}
	if action == "" && r.Method == http.MethodGet {
		dto, err := s.toContentDTO(r.Context(), current)
		if err != nil {
			writeClassroomAdminError(w, err)
			return
		}
		httpx.OK(w, dto)
		return
	}
	if action == "" && r.Method == http.MethodPut {
		if current.Status != classroom.ContentDraft {
			httpx.Fail(w, http.StatusConflict, "only draft content can be edited")
			return
		}
		var in contentWriteInput
		if !decodeClassroomJSON(w, r, &in) {
			return
		}
		next := in.content()
		next.ID = id
		next.Status = current.Status
		next.MediaAssetID = current.MediaAssetID
		next.AccessLevel = current.AccessLevel
		next.PriceCents = current.PriceCents
		next.PlaybackBlocked = current.PlaybackBlocked
		next.PublishedAt = current.PublishedAt
		next.CreatedBy = current.CreatedBy
		next.CreatedAt = current.CreatedAt
		s.updateContent(w, r, current, next, in.ExpectedUpdatedAt, "update", "更新课件草稿")
		return
	}
	if action == "" && r.Method == http.MethodDelete {
		expected, ok := expectedUpdatedAtFromQuery(r)
		if !ok {
			httpx.Fail(w, http.StatusBadRequest, "expectedUpdatedAt is required")
			return
		}
		if err := s.classroomAdmin.DeleteContent(r.Context(), id, expected); err != nil {
			writeClassroomAdminError(w, err)
			return
		}
		if err := s.recordClassroomAudit(r, "delete", "classroom_content", id, current, nil, classroomAuditSummary("删除课件草稿", r.URL.Query().Get("reason"))); err != nil {
			httpx.Fail(w, 500, "record classroom audit failed")
			return
		}
		httpx.OK(w, map[string]any{"deleted": true})
		return
	}
	s.mutateContent(w, r, current, action)
}
func (s *Server) mutateContent(w http.ResponseWriter, r *http.Request, current classroom.Content, action string) {
	if r.Method != http.MethodPost {
		httpx.Fail(w, 405, "Method Not Allowed")
		return
	}
	next := current
	var expected time.Time
	var actionBody classroomActionInput
	if !decodeClassroomJSON(w, r, &actionBody) {
		return
	}
	if actionBody.ExpectedUpdatedAt.IsZero() {
		httpx.Fail(w, http.StatusBadRequest, "expectedUpdatedAt is required")
		return
	}
	expected = actionBody.ExpectedUpdatedAt
	var reason string
	switch action {
	case "publish":
		next.Status = classroom.ContentPublished
		now := time.Now()
		next.PublishedAt = &now
		reason = classroomAuditSummary("发布课件", actionBody.Reason)
	case "offline":
		next.Status = classroom.ContentOffline
		reason = classroomAuditSummary("下线课件", actionBody.Reason)
	case "price":
		var in priceInput
		in = priceInput{AccessLevel: actionBody.AccessLevel, PriceCents: actionBody.PriceCents, ExpectedUpdatedAt: expected}
		if !validClassroomPriceForContent(w, current.SeriesID, in.AccessLevel, in.PriceCents) {
			return
		}
		next.AccessLevel = in.AccessLevel
		next.PriceCents = in.PriceCents
		reason = classroomAuditSummary("调整课件价格", actionBody.Reason)
	case "playback-blocked":
		next.PlaybackBlocked = actionBody.Blocked
		reason = classroomAuditSummary("调整课件停播状态", actionBody.Reason)
	default:
		httpx.Fail(w, 404, "Not Found")
		return
	}
	s.updateContent(w, r, current, next, expected, action, reason)
}
func (s *Server) updateContent(w http.ResponseWriter, r *http.Request, before, next classroom.Content, expected time.Time, action, summary string) {
	if expected.IsZero() {
		httpx.Fail(w, http.StatusBadRequest, "expectedUpdatedAt is required")
		return
	}
	uid := userFromRequest(r).ID
	next.UpdatedBy = &uid
	updated, err := s.classroomAdmin.UpdateContent(r.Context(), next, expected)
	if err != nil {
		writeClassroomAdminError(w, err)
		return
	}
	if err := s.recordClassroomAudit(r, action, "classroom_content", updated.ID, before, updated, summary); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "record classroom audit failed")
		return
	}
	dto, err := s.toContentDTO(r.Context(), updated)
	if err != nil {
		writeClassroomAdminError(w, err)
		return
	}
	httpx.OK(w, dto)
}

func (s *Server) classroomUploadTasks(w http.ResponseWriter, r *http.Request) {
	if !s.classroomReady(w) {
		return
	}
	if r.Method != http.MethodGet {
		httpx.Fail(w, 405, "Method Not Allowed")
		return
	}
	page, size, err := adminClassroomPage(r)
	if err != nil {
		httpx.Fail(w, 400, "invalid pagination")
		return
	}
	items, total, err := s.classroomAdmin.ListUploadTasks(r.Context(), size, (page-1)*size)
	if err != nil {
		writeClassroomAdminError(w, err)
		return
	}
	out := make([]classroomUploadTaskDTO, 0, len(items))
	for _, v := range items {
		out = append(out, classroomUploadTaskDTO{v.ID, v.ContentID, v.ExpectedSize, v.PartSize, v.MaxParts, v.Status, v.ExpiresAt, v.AttemptCount, v.CleanupStatus, v.MediaAssetID, v.FailureReason, v.CreatedAt, v.UpdatedAt})
	}
	httpx.OK(w, classroomPage[classroomUploadTaskDTO]{out, total, page, size})
}

type seriesWriteInput struct {
	Title             string    `json:"title"`
	Summary           string    `json:"summary"`
	CoverURL          string    `json:"coverUrl"`
	CoverAssetID      *int64    `json:"coverAssetId"`
	TeacherKey        string    `json:"teacherKey"`
	TeacherName       string    `json:"teacherName"`
	SortOrder         int       `json:"sortOrder"`
	ExpectedUpdatedAt time.Time `json:"expectedUpdatedAt"`
}

func (i seriesWriteInput) series() classroom.Series {
	return classroom.Series{Title: strings.TrimSpace(i.Title), Summary: i.Summary, CoverURL: i.CoverURL, CoverAssetID: i.CoverAssetID, TeacherKey: i.TeacherKey, TeacherNameSnapshot: i.TeacherName, SortOrder: i.SortOrder, Status: classroom.SeriesDraft, AccessLevel: classroom.AccessPublic}
}

type contentWriteInput struct {
	SeriesID          *int64                `json:"seriesId"`
	ShowAsStandalone  bool                  `json:"showAsStandalone"`
	Title             string                `json:"title"`
	Description       string                `json:"description"`
	ContentType       classroom.ContentType `json:"contentType"`
	CoverURL          string                `json:"coverUrl"`
	DurationSeconds   int                   `json:"durationSeconds"`
	TeacherKey        string                `json:"teacherKey"`
	TeacherName       string                `json:"teacherName"`
	RecordedAt        *time.Time            `json:"recordedAt"`
	Badge             string                `json:"badge"`
	Tags              []string              `json:"tags"`
	EpisodeNo         int                   `json:"episodeNo"`
	SortOrder         int                   `json:"sortOrder"`
	ExpectedUpdatedAt time.Time             `json:"expectedUpdatedAt"`
}

func (i contentWriteInput) content() classroom.Content {
	return classroom.Content{SeriesID: i.SeriesID, ShowAsStandalone: i.ShowAsStandalone, Title: strings.TrimSpace(i.Title), Description: i.Description, ContentType: i.ContentType, CoverURL: i.CoverURL, DurationSeconds: i.DurationSeconds, TeacherKey: i.TeacherKey, TeacherNameSnapshot: i.TeacherName, RecordedAt: i.RecordedAt, Badge: i.Badge, Tags: i.Tags, EpisodeNo: i.EpisodeNo, SortOrder: i.SortOrder, Status: classroom.ContentDraft, AccessLevel: classroom.AccessPublic}
}

type priceInput struct {
	AccessLevel       classroom.AccessLevel `json:"accessLevel"`
	PriceCents        int                   `json:"priceCents"`
	ExpectedUpdatedAt time.Time             `json:"expectedUpdatedAt"`
}
type blockInput struct {
	Blocked           bool      `json:"blocked"`
	ExpectedUpdatedAt time.Time `json:"expectedUpdatedAt"`
}
type classroomActionInput struct {
	ExpectedUpdatedAt time.Time             `json:"expectedUpdatedAt"`
	Reason            string                `json:"reason"`
	AccessLevel       classroom.AccessLevel `json:"accessLevel"`
	PriceCents        int                   `json:"priceCents"`
	Blocked           bool                  `json:"blocked"`
}

func validClassroomPrice(w http.ResponseWriter, access classroom.AccessLevel, price int) bool {
	if access == classroom.AccessInherit || price < 0 || price > maxClassroomPriceCents || (access == classroom.AccessPaid && price <= 0) || (access != classroom.AccessPaid && price != 0) {
		httpx.Fail(w, 400, "invalid CNY price")
		return false
	}
	return true
}
func validClassroomPriceForContent(w http.ResponseWriter, seriesID *int64, access classroom.AccessLevel, price int) bool {
	if seriesID == nil && access == classroom.AccessInherit {
		httpx.Fail(w, 400, "standalone content cannot inherit access")
		return false
	}
	if access == classroom.AccessInherit {
		if price != 0 {
			httpx.Fail(w, http.StatusBadRequest, "inherited access must have zero price")
			return false
		}
		return true
	}
	return validClassroomPrice(w, access, price)
}
func decodeClassroomJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		httpx.Fail(w, 400, "invalid JSON payload")
		return false
	}
	if ensureJSONEOF(d) != nil {
		httpx.Fail(w, 400, "invalid JSON payload")
		return false
	}
	return true
}
func parseClassroomItemPath(path, kind string) (int64, string, bool) {
	prefix := "/api/admin/classroom/" + kind + "/"
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	parts := strings.Split(rest, "/")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, "", false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	return id, action, true
}
func expectedUpdatedAtFromQuery(r *http.Request) (time.Time, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("expectedUpdatedAt"))
	if raw == "" {
		return time.Time{}, false
	}
	v, err := time.Parse(time.RFC3339Nano, raw)
	return v, err == nil && !v.IsZero()
}
func classroomAuditSummary(action, reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return action
	}
	return action + "：" + reason
}
func toSeriesDTO(v classroom.Series) classroomSeriesDTO {
	return classroomSeriesDTO{v.ID, v.Title, v.Summary, v.CoverURL, v.TeacherKey, v.TeacherNameSnapshot, v.SortOrder, v.Status, v.PlaybackBlocked, v.AccessLevel, v.PriceCents, v.PublishedAt, v.CreatedAt, v.UpdatedAt}
}
func (s *Server) toContentDTO(ctx context.Context, v classroom.Content) (classroomContentDTO, error) {
	effective, price, target := v.AccessLevel, v.PriceCents, ""
	if v.AccessLevel == classroom.AccessInherit && v.SeriesID != nil {
		parent, err := s.classroomAdmin.GetSeries(ctx, *v.SeriesID)
		if err != nil {
			return classroomContentDTO{}, err
		}
		effective = parent.AccessLevel
		price = parent.PriceCents
	}
	if effective == classroom.AccessPaid {
		if v.AccessLevel == classroom.AccessInherit {
			target = "series"
		} else {
			target = "content"
		}
	}
	return classroomContentDTO{v.ID, v.SeriesID, v.ShowAsStandalone, v.Title, v.Description, v.ContentType, v.MediaAssetID, v.CoverURL, v.DurationSeconds, v.TeacherKey, v.TeacherNameSnapshot, v.RecordedAt, v.Badge, v.Tags, v.EpisodeNo, v.SortOrder, v.Status, v.PlaybackBlocked, v.AccessLevel, effective, v.PriceCents, price, target, v.PublishedAt, v.CreatedAt, v.UpdatedAt}, nil
}
func writeClassroomAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, classroom.ErrNotFound):
		httpx.Fail(w, 404, "classroom record not found")
	case errors.Is(err, classroom.ErrConflict):
		httpx.Fail(w, 409, "classroom record was modified")
	case strings.Contains(err.Error(), "only draft"), strings.Contains(err.Error(), "dependent"), strings.Contains(err.Error(), "dependencies"):
		httpx.Fail(w, http.StatusConflict, err.Error())
	default:
		var status = 500
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must") || strings.Contains(err.Error(), "price") {
			status = 400
		}
		httpx.Fail(w, status, err.Error())
	}
}
func (s *Server) recordClassroomAudit(r *http.Request, action, target string, id int64, before, after any, summary string) error {
	if s.classroomAudit == nil {
		if s.auditLogs == nil {
			return nil
		}
		s.classroomAudit = s.auditLogs
	}
	u := userFromRequest(r)
	return s.classroomAudit.Record(r.Context(), auditlog.Entry{OperatorID: u.ID, OperatorName: firstNonEmpty(strings.TrimSpace(u.RealName), strings.TrimSpace(u.Username), strconv.FormatInt(u.ID, 10)), Action: action, TargetType: target, TargetID: strconv.FormatInt(id, 10), IP: r.RemoteAddr, UserAgent: r.UserAgent(), Before: before, After: after, Summary: summary})
}
