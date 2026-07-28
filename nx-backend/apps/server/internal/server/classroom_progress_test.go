package server

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/config"
)

type fakeClassroomProgressService struct {
	updated  classroomProgressView
	items    []classroomContinueLearningItem
	err      error
	uid      int64
	content  int64
	position int
}

func (f *fakeClassroomProgressService) Update(_ context.Context, uid, contentID int64, position int) (classroomProgressView, error) {
	f.uid, f.content, f.position = uid, contentID, position
	return f.updated, f.err
}

func (f *fakeClassroomProgressService) ContinueLearning(_ context.Context, uid int64) ([]classroomContinueLearningItem, error) {
	f.uid = uid
	return f.items, f.err
}

func miniappClassroomToken(t *testing.T, secret string, uid int64) string {
	t.Helper()
	token, err := auth.Sign(auth.UserInfo{ID: uid, Roles: []string{miniappRole}, TokenKind: auth.TokenKindMiniapp}, secret)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func progressTestMux(s *Server) *http.ServeMux {
	mux := http.NewServeMux()
	registerClassroomPublicRoutes(mux, s)
	registerClassroomProgressRoutes(mux, s.requireMiniapp, s)
	return mux
}

func TestClassroomProgressRoutesRequireMiniappJWTAndExposeUpdateAndContinue(t *testing.T) {
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	svc := &fakeClassroomProgressService{
		updated: classroomProgressView{ContentID: 21, PositionSeconds: 90, Completed: true, LastPlayedAt: now},
		items: []classroomContinueLearningItem{{
			classroomPublicContent: classroomPublicContent{ID: 21, Title: "第一课", ContentType: classroom.ContentVideo, CanPlay: true},
			PositionSeconds:        90, Completed: true, LastPlayedAt: now,
		}},
	}
	s := &Server{env: config.Env{JWTSecret: "progress-secret"}, classroomProgress: svc}
	mux := progressTestMux(s)

	unauthenticated := httptest.NewRecorder()
	mux.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPut, "/api/miniapp/classroom/content/21/progress", strings.NewReader(`{"positionSeconds":90}`)))
	if unauthenticated.Code != http.StatusUnauthorized || svc.uid != 0 {
		t.Fatalf("anonymous progress status=%d body=%s uid=%d", unauthenticated.Code, unauthenticated.Body.String(), svc.uid)
	}

	token := miniappClassroomToken(t, "progress-secret", 42)
	updateReq := httptest.NewRequest(http.MethodPut, "/api/miniapp/classroom/content/21/progress", strings.NewReader(`{"positionSeconds":90}`))
	updateReq.Header.Set("Authorization", "Bearer "+token)
	update := httptest.NewRecorder()
	mux.ServeHTTP(update, updateReq)
	if update.Code != http.StatusOK || update.Header().Get("Cache-Control") != "private, no-store" || svc.uid != 42 || svc.content != 21 || svc.position != 90 || !strings.Contains(update.Body.String(), `"completed":true`) {
		t.Fatalf("update status=%d body=%s call=(%d,%d,%d)", update.Code, update.Body.String(), svc.uid, svc.content, svc.position)
	}

	continueReq := httptest.NewRequest(http.MethodGet, "/api/miniapp/classroom/continue-learning", nil)
	continueReq.Header.Set("Authorization", "Bearer "+token)
	continued := httptest.NewRecorder()
	mux.ServeHTTP(continued, continueReq)
	if continued.Code != http.StatusOK || continued.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(continued.Body.String(), `"title":"第一课"`) || !strings.Contains(continued.Body.String(), `"positionSeconds":90`) {
		t.Fatalf("continue status=%d body=%s", continued.Code, continued.Body.String())
	}
}

func TestClassroomProgressRouteStrictInputBoundaries(t *testing.T) {
	svc := &fakeClassroomProgressService{}
	s := &Server{env: config.Env{JWTSecret: "secret"}, classroomProgress: svc}
	mux := progressTestMux(s)
	token := miniappClassroomToken(t, "secret", 7)

	for _, body := range []string{
		`{}`,
		`{"positionSeconds":-1}`,
		`{"positionSeconds":1.5}`,
		`{"positionSeconds":2592001}`,
		`{"positionSeconds":1,"completed":true}`,
		`{"positionSeconds":1} trailing`,
	} {
		req := httptest.NewRequest(http.MethodPut, "/api/miniapp/classroom/content/21/progress", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("body=%q status=%d response=%s", body, rr.Code, rr.Body.String())
		}
	}

	huge := httptest.NewRequest(http.MethodPut, "/api/miniapp/classroom/content/21/progress", strings.NewReader(`{"positionSeconds":1,"padding":"`+strings.Repeat("x", 5000)+`"}`))
	huge.Header.Set("Authorization", "Bearer "+token)
	hugeResponse := httptest.NewRecorder()
	mux.ServeHTTP(hugeResponse, huge)
	if hugeResponse.Code != http.StatusBadRequest {
		t.Fatalf("oversize status=%d body=%s", hugeResponse.Code, hugeResponse.Body.String())
	}

	wrongMethod := httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/content/21/progress", strings.NewReader(`{"positionSeconds":1}`))
	wrongMethod.Header.Set("Authorization", "Bearer "+token)
	wrongMethodResponse := httptest.NewRecorder()
	mux.ServeHTTP(wrongMethodResponse, wrongMethod)
	if wrongMethodResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status=%d", wrongMethodResponse.Code)
	}
	if svc.position != 0 {
		t.Fatalf("invalid requests reached service: %+v", svc)
	}
}

func TestClassroomProgressRouteRateLimitsPerUserAndContent(t *testing.T) {
	svc := &fakeClassroomProgressService{updated: classroomProgressView{ContentID: 21, PositionSeconds: 1}}
	s := &Server{
		env:                      config.Env{JWTSecret: "secret"},
		classroomProgress:        svc,
		classroomProgressLimiter: newBoundedStrRateLimiter(1, time.Minute, 10),
		now:                      func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC) },
	}
	mux := progressTestMux(s)
	token := miniappClassroomToken(t, "secret", 7)
	for i, contentID := range []string{"21", "21", "22"} {
		req := httptest.NewRequest(http.MethodPut, "/api/miniapp/classroom/content/"+contentID+"/progress", strings.NewReader(`{"positionSeconds":1}`))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		want := http.StatusOK
		if i == 1 {
			want = http.StatusTooManyRequests
		}
		if rr.Code != want {
			t.Fatalf("request %d content=%s status=%d want=%d body=%s", i, contentID, rr.Code, want, rr.Body.String())
		}
	}
}

func TestClassroomContinueLearningRouteRateLimitsPerUserWithBoundedKeys(t *testing.T) {
	svc := &fakeClassroomProgressService{items: []classroomContinueLearningItem{}}
	s := &Server{
		env:                      config.Env{JWTSecret: "secret"},
		classroomProgress:        svc,
		classroomContinueLimiter: newBoundedStrRateLimiter(1, time.Minute, 1),
		now:                      func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC) },
	}
	mux := progressTestMux(s)

	request := func(uid int64) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/miniapp/classroom/continue-learning", nil)
		req.Header.Set("Authorization", "Bearer "+miniappClassroomToken(t, "secret", uid))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	if rr := request(7); rr.Code != http.StatusOK {
		t.Fatalf("first request status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := request(7); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("repeated request status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := request(8); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("new key beyond bounded capacity status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := len(s.classroomContinueLimiter.keys); got != 1 {
		t.Fatalf("continue-learning limiter keys=%d want=1", got)
	}
}

type fakeClassroomProgressAccess struct {
	source classroomPlaybackSource
	err    error
}

func (f fakeClassroomProgressAccess) Playback(context.Context, int64, int64) (classroomPlaybackSource, error) {
	return f.source, f.err
}

type fakeClassroomProgressStore struct {
	got classroom.Progress
}

func (f *fakeClassroomProgressStore) UpsertProgress(_ context.Context, progress classroom.Progress) (classroom.Progress, error) {
	f.got = progress
	progress.CreatedAt = progress.LastPlayedAt
	progress.UpdatedAt = progress.LastPlayedAt
	return progress, nil
}

func TestClassroomProgressDBUsesMediaDurationForNinetyPercentAndClampsPosition(t *testing.T) {
	now := time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)
	store := &fakeClassroomProgressStore{}
	db := &classroomProgressDB{
		store: store,
		access: fakeClassroomProgressAccess{source: classroomPlaybackSource{
			Content: classroom.Content{ID: 21, DurationSeconds: 999},
			Media:   classroom.MediaAsset{DurationSeconds: 100, StorageStatus: classroom.MediaReady},
		}},
		now: func() time.Time { return now },
	}

	at89, err := db.Update(context.Background(), 42, 21, 89)
	if err != nil || at89.Completed {
		t.Fatalf("89%% progress=%+v err=%v", at89, err)
	}
	at90, err := db.Update(context.Background(), 42, 21, 90)
	if err != nil || !at90.Completed {
		t.Fatalf("90%% progress=%+v err=%v", at90, err)
	}
	clamped, err := db.Update(context.Background(), 42, 21, 120)
	if err != nil || clamped.PositionSeconds != 100 || store.got.PositionSeconds != 100 {
		t.Fatalf("clamped progress=%+v stored=%+v err=%v", clamped, store.got, err)
	}
}

func TestClassroomProgressDBRejectsInaccessibleContentAndAllowsOwnedOffline(t *testing.T) {
	store := &fakeClassroomProgressStore{}
	denied := &classroomProgressDB{store: store, access: fakeClassroomProgressAccess{err: classroom.ErrNotFound}}
	if _, err := denied.Update(context.Background(), 42, 7, 10); !errors.Is(err, classroom.ErrNotFound) {
		t.Fatalf("denied update err=%v", err)
	}
	if store.got.ContentID != 0 {
		t.Fatal("denied content wrote progress")
	}

	offline := &classroomProgressDB{
		store: store,
		access: fakeClassroomProgressAccess{source: classroomPlaybackSource{
			Content: classroom.Content{ID: 7, Status: classroom.ContentOffline, AccessLevel: classroom.AccessPaid},
			Media:   classroom.MediaAsset{DurationSeconds: 60, StorageStatus: classroom.MediaReady},
		}},
	}
	if _, err := offline.Update(context.Background(), 42, 7, 30); err != nil || store.got.ContentID != 7 {
		t.Fatalf("owned offline progress err=%v stored=%+v", err, store.got)
	}
}

func TestClassroomPlaybackAccessAllowsOnlyPublishedOrOwnedOfflineLifecycle(t *testing.T) {
	contentID := int64(7)
	owned := classroomAccessSnapshot{loggedIn: true, contentOwned: map[int64]bool{contentID: true}, seriesOwned: map[int64]bool{}}
	content := classroom.Content{ID: contentID, Status: classroom.ContentDraft, AccessLevel: classroom.AccessPaid}
	if classroomPlaybackAccessible(content, nil, owned) {
		t.Fatal("draft content must stay inaccessible even when a stale entitlement exists")
	}
	content.Status = classroom.ContentOffline
	if !classroomPlaybackAccessible(content, nil, owned) {
		t.Fatal("owned offline content must retain access")
	}
	content.Status = classroom.ContentPublished
	if !classroomPlaybackAccessible(content, nil, owned) {
		t.Fatal("owned published content must remain accessible")
	}
}

func TestClassroomContinueLearningFiltersAccessAndSortsNewestFirst(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	var candidateQuery string
	db := openClassroomTestDB(t, func(q string, _ []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(q, "SELECT member_level,member_expires_at"):
			return &classroomRows{cols: []string{"level", "expires"}, values: [][]driver.Value{{int64(0), nil}}}, nil
		case strings.Contains(q, "SELECT series_id,content_id FROM classroom_entitlements"):
			return &classroomRows{cols: []string{"series_id", "content_id"}, values: [][]driver.Value{{int64(8), nil}}}, nil
		case strings.Contains(q, "FROM classroom_progress p"):
			candidateQuery = q
			return &classroomRows{cols: make([]string, 24), values: [][]driver.Value{
				{int64(31), int64(20), false, now, "Newest denied", "", "", "video", int64(100), int64(9), false, "offline", false, "inherit", int64(0), "", "16:9", "", int64(9), "offline", false, "paid", int64(2990), "Teacher"},
				{int64(30), int64(50), false, now.Add(-time.Minute), "Owned offline", "desc", "cover", "audio", int64(100), int64(8), false, "offline", false, "inherit", int64(0), "", "16:9", "", int64(8), "offline", false, "paid", int64(2990), "Teacher"},
				{int64(29), int64(10), false, now.Add(-2 * time.Minute), "Public", "desc", "cover", "video", int64(100), nil, false, "published", false, "public", int64(0), "", "16:9", "", nil, nil, nil, nil, nil, "Teacher"},
			}}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", q)
		}
	})

	items, err := newClassroomProgressDB(db).ContinueLearning(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != 30 || items[1].ID != 29 || items[0].Title != "Owned offline" || items[1].Title != "Public" {
		t.Fatalf("unexpected continue-learning items: %+v", items)
	}
	if !items[0].CanPlay || !strings.Contains(candidateQuery, "ORDER BY p.last_played_at DESC,c.id DESC") || !strings.Contains(candidateQuery, "LIMIT") {
		t.Fatalf("missing access/sort contract: query=%s items=%+v", candidateQuery, items)
	}
}

func TestClassroomContinueLearningResolvesEffectiveCover(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	db := openClassroomTestDB(t, func(q string, _ []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(q, "SELECT member_level,member_expires_at"):
			return &classroomRows{cols: []string{"level", "expires"}, values: [][]driver.Value{{int64(0), nil}}}, nil
		case strings.Contains(q, "SELECT series_id,content_id FROM classroom_entitlements"):
			return &classroomRows{cols: []string{"series_id", "content_id"}}, nil
		case strings.Contains(q, "FROM classroom_progress p"):
			return &classroomRows{cols: make([]string, 24), values: [][]driver.Value{{
				int64(29), int64(10), false, now, "Public", "desc", "https://legacy.example/cover.jpg", "video", int64(100), nil, false, "published", false, "public", int64(0),
				"", "9:16", "classroom/covers/generated/29.jpg",
				nil, nil, nil, nil, nil, "Teacher",
			}}}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", q)
		}
	})
	signer := &recordingClassroomCoverSigner{}
	items, err := newClassroomProgressDBWithCovers(db, signer, 30*time.Minute).ContinueLearning(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].CoverURL != "https://signed.example/classroom-cover" || items[0].CoverAspectRatio != classroom.CoverAspectRatio9x16 {
		t.Fatalf("items=%+v", items)
	}
	if signer.key != "classroom/covers/generated/29.jpg" || signer.ttl != 30*time.Minute {
		t.Fatalf("sign key=%q ttl=%s", signer.key, signer.ttl)
	}
}

func classroomContinueRow(id int64, playedAt time.Time, title, contentStatus string, contentAccess string, seriesID any, standalone bool, seriesStatus any, seriesAccess any) []driver.Value {
	return []driver.Value{
		id, int64(10), false, playedAt,
		title, "desc", "cover", "video", int64(100), seriesID, standalone, contentStatus, false, contentAccess, int64(0),
		"", "16:9", "",
		seriesID, seriesStatus, false, seriesAccess, int64(2990), "Teacher",
	}
}

func TestClassroomContinueLearningScansKeysetBatchesUntilTwentyAccessibleItems(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	progressQueries := 0
	var cursors []struct {
		at time.Time
		id int64
	}
	db := openClassroomTestDB(t, func(q string, args []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(q, "SELECT member_level,member_expires_at"):
			return &classroomRows{cols: []string{"level", "expires"}, values: [][]driver.Value{{int64(0), nil}}}, nil
		case strings.Contains(q, "SELECT series_id,content_id FROM classroom_entitlements"):
			return &classroomRows{cols: []string{"series_id", "content_id"}}, nil
		case strings.Contains(q, "FROM classroom_progress p"):
			progressQueries++
			if !strings.Contains(q, "p.last_played_at <") || !strings.Contains(q, "p.last_played_at=") || !strings.Contains(q, "c.id <") {
				t.Fatalf("missing stable keyset cursor: %s", q)
			}
			if !strings.Contains(q, "c.playback_blocked=false") || !strings.Contains(q, "s.playback_blocked=false") || !strings.Contains(q, "c.status IN") || !strings.Contains(q, "m.storage_status") {
				t.Fatalf("missing SQL lifecycle/media/hard-stop filters: %s", q)
			}
			if len(args) != 9 || args[8].Value != int64(40) {
				t.Fatalf("batch args=%+v", args)
			}
			if progressQueries > 1 {
				cursors = append(cursors, struct {
					at time.Time
					id int64
				}{at: args[6].Value.(time.Time), id: args[7].Value.(int64)})
			}
			values := make([][]driver.Value, 0, 40)
			baseID := int64(1000 - (progressQueries-1)*40)
			for i := 0; i < 40; i++ {
				id := baseID - int64(i)
				title, access := fmt.Sprintf("Denied %d", id), "paid"
				if progressQueries == 2 && i < 25 {
					title, access = fmt.Sprintf("Allowed %02d", i), "public"
				}
				values = append(values, classroomContinueRow(id, now.Add(-time.Duration(progressQueries-1)*time.Minute), title, "published", access, nil, false, nil, nil))
			}
			return &classroomRows{cols: make([]string, 24), values: values}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", q)
		}
	})

	items, err := newClassroomProgressDB(db).ContinueLearning(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if progressQueries != 2 || len(items) != 20 {
		t.Fatalf("queries=%d items=%d", progressQueries, len(items))
	}
	if items[0].Title != "Allowed 00" || items[19].Title != "Allowed 19" {
		t.Fatalf("unexpected bounded result order: first=%q last=%q", items[0].Title, items[19].Title)
	}
	if len(cursors) != 1 || !cursors[0].at.Equal(now) || cursors[0].id != 961 {
		t.Fatalf("second batch cursor=%+v", cursors)
	}
}

func TestClassroomContinueLearningStopsAfterBoundedCandidateBudget(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	progressQueries := 0
	db := openClassroomTestDB(t, func(q string, args []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(q, "SELECT member_level,member_expires_at"):
			return &classroomRows{cols: []string{"level", "expires"}, values: [][]driver.Value{{int64(0), nil}}}, nil
		case strings.Contains(q, "SELECT series_id,content_id FROM classroom_entitlements"):
			return &classroomRows{cols: []string{"series_id", "content_id"}}, nil
		case strings.Contains(q, "FROM classroom_progress p"):
			progressQueries++
			if len(args) != 9 || args[8].Value != int64(40) {
				t.Fatalf("batch args=%+v", args)
			}
			values := make([][]driver.Value, 0, 40)
			baseID := int64(1000 - (progressQueries-1)*40)
			for i := 0; i < 40; i++ {
				id := baseID - int64(i)
				values = append(values, classroomContinueRow(id, now.Add(-time.Duration(progressQueries-1)*time.Minute), fmt.Sprintf("Denied %d", id), "published", "paid", nil, false, nil, nil))
			}
			return &classroomRows{cols: make([]string, 24), values: values}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", q)
		}
	})

	items, err := newClassroomProgressDB(db).ContinueLearning(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 || progressQueries != 5 {
		t.Fatalf("items=%d progress queries=%d want bounded 5", len(items), progressQueries)
	}
}

func TestClassroomContinueLearningPreservesStandaloneAndOfflineOwnedSemantics(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	db := openClassroomTestDB(t, func(q string, _ []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(q, "SELECT member_level,member_expires_at"):
			return &classroomRows{cols: []string{"level", "expires"}, values: [][]driver.Value{{int64(0), nil}}}, nil
		case strings.Contains(q, "SELECT series_id,content_id FROM classroom_entitlements"):
			return &classroomRows{cols: []string{"series_id", "content_id"}, values: [][]driver.Value{{int64(8), nil}, {nil, int64(77)}}}, nil
		case strings.Contains(q, "FROM classroom_progress p"):
			return &classroomRows{cols: make([]string, 24), values: [][]driver.Value{
				classroomContinueRow(80, now, "Standalone under draft series", "published", "public", int64(9), true, "draft", "paid"),
				classroomContinueRow(79, now.Add(-time.Minute), "Owned offline series lesson", "published", "inherit", int64(8), false, "offline", "paid"),
				classroomContinueRow(78, now.Add(-2*time.Minute), "Unowned offline series lesson", "published", "inherit", int64(7), false, "offline", "paid"),
				classroomContinueRow(77, now.Add(-3*time.Minute), "Owned offline content", "offline", "paid", nil, false, nil, nil),
				classroomContinueRow(76, now.Add(-4*time.Minute), "Unowned offline content", "offline", "public", nil, false, nil, nil),
			}}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", q)
		}
	})

	items, err := newClassroomProgressDB(db).ContinueLearning(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].ID != 80 || items[1].ID != 79 || items[2].ID != 77 {
		t.Fatalf("unexpected lifecycle/access items: %+v", items)
	}
}
