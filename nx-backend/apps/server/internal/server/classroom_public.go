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
	PlaybackTicket  string                `json:"playbackTicket,omitempty"`
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
	Playback(context.Context, int64, int64) (classroomPlaybackSource, error)
}

type classroomTicketClaims struct {
	ContentID    int64     `json:"content_id"`
	MediaVersion string    `json:"media_version"`
	ExpiresAt    time.Time `json:"exp"`
	Nonce        string    `json:"nonce"`
}

var errClassroomTicket = errors.New("invalid classroom playback ticket")

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
	mux.HandleFunc("/api/public/classroom/content/", s.method(http.MethodGet, s.classroomContentPublic))
	mux.HandleFunc("/api/miniapp/classroom/content/", s.classroomPlaybackPublic)
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
	if u.ID == 0 {
		for i := range items {
			if src, e := s.classroomPublic.Playback(r.Context(), items[i].ID, 0); e == nil && items[i].EffectiveAccess == classroom.AccessPublic {
				items[i].PlaybackTicket, _, _ = s.signClassroomTicket(items[i].ID, src.Media.ETag)
			}
		}
	}
	data := map[string]any{"items": items, "total": total}
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
	if u.ID == 0 {
		for i := range d.Contents {
			if d.Contents[i].EffectiveAccess == classroom.AccessPublic {
				if src, e := s.classroomPublic.Playback(r.Context(), d.Contents[i].ID, 0); e == nil {
					d.Contents[i].PlaybackTicket, _, _ = s.signClassroomTicket(d.Contents[i].ID, src.Media.ETag)
				}
			}
		}
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
	if u.ID == 0 && d.EffectiveAccess == classroom.AccessPublic {
		if src, e := s.classroomPublic.Playback(r.Context(), id, 0); e == nil {
			d.PlaybackTicket, _, _ = s.signClassroomTicket(id, src.Media.ETag)
		}
	}
	if setClassroomCache(w, r, d) {
		return
	}
	httpx.OK(w, d)
}
func (s *Server) classroomPlaybackPublic(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		httpx.Fail(w, 405, "Method Not Allowed")
		return
	}
	id, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/miniapp/classroom/content/"), "/play"), 10, 64)
	u, logged, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	src, err := s.classroomPublic.Playback(r.Context(), id, u.ID)
	if err != nil {
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
func canAccess(ctx context.Context, d *classroomPublicDB, a classroom.AccessLevel, uid int64, c classroom.Content, parent *classroom.Series) bool {
	switch a {
	case classroom.AccessPublic:
		return true
	case classroom.AccessLogin:
		return uid > 0
	case classroom.AccessMember:
		return d.member(ctx, uid)
	case classroom.AccessPaid:
		return d.entitled(ctx, uid, c.ID, c.SeriesID)
	}
	return false
}
func (d *classroomPublicDB) contentView(ctx context.Context, c classroom.Content, uid int64, parent *classroom.Series) (classroomPublicContent, error) {
	if c.Status != classroom.ContentPublished || c.MediaAssetID == nil {
		return classroomPublicContent{}, classroom.ErrNotFound
	}
	m, e := d.store.GetMediaAsset(ctx, *c.MediaAssetID)
	if e != nil || m.StorageStatus != classroom.MediaReady {
		return classroomPublicContent{}, classroom.ErrNotFound
	}
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
	v := classroomPublicContent{ID: c.ID, SeriesID: c.SeriesID, Title: c.Title, Description: c.Description, CoverURL: c.CoverURL, TeacherName: c.TeacherNameSnapshot, ContentType: c.ContentType, DurationSeconds: c.DurationSeconds, EffectiveAccess: a, PriceCents: price, CanPlay: ok && !c.PlaybackBlocked && (parent == nil || !parent.PlaybackBlocked), PurchaseState: pstate, PlaybackBlocked: c.PlaybackBlocked}
	return v, nil
}
func (d *classroomPublicDB) ListSeries(ctx context.Context, q classroomPublicQuery, uid int64) ([]classroomPublicSeries, int, error) {
	xs, e := d.store.ListSeries(ctx, classroom.SeriesFilter{Status: classroom.SeriesPublished, Limit: q.Limit, Offset: q.Offset})
	if e != nil {
		return nil, 0, e
	}
	out := make([]classroomPublicSeries, 0, len(xs))
	for _, x := range xs {
		a := x.AccessLevel
		ok := false
		switch a {
		case classroom.AccessPublic:
			ok = true
		case classroom.AccessLogin:
			ok = uid > 0
		case classroom.AccessMember:
			ok = d.member(ctx, uid)
		case classroom.AccessPaid:
			ok = d.seriesEntitled(ctx, uid, x.ID)
		}
		lessons, _ := d.store.ListContents(ctx, classroom.ContentFilter{SeriesID: &x.ID, Status: classroom.ContentPublished, Limit: 1})
		if len(lessons) == 0 {
			continue
		}
		if _, err := d.contentView(ctx, lessons[0], uid, &x); err != nil {
			continue
		}
		out = append(out, classroomPublicSeries{ID: x.ID, Title: x.Title, Summary: x.Summary, CoverURL: x.CoverURL, TeacherName: x.TeacherNameSnapshot, EffectiveAccess: a, PriceCents: x.PriceCents, CanPlay: ok && !x.PlaybackBlocked, PlaybackBlocked: x.PlaybackBlocked})
	}
	return out, len(out), nil
}
func (d *classroomPublicDB) ListStandalone(ctx context.Context, q classroomPublicQuery, uid int64) ([]classroomPublicContent, int, error) {
	xs, e := d.store.ListContents(ctx, classroom.ContentFilter{Status: classroom.ContentPublished, ContentType: q.ContentType, StandaloneOnly: true, Limit: q.Limit, Offset: q.Offset})
	if e != nil {
		return nil, 0, e
	}
	out := make([]classroomPublicContent, 0, len(xs))
	for _, c := range xs {
		var p *classroom.Series
		if c.SeriesID != nil {
			v, er := d.store.GetSeries(ctx, *c.SeriesID)
			if er == nil {
				p = &v
			}
		}
		if p != nil && p.Status != classroom.SeriesPublished && !(c.ShowAsStandalone && c.Status == classroom.ContentPublished) {
			continue
		}
		v, er := d.contentView(ctx, c, uid, p)
		if er == nil {
			out = append(out, v)
		}
	}
	return out, len(out), nil
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
	xs, e := d.store.ListContents(ctx, classroom.ContentFilter{SeriesID: &id, Status: classroom.ContentPublished, Limit: 100})
	if e != nil {
		return classroomPublicSeriesDetail{}, e
	}
	out := make([]classroomPublicContent, 0, len(xs))
	for _, c := range xs {
		if v, er := d.contentView(ctx, c, uid, &s); er == nil {
			out = append(out, v)
		}
	}
	return classroomPublicSeriesDetail{Series: classroomPublicSeries{ID: s.ID, Title: s.Title, Summary: s.Summary, CoverURL: s.CoverURL, TeacherName: s.TeacherNameSnapshot, EffectiveAccess: a, PriceCents: s.PriceCents, CanPlay: ok, PlaybackBlocked: s.PlaybackBlocked}, Contents: out}, nil
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
