package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
)

type classroomPublicQuery struct {
	Limit, Offset int
	ContentType   classroom.ContentType
}
type classroomPublicSeries struct {
	ID              int64                 `json:"id"`
	Title           string                `json:"title"`
	Summary         string                `json:"summary,omitempty"`
	CoverURL        string                `json:"coverUrl,omitempty"`
	TeacherName     string                `json:"teacherName,omitempty"`
	EffectiveAccess classroom.AccessLevel `json:"effectiveAccess"`
	PriceCents      int                   `json:"priceCents"`
	CanPlay         bool                  `json:"canPlay"`
	PurchaseState   string                `json:"purchaseState"`
	PlaybackBlocked bool                  `json:"playbackBlocked"`
}
type classroomPublicContent struct {
	ID              int64                 `json:"id"`
	SeriesID        *int64                `json:"seriesId,omitempty"`
	Title           string                `json:"title"`
	Description     string                `json:"description,omitempty"`
	CoverURL        string                `json:"coverUrl,omitempty"`
	TeacherName     string                `json:"teacherName,omitempty"`
	ContentType     classroom.ContentType `json:"contentType"`
	DurationSeconds int                   `json:"durationSeconds"`
	EffectiveAccess classroom.AccessLevel `json:"effectiveAccess"`
	PriceCents      int                   `json:"priceCents"`
	CanPlay         bool                  `json:"canPlay"`
	PurchaseState   string                `json:"purchaseState"`
	PlaybackBlocked bool                  `json:"playbackBlocked"`
}
type classroomPublicSeriesDetail struct {
	Series   classroomPublicSeries    `json:"series"`
	Contents []classroomPublicContent `json:"contents"`
}
type classroomPlaybackSource struct {
	Content classroom.Content
	Media   classroom.MediaAsset
	Series  *classroom.Series
}
type classroomPublicService interface {
	ListSeries(context.Context, classroomPublicQuery, int64) ([]classroomPublicSeries, int, error)
	ListStandalone(context.Context, classroomPublicQuery, int64) ([]classroomPublicContent, int, error)
	GetSeries(context.Context, int64, int64) (classroomPublicSeriesDetail, error)
	GetContent(context.Context, int64, int64) (classroomPublicContent, error)
	Playback(ctx context.Context, userID, contentID int64) (classroomPlaybackSource, error)
}

type classroomTicketClaims struct {
	ContentID    int64     `json:"content_id"`
	MediaVersion string    `json:"media_version"`
	ExpiresAt    time.Time `json:"exp"`
	Nonce        string    `json:"nonce"`
}

var (
	errClassroomTicket          = errors.New("invalid classroom playback ticket")
	errClassroomPlaybackBlocked = errors.New("classroom playback blocked")
)

func (s *Server) nowTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}
func (s *Server) signClassroomTicket(contentID int64, mediaVersion string) (string, classroomTicketClaims, error) {
	now := s.nowTime()
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", classroomTicketClaims{}, err
	}
	c := classroomTicketClaims{ContentID: contentID, MediaVersion: mediaVersion, ExpiresAt: now.Add(5 * time.Minute), Nonce: base64.RawURLEncoding.EncodeToString(nonce)}
	b, _ := json.Marshal(c)
	raw := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, []byte(s.env.JWTSecret))
	_, _ = mac.Write([]byte(raw))
	return raw + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), c, nil
}
func (s *Server) verifyClassroomTicket(token string, contentID int64, mediaVersion string) (classroomTicketClaims, error) {
	p := strings.Split(token, ".")
	if len(p) != 2 {
		return classroomTicketClaims{}, errClassroomTicket
	}
	mac := hmac.New(sha256.New, []byte(s.env.JWTSecret))
	_, _ = mac.Write([]byte(p[0]))
	sig, err := base64.RawURLEncoding.DecodeString(p[1])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return classroomTicketClaims{}, errClassroomTicket
	}
	b, err := base64.RawURLEncoding.DecodeString(p[0])
	if err != nil {
		return classroomTicketClaims{}, errClassroomTicket
	}
	var c classroomTicketClaims
	if json.Unmarshal(b, &c) != nil || c.ContentID != contentID || c.MediaVersion != mediaVersion || !s.nowTime().Before(c.ExpiresAt) {
		return classroomTicketClaims{}, errClassroomTicket
	}
	return c, nil
}

func registerClassroomPublicRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("/api/public/classroom/series", s.method(http.MethodGet, s.classroomSeriesPublic))
	mux.HandleFunc("/api/public/classroom/standalone", s.method(http.MethodGet, s.classroomStandalonePublic))
	mux.HandleFunc("/api/public/classroom/series/", s.method(http.MethodGet, s.classroomSeriesDetailPublic))
	mux.HandleFunc("/api/public/classroom/content/", s.classroomContentPublicRouter)
	mux.HandleFunc("/api/miniapp/classroom/content/", s.classroomPlaybackPublic)
}
func (s *Server) classroomContentPublicRouter(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/ticket") {
		if r.Method != http.MethodPost {
			httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		s.classroomAnonymousTicket(w, r)
		return
	}
	if r.Method != http.MethodGet {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	s.classroomContentPublic(w, r)
}

func (s *Server) optionalMiniapp(r *http.Request) (auth.UserInfo, bool) {
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		return auth.UserInfo{}, false
	}
	u, err := auth.BearerUserWithKind(r.Header.Get("Authorization"), s.env.JWTSecret, auth.TokenKindMiniapp)
	if err != nil || u.ID <= 0 {
		return auth.UserInfo{}, false
	}
	return u, true
}
func (s *Server) classroomViewer(w http.ResponseWriter, r *http.Request) (auth.UserInfo, bool, bool) {
	u, ok := s.optionalMiniapp(r)
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" && !ok {
		httpx.Fail(w, http.StatusUnauthorized, "Unauthorized Exception")
		return auth.UserInfo{}, false, false
	}
	return u, ok, true
}
func classroomPublicPage(r *http.Request) classroomPublicQuery {
	q := r.URL.Query()
	l, _ := strconv.Atoi(q.Get("limit"))
	o, _ := strconv.Atoi(q.Get("offset"))
	if l <= 0 || l > 100 {
		l = 20
	}
	if o < 0 {
		o = 0
	}
	return classroomPublicQuery{Limit: l, Offset: o, ContentType: classroom.ContentType(q.Get("contentType"))}
}
func setClassroomCache(w http.ResponseWriter, r *http.Request, body any) bool {
	b, _ := json.Marshal(body)
	h := sha256.Sum256(b)
	etag := `"` + fmt.Sprintf("%x", h[:8]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Vary", "Authorization")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}
func (s *Server) classroomSeriesPublic(w http.ResponseWriter, r *http.Request) {
	if s.classroomPublic == nil {
		httpx.Fail(w, 500, "classroom unavailable")
		return
	}
	u, _, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	items, total, err := s.classroomPublic.ListSeries(r.Context(), classroomPublicPage(r), u.ID)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	data := map[string]any{"items": items, "total": total, "limit": classroomPublicPage(r).Limit, "offset": classroomPublicPage(r).Offset}
	if setClassroomCache(w, r, data) {
		return
	}
	httpx.OK(w, data)
}
func (s *Server) classroomStandalonePublic(w http.ResponseWriter, r *http.Request) {
	if s.classroomPublic == nil {
		httpx.Fail(w, 500, "classroom unavailable")
		return
	}
	u, _, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	items, total, err := s.classroomPublic.ListStandalone(r.Context(), classroomPublicPage(r), u.ID)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	page := classroomPublicPage(r)
	data := map[string]any{"items": items, "total": total, "limit": page.Limit, "offset": page.Offset}
	if setClassroomCache(w, r, data) {
		return
	}
	httpx.OK(w, data)
}
func (s *Server) classroomSeriesDetailPublic(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/public/classroom/series/"), "/"), 10, 64)
	u, _, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	d, err := s.classroomPublic.GetSeries(r.Context(), id, u.ID)
	if err != nil {
		httpx.Fail(w, 404, "Not Found")
		return
	}
	if setClassroomCache(w, r, d) {
		return
	}
	httpx.OK(w, d)
}
func (s *Server) classroomContentPublic(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/public/classroom/content/"), "/"), 10, 64)
	u, _, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	d, err := s.classroomPublic.GetContent(r.Context(), id, u.ID)
	if err != nil {
		httpx.Fail(w, 404, "Not Found")
		return
	}
	if setClassroomCache(w, r, d) {
		return
	}
	httpx.OK(w, d)
}
func (s *Server) classroomAnonymousTicket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	idPath := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/public/classroom/content/"), "/")
	idPath = strings.TrimSuffix(idPath, "/ticket")
	id, _ := strconv.ParseInt(idPath, 10, 64)
	if _, logged, valid := s.classroomViewer(w, r); !valid {
		return
	} else if logged {
		httpx.Fail(w, http.StatusBadRequest, "JWT playback does not require an anonymous ticket")
		return
	}
	d, err := s.classroomPublic.GetContent(r.Context(), id, 0)
	if err != nil || d.EffectiveAccess != classroom.AccessPublic || !d.CanPlay {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	key := "ticket:" + s.clientIP(r) + ":" + r.Header.Get("X-Device-ID") + ":" + strconv.FormatInt(id, 10)
	if s.classroomPlaybackLimiter != nil && !s.classroomPlaybackLimiter.Allow(key, s.nowTime()) {
		httpx.Fail(w, http.StatusTooManyRequests, "Too Many Requests")
		return
	}
	src, err := s.classroomPublic.Playback(r.Context(), 0, id)
	if err != nil {
		if errors.Is(err, errClassroomPlaybackBlocked) {
			httpx.Fail(w, http.StatusLocked, "Playback Blocked")
			return
		}
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	ticket, _, err := s.signClassroomTicket(id, src.Media.ETag)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ticket unavailable")
		return
	}
	httpx.OK(w, map[string]any{"ticket": ticket, "expiresIn": 300})
}
func (s *Server) classroomPlaybackPublic(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" {
		httpx.Fail(w, 405, "Method Not Allowed")
		return
	}
	id, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/miniapp/classroom/content/"), "/play"), 10, 64)
	u, logged, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	src, err := s.classroomPublic.Playback(r.Context(), u.ID, id)
	if err != nil {
		if errors.Is(err, errClassroomPlaybackBlocked) {
			httpx.Fail(w, http.StatusLocked, "Playback Blocked")
			return
		}
		httpx.Fail(w, 404, "Not Found")
		return
	}
	if src.Content.PlaybackBlocked || (src.Series != nil && src.Series.PlaybackBlocked) {
		httpx.Fail(w, 423, "Playback Blocked")
		return
	}
	if !logged {
		key := s.clientIP(r) + ":" + r.Header.Get("X-Device-ID") + ":" + strconv.FormatInt(id, 10)
		if s.classroomPlaybackLimiter != nil && !s.classroomPlaybackLimiter.Allow(key, s.nowTime()) {
			httpx.Fail(w, http.StatusTooManyRequests, "Too Many Requests")
			return
		}
		var body struct {
			Ticket string `json:"ticket"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, err = s.verifyClassroomTicket(body.Ticket, id, src.Media.ETag); err != nil {
			httpx.Fail(w, 401, "Unauthorized")
			return
		}
	}
	signer := s.classroomPlaybackSigner
	if signer == nil {
		httpx.Fail(w, 503, "Playback unavailable")
		return
	}
	url, err := signer.PresignGetURL(r.Context(), src.Media.ObjectKey, 5*time.Minute)
	if err != nil {
		httpx.Fail(w, 503, "Playback unavailable")
		return
	}
	httpx.OK(w, map[string]any{"url": url, "expiresIn": 300, "contentType": src.Content.ContentType})
}

// db-backed implementation is installed by New when classroom tables are available.
type classroomPublicDB struct {
	store *classroom.Store
	db    *sql.DB
}

func newClassroomPublicDB(db *sql.DB) classroomPublicService {
	return &classroomPublicDB{store: classroom.NewStore(db), db: db}
}
func (d *classroomPublicDB) member(ctx context.Context, uid int64) bool {
	if uid <= 0 {
		return false
	}
	var level int
	var exp *time.Time
	if d.db.QueryRowContext(ctx, "SELECT member_level,member_expires_at FROM wx_users WHERE id=$1", uid).Scan(&level, &exp) != nil {
		return false
	}
	return miniapp.IsMembershipActive(level, exp, time.Now())
}
func (d *classroomPublicDB) entitled(ctx context.Context, uid, contentID int64, seriesID *int64) bool {
	if uid <= 0 {
		return false
	}
	var n int
	q := `SELECT count(*) FROM classroom_entitlements WHERE wx_user_id=$1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()) AND ((content_id=$2) OR (series_id IS NOT NULL AND series_id=$3))`
	sid := int64(0)
	if seriesID != nil {
		sid = *seriesID
	}
	if d.db.QueryRowContext(ctx, q, uid, contentID, sid).Scan(&n) != nil {
		return false
	}
	return n > 0
}
func (d *classroomPublicDB) seriesEntitled(ctx context.Context, uid, id int64) bool {
	if uid <= 0 {
		return false
	}
	var n int
	return d.db.QueryRowContext(ctx, `SELECT count(*) FROM classroom_entitlements WHERE wx_user_id=$1 AND series_id=$2 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now())`, uid, id).Scan(&n) == nil && n > 0
}
func accessFor(a classroom.AccessLevel, parent *classroom.Series) classroom.AccessLevel {
	if a == classroom.AccessInherit && parent != nil {
		return parent.AccessLevel
	}
	return a
}
func classroomPurchaseState(access classroom.AccessLevel, canPlay bool) string {
	if access != classroom.AccessPaid {
		return "available"
	}
	if canPlay {
		return "owned"
	}
	return "purchase_required"
}
func classroomAccessAllowed(access classroom.AccessLevel, loggedIn, member, entitled bool) bool {
	switch access {
	case classroom.AccessPublic:
		return true
	case classroom.AccessLogin:
		return loggedIn
	case classroom.AccessMember:
		return member
	case classroom.AccessPaid:
		return entitled
	}
	return false
}
func canAccess(ctx context.Context, d *classroomPublicDB, a classroom.AccessLevel, uid int64, c classroom.Content, parent *classroom.Series) bool {
	member, entitled := false, false
	if a == classroom.AccessMember {
		member = d.member(ctx, uid)
	}
	if a == classroom.AccessPaid {
		entitled = d.entitled(ctx, uid, c.ID, c.SeriesID)
	}
	return classroomAccessAllowed(a, uid > 0, member, entitled)
}
func (d *classroomPublicDB) contentView(ctx context.Context, c classroom.Content, uid int64, parent *classroom.Series) (classroomPublicContent, error) {
	if c.Status != classroom.ContentPublished || c.MediaAssetID == nil {
		return classroomPublicContent{}, classroom.ErrNotFound
	}
	m, e := d.store.GetMediaAsset(ctx, *c.MediaAssetID)
	if e != nil || m.StorageStatus != classroom.MediaReady {
		return classroomPublicContent{}, classroom.ErrNotFound
	}
	return d.contentViewReady(ctx, c, uid, parent), nil
}
func (d *classroomPublicDB) contentViewReady(ctx context.Context, c classroom.Content, uid int64, parent *classroom.Series) classroomPublicContent {
	a := accessFor(c.AccessLevel, parent)
	ok := canAccess(ctx, d, a, uid, c, parent)
	pstate := "available"
	if a == classroom.AccessPaid && !ok {
		pstate = "purchase_required"
	} else if a == classroom.AccessPaid {
		pstate = "owned"
	}
	price := c.PriceCents
	if c.AccessLevel == classroom.AccessInherit && parent != nil {
		price = parent.PriceCents
	}
	blocked := c.PlaybackBlocked || (parent != nil && parent.PlaybackBlocked)
	v := classroomPublicContent{ID: c.ID, SeriesID: c.SeriesID, Title: c.Title, Description: c.Description, CoverURL: c.CoverURL, TeacherName: c.TeacherNameSnapshot, ContentType: c.ContentType, DurationSeconds: c.DurationSeconds, EffectiveAccess: a, PriceCents: price, CanPlay: ok && !blocked, PurchaseState: pstate, PlaybackBlocked: blocked}
	return v
}
func (d *classroomPublicDB) ListSeries(ctx context.Context, q classroomPublicQuery, uid int64) ([]classroomPublicSeries, int, error) {
	const eligible = `s.status=$1 AND EXISTS (SELECT 1 FROM classroom_contents c JOIN classroom_media_assets m ON m.id=c.media_asset_id WHERE c.series_id=s.id AND c.status=$2 AND m.storage_status=$3 AND ($4='' OR c.content_type=$4))`
	args := []any{classroom.SeriesPublished, classroom.ContentPublished, classroom.MediaReady, q.ContentType}
	var total int
	if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM classroom_series s WHERE `+eligible, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT s.id,s.title,s.summary,s.cover_url,s.teacher_name_snapshot,s.access_level,s.price_cents,s.playback_blocked FROM classroom_series s WHERE `+eligible+` ORDER BY s.sort_order,s.id LIMIT $5 OFFSET $6`, append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]classroomPublicSeries, 0, q.Limit)
	for rows.Next() {
		var v classroomPublicSeries
		if err = rows.Scan(&v.ID, &v.Title, &v.Summary, &v.CoverURL, &v.TeacherName, &v.EffectiveAccess, &v.PriceCents, &v.PlaybackBlocked); err != nil {
			return nil, 0, err
		}
		allowed := false
		switch v.EffectiveAccess {
		case classroom.AccessPublic:
			allowed = true
		case classroom.AccessLogin:
			allowed = uid > 0
		case classroom.AccessMember:
			allowed = d.member(ctx, uid)
		case classroom.AccessPaid:
			allowed = d.seriesEntitled(ctx, uid, v.ID)
		}
		v.CanPlay = allowed && !v.PlaybackBlocked
		v.PurchaseState = classroomPurchaseState(v.EffectiveAccess, allowed)
		out = append(out, v)
	}
	return out, total, rows.Err()
}
func (d *classroomPublicDB) ListStandalone(ctx context.Context, q classroomPublicQuery, uid int64) ([]classroomPublicContent, int, error) {
	const eligible = `c.status=$1 AND m.storage_status=$2 AND (c.series_id IS NULL OR c.show_as_standalone=true) AND ($3='' OR c.content_type=$3)`
	args := []any{classroom.ContentPublished, classroom.MediaReady, q.ContentType}
	var total int
	if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM classroom_contents c JOIN classroom_media_assets m ON m.id=c.media_asset_id LEFT JOIN classroom_series s ON s.id=c.series_id WHERE `+eligible, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT c.id,c.series_id,c.show_as_standalone,c.title,c.description,c.content_type,c.media_asset_id,c.cover_url,c.duration_seconds,c.teacher_name_snapshot,c.access_level,c.price_cents,c.playback_blocked,s.id,s.status,s.access_level,s.price_cents,s.playback_blocked FROM classroom_contents c JOIN classroom_media_assets m ON m.id=c.media_asset_id LEFT JOIN classroom_series s ON s.id=c.series_id WHERE `+eligible+` ORDER BY c.sort_order,c.id LIMIT $4 OFFSET $5`, append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]classroomPublicContent, 0, q.Limit)
	for rows.Next() {
		var c classroom.Content
		var parentID sql.NullInt64
		var parentStatus, parentAccess sql.NullString
		var parentPrice sql.NullInt64
		var parentBlocked sql.NullBool
		if err = rows.Scan(&c.ID, &c.SeriesID, &c.ShowAsStandalone, &c.Title, &c.Description, &c.ContentType, &c.MediaAssetID, &c.CoverURL, &c.DurationSeconds, &c.TeacherNameSnapshot, &c.AccessLevel, &c.PriceCents, &c.PlaybackBlocked, &parentID, &parentStatus, &parentAccess, &parentPrice, &parentBlocked); err != nil {
			return nil, 0, err
		}
		c.Status = classroom.ContentPublished
		var p *classroom.Series
		if parentID.Valid {
			p = &classroom.Series{ID: parentID.Int64, Status: classroom.SeriesStatus(parentStatus.String), AccessLevel: classroom.AccessLevel(parentAccess.String), PriceCents: int(parentPrice.Int64), PlaybackBlocked: parentBlocked.Bool}
		}
		out = append(out, d.contentViewReady(ctx, c, uid, p))
	}
	return out, total, rows.Err()
}
func (d *classroomPublicDB) GetSeries(ctx context.Context, id, uid int64) (classroomPublicSeriesDetail, error) {
	s, e := d.store.GetSeries(ctx, id)
	if e != nil || s.Status != classroom.SeriesPublished {
		return classroomPublicSeriesDetail{}, classroom.ErrNotFound
	}
	a := s.AccessLevel
	ok := canAccess(ctx, d, a, uid, classroom.Content{}, nil)
	if a == classroom.AccessPaid {
		ok = d.seriesEntitled(ctx, uid, s.ID)
	}
	rows, e := d.db.QueryContext(ctx, `SELECT c.id,c.series_id,c.title,c.description,c.content_type,c.cover_url,c.duration_seconds,c.teacher_name_snapshot,c.access_level,c.price_cents,c.playback_blocked FROM classroom_contents c JOIN classroom_media_assets m ON m.id=c.media_asset_id WHERE c.series_id=$1 AND c.status=$2 AND m.storage_status=$3 ORDER BY c.sort_order,c.id`, id, classroom.ContentPublished, classroom.MediaReady)
	if e != nil {
		return classroomPublicSeriesDetail{}, e
	}
	defer rows.Close()
	out := make([]classroomPublicContent, 0)
	for rows.Next() {
		var c classroom.Content
		if e = rows.Scan(&c.ID, &c.SeriesID, &c.Title, &c.Description, &c.ContentType, &c.CoverURL, &c.DurationSeconds, &c.TeacherNameSnapshot, &c.AccessLevel, &c.PriceCents, &c.PlaybackBlocked); e != nil {
			return classroomPublicSeriesDetail{}, e
		}
		out = append(out, d.contentViewReady(ctx, c, uid, &s))
	}
	if e = rows.Err(); e != nil {
		return classroomPublicSeriesDetail{}, e
	}
	canPlay := ok && !s.PlaybackBlocked
	return classroomPublicSeriesDetail{Series: classroomPublicSeries{ID: s.ID, Title: s.Title, Summary: s.Summary, CoverURL: s.CoverURL, TeacherName: s.TeacherNameSnapshot, EffectiveAccess: a, PriceCents: s.PriceCents, CanPlay: canPlay, PurchaseState: classroomPurchaseState(a, ok), PlaybackBlocked: s.PlaybackBlocked}, Contents: out}, nil
}
func (d *classroomPublicDB) GetContent(ctx context.Context, id, uid int64) (classroomPublicContent, error) {
	c, e := d.store.GetContent(ctx, id)
	if e != nil {
		return classroomPublicContent{}, e
	}
	var p *classroom.Series
	if c.SeriesID != nil {
		v, er := d.store.GetSeries(ctx, *c.SeriesID)
		if er == nil {
			p = &v
		}
	}
	if p != nil && p.Status != classroom.SeriesPublished && !c.ShowAsStandalone {
		return classroomPublicContent{}, classroom.ErrNotFound
	}
	return d.contentView(ctx, c, uid, p)
}
func (d *classroomPublicDB) Playback(ctx context.Context, uid, id int64) (classroomPlaybackSource, error) {
	c, e := d.store.GetContent(ctx, id)
	if e != nil {
		return classroomPlaybackSource{}, e
	}
	var p *classroom.Series
	if c.SeriesID != nil {
		v, er := d.store.GetSeries(ctx, *c.SeriesID)
		if er == nil {
			p = &v
		}
	}
	if c.MediaAssetID == nil {
		return classroomPlaybackSource{}, classroom.ErrNotFound
	}
	if c.PlaybackBlocked || (p != nil && p.PlaybackBlocked) {
		return classroomPlaybackSource{}, errClassroomPlaybackBlocked
	}
	m, e := d.store.GetMediaAsset(ctx, *c.MediaAssetID)
	if e != nil || m.StorageStatus != classroom.MediaReady {
		return classroomPlaybackSource{}, classroom.ErrNotFound
	}
	a := accessFor(c.AccessLevel, p)
	acquired := d.entitled(ctx, uid, c.ID, c.SeriesID)
	if (c.Status != classroom.ContentPublished && !acquired) || (p != nil && p.Status != classroom.SeriesPublished && !c.ShowAsStandalone && !acquired) || (!canAccess(ctx, d, a, uid, c, p) && !acquired) {
		return classroomPlaybackSource{}, classroom.ErrNotFound
	}
	return classroomPlaybackSource{Content: c, Media: m, Series: p}, nil
}
