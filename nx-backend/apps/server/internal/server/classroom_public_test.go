package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/config"
)

type fakeClassroomPublicService struct {
	series  []classroomPublicSeries
	content classroomPublicContent
	play    classroomPlaybackSource
}
type fakeClassroomSigner struct{ key string }

func (f fakeClassroomSigner) PresignGetURL(context.Context, string, time.Duration) (string, error) {
	return "https://cdn.example/" + f.key, nil
}

func (f *fakeClassroomPublicService) ListSeries(context.Context, classroomPublicQuery, int64) ([]classroomPublicSeries, int, error) {
	return f.series, len(f.series), nil
}
func (f *fakeClassroomPublicService) ListStandalone(context.Context, classroomPublicQuery, int64) ([]classroomPublicContent, int, error) {
	return []classroomPublicContent{f.content}, 1, nil
}
func (f *fakeClassroomPublicService) GetSeries(context.Context, int64, int64) (classroomPublicSeriesDetail, error) {
	return classroomPublicSeriesDetail{Series: f.series[0], Contents: []classroomPublicContent{f.content}}, nil
}
func (f *fakeClassroomPublicService) GetContent(context.Context, int64, int64) (classroomPublicContent, error) {
	return f.content, nil
}
func (f *fakeClassroomPublicService) Playback(context.Context, int64, int64) (classroomPlaybackSource, error) {
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
}
