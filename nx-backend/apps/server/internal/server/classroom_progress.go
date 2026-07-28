package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/storage"
)

const (
	maxClassroomProgressPositionSeconds    = 30 * 24 * 60 * 60
	maxClassroomContinueLearningItems      = 20
	classroomContinueLearningBatchSize     = 40
	classroomContinueLearningMaxBatches    = 5
	classroomContinueLearningMaxCandidates = classroomContinueLearningBatchSize * classroomContinueLearningMaxBatches
)

type classroomProgressView struct {
	ContentID       int64     `json:"contentId"`
	PositionSeconds int       `json:"positionSeconds"`
	Completed       bool      `json:"completed"`
	LastPlayedAt    time.Time `json:"lastPlayedAt"`
}

type classroomContinueLearningItem struct {
	classroomPublicContent
	PositionSeconds int       `json:"positionSeconds"`
	Completed       bool      `json:"completed"`
	LastPlayedAt    time.Time `json:"lastPlayedAt"`
}

type classroomProgressService interface {
	Update(context.Context, int64, int64, int) (classroomProgressView, error)
	ContinueLearning(context.Context, int64) ([]classroomContinueLearningItem, error)
}

type classroomProgressStore interface {
	UpsertProgress(context.Context, classroom.Progress) (classroom.Progress, error)
}

type classroomProgressAccess interface {
	Playback(context.Context, int64, int64) (classroomPlaybackSource, error)
}

type classroomProgressDB struct {
	db           *sql.DB
	store        classroomProgressStore
	access       classroomProgressAccess
	loadSnapshot func(context.Context, int64) (classroomAccessSnapshot, error)
	public       *classroomPublicDB
	now          func() time.Time
}

func newClassroomProgressDB(db *sql.DB) classroomProgressService {
	return newClassroomProgressDBWithCovers(db, nil, 0)
}

func newClassroomProgressDBWithCovers(db *sql.DB, signer storage.ObjectSigner, ttl time.Duration) classroomProgressService {
	if db == nil {
		return nil
	}
	public := newClassroomPublicDBWithCovers(db, signer, ttl).(*classroomPublicDB)
	return &classroomProgressDB{
		db:           db,
		store:        classroom.NewStore(db),
		access:       public,
		loadSnapshot: public.loadAccessSnapshot,
		public:       public,
		now:          time.Now,
	}
}

func registerClassroomProgressRoutes(mux *http.ServeMux, authn func(http.HandlerFunc) http.HandlerFunc, s *Server) {
	mux.HandleFunc("/api/miniapp/classroom/continue-learning", s.method(http.MethodGet, authn(s.classroomContinueLearning)))
}

func (s *Server) classroomProgressUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if s.classroomProgress == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "Classroom progress unavailable")
		return
	}
	contentID, err := classroomID(r.URL.Path, "/api/miniapp/classroom/content/", "/progress")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	var body struct {
		PositionSeconds *int `json:"positionSeconds"`
	}
	if err = decodeClassroomPublicJSON(w, r, &body, false); err != nil || body.PositionSeconds == nil || *body.PositionSeconds < 0 || *body.PositionSeconds > maxClassroomProgressPositionSeconds {
		httpx.Fail(w, http.StatusBadRequest, "Invalid progress payload")
		return
	}
	uid := userFromRequest(r).ID
	if s.classroomProgressLimiter != nil && !s.classroomProgressLimiter.Allow(fmt.Sprintf("%d:%d", uid, contentID), s.nowTime()) {
		httpx.Fail(w, http.StatusTooManyRequests, "Too Many Requests")
		return
	}
	result, err := s.classroomProgress.Update(r.Context(), uid, contentID, *body.PositionSeconds)
	if err != nil {
		writeClassroomProgressError(w, err)
		return
	}
	httpx.OK(w, result)
}

func (s *Server) classroomContinueLearning(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if s.classroomProgress == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "Classroom progress unavailable")
		return
	}
	uid := userFromRequest(r).ID
	if s.classroomContinueLimiter != nil && !s.classroomContinueLimiter.Allow(fmt.Sprintf("%d", uid), s.nowTime()) {
		httpx.Fail(w, http.StatusTooManyRequests, "Too Many Requests")
		return
	}
	items, err := s.classroomProgress.ContinueLearning(r.Context(), uid)
	if err != nil {
		writeClassroomProgressError(w, err)
		return
	}
	httpx.OK(w, map[string]any{"items": items})
}

func writeClassroomProgressError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errClassroomPlaybackBlocked):
		httpx.Fail(w, http.StatusLocked, "Playback Blocked")
	case errors.Is(err, classroom.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		httpx.Fail(w, http.StatusNotFound, "Not Found")
	default:
		failClassroomInternal(w, "progress", err)
	}
}

func (d *classroomProgressDB) Update(ctx context.Context, uid, contentID int64, positionSeconds int) (classroomProgressView, error) {
	if uid <= 0 || contentID <= 0 || positionSeconds < 0 || positionSeconds > maxClassroomProgressPositionSeconds {
		return classroomProgressView{}, errors.New("invalid classroom progress")
	}
	source, err := d.access.Playback(ctx, uid, contentID)
	if err != nil {
		return classroomProgressView{}, err
	}
	duration := source.Media.DurationSeconds
	if duration > 0 && positionSeconds > duration {
		positionSeconds = duration
	}
	completed := duration > 0 && int64(positionSeconds)*10 >= int64(duration)*9
	now := time.Now()
	if d.now != nil {
		now = d.now()
	}
	stored, err := d.store.UpsertProgress(ctx, classroom.Progress{
		WXUserID: uid, ContentID: contentID, PositionSeconds: positionSeconds, Completed: completed, LastPlayedAt: now,
	})
	if err != nil {
		return classroomProgressView{}, err
	}
	return classroomProgressView{ContentID: stored.ContentID, PositionSeconds: stored.PositionSeconds, Completed: stored.Completed, LastPlayedAt: stored.LastPlayedAt}, nil
}

func (d *classroomProgressDB) ContinueLearning(ctx context.Context, uid int64) ([]classroomContinueLearningItem, error) {
	if uid <= 0 || d.db == nil || d.loadSnapshot == nil {
		return nil, errors.New("invalid classroom progress viewer")
	}
	snapshot, err := d.loadSnapshot(ctx, uid)
	if err != nil {
		return nil, err
	}
	items := make([]classroomContinueLearningItem, 0, maxClassroomContinueLearningItems)
	var cursorAt *time.Time
	var cursorID int64
	candidates := 0
	for batch := 0; batch < classroomContinueLearningMaxBatches && candidates < classroomContinueLearningMaxCandidates && len(items) < maxClassroomContinueLearningItems; batch++ {
		limit := min(classroomContinueLearningBatchSize, classroomContinueLearningMaxCandidates-candidates)
		rows, queryErr := d.db.QueryContext(ctx, `SELECT
			p.content_id,p.position_seconds,p.completed,p.last_played_at,
			c.title,c.description,c.cover_url,c.content_type,c.duration_seconds,c.series_id,c.show_as_standalone,c.status,c.playback_blocked,c.access_level,c.price_cents,c.manual_cover_object_key,c.cover_aspect_ratio,m.cover_object_key,
			s.id,s.status,s.playback_blocked,s.access_level,s.price_cents,c.teacher_name_snapshot
			FROM classroom_progress p
			JOIN classroom_contents c ON c.id=p.content_id
			JOIN classroom_media_assets m ON m.id=c.media_asset_id
			LEFT JOIN classroom_series s ON s.id=c.series_id
			WHERE p.wx_user_id=$1
			AND m.storage_status=$2
			AND c.status IN ($3,$4)
			AND c.playback_blocked=false
			AND (s.id IS NULL OR s.playback_blocked=false)
			AND (s.id IS NULL OR c.show_as_standalone=true OR s.status IN ($5,$6))
			AND ($7::timestamptz IS NULL OR p.last_played_at < $7 OR (p.last_played_at=$7 AND c.id < $8))
			ORDER BY p.last_played_at DESC,c.id DESC
			LIMIT $9`, uid, classroom.MediaReady, classroom.ContentPublished, classroom.ContentOffline, classroom.SeriesPublished, classroom.SeriesOffline, cursorAt, cursorID, limit)
		if queryErr != nil {
			return nil, queryErr
		}

		batchCandidates := 0
		for rows.Next() {
			var (
				content                    classroom.Content
				position                   int
				completed                  bool
				lastPlayedAt               time.Time
				parentID, parentPrice      sql.NullInt64
				parentStatus, parentAccess sql.NullString
				parentBlocked              sql.NullBool
				generatedCover             string
			)
			if err = rows.Scan(
				&content.ID, &position, &completed, &lastPlayedAt,
				&content.Title, &content.Description, &content.CoverURL, &content.ContentType, &content.DurationSeconds, &content.SeriesID, &content.ShowAsStandalone, &content.Status, &content.PlaybackBlocked, &content.AccessLevel, &content.PriceCents, &content.ManualCoverObjectKey, &content.CoverAspectRatio, &generatedCover,
				&parentID, &parentStatus, &parentBlocked, &parentAccess, &parentPrice, &content.TeacherNameSnapshot,
			); err != nil {
				_ = rows.Close()
				return nil, err
			}
			batchCandidates++
			candidates++
			cursorAt, cursorID = &lastPlayedAt, content.ID

			var parent *classroom.Series
			if parentID.Valid {
				parent = &classroom.Series{ID: parentID.Int64, Status: classroom.SeriesStatus(parentStatus.String), PlaybackBlocked: parentBlocked.Bool, AccessLevel: classroom.AccessLevel(parentAccess.String), PriceCents: int(parentPrice.Int64)}
			}
			if !classroomPlaybackAccessible(content, parent, snapshot) {
				continue
			}
			signedCover, resolveErr := d.public.resolveContentCover(ctx, &content, generatedCover)
			if resolveErr != nil {
				_ = rows.Close()
				return nil, resolveErr
			}
			access := accessFor(content.AccessLevel, parent)
			view := contentViewResolved(content, parent, access, true, signedCover)
			items = append(items, classroomContinueLearningItem{classroomPublicContent: view, PositionSeconds: position, Completed: completed, LastPlayedAt: lastPlayedAt})
			if len(items) == maxClassroomContinueLearningItems {
				break
			}
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err = rows.Close(); err != nil {
			return nil, err
		}
		if batchCandidates < limit {
			break
		}
	}
	return items, nil
}
