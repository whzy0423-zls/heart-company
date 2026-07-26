package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/config"
)

type classroomSQLDriver struct {
	query func(string, []driver.NamedValue) (driver.Rows, error)
}
type classroomSQLConn struct{ d *classroomSQLDriver }

func (d *classroomSQLDriver) Open(string) (driver.Conn, error)  { return &classroomSQLConn{d: d}, nil }
func (c *classroomSQLConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *classroomSQLConn) Close() error                        { return nil }
func (c *classroomSQLConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c *classroomSQLConn) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	return c.d.query(q, args)
}

type classroomRows struct {
	cols   []string
	values [][]driver.Value
	i      int
}

func (r *classroomRows) Columns() []string { return r.cols }
func (r *classroomRows) Close() error      { return nil }
func (r *classroomRows) Next(dst []driver.Value) error {
	if r.i >= len(r.values) {
		return io.EOF
	}
	copy(dst, r.values[r.i])
	r.i++
	return nil
}
func openClassroomTestDB(t *testing.T, fn func(string, []driver.NamedValue) (driver.Rows, error)) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("classroom-public-%d", classroomDriverSequence.Add(1))
	sql.Register(name, &classroomSQLDriver{query: fn})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

var classroomDriverSequence atomic.Uint64

type fakeClassroomPublicService struct {
	series                    []classroomPublicSeries
	content                   classroomPublicContent
	play                      classroomPlaybackSource
	playUserID, playContentID int64
	playCalls                 int
	viewerAware               bool
}
type fakeClassroomSigner struct{ key string }

func (f fakeClassroomSigner) PresignGetURL(context.Context, string, time.Duration) (string, error) {
	return "https://cdn.example/" + f.key, nil
}

func (f *fakeClassroomPublicService) ListSeries(context.Context, classroomPublicQuery, int64) ([]classroomPublicSeries, int, error) {
	return f.series, len(f.series), nil
}
func (f *fakeClassroomPublicService) ListStandalone(_ context.Context, _ classroomPublicQuery, userID int64) ([]classroomPublicContent, int, error) {
	v := f.content
	if f.viewerAware {
		v.CanPlay = userID > 0
		if v.CanPlay {
			v.PurchaseState = "owned"
		} else {
			v.PurchaseState = "purchase_required"
		}
	}
	return []classroomPublicContent{v}, 1, nil
}
func (f *fakeClassroomPublicService) GetSeries(context.Context, int64, int64) (classroomPublicSeriesDetail, error) {
	return classroomPublicSeriesDetail{Series: f.series[0], Contents: []classroomPublicContent{f.content}}, nil
}
func (f *fakeClassroomPublicService) GetContent(context.Context, int64, int64) (classroomPublicContent, error) {
	return f.content, nil
}

func (f *fakeClassroomPublicService) Playback(_ context.Context, userID, contentID int64) (classroomPlaybackSource, error) {
	f.playUserID, f.playContentID = userID, contentID
	f.playCalls++
	return f.play, nil
}

func TestClassroomPublicRoutesExposeSafePaidCardsAndCacheIsolation(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	f := &fakeClassroomPublicService{
		series:  []classroomPublicSeries{{ID: 1, Title: "Series", EffectiveAccess: classroom.AccessPaid, PriceCents: 9900}},
		content: classroomPublicContent{ID: 2, Title: "Paid lesson", ContentType: classroom.ContentVideo, EffectiveAccess: classroom.AccessPaid, PriceCents: 1990, CanPlay: false},
	}
	s := &Server{classroomPublic: f, now: func() time.Time { return now }, env: config.Env{JWTSecret: "ticket-secret"}}
	mux := http.NewServeMux()
	registerClassroomPublicRoutes(mux, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/classroom/standalone?limit=10", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"canPlay":false`) || strings.Contains(rr.Body.String(), "objectKey") {
		t.Fatalf("unsafe or unavailable paid card: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("ETag") == "" || rr.Header().Get("Vary") != "Authorization" {
		t.Fatalf("missing cache isolation headers: %v", rr.Header())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/public/classroom/standalone?limit=10", nil)
	req.Header.Set("If-None-Match", rr.Header().Get("ETag"))
	cached := httptest.NewRecorder()
	mux.ServeHTTP(cached, req)
	if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
		t.Fatalf("expected 304 revalidation, got %d %s", cached.Code, cached.Body.String())
	}
}

func TestClassroomPlaybackAnonymousTicketExpiryAndCrossContentReplay(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	s := &Server{now: func() time.Time { return now }, env: config.Env{JWTSecret: "ticket-secret"}}
	ticket, claims, err := s.signClassroomTicket(7, "media-v1")
	if err != nil || claims.ExpiresAt.Sub(now) != 5*time.Minute {
		t.Fatalf("ticket TTL: claims=%+v err=%v", claims, err)
	}
	if _, err = s.verifyClassroomTicket(ticket, 8, "media-v1"); err == nil {
		t.Fatal("cross-content replay must fail")
	}
	if _, err = s.verifyClassroomTicket(ticket, 7, "media-v2"); err == nil {
		t.Fatal("cross-media-version replay must fail")
	}
	s.now = func() time.Time { return now.Add(5*time.Minute + time.Second) }
	if _, err = s.verifyClassroomTicket(ticket, 7, "media-v1"); err == nil {
		t.Fatal("expired ticket must fail")
	}
}

func TestClassroomPlaybackRouteRequiresBoundTicketAndRateLimitsAnonymousRefresh(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	f := &fakeClassroomPublicService{play: classroomPlaybackSource{Content: classroom.Content{ID: 7, Status: classroom.ContentPublished, ContentType: classroom.ContentVideo, AccessLevel: classroom.AccessPublic}, Media: classroom.MediaAsset{ID: 3, ObjectKey: "private/video.mp4", ETag: "v1", StorageStatus: classroom.MediaReady}}}
	s := &Server{classroomPublic: f, classroomPlaybackSigner: fakeClassroomSigner{key: "signed"}, classroomPlaybackLimiter: newStrRateLimiter(1, time.Minute), now: func() time.Time { return now }, env: config.Env{JWTSecret: "ticket-secret"}}
	ticket, _, _ := s.signClassroomTicket(7, "v1")
	mux := http.NewServeMux()
	registerClassroomPublicRoutes(mux, s)
	req := httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/7/play", strings.NewReader(`{"ticket":"`+ticket+`"}`))
	req.Header.Set("X-Device-ID", "device-a")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "private/video.mp4") {
		t.Fatalf("playback response=%d %s", rr.Code, rr.Body.String())
	}
	if f.playUserID != 0 || f.playContentID != 7 {
		t.Fatalf("anonymous Playback args=(%d,%d)", f.playUserID, f.playContentID)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/7/play", strings.NewReader(`{"ticket":"`+ticket+`"}`))
	req.Header.Set("X-Device-ID", "device-a")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected anonymous refresh rate limit, got %d", rr.Code)
	}
}

func TestClassroomPlaybackAcceptsMiniappJWTWithoutAnonymousTicket(t *testing.T) {
	f := &fakeClassroomPublicService{play: classroomPlaybackSource{Content: classroom.Content{ID: 9, Status: classroom.ContentPublished, ContentType: classroom.ContentAudio, AccessLevel: classroom.AccessLogin}, Media: classroom.MediaAsset{ObjectKey: "private/audio.m4a", ETag: "audio-v1", StorageStatus: classroom.MediaReady}}}
	s := &Server{classroomPublic: f, classroomPlaybackSigner: fakeClassroomSigner{key: "audio"}, env: config.Env{JWTSecret: "jwt-secret"}}
	token, err := auth.Sign(auth.UserInfo{ID: 42, Roles: []string{miniappRole}, TokenKind: auth.TokenKindMiniapp}, "jwt-secret")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerClassroomPublicRoutes(mux, s)
	req := httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/9/play", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "https://cdn.example/audio") {
		t.Fatalf("jwt playback=%d %s", rr.Code, rr.Body.String())
	}
	if f.playUserID != 42 || f.playContentID != 9 {
		t.Fatalf("JWT Playback args=(%d,%d)", f.playUserID, f.playContentID)
	}
}

func TestClassroomPublicContentMetadataETagIsStableAndContainsNoTicket(t *testing.T) {
	f := &fakeClassroomPublicService{content: classroomPublicContent{ID: 5, Title: "Public", EffectiveAccess: classroom.AccessPublic, CanPlay: true}, play: classroomPlaybackSource{Content: classroom.Content{ID: 5, Status: classroom.ContentPublished, AccessLevel: classroom.AccessPublic}, Media: classroom.MediaAsset{ETag: "media-v2", StorageStatus: classroom.MediaReady}}}
	s := &Server{classroomPublic: f, env: config.Env{JWTSecret: "secret"}}
	mux := http.NewServeMux()
	registerClassroomPublicRoutes(mux, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/classroom/content/5", nil))
	if rr.Code != 200 || strings.Contains(rr.Body.String(), "playbackTicket") {
		t.Fatalf("first response=%d %s", rr.Code, rr.Body.String())
	}
	etag := rr.Header().Get("ETag")
	if f.playCalls != 0 {
		t.Fatalf("metadata must not fetch ticket source, calls=%d", f.playCalls)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/public/classroom/content/5", nil)
	req.Header.Set("If-None-Match", etag)
	cached := httptest.NewRecorder()
	mux.ServeHTTP(cached, req)
	if cached.Code != http.StatusNotModified || f.playCalls != 0 {
		t.Fatalf("304=%d source calls=%d", cached.Code, f.playCalls)
	}
}

func TestClassroomAnonymousTicketExpiresAndRefreshesThroughNoStoreEndpoint(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	f := &fakeClassroomPublicService{content: classroomPublicContent{ID: 5, EffectiveAccess: classroom.AccessPublic, CanPlay: true}, play: classroomPlaybackSource{Content: classroom.Content{ID: 5, Status: classroom.ContentPublished, AccessLevel: classroom.AccessPublic}, Media: classroom.MediaAsset{ETag: "media-v1", ObjectKey: "private/v.mp4", StorageStatus: classroom.MediaReady}}}
	s := &Server{classroomPublic: f, classroomPlaybackSigner: fakeClassroomSigner{key: "fresh"}, now: func() time.Time { return now }, env: config.Env{JWTSecret: "secret"}}
	mux := http.NewServeMux()
	registerClassroomPublicRoutes(mux, s)
	issue := func() string {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/public/classroom/content/5/ticket", nil))
		if rr.Code != 200 || rr.Header().Get("Cache-Control") != "no-store" || rr.Header().Get("ETag") != "" {
			t.Fatalf("ticket response=%d headers=%v body=%s", rr.Code, rr.Header(), rr.Body.String())
		}
		var body struct {
			Data struct {
				Ticket string `json:"ticket"`
			} `json:"data"`
		}
		if json.Unmarshal(rr.Body.Bytes(), &body) != nil || body.Data.Ticket == "" {
			t.Fatalf("ticket body=%s", rr.Body.String())
		}
		return body.Data.Ticket
	}
	old := issue()
	now = now.Add(5*time.Minute + time.Second)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/5/play", strings.NewReader(`{"ticket":"`+old+`"}`)))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expired status=%d", rr.Code)
	}
	fresh := issue()
	if fresh == old {
		t.Fatal("refresh must issue a fresh ticket")
	}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/5/play", strings.NewReader(`{"ticket":"`+fresh+`"}`)))
	if rr.Code != 200 {
		t.Fatalf("fresh status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestClassroomPublicEffectiveAccessAndPurchaseStates(t *testing.T) {
	series := classroom.Series{AccessLevel: classroom.AccessPaid}
	if got := accessFor(classroom.AccessInherit, &series); got != classroom.AccessPaid {
		t.Fatalf("inherit=%s", got)
	}
	cases := []struct {
		name                        string
		access                      classroom.AccessLevel
		logged, member, owned, want bool
	}{{"public", classroom.AccessPublic, false, false, false, true}, {"login anonymous", classroom.AccessLogin, false, false, false, false}, {"login JWT", classroom.AccessLogin, true, false, false, true}, {"member active", classroom.AccessMember, true, true, false, true}, {"member expired", classroom.AccessMember, true, false, false, false}, {"paid owned", classroom.AccessPaid, true, false, true, true}, {"paid missing", classroom.AccessPaid, true, false, false, false}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classroomAccessAllowed(tc.access, tc.logged, tc.member, tc.owned); got != tc.want {
				t.Fatalf("allowed=%v", got)
			}
		})
	}
	if classroomPurchaseState(classroom.AccessPaid, false) != "purchase_required" || classroomPurchaseState(classroom.AccessPaid, true) != "owned" {
		t.Fatal("paid purchase states")
	}
}

func TestClassroomDBBackedStandaloneFiltersBeforePaginationAndReturnsTrueTotal(t *testing.T) {
	var sawCount, sawPage bool
	db := openClassroomTestDB(t, func(q string, args []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(q, "count(*)") {
			sawCount = strings.Contains(q, "c.status=$1") && strings.Contains(q, "m.storage_status=$2") && strings.Contains(q, "show_as_standalone=true") && strings.Contains(q, "content_type=$3")
			return &classroomRows{cols: []string{"count"}, values: [][]driver.Value{{int64(3)}}}, nil
		}
		if strings.Contains(q, "ORDER BY c.sort_order,c.id") {
			sawPage = strings.Contains(q, "LIMIT $4 OFFSET $5") && len(args) == 5 && fmt.Sprint(args[3].Value) == "1" && fmt.Sprint(args[4].Value) == "2"
		}
		cols := []string{"id", "series_id", "show_as_standalone", "title", "description", "content_type", "media_asset_id", "cover_url", "duration_seconds", "teacher_name_snapshot", "access_level", "price_cents", "playback_blocked", "parent_id", "parent_status", "parent_access", "parent_price", "parent_blocked"}
		return &classroomRows{cols: cols, values: [][]driver.Value{{int64(12), nil, false, "Audio", "desc", "audio", int64(8), "cover", int64(90), "Teacher", "public", int64(0), false, nil, nil, nil, nil, nil}}}, nil
	})
	s := &Server{classroomPublic: newClassroomPublicDB(db), mux: http.NewServeMux()}
	registerClassroomPublicRoutes(s.mux, s)
	rr := httptest.NewRecorder()
	s.mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/classroom/standalone?contentType=audio&limit=1&offset=2", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"total":3`) || !strings.Contains(rr.Body.String(), `"limit":1`) || !strings.Contains(rr.Body.String(), `"offset":2`) {
		t.Fatalf("response=%d %s", rr.Code, rr.Body.String())
	}
	if !sawCount || !sawPage {
		t.Fatalf("filter/page SQL count=%v page=%v", sawCount, sawPage)
	}
}

func TestClassroomPublicDBBackedSeriesUsesAnyReadyLessonAndStablePagination(t *testing.T) {
	var eligible, stable bool
	db := openClassroomTestDB(t, func(q string, args []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(q, "classroom_entitlements") {
			return &classroomRows{cols: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
		}
		if strings.Contains(q, "count(*)") {
			eligible = strings.Contains(q, "EXISTS (SELECT 1") && strings.Contains(q, "m.storage_status=$3")
			return &classroomRows{cols: []string{"count"}, values: [][]driver.Value{{int64(4)}}}, nil
		}
		stable = strings.Contains(q, "ORDER BY s.sort_order,s.id LIMIT $5 OFFSET $6")
		return &classroomRows{cols: []string{"id", "title", "summary", "cover", "teacher", "access", "price", "blocked"}, values: [][]driver.Value{{int64(6), "Series", "summary", "cover", "Teacher", "paid", int64(9900), false}}}, nil
	})
	d := newClassroomPublicDB(db)
	items, total, err := d.ListSeries(context.Background(), classroomPublicQuery{Limit: 1, Offset: 3}, 0)
	if err != nil || total != 4 || len(items) != 1 {
		t.Fatalf("items=%+v total=%d err=%v", items, total, err)
	}
	if !eligible || !stable || items[0].PurchaseState != "purchase_required" || items[0].CanPlay {
		t.Fatalf("eligible=%v stable=%v item=%+v", eligible, stable, items[0])
	}
	s := &Server{classroomPublic: d, mux: http.NewServeMux()}
	registerClassroomPublicRoutes(s.mux, s)
	rr := httptest.NewRecorder()
	s.mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/classroom/series?limit=1&offset=3", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"total":4`) || !strings.Contains(rr.Body.String(), `"purchaseState":"purchase_required"`) {
		t.Fatalf("HTTP=%d %s", rr.Code, rr.Body.String())
	}
}

func TestClassroomPlaybackRejectsParentHardBlock(t *testing.T) {
	f := &fakeClassroomPublicService{play: classroomPlaybackSource{Content: classroom.Content{ID: 4, Status: classroom.ContentPublished, AccessLevel: classroom.AccessPublic}, Media: classroom.MediaAsset{ETag: "v1", StorageStatus: classroom.MediaReady}, Series: &classroom.Series{ID: 2, PlaybackBlocked: true}}}
	s := &Server{classroomPublic: f, env: config.Env{JWTSecret: "secret"}}
	ticket, _, _ := s.signClassroomTicket(4, "v1")
	mux := http.NewServeMux()
	registerClassroomPublicRoutes(mux, s)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/4/play", strings.NewReader(`{"ticket":"`+ticket+`"}`)))
	if rr.Code != http.StatusLocked {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestClassroomPublicContentMergesParentAccessPriceAndHardBlock(t *testing.T) {
	sid := int64(2)
	d := &classroomPublicDB{}
	c := classroom.Content{ID: 3, SeriesID: &sid, AccessLevel: classroom.AccessInherit, ContentType: classroom.ContentVideo}
	p := &classroom.Series{ID: sid, AccessLevel: classroom.AccessPublic, PriceCents: 0, PlaybackBlocked: true}
	v := d.contentViewReady(context.Background(), c, 0, p)
	if v.EffectiveAccess != classroom.AccessPublic || v.CanPlay || !v.PlaybackBlocked {
		t.Fatalf("view=%+v", v)
	}
}

func playbackDB(t *testing.T, access classroom.AccessLevel) *sql.DB {
	t.Helper()
	return openClassroomTestDB(t, func(q string, args []driver.NamedValue) (driver.Rows, error) {
		now := time.Now()
		if strings.Contains(q, "FROM classroom_contents WHERE id") {
			if len(args) != 1 || args[0].Value != int64(7) {
				t.Fatalf("content args=%v", args)
			}
			return &classroomRows{cols: make([]string, 25), values: [][]driver.Value{{int64(7), nil, false, "Lesson", "desc", "video", int64(3), "cover", int64(60), "teacher", "Teacher", nil, "", []byte(`[]`), int64(0), int64(0), "published", false, string(access), int64(0), now, nil, nil, now, now}}}, nil
		}
		if strings.Contains(q, "FROM classroom_media_assets WHERE id") {
			return &classroomRows{cols: make([]string, 15), values: [][]driver.Value{{int64(3), "bucket", "private/media.mp4", "media-v1", "crc", "video", int64(100), int64(60), int64(1280), int64(720), "", "ready", nil, now, now}}}, nil
		}
		return &classroomRows{cols: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
	})
}

func TestClassroomDBBackedPlaybackHandlerAnonymousAndJWTPaths(t *testing.T) {
	t.Run("anonymous public", func(t *testing.T) {
		db := playbackDB(t, classroom.AccessPublic)
		s := &Server{classroomPublic: newClassroomPublicDB(db), classroomPlaybackSigner: fakeClassroomSigner{key: "anon"}, env: config.Env{JWTSecret: "secret"}}
		ticket, _, _ := s.signClassroomTicket(7, "media-v1")
		mux := http.NewServeMux()
		registerClassroomPublicRoutes(mux, s)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/7/play", strings.NewReader(`{"ticket":"`+ticket+`"}`)))
		if rr.Code != http.StatusOK {
			t.Fatalf("response=%d %s", rr.Code, rr.Body.String())
		}
	})
	t.Run("JWT login", func(t *testing.T) {
		db := playbackDB(t, classroom.AccessLogin)
		s := &Server{classroomPublic: newClassroomPublicDB(db), classroomPlaybackSigner: fakeClassroomSigner{key: "jwt"}, env: config.Env{JWTSecret: "secret"}}
		token, _ := auth.Sign(auth.UserInfo{ID: 42, Roles: []string{miniappRole}, TokenKind: auth.TokenKindMiniapp}, "secret")
		mux := http.NewServeMux()
		registerClassroomPublicRoutes(mux, s)
		req := httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/7/play", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("response=%d %s", rr.Code, rr.Body.String())
		}
	})
}

func TestClassroomPublicSeriesDetailHTTPFiltersToPublishedReadyLessons(t *testing.T) {
	var readyFilter bool
	now := time.Now()
	db := openClassroomTestDB(t, func(q string, args []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(q, "FROM classroom_series WHERE id") {
			return &classroomRows{cols: make([]string, 17), values: [][]driver.Value{{int64(2), "Series", "summary", "cover", nil, "teacher", "Teacher", int64(1), "published", false, "public", int64(0), now, nil, nil, now, now}}}, nil
		}
		if strings.Contains(q, "JOIN classroom_media_assets") {
			readyFilter = strings.Contains(q, "c.status=$2") && strings.Contains(q, "m.storage_status=$3") && strings.Contains(q, "ORDER BY c.sort_order,c.id")
			return &classroomRows{cols: make([]string, 11), values: [][]driver.Value{{int64(11), int64(2), "Ready lesson", "desc", "video", "cover", int64(60), "Teacher", "inherit", int64(0), false}}}, nil
		}
		return &classroomRows{cols: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
	})
	s := &Server{classroomPublic: newClassroomPublicDB(db), mux: http.NewServeMux()}
	registerClassroomPublicRoutes(s.mux, s)
	rr := httptest.NewRecorder()
	s.mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/classroom/series/2", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "Ready lesson") || !readyFilter {
		t.Fatalf("HTTP=%d filter=%v body=%s", rr.Code, readyFilter, rr.Body.String())
	}
}

func TestClassroomPublicSeriesOfflineHiddenStandaloneVisibleAndPurchasedPlaybackAllowed(t *testing.T) {
	now := time.Now()
	db := openClassroomTestDB(t, func(q string, args []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(q, "SELECT count(*) FROM classroom_series s"):
			return &classroomRows{cols: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
		case strings.Contains(q, "SELECT s.id,s.title"):
			return &classroomRows{cols: make([]string, 8)}, nil
		case strings.Contains(q, "SELECT count(*) FROM classroom_contents c JOIN"):
			return &classroomRows{cols: []string{"count"}, values: [][]driver.Value{{int64(1)}}}, nil
		case strings.Contains(q, "SELECT c.id,c.series_id,c.show_as_standalone"):
			return &classroomRows{cols: make([]string, 18), values: [][]driver.Value{{int64(7), int64(2), true, "Standalone", "desc", "video", int64(3), "cover", int64(60), "Teacher", "inherit", int64(0), false, int64(2), "offline", "paid", int64(9900), false}}}, nil
		case strings.Contains(q, "FROM classroom_contents WHERE id"):
			return &classroomRows{cols: make([]string, 25), values: [][]driver.Value{{int64(7), int64(2), true, "Standalone", "desc", "video", int64(3), "cover", int64(60), "teacher", "Teacher", nil, "", []byte(`[]`), int64(0), int64(0), "published", false, "inherit", int64(0), now, nil, nil, now, now}}}, nil
		case strings.Contains(q, "FROM classroom_series WHERE id"):
			return &classroomRows{cols: make([]string, 17), values: [][]driver.Value{{int64(2), "Series", "summary", "cover", nil, "teacher", "Teacher", int64(0), "offline", false, "paid", int64(9900), now, nil, nil, now, now}}}, nil
		case strings.Contains(q, "FROM classroom_media_assets WHERE id"):
			return &classroomRows{cols: make([]string, 15), values: [][]driver.Value{{int64(3), "bucket", "private/media.mp4", "v1", "crc", "video", int64(100), int64(60), int64(1), int64(1), "", "ready", nil, now, now}}}, nil
		case strings.Contains(q, "classroom_entitlements"):
			return &classroomRows{cols: []string{"count"}, values: [][]driver.Value{{int64(1)}}}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", q)
		}
	})
	s := &Server{classroomPublic: newClassroomPublicDB(db), classroomPlaybackSigner: fakeClassroomSigner{key: "owned"}, mux: http.NewServeMux(), env: config.Env{JWTSecret: "secret"}}
	registerClassroomPublicRoutes(s.mux, s)
	series := httptest.NewRecorder()
	s.mux.ServeHTTP(series, httptest.NewRequest(http.MethodGet, "/api/public/classroom/series", nil))
	if series.Code != 200 || !strings.Contains(series.Body.String(), `"total":0`) {
		t.Fatalf("series=%d %s", series.Code, series.Body.String())
	}
	standalone := httptest.NewRecorder()
	s.mux.ServeHTTP(standalone, httptest.NewRequest(http.MethodGet, "/api/public/classroom/standalone", nil))
	if standalone.Code != 200 || !strings.Contains(standalone.Body.String(), "Standalone") || !strings.Contains(standalone.Body.String(), `"purchaseState":"purchase_required"`) {
		t.Fatalf("standalone=%d %s", standalone.Code, standalone.Body.String())
	}
	token, _ := auth.Sign(auth.UserInfo{ID: 42, Roles: []string{miniappRole}, TokenKind: auth.TokenKindMiniapp}, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/7/play", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	play := httptest.NewRecorder()
	s.mux.ServeHTTP(play, req)
	if play.Code != 200 {
		t.Fatalf("play=%d %s", play.Code, play.Body.String())
	}
}

func TestClassroomPlaybackContentOfflinePurchasedPlaybackIsRetained(t *testing.T) {
	now := time.Now()
	db := openClassroomTestDB(t, func(q string, args []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(q, "FROM classroom_contents WHERE id"):
			return &classroomRows{cols: make([]string, 25), values: [][]driver.Value{{int64(7), nil, false, "Offline", "desc", "audio", int64(3), "cover", int64(60), "teacher", "Teacher", nil, "", []byte(`[]`), int64(0), int64(0), "offline", false, "paid", int64(1900), now, nil, nil, now, now}}}, nil
		case strings.Contains(q, "FROM classroom_media_assets WHERE id"):
			return &classroomRows{cols: make([]string, 15), values: [][]driver.Value{{int64(3), "bucket", "private/media.m4a", "v1", "crc", "audio", int64(100), int64(60), int64(0), int64(0), "", "ready", nil, now, now}}}, nil
		case strings.Contains(q, "classroom_entitlements"):
			return &classroomRows{cols: []string{"count"}, values: [][]driver.Value{{int64(1)}}}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", q)
		}
	})
	s := &Server{classroomPublic: newClassroomPublicDB(db), classroomPlaybackSigner: fakeClassroomSigner{key: "offline-owned"}, mux: http.NewServeMux(), env: config.Env{JWTSecret: "secret"}}
	registerClassroomPublicRoutes(s.mux, s)
	token, _ := auth.Sign(auth.UserInfo{ID: 42, Roles: []string{miniappRole}, TokenKind: auth.TokenKindMiniapp}, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/7/play", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("play=%d %s", rr.Code, rr.Body.String())
	}
}

func TestClassroomPublicAnonymousAndJWTMetadataAreIsolatedByVaryAndETag(t *testing.T) {
	f := &fakeClassroomPublicService{viewerAware: true, content: classroomPublicContent{ID: 8, Title: "Paid", EffectiveAccess: classroom.AccessPaid, PurchaseState: "purchase_required"}}
	s := &Server{classroomPublic: f, mux: http.NewServeMux(), env: config.Env{JWTSecret: "secret"}}
	registerClassroomPublicRoutes(s.mux, s)
	anon := httptest.NewRecorder()
	s.mux.ServeHTTP(anon, httptest.NewRequest(http.MethodGet, "/api/public/classroom/standalone", nil))
	token, _ := auth.Sign(auth.UserInfo{ID: 42, Roles: []string{miniappRole}, TokenKind: auth.TokenKindMiniapp}, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/public/classroom/standalone", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	logged := httptest.NewRecorder()
	s.mux.ServeHTTP(logged, req)
	if anon.Header().Get("Vary") != "Authorization" || logged.Header().Get("Vary") != "Authorization" || anon.Header().Get("ETag") == logged.Header().Get("ETag") || !strings.Contains(anon.Body.String(), `"canPlay":false`) || !strings.Contains(logged.Body.String(), `"canPlay":true`) {
		t.Fatalf("anon headers=%v body=%s logged headers=%v body=%s", anon.Header(), anon.Body.String(), logged.Header(), logged.Body.String())
	}
}
