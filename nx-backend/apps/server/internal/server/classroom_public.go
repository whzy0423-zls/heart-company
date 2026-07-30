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
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/storage"
)

type classroomPublicQuery struct {
	Limit, Offset int
	ContentType   classroom.ContentType
}
type classroomPublicSeries struct {
	ID               int64                      `json:"id"`
	Title            string                     `json:"title"`
	Summary          string                     `json:"summary,omitempty"`
	CoverURL         string                     `json:"coverUrl,omitempty"`
	CoverAspectRatio classroom.CoverAspectRatio `json:"coverAspectRatio"`
	TeacherName      string                     `json:"teacherName,omitempty"`
	EffectiveAccess  classroom.AccessLevel      `json:"effectiveAccess"`
	PriceCents       int                        `json:"priceCents"`
	CanPlay          bool                       `json:"canPlay"`
	PurchaseState    string                     `json:"purchaseState"`
	PlaybackBlocked  bool                       `json:"playbackBlocked"`
	signedCover      bool
}
type classroomPublicContent struct {
	ID               int64                      `json:"id"`
	SeriesID         *int64                     `json:"seriesId,omitempty"`
	Title            string                     `json:"title"`
	Description      string                     `json:"description,omitempty"`
	CoverURL         string                     `json:"coverUrl,omitempty"`
	CoverAspectRatio classroom.CoverAspectRatio `json:"coverAspectRatio"`
	TeacherName      string                     `json:"teacherName,omitempty"`
	ContentType      classroom.ContentType      `json:"contentType"`
	DurationSeconds  int                        `json:"durationSeconds"`
	AccessLevel      classroom.AccessLevel      `json:"accessLevel"`
	EffectiveAccess  classroom.AccessLevel      `json:"effectiveAccess"`
	PriceCents       int                        `json:"priceCents"`
	CanPlay          bool                       `json:"canPlay"`
	PurchaseState    string                     `json:"purchaseState"`
	PlaybackBlocked  bool                       `json:"playbackBlocked"`
	cacheVersion     string
	signedCover      bool
}
type classroomPublicSeriesDetail struct {
	Series   classroomPublicSeries    `json:"series"`
	Contents []classroomPublicContent `json:"contents"`
}
type classroomPublicRecentItem struct {
	ItemType          string                     `json:"itemType"`
	ID                int64                      `json:"id"`
	SeriesID          *int64                     `json:"seriesId,omitempty"`
	Title             string                     `json:"title"`
	Summary           string                     `json:"summary,omitempty"`
	Description       string                     `json:"description,omitempty"`
	CoverURL          string                     `json:"coverUrl,omitempty"`
	CoverAspectRatio  classroom.CoverAspectRatio `json:"coverAspectRatio"`
	TeacherName       string                     `json:"teacherName,omitempty"`
	ContentType       classroom.ContentType      `json:"contentType,omitempty"`
	DurationSeconds   int                        `json:"durationSeconds,omitempty"`
	AccessLevel       classroom.AccessLevel      `json:"accessLevel,omitempty"`
	EffectiveAccess   classroom.AccessLevel      `json:"effectiveAccess"`
	PriceCents        int                        `json:"priceCents"`
	CanPlay           bool                       `json:"canPlay"`
	PurchaseState     string                     `json:"purchaseState"`
	PlaybackBlocked   bool                       `json:"playbackBlocked"`
	LessonCount       int                        `json:"lessonCount,omitempty"`
	LatestPublishedAt time.Time                  `json:"latestPublishedAt"`
	signedCover       bool
}
type classroomPlaybackSource struct {
	Content classroom.Content
	Media   classroom.MediaAsset
	Series  *classroom.Series
}
type classroomPublicService interface {
	ListSeries(context.Context, classroomPublicQuery, int64) ([]classroomPublicSeries, int, error)
	ListRecent(context.Context, classroomPublicQuery, int64) ([]classroomPublicRecentItem, error)
	ListStandalone(context.Context, classroomPublicQuery, int64) ([]classroomPublicContent, int, error)
	GetSeries(context.Context, int64, int64) (classroomPublicSeriesDetail, error)
	GetContent(context.Context, int64, int64) (classroomPublicContent, error)
	Playback(ctx context.Context, userID, contentID int64) (classroomPlaybackSource, error)
}

type classroomTicketClaims struct {
	Version      int       `json:"v"`
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
	if strings.TrimSpace(mediaVersion) == "" || contentID <= 0 {
		return "", classroomTicketClaims{}, errClassroomTicket
	}
	c := classroomTicketClaims{Version: 1, ContentID: contentID, MediaVersion: mediaVersion, ExpiresAt: now.Add(5 * time.Minute), Nonce: base64.RawURLEncoding.EncodeToString(nonce)}
	b, _ := json.Marshal(c)
	raw := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, []byte(s.env.JWTSecret))
	_, _ = mac.Write([]byte("classroom-ticket:v1:" + raw))
	return raw + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), c, nil
}
func (s *Server) verifyClassroomTicket(token string, contentID int64, mediaVersion string) (classroomTicketClaims, error) {
	p := strings.Split(token, ".")
	if len(p) != 2 {
		return classroomTicketClaims{}, errClassroomTicket
	}
	mac := hmac.New(sha256.New, []byte(s.env.JWTSecret))
	_, _ = mac.Write([]byte("classroom-ticket:v1:" + p[0]))
	sig, err := base64.RawURLEncoding.DecodeString(p[1])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return classroomTicketClaims{}, errClassroomTicket
	}
	b, err := base64.RawURLEncoding.DecodeString(p[0])
	if err != nil {
		return classroomTicketClaims{}, errClassroomTicket
	}
	var c classroomTicketClaims
	if json.Unmarshal(b, &c) != nil {
		return classroomTicketClaims{}, errClassroomTicket
	}
	remaining := c.ExpiresAt.Sub(s.nowTime())
	if c.Version != 1 || c.ContentID != contentID || c.MediaVersion != mediaVersion || strings.TrimSpace(c.MediaVersion) == "" || strings.TrimSpace(c.Nonce) == "" || remaining <= 0 || remaining > 5*time.Minute {
		return classroomTicketClaims{}, errClassroomTicket
	}
	return c, nil
}

func registerClassroomPublicRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc(classroomAudioCoverPath, s.method(http.MethodGet, classroomAudioCover))
	mux.HandleFunc("/api/public/classroom/series", s.method(http.MethodGet, s.classroomSeriesPublic))
	mux.HandleFunc("/api/public/classroom/standalone", s.method(http.MethodGet, s.classroomStandalonePublic))
	mux.HandleFunc("/api/public/classroom/recent", s.method(http.MethodGet, s.classroomRecentPublic))
	mux.HandleFunc("/api/public/classroom/series/", s.method(http.MethodGet, s.classroomSeriesDetailPublic))
	mux.HandleFunc("/api/public/classroom/content/", s.classroomContentPublicRouter)
	mux.HandleFunc("/api/miniapp/classroom/content/", s.classroomMiniappContentRouter)
}

func (s *Server) classroomRecentPublic(w http.ResponseWriter, r *http.Request) {
	if s.classroomPublic == nil {
		failClassroomInternal(w, "list_recent", errors.New("classroom service unavailable"))
		return
	}
	u, _, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	page, err := classroomPublicPage(r)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := s.classroomPublic.ListRecent(r.Context(), page, u.ID)
	if err != nil {
		failClassroomInternal(w, "list_recent", err)
		return
	}
	data := map[string]any{"items": items, "limit": page.Limit, "offset": page.Offset}
	for _, item := range items {
		if item.signedCover {
			w.Header().Set("Cache-Control", "private, no-store")
			httpx.OK(w, data)
			return
		}
	}
	if setClassroomCache(w, r, data) {
		return
	}
	httpx.OK(w, data)
}

func (s *Server) classroomMiniappContentRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case strings.HasSuffix(path, "/play"):
		s.classroomPlaybackPublic(w, r)
	case strings.HasSuffix(path, "/progress"):
		if r.Method != http.MethodPut {
			httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		s.requireMiniapp(s.classroomProgressUpdate)(w, r)
	default:
		httpx.Fail(w, http.StatusNotFound, "Not Found")
	}
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
func classroomID(path, prefix, suffix string) (int64, error) {
	raw := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	raw = strings.Trim(raw, "/")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid classroom id")
	}
	return id, nil
}
func normalizedClassroomDevice(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) > 128 {
		value = value[:128]
	}
	if value == "" {
		value = "unknown"
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:12])
}
func (s *Server) allowAnonymousClassroom(w http.ResponseWriter, r *http.Request, id int64) bool {
	now := s.nowTime()
	ip := s.clientIP(r)
	rough := ip + ":" + strconv.FormatInt(id, 10)
	if s.classroomPlaybackIPLimiter != nil && !s.classroomPlaybackIPLimiter.Allow(rough, now) {
		httpx.Fail(w, http.StatusTooManyRequests, "Too Many Requests")
		return false
	}
	fine := ip + ":" + normalizedClassroomDevice(r.Header.Get("X-Device-ID")) + ":" + strconv.FormatInt(id, 10)
	if s.classroomPlaybackLimiter != nil && !s.classroomPlaybackLimiter.Allow(fine, now) {
		httpx.Fail(w, http.StatusTooManyRequests, "Too Many Requests")
		return false
	}
	return true
}
func failClassroomInternal(w http.ResponseWriter, op string, err error) {
	slog.Error("classroom public request failed", "operation", op, "error", err)
	if errors.Is(err, classroom.ErrCoverSigningUnavailable) {
		httpx.Fail(w, http.StatusServiceUnavailable, "Classroom cover unavailable")
		return
	}
	httpx.Fail(w, http.StatusInternalServerError, "Internal Server Error")
}
func classroomPublicPage(r *http.Request) (classroomPublicQuery, error) {
	q := r.URL.Query()
	l := 20
	if raw := q.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 || value > 100 {
			return classroomPublicQuery{}, errors.New("invalid limit")
		}
		l = value
	}
	o := 0
	if raw := q.Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return classroomPublicQuery{}, errors.New("invalid offset")
		}
		o = value
	}
	if o < 0 || o > 10_000 {
		return classroomPublicQuery{}, errors.New("invalid offset")
	}
	typ := classroom.ContentType(q.Get("contentType"))
	if typ != "" && typ != classroom.ContentVideo && typ != classroom.ContentAudio {
		return classroomPublicQuery{}, errors.New("invalid contentType")
	}
	return classroomPublicQuery{Limit: l, Offset: o, ContentType: typ}, nil
}
func mergeVary(h http.Header, value string) {
	for _, part := range strings.Split(h.Get("Vary"), ",") {
		if strings.EqualFold(strings.TrimSpace(part), value) {
			return
		}
	}
	h.Add("Vary", value)
}
func decodeClassroomPublicJSON(w http.ResponseWriter, r *http.Request, dst any, allowEmpty bool) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if errors.Is(err, io.EOF) && allowEmpty {
		return nil
	}
	if err != nil {
		return err
	}
	var extra any
	if err = dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON")
		}
		return err
	}
	return nil
}
func setClassroomCache(w http.ResponseWriter, r *http.Request, body any) bool {
	cacheBody := body
	if content, ok := body.(classroomPublicContent); ok {
		cacheBody = struct {
			Body         classroomPublicContent `json:"body"`
			CacheVersion string                 `json:"cacheVersion"`
		}{Body: content, CacheVersion: content.cacheVersion}
	}
	b, _ := json.Marshal(cacheBody)
	h := sha256.Sum256(b)
	etag := `"` + fmt.Sprintf("%x", h[:8]) + `"`
	w.Header().Set("ETag", etag)
	mergeVary(w.Header(), "Authorization")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}
func (s *Server) classroomSeriesPublic(w http.ResponseWriter, r *http.Request) {
	if s.classroomPublic == nil {
		failClassroomInternal(w, "list_series", errors.New("classroom service unavailable"))
		return
	}
	u, _, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	page, pageErr := classroomPublicPage(r)
	if pageErr != nil {
		httpx.Fail(w, http.StatusBadRequest, pageErr.Error())
		return
	}
	items, total, err := s.classroomPublic.ListSeries(r.Context(), page, u.ID)
	if err != nil {
		failClassroomInternal(w, "list_series", err)
		return
	}
	data := map[string]any{"items": items, "total": total, "limit": page.Limit, "offset": page.Offset}
	for _, item := range items {
		if item.signedCover {
			w.Header().Set("Cache-Control", "private, no-store")
			httpx.OK(w, data)
			return
		}
	}
	if setClassroomCache(w, r, data) {
		return
	}
	httpx.OK(w, data)
}
func (s *Server) classroomStandalonePublic(w http.ResponseWriter, r *http.Request) {
	if s.classroomPublic == nil {
		failClassroomInternal(w, "list_standalone", errors.New("classroom service unavailable"))
		return
	}
	u, _, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	page, pageErr := classroomPublicPage(r)
	if pageErr != nil {
		httpx.Fail(w, http.StatusBadRequest, pageErr.Error())
		return
	}
	items, total, err := s.classroomPublic.ListStandalone(r.Context(), page, u.ID)
	if err != nil {
		failClassroomInternal(w, "list_standalone", err)
		return
	}
	data := map[string]any{"items": items, "total": total, "limit": page.Limit, "offset": page.Offset}
	if classroomContentsHaveSignedCover(items) {
		w.Header().Set("Cache-Control", "private, no-store")
		httpx.OK(w, data)
		return
	}
	if setClassroomCache(w, r, data) {
		return
	}
	httpx.OK(w, data)
}
func (s *Server) classroomSeriesDetailPublic(w http.ResponseWriter, r *http.Request) {
	if s.classroomPublic == nil {
		failClassroomInternal(w, "get_series", errors.New("classroom service unavailable"))
		return
	}
	id, idErr := classroomID(r.URL.Path, "/api/public/classroom/series/", "")
	if idErr != nil {
		httpx.Fail(w, http.StatusBadRequest, idErr.Error())
		return
	}
	u, _, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	d, err := s.classroomPublic.GetSeries(r.Context(), id, u.ID)
	if err != nil {
		if errors.Is(err, classroom.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "Not Found")
		} else {
			failClassroomInternal(w, "get_series", err)
		}
		return
	}
	if d.Series.signedCover || classroomContentsHaveSignedCover(d.Contents) {
		w.Header().Set("Cache-Control", "private, no-store")
	} else if setClassroomCache(w, r, d) {
		return
	}
	httpx.OK(w, d)
}
func (s *Server) classroomContentPublic(w http.ResponseWriter, r *http.Request) {
	if s.classroomPublic == nil {
		failClassroomInternal(w, "get_content", errors.New("classroom service unavailable"))
		return
	}
	id, idErr := classroomID(r.URL.Path, "/api/public/classroom/content/", "")
	if idErr != nil {
		httpx.Fail(w, http.StatusBadRequest, idErr.Error())
		return
	}
	u, _, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	d, err := s.classroomPublic.GetContent(r.Context(), id, u.ID)
	if err != nil {
		if errors.Is(err, classroom.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "Not Found")
		} else {
			failClassroomInternal(w, "get_content", err)
		}
		return
	}
	if d.signedCover {
		w.Header().Set("Cache-Control", "private, no-store")
	} else if setClassroomCache(w, r, d) {
		return
	}
	httpx.OK(w, d)
}
func (s *Server) classroomAnonymousTicket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.classroomPublic == nil {
		failClassroomInternal(w, "ticket", errors.New("classroom service unavailable"))
		return
	}
	id, idErr := classroomID(r.URL.Path, "/api/public/classroom/content/", "/ticket")
	if idErr != nil {
		httpx.Fail(w, http.StatusBadRequest, idErr.Error())
		return
	}
	var body struct{}
	if err := decodeClassroomPublicJSON(w, r, &body, true); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if _, logged, valid := s.classroomViewer(w, r); !valid {
		return
	} else if logged {
		httpx.Fail(w, http.StatusBadRequest, "JWT playback does not require an anonymous ticket")
		return
	}
	if !s.allowAnonymousClassroom(w, r, id) {
		return
	}
	d, err := s.classroomPublic.GetContent(r.Context(), id, 0)
	if err != nil {
		if errors.Is(err, classroom.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "Not Found")
		} else {
			failClassroomInternal(w, "ticket_content", err)
		}
		return
	}
	if d.EffectiveAccess != classroom.AccessPublic || !d.CanPlay {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	src, err := s.classroomPublic.Playback(r.Context(), 0, id)
	if err != nil {
		if errors.Is(err, errClassroomPlaybackBlocked) {
			httpx.Fail(w, http.StatusLocked, "Playback Blocked")
			return
		}
		if errors.Is(err, classroom.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "Not Found")
		} else {
			failClassroomInternal(w, "ticket_playback", err)
		}
		return
	}
	ticket, _, err := s.signClassroomTicket(id, src.Media.ETag)
	if err != nil {
		failClassroomInternal(w, "sign_ticket", err)
		return
	}
	httpx.OK(w, map[string]any{"ticket": ticket, "expiresIn": 300})
}
func (s *Server) classroomPlaybackPublic(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.classroomPublic == nil {
		failClassroomInternal(w, "playback", errors.New("classroom service unavailable"))
		return
	}
	if r.Method != "POST" {
		httpx.Fail(w, 405, "Method Not Allowed")
		return
	}
	id, idErr := classroomID(r.URL.Path, "/api/miniapp/classroom/content/", "/play")
	if idErr != nil {
		httpx.Fail(w, http.StatusBadRequest, idErr.Error())
		return
	}
	u, logged, valid := s.classroomViewer(w, r)
	if !valid {
		return
	}
	var body struct {
		Ticket string `json:"ticket"`
	}
	if err := decodeClassroomPublicJSON(w, r, &body, true); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if !logged && !s.allowAnonymousClassroom(w, r, id) {
		return
	}
	src, err := s.classroomPublic.Playback(r.Context(), u.ID, id)
	if err != nil {
		if errors.Is(err, errClassroomPlaybackBlocked) {
			httpx.Fail(w, http.StatusLocked, "Playback Blocked")
			return
		}
		if errors.Is(err, classroom.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "Not Found")
		} else {
			failClassroomInternal(w, "playback_authorize", err)
		}
		return
	}
	if src.Content.PlaybackBlocked || (src.Series != nil && src.Series.PlaybackBlocked) {
		httpx.Fail(w, 423, "Playback Blocked")
		return
	}
	if !logged {
		if _, err = s.verifyClassroomTicket(body.Ticket, id, src.Media.ETag); err != nil {
			httpx.Fail(w, 401, "Unauthorized")
			return
		}
	}
	signer := s.classroomPlaybackSigner
	if signer == nil {
		slog.Error("classroom playback signer unavailable")
		httpx.Fail(w, http.StatusServiceUnavailable, "Playback unavailable")
		return
	}
	url, err := signer.PresignGetURL(r.Context(), src.Media.ObjectKey, 5*time.Minute)
	if err != nil {
		slog.Error("classroom playback signing failed", "error", err)
		httpx.Fail(w, http.StatusServiceUnavailable, "Playback unavailable")
		return
	}
	httpx.OK(w, map[string]any{"url": url, "expiresIn": 300, "contentType": src.Content.ContentType})
}

// db-backed implementation is installed by New when classroom tables are available.
type classroomPublicDB struct {
	store       *classroom.Store
	db          *sql.DB
	coverSigner storage.ObjectSigner
	coverTTL    time.Duration
}

func newClassroomPublicDB(db *sql.DB) classroomPublicService {
	return &classroomPublicDB{store: classroom.NewStore(db), db: db}
}

func newClassroomPublicDBWithCovers(db *sql.DB, signer storage.ObjectSigner, ttl time.Duration) classroomPublicService {
	return &classroomPublicDB{store: classroom.NewStore(db), db: db, coverSigner: signer, coverTTL: ttl}
}

func classroomContentsHaveSignedCover(items []classroomPublicContent) bool {
	for _, item := range items {
		if item.signedCover {
			return true
		}
	}
	return false
}

func (d *classroomPublicDB) resolveContentCover(ctx context.Context, content *classroom.Content, generatedObjectKey string) (bool, error) {
	resolved, err := classroom.ResolveEffectiveCover(ctx, classroom.CoverInput{
		ContentType: content.ContentType, ManualObjectKey: content.ManualCoverObjectKey,
		GeneratedObjectKey: generatedObjectKey, LegacyURL: content.CoverURL,
	}, d.coverSigner, d.coverTTL, classroomAudioCoverPath)
	if err != nil {
		return false, err
	}
	content.CoverURL = resolved.URL
	return resolved.Signed, nil
}

func (d *classroomPublicDB) resolveSeriesCover(ctx context.Context, series *classroom.Series, fallback classroom.CoverInput) (bool, error) {
	ratio, err := classroom.NormalizeCoverAspectRatio(series.CoverAspectRatio)
	if err != nil {
		return false, err
	}
	series.CoverAspectRatio = ratio
	if key := strings.TrimSpace(series.ManualCoverObjectKey); key != "" {
		if d.coverSigner == nil {
			return false, classroom.ErrCoverSigningUnavailable
		}
		url, err := d.coverSigner.PresignGetURL(ctx, key, d.coverTTL)
		if err != nil {
			return false, fmt.Errorf("%w: %v", classroom.ErrCoverSigningUnavailable, err)
		}
		series.CoverURL = url
		return true, nil
	}
	if strings.TrimSpace(series.CoverURL) != "" {
		return false, nil
	}
	resolved, err := classroom.ResolveEffectiveCover(ctx, fallback, d.coverSigner, d.coverTTL, classroomAudioCoverPath)
	if err != nil {
		return false, err
	}
	series.CoverURL = resolved.URL
	return resolved.Signed, nil
}

type classroomAccessSnapshot struct {
	loggedIn, member          bool
	seriesOwned, contentOwned map[int64]bool
}

func (d *classroomPublicDB) loadAccessSnapshot(ctx context.Context, uid int64) (classroomAccessSnapshot, error) {
	v := classroomAccessSnapshot{loggedIn: uid > 0, seriesOwned: map[int64]bool{}, contentOwned: map[int64]bool{}}
	if uid <= 0 {
		return v, nil
	}
	var level int
	var exp *time.Time
	if err := d.db.QueryRowContext(ctx, "SELECT member_level,member_expires_at FROM wx_users WHERE id=$1", uid).Scan(&level, &exp); err != nil {
		return v, err
	}
	v.member = miniapp.IsMembershipActive(level, exp, time.Now())
	rows, err := d.db.QueryContext(ctx, `SELECT series_id,content_id FROM classroom_entitlements WHERE wx_user_id=$1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now())`, uid)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var sid, cid sql.NullInt64
		if err = rows.Scan(&sid, &cid); err != nil {
			return v, err
		}
		if sid.Valid {
			v.seriesOwned[sid.Int64] = true
		}
		if cid.Valid {
			v.contentOwned[cid.Int64] = true
		}
	}
	return v, rows.Err()
}
func (v classroomAccessSnapshot) allows(access classroom.AccessLevel, contentID int64, seriesID *int64) bool {
	owned := v.contentOwned[contentID]
	if seriesID != nil {
		owned = owned || v.seriesOwned[*seriesID]
	}
	return classroomAccessAllowed(access, v.loggedIn, v.member, owned)
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
func contentViewResolved(c classroom.Content, parent *classroom.Series, a classroom.AccessLevel, ok bool, signedCover ...bool) classroomPublicContent {
	ratio, err := classroom.NormalizeCoverAspectRatio(c.CoverAspectRatio)
	if err != nil {
		ratio = classroom.CoverAspectRatio16x9
	}
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
	v := classroomPublicContent{ID: c.ID, SeriesID: c.SeriesID, Title: c.Title, Description: c.Description, CoverURL: c.CoverURL, CoverAspectRatio: ratio, TeacherName: c.TeacherNameSnapshot, ContentType: c.ContentType, DurationSeconds: c.DurationSeconds, AccessLevel: c.AccessLevel, EffectiveAccess: a, PriceCents: price, CanPlay: ok && !blocked, PurchaseState: pstate, PlaybackBlocked: blocked}
	if len(signedCover) > 0 {
		v.signedCover = signedCover[0]
	}
	return v
}
func (d *classroomPublicDB) ListSeries(ctx context.Context, q classroomPublicQuery, uid int64) ([]classroomPublicSeries, int, error) {
	const eligible = `s.status=$1 AND EXISTS (SELECT 1 FROM classroom_contents c JOIN classroom_media_assets m ON m.id=c.media_asset_id WHERE c.series_id=s.id AND c.status=$2 AND m.storage_status=$3 AND ($4='' OR c.content_type=$4))`
	args := []any{classroom.SeriesPublished, classroom.ContentPublished, classroom.MediaReady, q.ContentType}
	var total int
	if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM classroom_series s WHERE `+eligible, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT s.id,s.title,s.summary,s.cover_url,s.manual_cover_object_key,s.cover_aspect_ratio,s.teacher_name_snapshot,s.access_level,s.price_cents,s.playback_blocked,
		fallback.content_type,fallback.cover_url,fallback.manual_cover_object_key,fallback.generated_cover_object_key
		FROM classroom_series s
		LEFT JOIN LATERAL (
			SELECT c.content_type,c.cover_url,c.manual_cover_object_key,m.cover_object_key AS generated_cover_object_key
			FROM classroom_contents c JOIN classroom_media_assets m ON m.id=c.media_asset_id
			WHERE c.series_id=s.id AND c.status=$2 AND m.storage_status=$3 AND ($4='' OR c.content_type=$4)
			ORDER BY c.sort_order,c.id LIMIT 1
		) fallback ON true
		WHERE `+eligible+` ORDER BY s.sort_order,s.id LIMIT $5 OFFSET $6`, append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	out := make([]classroomPublicSeries, 0, q.Limit)
	for rows.Next() {
		var v classroomPublicSeries
		var manualKey, fallbackType, fallbackURL, fallbackManual, fallbackGenerated sql.NullString
		if err = rows.Scan(&v.ID, &v.Title, &v.Summary, &v.CoverURL, &manualKey, &v.CoverAspectRatio, &v.TeacherName, &v.EffectiveAccess, &v.PriceCents, &v.PlaybackBlocked, &fallbackType, &fallbackURL, &fallbackManual, &fallbackGenerated); err != nil {
			return nil, 0, err
		}
		series := classroom.Series{ID: v.ID, CoverURL: v.CoverURL, ManualCoverObjectKey: manualKey.String, CoverAspectRatio: v.CoverAspectRatio}
		v.signedCover, err = d.resolveSeriesCover(ctx, &series, classroom.CoverInput{ContentType: classroom.ContentType(fallbackType.String), ManualObjectKey: fallbackManual.String, GeneratedObjectKey: fallbackGenerated.String, LegacyURL: fallbackURL.String})
		if err != nil {
			return nil, 0, err
		}
		v.CoverURL, v.CoverAspectRatio = series.CoverURL, series.CoverAspectRatio
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, err
	}
	if err = rows.Close(); err != nil {
		return nil, 0, err
	}
	snapshot, err := d.loadAccessSnapshot(ctx, uid)
	if err != nil {
		return nil, 0, err
	}
	for i := range out {
		allowed := snapshot.allows(out[i].EffectiveAccess, 0, &out[i].ID)
		out[i].CanPlay = allowed && !out[i].PlaybackBlocked
		out[i].PurchaseState = classroomPurchaseState(out[i].EffectiveAccess, allowed)
	}
	return out, total, nil
}

func (d *classroomPublicDB) ListRecent(ctx context.Context, q classroomPublicQuery, uid int64) ([]classroomPublicRecentItem, error) {
	const query = `WITH recent_items AS (
		SELECT 'series' AS item_type,s.id,
			GREATEST(s.updated_at,COALESCE(s.published_at,s.updated_at),MAX(GREATEST(c.updated_at,COALESCE(c.published_at,c.updated_at)))) AS latest_published_at
		FROM classroom_series s
		JOIN classroom_contents c ON c.series_id=s.id
		JOIN classroom_media_assets m ON m.id=c.media_asset_id
		WHERE s.status=$1 AND c.status=$2 AND m.storage_status=$3 AND ($4='' OR c.content_type=$4)
		GROUP BY s.id,s.published_at,s.updated_at
		UNION ALL
		SELECT 'content' AS item_type,c.id,GREATEST(c.updated_at,COALESCE(c.published_at,c.updated_at)) AS latest_published_at
		FROM classroom_contents c
		JOIN classroom_media_assets m ON m.id=c.media_asset_id
		WHERE c.status=$2 AND m.storage_status=$3
			AND (c.series_id IS NULL OR c.show_as_standalone=true)
			AND ($4='' OR c.content_type=$4)
	)
	SELECT item_type,id,latest_published_at FROM recent_items
	ORDER BY latest_published_at DESC,item_type,id DESC LIMIT $5 OFFSET $6`
	rows, err := d.db.QueryContext(ctx, query, classroom.SeriesPublished, classroom.ContentPublished, classroom.MediaReady, q.ContentType, q.Limit, q.Offset)
	if err != nil {
		return nil, err
	}
	type recentRef struct {
		itemType string
		id       int64
		latest   time.Time
	}
	refs := make([]recentRef, 0, q.Limit)
	for rows.Next() {
		var ref recentRef
		if err := rows.Scan(&ref.itemType, &ref.id, &ref.latest); err != nil {
			_ = rows.Close()
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := make([]classroomPublicRecentItem, 0, len(refs))
	for _, ref := range refs {
		switch ref.itemType {
		case "series":
			detail, err := d.GetSeries(ctx, ref.id, uid)
			if err != nil {
				if errors.Is(err, classroom.ErrNotFound) {
					continue
				}
				return nil, err
			}
			v := detail.Series
			item := classroomPublicRecentItem{
				ItemType: "series", ID: v.ID, Title: v.Title, Summary: v.Summary,
				CoverURL: v.CoverURL, CoverAspectRatio: v.CoverAspectRatio,
				TeacherName: v.TeacherName, EffectiveAccess: v.EffectiveAccess,
				PriceCents: v.PriceCents, CanPlay: v.CanPlay, PurchaseState: v.PurchaseState,
				PlaybackBlocked: v.PlaybackBlocked, LessonCount: len(detail.Contents),
				LatestPublishedAt: ref.latest,
			}
			item.signedCover = v.signedCover
			for _, content := range detail.Contents {
				item.signedCover = item.signedCover || content.signedCover
			}
			out = append(out, item)
		case "content":
			v, err := d.GetContent(ctx, ref.id, uid)
			if err != nil {
				if errors.Is(err, classroom.ErrNotFound) {
					continue
				}
				return nil, err
			}
			out = append(out, classroomPublicRecentItem{
				ItemType: "content", ID: v.ID, SeriesID: v.SeriesID, Title: v.Title,
				Description: v.Description, CoverURL: v.CoverURL, CoverAspectRatio: v.CoverAspectRatio,
				TeacherName: v.TeacherName, ContentType: v.ContentType, DurationSeconds: v.DurationSeconds,
				AccessLevel: v.AccessLevel, EffectiveAccess: v.EffectiveAccess, PriceCents: v.PriceCents,
				CanPlay: v.CanPlay, PurchaseState: v.PurchaseState, PlaybackBlocked: v.PlaybackBlocked,
				LatestPublishedAt: ref.latest, signedCover: v.signedCover,
			})
		default:
			return nil, fmt.Errorf("unknown recent classroom item type %q", ref.itemType)
		}
	}
	return out, nil
}

func (d *classroomPublicDB) ListStandalone(ctx context.Context, q classroomPublicQuery, uid int64) ([]classroomPublicContent, int, error) {
	const eligible = `c.status=$1 AND m.storage_status=$2 AND (c.series_id IS NULL OR c.show_as_standalone=true) AND ($3='' OR c.content_type=$3)`
	args := []any{classroom.ContentPublished, classroom.MediaReady, q.ContentType}
	var total int
	if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM classroom_contents c JOIN classroom_media_assets m ON m.id=c.media_asset_id LEFT JOIN classroom_series s ON s.id=c.series_id WHERE `+eligible, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT c.id,c.series_id,c.show_as_standalone,c.title,c.description,c.content_type,c.media_asset_id,c.cover_url,c.duration_seconds,c.teacher_name_snapshot,c.access_level,c.price_cents,c.playback_blocked,c.manual_cover_object_key,c.cover_aspect_ratio,m.cover_object_key,s.id,s.status,s.access_level,s.price_cents,s.playback_blocked FROM classroom_contents c JOIN classroom_media_assets m ON m.id=c.media_asset_id LEFT JOIN classroom_series s ON s.id=c.series_id WHERE `+eligible+` ORDER BY c.sort_order,c.id LIMIT $4 OFFSET $5`, append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	type rowItem struct {
		content     classroom.Content
		parent      *classroom.Series
		signedCover bool
	}
	raw := make([]rowItem, 0, q.Limit)
	for rows.Next() {
		var c classroom.Content
		var parentID sql.NullInt64
		var parentStatus, parentAccess sql.NullString
		var parentPrice sql.NullInt64
		var parentBlocked sql.NullBool
		var generatedCover string
		if err = rows.Scan(&c.ID, &c.SeriesID, &c.ShowAsStandalone, &c.Title, &c.Description, &c.ContentType, &c.MediaAssetID, &c.CoverURL, &c.DurationSeconds, &c.TeacherNameSnapshot, &c.AccessLevel, &c.PriceCents, &c.PlaybackBlocked, &c.ManualCoverObjectKey, &c.CoverAspectRatio, &generatedCover, &parentID, &parentStatus, &parentAccess, &parentPrice, &parentBlocked); err != nil {
			return nil, 0, err
		}
		signedCover, resolveErr := d.resolveContentCover(ctx, &c, generatedCover)
		if resolveErr != nil {
			return nil, 0, resolveErr
		}
		c.Status = classroom.ContentPublished
		var p *classroom.Series
		if parentID.Valid {
			p = &classroom.Series{ID: parentID.Int64, Status: classroom.SeriesStatus(parentStatus.String), AccessLevel: classroom.AccessLevel(parentAccess.String), PriceCents: int(parentPrice.Int64), PlaybackBlocked: parentBlocked.Bool}
		}
		raw = append(raw, rowItem{content: c, parent: p, signedCover: signedCover})
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, err
	}
	if err = rows.Close(); err != nil {
		return nil, 0, err
	}
	snapshot, err := d.loadAccessSnapshot(ctx, uid)
	if err != nil {
		return nil, 0, err
	}
	out := make([]classroomPublicContent, 0, len(raw))
	for _, item := range raw {
		a := accessFor(item.content.AccessLevel, item.parent)
		out = append(out, contentViewResolved(item.content, item.parent, a, snapshot.allows(a, item.content.ID, item.content.SeriesID), item.signedCover))
	}
	return out, total, nil
}
func (d *classroomPublicDB) GetSeries(ctx context.Context, id, uid int64) (classroomPublicSeriesDetail, error) {
	s, e := d.store.GetSeries(ctx, id)
	if e != nil {
		return classroomPublicSeriesDetail{}, e
	}
	if s.Status != classroom.SeriesPublished {
		return classroomPublicSeriesDetail{}, classroom.ErrNotFound
	}
	a := s.AccessLevel
	rows, e := d.db.QueryContext(ctx, `SELECT c.id,c.series_id,c.title,c.description,c.content_type,c.cover_url,c.duration_seconds,c.teacher_name_snapshot,c.access_level,c.price_cents,c.playback_blocked,c.manual_cover_object_key,c.cover_aspect_ratio,m.cover_object_key FROM classroom_contents c JOIN classroom_media_assets m ON m.id=c.media_asset_id WHERE c.series_id=$1 AND c.status=$2 AND m.storage_status=$3 ORDER BY c.sort_order,c.id`, id, classroom.ContentPublished, classroom.MediaReady)
	if e != nil {
		return classroomPublicSeriesDetail{}, e
	}
	defer rows.Close()
	type seriesContent struct {
		content     classroom.Content
		signedCover bool
	}
	raw := make([]seriesContent, 0)
	for rows.Next() {
		var c classroom.Content
		var generatedCover string
		if e = rows.Scan(&c.ID, &c.SeriesID, &c.Title, &c.Description, &c.ContentType, &c.CoverURL, &c.DurationSeconds, &c.TeacherNameSnapshot, &c.AccessLevel, &c.PriceCents, &c.PlaybackBlocked, &c.ManualCoverObjectKey, &c.CoverAspectRatio, &generatedCover); e != nil {
			return classroomPublicSeriesDetail{}, e
		}
		signedCover, resolveErr := d.resolveContentCover(ctx, &c, generatedCover)
		if resolveErr != nil {
			return classroomPublicSeriesDetail{}, resolveErr
		}
		raw = append(raw, seriesContent{content: c, signedCover: signedCover})
	}
	if e = rows.Err(); e != nil {
		_ = rows.Close()
		return classroomPublicSeriesDetail{}, e
	}
	if e = rows.Close(); e != nil {
		return classroomPublicSeriesDetail{}, e
	}
	snapshot, e := d.loadAccessSnapshot(ctx, uid)
	if e != nil {
		return classroomPublicSeriesDetail{}, e
	}
	ok := snapshot.allows(a, 0, &s.ID)
	out := make([]classroomPublicContent, 0, len(raw))
	for _, item := range raw {
		c := item.content
		effective := accessFor(c.AccessLevel, &s)
		out = append(out, contentViewResolved(c, &s, effective, snapshot.allows(effective, c.ID, c.SeriesID), item.signedCover))
	}
	hadSeriesCover := strings.TrimSpace(s.ManualCoverObjectKey) != "" || strings.TrimSpace(s.CoverURL) != ""
	fallback := classroom.CoverInput{}
	if len(raw) > 0 && len(out) > 0 {
		fallback = classroom.CoverInput{ContentType: raw[0].content.ContentType, LegacyURL: out[0].CoverURL}
	}
	seriesSigned, e := d.resolveSeriesCover(ctx, &s, fallback)
	if e != nil {
		return classroomPublicSeriesDetail{}, e
	}
	if !hadSeriesCover && len(raw) > 0 {
		seriesSigned = raw[0].signedCover
	}
	canPlay := ok && !s.PlaybackBlocked
	return classroomPublicSeriesDetail{Series: classroomPublicSeries{ID: s.ID, Title: s.Title, Summary: s.Summary, CoverURL: s.CoverURL, CoverAspectRatio: s.CoverAspectRatio, TeacherName: s.TeacherNameSnapshot, EffectiveAccess: a, PriceCents: s.PriceCents, CanPlay: canPlay, PurchaseState: classroomPurchaseState(a, ok), PlaybackBlocked: s.PlaybackBlocked, signedCover: seriesSigned}, Contents: out}, nil
}
func (d *classroomPublicDB) GetContent(ctx context.Context, id, uid int64) (classroomPublicContent, error) {
	const query = `SELECT c.id,c.series_id,c.show_as_standalone,c.title,c.description,c.content_type,c.media_asset_id,c.cover_url,c.duration_seconds,c.teacher_name_snapshot,c.access_level,c.price_cents,c.playback_blocked,c.status,c.manual_cover_object_key,c.cover_aspect_ratio,m.cover_object_key,m.etag,s.id,s.status,s.access_level,s.price_cents,s.playback_blocked
		FROM classroom_contents c JOIN classroom_media_assets m ON m.id=c.media_asset_id LEFT JOIN classroom_series s ON s.id=c.series_id
		WHERE c.id=$1 AND c.status=$2 AND m.storage_status=$3`
	var c classroom.Content
	var generatedCover, mediaETag string
	var parentID, parentPrice sql.NullInt64
	var parentStatus, parentAccess sql.NullString
	var parentBlocked sql.NullBool
	e := d.db.QueryRowContext(ctx, query, id, classroom.ContentPublished, classroom.MediaReady).Scan(&c.ID, &c.SeriesID, &c.ShowAsStandalone, &c.Title, &c.Description, &c.ContentType, &c.MediaAssetID, &c.CoverURL, &c.DurationSeconds, &c.TeacherNameSnapshot, &c.AccessLevel, &c.PriceCents, &c.PlaybackBlocked, &c.Status, &c.ManualCoverObjectKey, &c.CoverAspectRatio, &generatedCover, &mediaETag, &parentID, &parentStatus, &parentAccess, &parentPrice, &parentBlocked)
	if errors.Is(e, sql.ErrNoRows) {
		return classroomPublicContent{}, classroom.ErrNotFound
	}
	if e != nil {
		return classroomPublicContent{}, e
	}
	var p *classroom.Series
	if parentID.Valid {
		p = &classroom.Series{ID: parentID.Int64, Status: classroom.SeriesStatus(parentStatus.String), AccessLevel: classroom.AccessLevel(parentAccess.String), PriceCents: int(parentPrice.Int64), PlaybackBlocked: parentBlocked.Bool}
	}
	if p != nil && p.Status != classroom.SeriesPublished && !c.ShowAsStandalone {
		return classroomPublicContent{}, classroom.ErrNotFound
	}
	signedCover, e := d.resolveContentCover(ctx, &c, generatedCover)
	if e != nil {
		return classroomPublicContent{}, e
	}
	snapshot, e := d.loadAccessSnapshot(ctx, uid)
	if e != nil {
		return classroomPublicContent{}, e
	}
	access := accessFor(c.AccessLevel, p)
	view := contentViewResolved(c, p, access, snapshot.allows(access, c.ID, c.SeriesID), signedCover)
	view.cacheVersion = mediaETag
	return view, nil
}
func (d *classroomPublicDB) Playback(ctx context.Context, uid, id int64) (classroomPlaybackSource, error) {
	const query = `SELECT c.id,c.series_id,c.show_as_standalone,c.title,c.content_type,c.status,c.playback_blocked,c.access_level,c.price_cents,c.media_asset_id,m.id,m.bucket,m.object_key,m.etag,m.content_type,m.storage_status,m.duration_seconds,s.id,s.status,s.playback_blocked,s.access_level,s.price_cents FROM classroom_contents c JOIN classroom_media_assets m ON m.id=c.media_asset_id LEFT JOIN classroom_series s ON s.id=c.series_id WHERE c.id=$1 AND m.storage_status=$2`
	var c classroom.Content
	var m classroom.MediaAsset
	var parentID sql.NullInt64
	var parentStatus, parentAccess sql.NullString
	var parentBlocked sql.NullBool
	var parentPrice sql.NullInt64
	err := d.db.QueryRowContext(ctx, query, id, classroom.MediaReady).Scan(&c.ID, &c.SeriesID, &c.ShowAsStandalone, &c.Title, &c.ContentType, &c.Status, &c.PlaybackBlocked, &c.AccessLevel, &c.PriceCents, &c.MediaAssetID, &m.ID, &m.Bucket, &m.ObjectKey, &m.ETag, &m.ContentType, &m.StorageStatus, &m.DurationSeconds, &parentID, &parentStatus, &parentBlocked, &parentAccess, &parentPrice)
	if errors.Is(err, sql.ErrNoRows) {
		return classroomPlaybackSource{}, classroom.ErrNotFound
	}
	if err != nil {
		return classroomPlaybackSource{}, err
	}
	var p *classroom.Series
	if parentID.Valid {
		p = &classroom.Series{ID: parentID.Int64, Status: classroom.SeriesStatus(parentStatus.String), PlaybackBlocked: parentBlocked.Bool, AccessLevel: classroom.AccessLevel(parentAccess.String), PriceCents: int(parentPrice.Int64)}
	}
	if c.PlaybackBlocked || (p != nil && p.PlaybackBlocked) {
		return classroomPlaybackSource{}, errClassroomPlaybackBlocked
	}
	snapshot, err := d.loadAccessSnapshot(ctx, uid)
	if err != nil {
		return classroomPlaybackSource{}, err
	}
	if !classroomPlaybackAccessible(c, p, snapshot) {
		return classroomPlaybackSource{}, classroom.ErrNotFound
	}
	return classroomPlaybackSource{Content: c, Media: m, Series: p}, nil
}

func classroomPlaybackAccessible(c classroom.Content, parent *classroom.Series, snapshot classroomAccessSnapshot) bool {
	if c.PlaybackBlocked || (parent != nil && parent.PlaybackBlocked) {
		return false
	}
	access := accessFor(c.AccessLevel, parent)
	acquired := snapshot.contentOwned[c.ID]
	if c.SeriesID != nil {
		acquired = acquired || snapshot.seriesOwned[*c.SeriesID]
	}
	if c.Status != classroom.ContentPublished && (c.Status != classroom.ContentOffline || !acquired) {
		return false
	}
	if parent != nil && parent.Status != classroom.SeriesPublished && !c.ShowAsStandalone && (parent.Status != classroom.SeriesOffline || !acquired) {
		return false
	}
	return snapshot.allows(access, c.ID, c.SeriesID) || acquired
}
