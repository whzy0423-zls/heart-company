package classroom

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSeriesValidationAndTransitions(t *testing.T) {
	valid := Series{Title: "九型入门", Status: SeriesDraft, AccessLevel: AccessPublic}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid series rejected: %v", err)
	}
	if !CanTransitionSeries(SeriesDraft, SeriesPublished) || !CanTransitionSeries(SeriesPublished, SeriesOffline) {
		t.Fatal("expected documented series transitions to be allowed")
	}
	if CanTransitionSeries(SeriesDraft, SeriesOffline) || CanTransitionSeries(SeriesOffline, SeriesPublished) {
		t.Fatal("unexpected series transition allowed")
	}
	for _, tc := range []Series{
		{Title: "x", Status: "unknown", AccessLevel: AccessPublic},
		{Title: "x", Status: SeriesDraft, AccessLevel: AccessInherit},
		{Title: "x", Status: SeriesDraft, AccessLevel: AccessPaid},
		{Title: "x", Status: SeriesDraft, AccessLevel: AccessPublic, PriceCents: 1},
	} {
		if err := tc.Validate(); err == nil {
			t.Fatalf("invalid series accepted: %+v", tc)
		}
	}
	paid := Series{Title: "付费系列", Status: SeriesDraft, AccessLevel: AccessPaid, PriceCents: 9900}
	if err := paid.Validate(); err != nil {
		t.Fatalf("valid paid series rejected: %v", err)
	}
	paid.PlaybackBlocked = true
	if err := paid.Validate(); err != nil {
		t.Fatalf("playback_blocked must be independently valid: %v", err)
	}
}

func TestContentValidationAndTransitions(t *testing.T) {
	seriesID := int64(7)
	valid := Content{SeriesID: &seriesID, ShowAsStandalone: true, Title: "第一课", ContentType: ContentVideo, Status: ContentReady, AccessLevel: AccessInherit}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid series content rejected: %v", err)
	}
	if !CanTransitionContent(ContentDraft, ContentProcessing) || !CanTransitionContent(ContentProcessing, ContentReady) || !CanTransitionContent(ContentReady, ContentPublished) || !CanTransitionContent(ContentPublished, ContentOffline) || !CanTransitionContent(ContentOffline, ContentPublished) || !CanTransitionContent(ContentFailed, ContentDraft) {
		t.Fatal("expected documented content transitions to be allowed")
	}
	if CanTransitionContent(ContentDraft, ContentPublished) || CanTransitionContent(ContentFailed, ContentPublished) {
		t.Fatal("unexpected content transition allowed")
	}
	standalone := Content{Title: "独立音频", ContentType: ContentAudio, Status: ContentDraft, AccessLevel: AccessInherit}
	if err := standalone.Validate(); err == nil {
		t.Fatal("standalone content must not inherit access")
	}
	standalone.AccessLevel = AccessLogin
	standalone.ShowAsStandalone = true
	if err := standalone.Validate(); err == nil {
		t.Fatal("show_as_standalone requires a parent series")
	}
	standalone.ShowAsStandalone = false
	standalone.AccessLevel = AccessPaid
	if err := standalone.Validate(); err == nil {
		t.Fatal("paid content requires a positive price")
	}
	standalone.PriceCents = 1990
	standalone.PlaybackBlocked = true
	if err := standalone.Validate(); err != nil {
		t.Fatalf("valid paid content rejected: %v", err)
	}
}

func TestNormalizeCoverAspectRatio(t *testing.T) {
	for input, want := range map[CoverAspectRatio]CoverAspectRatio{
		"":     CoverAspectRatio16x9,
		"16:9": CoverAspectRatio16x9,
		"9:16": CoverAspectRatio9x16,
		"1:1":  CoverAspectRatio1x1,
	} {
		got, err := NormalizeCoverAspectRatio(input)
		if err != nil {
			t.Fatalf("NormalizeCoverAspectRatio(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeCoverAspectRatio(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := NormalizeCoverAspectRatio("4:3"); err == nil {
		t.Fatal("unsupported cover aspect ratio accepted")
	}
}

func TestContentValidationRejectsUnsupportedCoverAspectRatio(t *testing.T) {
	content := Content{Title: "课件", ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic, CoverAspectRatio: "4:3"}
	if err := content.Validate(); err == nil {
		t.Fatal("content accepted unsupported cover aspect ratio")
	}
	content.CoverAspectRatio = ""
	if err := content.Validate(); err != nil {
		t.Fatalf("empty cover aspect ratio should normalize to default: %v", err)
	}
}

func TestContentPublishRequiresReadyMediaAndPublishedParent(t *testing.T) {
	seriesID := int64(3)
	content := Content{SeriesID: &seriesID, Title: "第一课", ContentType: ContentVideo, Status: ContentReady, AccessLevel: AccessInherit, MediaAssetID: ptrInt64(9)}
	media := MediaAsset{ID: 9, ContentType: ContentVideo, StorageStatus: MediaReady, Bucket: "private", ObjectKey: "classroom/video/9.mp4", ETag: "etag", Checksum: "sha256", SizeBytes: 1024, DurationSeconds: 60}
	publishedParent := Series{ID: 3, Title: "系列", Status: SeriesPublished, AccessLevel: AccessPublic}
	if err := ValidateContentPublish(content, media, &publishedParent); err != nil {
		t.Fatalf("publishable content rejected: %v", err)
	}
	media.StorageStatus = MediaProcessing
	if err := ValidateContentPublish(content, media, &publishedParent); err == nil {
		t.Fatal("content published before media ready")
	}
	media.StorageStatus = MediaReady
	publishedParent.Status = SeriesDraft
	if err := ValidateContentPublish(content, media, &publishedParent); err == nil {
		t.Fatal("content published before parent series")
	}
	if err := ValidateContentPublish(content, media, nil); err == nil {
		t.Fatal("series content published without parent snapshot")
	}
}

func TestMediaUploadEntitlementAndProgressValidation(t *testing.T) {
	media := MediaAsset{Bucket: "private", ObjectKey: "classroom/audio/a.mp3", ETag: "etag", Checksum: "sha256", ContentType: ContentAudio, SizeBytes: 10, DurationSeconds: 2, StorageStatus: MediaReady}
	if err := media.Validate(); err != nil {
		t.Fatalf("valid media rejected: %v", err)
	}
	media.ObjectKey = ""
	if err := media.Validate(); err == nil {
		t.Fatal("ready media without object metadata accepted")
	}
	task := UploadTask{ContentID: 4, CreatorID: 8, OSSUploadID: "upload", ObjectKey: "classroom/video/4.mp4", ExpectedSize: 100, Checksum: "sum", PartSize: 10, MaxParts: 10, Status: UploadInitiated, ExpiresAt: time.Now().Add(time.Hour)}
	if err := task.Validate(); err != nil {
		t.Fatalf("valid upload task rejected: %v", err)
	}
	task.ContentID = 0
	if err := task.Validate(); err == nil {
		t.Fatal("upload task without unique draft binding accepted")
	}
	seriesID, contentID := int64(1), int64(2)
	for _, entitlement := range []Entitlement{{WXUserID: 1, SeriesID: &seriesID, Source: EntitlementPurchase}, {WXUserID: 1, ContentID: &contentID, Source: EntitlementManual}} {
		if err := entitlement.Validate(); err != nil {
			t.Fatalf("valid entitlement rejected: %v", err)
		}
	}
	if err := (Entitlement{WXUserID: 1, SeriesID: &seriesID, ContentID: &contentID, Source: EntitlementPurchase}).Validate(); err == nil {
		t.Fatal("entitlement targeting both series and content accepted")
	}
	if err := (Entitlement{WXUserID: 1, Source: EntitlementPurchase}).Validate(); err == nil {
		t.Fatal("entitlement without target accepted")
	}
}

func TestStoreUpsertProgressKeepsMonotonicStateAndRefreshesRecentPlayback(t *testing.T) {
	now := time.Date(2026, 7, 27, 9, 30, 0, 0, time.UTC)
	created := now.Add(-time.Hour)
	db := openClassroomQueryDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		for _, fragment := range []string{
			"GREATEST(classroom_progress.position_seconds,EXCLUDED.position_seconds)",
			"classroom_progress.completed OR EXCLUDED.completed",
			"GREATEST(classroom_progress.last_played_at,EXCLUDED.last_played_at)",
			"EXCLUDED.last_played_at > classroom_progress.last_played_at",
		} {
			if !strings.Contains(query, fragment) {
				t.Fatalf("progress upsert is missing monotonic guard %q: %s", fragment, query)
			}
		}
		if len(args) != 5 || args[0].Value != int64(8) || args[1].Value != int64(4) || args[2].Value != int64(45) || args[3].Value != true || args[4].Value != now {
			t.Fatalf("unexpected progress args: %+v", args)
		}
		return classroomRows(
			[]string{"position_seconds", "completed", "last_played_at", "created_at", "updated_at"},
			[][]driver.Value{{int64(80), true, now, created, now}},
		), nil
	})

	got, err := NewStore(db).UpsertProgress(context.Background(), Progress{
		WXUserID: 8, ContentID: 4, PositionSeconds: 45, Completed: true, LastPlayedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.PositionSeconds != 80 || !got.Completed || !got.LastPlayedAt.Equal(now) || !got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(now) {
		t.Fatalf("upsert must return persisted monotonic progress, got %+v", got)
	}
}

func ptrInt64(value int64) *int64 { return &value }

func TestUploadTaskBindsExactlyOneDraft(t *testing.T) {
	task := UploadTask{ContentID: 4, CreatorID: 8, OSSUploadID: "upload", ObjectKey: "classroom/video/4.mp4", ExpectedSize: 100, Checksum: "sum", PartSize: 10, MaxParts: 10, Status: UploadInitiated, ExpiresAt: time.Now().Add(time.Hour)}
	draft := Content{ID: 4, Title: "待上传", ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}
	if err := ValidateUploadDraftBinding(task, draft); err != nil {
		t.Fatalf("valid draft binding rejected: %v", err)
	}
	draft.Status = ContentReady
	if err := ValidateUploadDraftBinding(task, draft); err == nil {
		t.Fatal("upload task bound to non-draft content")
	}
	draft.Status = ContentFailed
	if err := ValidateUploadDraftBinding(task, draft); err == nil {
		t.Fatal("failed content must return to draft before a new upload task")
	}
	draft.Status = ContentDraft
	draft.ID = 5
	if err := ValidateUploadDraftBinding(task, draft); err == nil {
		t.Fatal("upload task bound to a different draft")
	}
}

func TestStoreCreateContentRejectsPublishedTarget(t *testing.T) {
	store := NewStore(nil)
	_, err := store.CreateContent(context.Background(), Content{Title: "绕过发布", ContentType: ContentVideo, Status: ContentPublished, AccessLevel: AccessPublic})
	if err == nil {
		t.Fatal("CreateContent accepted published content without ready media")
	}
}

func TestStoreCreateContentPersistsNilTagsAsJSONArray(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	db := openClassroomQueryDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		if !strings.Contains(query, "INSERT INTO classroom_contents") {
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
		if len(args) < 15 || args[7].Value != "covers/manual.jpg" || args[8].Value != string(CoverAspectRatio16x9) || args[14].Value != "[]" {
			t.Fatalf("unexpected cover/tags args: %+v", args)
		}
		return classroomRows(
			[]string{"id", "created_at", "updated_at"},
			[][]driver.Value{{int64(1), now, now}},
		), nil
	})

	created, err := NewStore(db).CreateContent(context.Background(), Content{
		Title:                "企业培训案例",
		ContentType:          ContentVideo,
		ManualCoverObjectKey: "covers/manual.jpg",
		Status:               ContentDraft,
		AccessLevel:          AccessPublic,
	})
	if err != nil {
		t.Fatalf("create content: %v", err)
	}
	if created.ID != 1 {
		t.Fatalf("created id = %d, want 1", created.ID)
	}
	if created.CoverAspectRatio != CoverAspectRatio16x9 {
		t.Fatalf("created cover ratio = %q, want default %q", created.CoverAspectRatio, CoverAspectRatio16x9)
	}
}

func TestStoreGetAndListContentsScanCoverSettings(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	want := Content{
		ID: 9, Title: "竖屏课件", ContentType: ContentVideo,
		ManualCoverObjectKey: "covers/portrait.jpg", CoverAspectRatio: CoverAspectRatio9x16,
		Status: ContentDraft, AccessLevel: AccessPublic, CreatedAt: now, UpdatedAt: now,
	}
	db := openClassroomQueryDB(t, func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
		if !strings.Contains(query, "FROM classroom_contents") {
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
		return classroomRows(contentColumns, [][]driver.Value{contentValues(want)}), nil
	})
	store := NewStore(db)
	got, err := store.GetContent(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("get content: %v", err)
	}
	if got.ManualCoverObjectKey != want.ManualCoverObjectKey || got.CoverAspectRatio != want.CoverAspectRatio {
		t.Fatalf("get cover settings = (%q, %q), want (%q, %q)", got.ManualCoverObjectKey, got.CoverAspectRatio, want.ManualCoverObjectKey, want.CoverAspectRatio)
	}
	items, err := store.ListContents(context.Background(), ContentFilter{})
	if err != nil {
		t.Fatalf("list contents: %v", err)
	}
	if len(items) != 1 || items[0].ManualCoverObjectKey != want.ManualCoverObjectKey || items[0].CoverAspectRatio != want.CoverAspectRatio {
		t.Fatalf("list cover settings = %+v, want %+v", items, want)
	}
}

func TestStoreUpdateContentPersistsNilTagsAsJSONArray(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	current := Content{
		ID:                   1,
		Title:                "企业培训案例",
		ContentType:          ContentVideo,
		ManualCoverObjectKey: "covers/square.jpg",
		CoverAspectRatio:     CoverAspectRatio1x1,
		Status:               ContentDraft,
		AccessLevel:          AccessPublic,
		UpdatedAt:            now,
	}
	state := &classroomTxState{}
	db := openClassroomTxDB(t, state, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(query, "FROM classroom_contents WHERE id"):
			return classroomRows(contentColumns, [][]driver.Value{contentValues(current)}), nil
		case strings.Contains(query, "UPDATE classroom_contents"):
			if len(args) < 25 || args[7].Value != current.ManualCoverObjectKey || args[8].Value != string(CoverAspectRatio1x1) || args[14].Value != "[]" {
				t.Fatalf("unexpected cover/tags args: %+v", args)
			}
			return classroomRows(
				[]string{"created_at", "updated_at"},
				[][]driver.Value{{now.Add(-time.Hour), now.Add(time.Second)}},
			), nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
	})

	if _, err := NewStore(db).UpdateContent(context.Background(), current, now); err != nil {
		t.Fatalf("update content: %v", err)
	}
}

func TestStoreUpdatePublishedContentRevalidatesInvariant(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	current := Content{ID: 7, Title: "已发布", ContentType: ContentVideo, Status: ContentPublished, AccessLevel: AccessPublic, UpdatedAt: now}
	db := openClassroomQueryDB(t, func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(query, "FROM classroom_contents WHERE id") {
			return classroomRows(contentColumns, [][]driver.Value{contentValues(current)}), nil
		}
		if strings.Contains(query, "UPDATE classroom_contents") {
			return classroomRows([]string{"created_at", "updated_at"}, [][]driver.Value{{now, now.Add(time.Second)}}), nil
		}
		return nil, fmt.Errorf("unexpected query: %s", query)
	})
	_, err := NewStore(db).UpdateContent(context.Background(), current, now)
	if err == nil {
		t.Fatal("published-to-published update removed media invariant")
	}
}

func TestStoreOptimisticUpdateRequiresVersionAndDetectsStaleWrite(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	series := Series{ID: 3, Title: "系列", Status: SeriesDraft, AccessLevel: AccessPublic, UpdatedAt: now}
	if _, err := NewStore(nil).UpdateSeries(context.Background(), series, time.Time{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("zero expectedUpdatedAt error = %v, want ErrConflict", err)
	}
	content := Content{ID: 4, Title: "课时", ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}
	if _, err := NewStore(nil).UpdateContent(context.Background(), content, time.Time{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("zero content expectedUpdatedAt error = %v, want ErrConflict", err)
	}

	db := openClassroomQueryDB(t, func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(query, "FROM classroom_series WHERE id") {
			return classroomRows(seriesColumns, [][]driver.Value{seriesValues(series)}), nil
		}
		if strings.Contains(query, "UPDATE classroom_series") {
			return classroomRows([]string{"created_at", "updated_at"}, nil), nil
		}
		return nil, fmt.Errorf("unexpected query: %s", query)
	})
	if _, err := NewStore(db).UpdateSeries(context.Background(), series, now.Add(-time.Second)); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update error = %v, want ErrConflict", err)
	}
}

func TestStoreOptimisticUpdateSucceedsWithMatchingVersion(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	series := Series{ID: 3, Title: "系列", Status: SeriesDraft, AccessLevel: AccessPublic, UpdatedAt: now}
	db := openClassroomQueryDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(query, "FROM classroom_series WHERE id") {
			return classroomRows(seriesColumns, [][]driver.Value{seriesValues(series)}), nil
		}
		if strings.Contains(query, "UPDATE classroom_series") {
			if len(args) != 15 || args[14].Value != now {
				t.Fatalf("series CAS args = %+v", args)
			}
			return classroomRows([]string{"created_at", "updated_at"}, [][]driver.Value{{now.Add(-time.Hour), now.Add(time.Second)}}), nil
		}
		return nil, fmt.Errorf("unexpected query: %s", query)
	})
	updated, err := NewStore(db).UpdateSeries(context.Background(), series, now)
	if err != nil {
		t.Fatalf("matching optimistic update rejected: %v", err)
	}
	if !updated.UpdatedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("updated timestamp = %v", updated.UpdatedAt)
	}
}

func TestStoreFinishUploadCleanupUsesCleaningVersionCAS(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	db := openClassroomQueryDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		if !strings.Contains(query, "status='cleaning' AND updated_at=$5") {
			t.Fatalf("cleanup finish is not guarded by cleaning version: %s", query)
		}
		if len(args) != 5 || args[4].Value != now {
			t.Fatalf("cleanup finish CAS args = %+v", args)
		}
		return classroomRows([]string{"id", "content_id", "creator_id", "oss_upload_id", "object_key", "expected_size", "checksum", "part_size", "max_parts", "status", "expires_at", "attempt_count", "cleanup_status", "media_asset_id", "failure_reason", "created_at", "updated_at"}, nil), nil
	})
	_, ok, err := NewStore(db).FinishUploadCleanup(context.Background(), UploadTask{ID: 1, UpdatedAt: now}, UploadExpired, "cleaned", "")
	if err != nil || ok {
		t.Fatalf("stale cleanup finish ok=%v err=%v", ok, err)
	}
}

func TestStoreListExpiredUploadTasksIncludesOnlyStaleCleaningRows(t *testing.T) {
	db := openClassroomQueryDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		if !strings.Contains(query, "status='cleaning'") || !strings.Contains(query, "updated_at <= now()-interval '15 minutes'") {
			t.Fatalf("expired cleanup query does not include stale cleaning lease: %s", query)
		}
		if len(args) != 1 || args[0].Value != int64(10) {
			t.Fatalf("unexpected list args: %+v", args)
		}
		return classroomRows([]string{"id", "content_id", "creator_id", "oss_upload_id", "object_key", "expected_size", "checksum", "part_size", "max_parts", "status", "expires_at", "attempt_count", "cleanup_status", "media_asset_id", "failure_reason", "created_at", "updated_at"}, nil), nil
	})
	if _, err := NewStore(db).ListExpiredUploadTasks(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
}

func TestStoreClaimUploadCleanupCanReclaimStaleCleaningLease(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	db := openClassroomQueryDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		if !strings.Contains(query, "status='cleaning'") || !strings.Contains(query, "updated_at <= now()-interval '15 minutes'") {
			t.Fatalf("cleanup claim query does not guard stale cleaning lease: %s", query)
		}
		return classroomRows([]string{"id", "content_id", "creator_id", "oss_upload_id", "object_key", "expected_size", "checksum", "part_size", "max_parts", "status", "expires_at", "attempt_count", "cleanup_status", "media_asset_id", "failure_reason", "created_at", "updated_at"}, nil), nil
	})
	_, ok, err := NewStore(db).ClaimUploadCleanup(context.Background(), UploadTask{ID: 1, Status: UploadCleaning, UpdatedAt: now}, UploadExpired)
	if err != nil || ok {
		t.Fatalf("stale cleanup claim ok=%v err=%v", ok, err)
	}
}

var classroomDriverSequence atomic.Int64

func openClassroomQueryDB(t *testing.T, query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("classroom_store_test_%d", classroomDriverSequence.Add(1))
	sql.Register(name, classroomQueryDriver{query: query})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type classroomQueryDriver struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (d classroomQueryDriver) Open(string) (driver.Conn, error) {
	return classroomQueryConn{query: d.query}, nil
}

type classroomQueryConn struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (classroomQueryConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (classroomQueryConn) Close() error                        { return nil }
func (classroomQueryConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c classroomQueryConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type classroomTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func classroomRows(columns []string, values [][]driver.Value) driver.Rows {
	return &classroomTestRows{columns: columns, values: values}
}
func (r *classroomTestRows) Columns() []string { return r.columns }
func (r *classroomTestRows) Close() error      { return nil }
func (r *classroomTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

var seriesColumns = []string{"id", "title", "summary", "cover_url", "cover_asset_id", "teacher_key", "teacher_name_snapshot", "sort_order", "status", "playback_blocked", "access_level", "price_cents", "published_at", "created_by", "updated_by", "created_at", "updated_at"}

func seriesValues(s Series) []driver.Value {
	return []driver.Value{s.ID, s.Title, s.Summary, s.CoverURL, nil, s.TeacherKey, s.TeacherNameSnapshot, int64(s.SortOrder), string(s.Status), s.PlaybackBlocked, string(s.AccessLevel), int64(s.PriceCents), nil, nil, nil, s.CreatedAt, s.UpdatedAt}
}

var contentColumns = []string{"id", "series_id", "show_as_standalone", "title", "description", "content_type", "media_asset_id", "cover_url", "manual_cover_object_key", "cover_aspect_ratio", "duration_seconds", "teacher_key", "teacher_name_snapshot", "recorded_at", "badge", "tags", "episode_no", "sort_order", "status", "playback_blocked", "access_level", "price_cents", "published_at", "created_by", "updated_by", "created_at", "updated_at"}

func contentValues(c Content) []driver.Value {
	return []driver.Value{c.ID, nullableInt64(c.SeriesID), c.ShowAsStandalone, c.Title, c.Description, string(c.ContentType), nullableInt64(c.MediaAssetID), c.CoverURL, c.ManualCoverObjectKey, string(c.CoverAspectRatio), int64(c.DurationSeconds), c.TeacherKey, c.TeacherNameSnapshot, nullableTime(c.RecordedAt), c.Badge, []byte(`[]`), int64(c.EpisodeNo), int64(c.SortOrder), string(c.Status), c.PlaybackBlocked, string(c.AccessLevel), int64(c.PriceCents), nullableTime(c.PublishedAt), nullableInt64(c.CreatedBy), nullableInt64(c.UpdatedBy), c.CreatedAt, c.UpdatedAt}
}
func nullableInt64(value *int64) driver.Value {
	if value == nil {
		return nil
	}
	return *value
}
func nullableTime(value *time.Time) driver.Value {
	if value == nil {
		return nil
	}
	return *value
}

func TestStoreCreateOnlyAcceptsDraftInitialState(t *testing.T) {
	for _, status := range []SeriesStatus{SeriesPublished, SeriesOffline} {
		_, err := NewStore(nil).CreateSeries(context.Background(), Series{Title: "非法初态", Status: status, AccessLevel: AccessPublic})
		if err == nil {
			t.Fatalf("CreateSeries accepted initial status %q", status)
		}
	}
	for _, status := range []ContentStatus{ContentProcessing, ContentReady, ContentPublished, ContentOffline, ContentFailed} {
		_, err := NewStore(nil).CreateContent(context.Background(), Content{Title: "非法初态", ContentType: ContentVideo, Status: status, AccessLevel: AccessPublic})
		if err == nil {
			t.Fatalf("CreateContent accepted initial status %q", status)
		}
	}
}

func TestStorePublishedContentUpdateLocksValidationRowsInTransaction(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	mediaID, seriesID := int64(9), int64(3)
	current := Content{ID: 7, SeriesID: &seriesID, Title: "已发布", ContentType: ContentVideo, MediaAssetID: &mediaID, Status: ContentPublished, AccessLevel: AccessInherit, UpdatedAt: now}
	parent := Series{ID: seriesID, Title: "系列", Status: SeriesPublished, AccessLevel: AccessPublic, UpdatedAt: now}
	media := MediaAsset{ID: mediaID, Bucket: "private", ObjectKey: "classroom/video/9.mp4", ETag: "etag", Checksum: "sum", ContentType: ContentVideo, SizeBytes: 10, DurationSeconds: 2, StorageStatus: MediaReady}
	state := &classroomTxState{}
	db := openClassroomTxDB(t, state, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(query, "FROM classroom_contents WHERE id"):
			if !strings.Contains(query, "FOR UPDATE") {
				t.Fatalf("content validation query lacks FOR UPDATE: %s", query)
			}
			return classroomRows(contentColumns, [][]driver.Value{contentValues(current)}), nil
		case strings.Contains(query, "FROM classroom_media_assets WHERE id"):
			if !strings.Contains(query, "FOR UPDATE") {
				t.Fatalf("media validation query lacks FOR UPDATE: %s", query)
			}
			return classroomRows(mediaColumns, [][]driver.Value{mediaValues(media)}), nil
		case strings.Contains(query, "FROM classroom_series WHERE id"):
			if !strings.Contains(query, "FOR UPDATE") {
				t.Fatalf("parent series query lacks FOR UPDATE: %s", query)
			}
			return classroomRows(seriesColumns, [][]driver.Value{seriesValues(parent)}), nil
		case strings.Contains(query, "UPDATE classroom_contents"):
			if len(args) != 25 || args[24].Value != now {
				t.Fatalf("content CAS args = %+v", args)
			}
			return classroomRows([]string{"created_at", "updated_at"}, [][]driver.Value{{now.Add(-time.Hour), now.Add(time.Second)}}), nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
	})
	updated, err := NewStore(db).UpdateContent(context.Background(), current, now)
	if err != nil {
		t.Fatalf("published update: %v", err)
	}
	if !state.began || !state.committed || state.rolledBack {
		t.Fatalf("transaction state: %+v", state)
	}
	if !updated.UpdatedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("updated timestamp = %v", updated.UpdatedAt)
	}
}

func TestStoreDatabaseErrorsIncludeOperationContext(t *testing.T) {
	sentinel := errors.New("db unavailable")
	db := openClassroomQueryDB(t, func(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) { return nil, sentinel })
	_, err := NewStore(db).GetSeries(context.Background(), 1)
	if !errors.Is(err, sentinel) {
		t.Fatalf("wrapped error lost cause: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "get classroom series") {
		t.Fatalf("database error lacks operation context: %v", err)
	}
}

var mediaColumns = []string{"id", "bucket", "object_key", "etag", "checksum", "content_type", "size_bytes", "duration_seconds", "width", "height", "cover_object_key", "storage_status", "created_by", "created_at", "updated_at"}

func mediaValues(m MediaAsset) []driver.Value {
	return []driver.Value{m.ID, m.Bucket, m.ObjectKey, m.ETag, m.Checksum, string(m.ContentType), m.SizeBytes, int64(m.DurationSeconds), int64(m.Width), int64(m.Height), m.CoverObjectKey, string(m.StorageStatus), nil, m.CreatedAt, m.UpdatedAt}
}

type classroomTxState struct{ began, committed, rolledBack bool }

func openClassroomTxDB(t *testing.T, state *classroomTxState, query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("classroom_tx_test_%d", classroomDriverSequence.Add(1))
	sql.Register(name, classroomTxDriver{state: state, query: query})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type classroomTxDriver struct {
	state *classroomTxState
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (d classroomTxDriver) Open(string) (driver.Conn, error) {
	return &classroomTxConn{state: d.state, query: d.query}, nil
}

type classroomTxConn struct {
	state *classroomTxState
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (*classroomTxConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*classroomTxConn) Close() error                        { return nil }
func (c *classroomTxConn) Begin() (driver.Tx, error) {
	c.state.began = true
	return classroomTx{state: c.state}, nil
}
func (c *classroomTxConn) QueryContext(ctx context.Context, q string, a []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, q, a)
}

type classroomTx struct{ state *classroomTxState }

func (t classroomTx) Commit() error   { t.state.committed = true; return nil }
func (t classroomTx) Rollback() error { t.state.rolledBack = true; return nil }

func TestStoreContentOptimisticUpdateDetectsStaleVersion(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	content := Content{ID: 4, Title: "草稿", ContentType: ContentAudio, Status: ContentDraft, AccessLevel: AccessPublic, UpdatedAt: now}
	state := &classroomTxState{}
	db := openClassroomTxDB(t, state, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(query, "FROM classroom_contents WHERE id") {
			return classroomRows(contentColumns, [][]driver.Value{contentValues(content)}), nil
		}
		if strings.Contains(query, "UPDATE classroom_contents") {
			if len(args) != 25 || args[24].Value != now.Add(-time.Second) {
				t.Fatalf("content stale CAS args = %+v", args)
			}
			return classroomRows([]string{"created_at", "updated_at"}, nil), nil
		}
		return nil, fmt.Errorf("unexpected query: %s", query)
	})
	_, err := NewStore(db).UpdateContent(context.Background(), content, now.Add(-time.Second))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale content update error = %v, want ErrConflict", err)
	}
	if !state.began || !state.rolledBack || state.committed {
		t.Fatalf("stale transaction state: %+v", state)
	}
}

func TestStoreSetContentManualCoverUsesTimestampCASAndReturnsLatestContent(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	want := Content{ID: 44, Title: "封面课件", ContentType: ContentVideo, Status: ContentPublished, AccessLevel: AccessPublic, ManualCoverObjectKey: "classroom/covers/manual/44/new.png", CoverAspectRatio: CoverAspectRatio16x9, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(time.Second)}
	db := openClassroomQueryDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		if !strings.Contains(query, "UPDATE classroom_contents SET manual_cover_object_key") || len(args) != 4 || args[0].Value != want.ManualCoverObjectKey || args[2].Value != int64(44) || args[3].Value != now {
			t.Fatalf("query=%s args=%+v", query, args)
		}
		return classroomRows(contentColumns, [][]driver.Value{contentValues(want)}), nil
	})
	got, err := NewStore(db).SetContentManualCover(context.Background(), 44, want.ManualCoverObjectKey, now, ptrInt64(7))
	if err != nil || got.ManualCoverObjectKey != want.ManualCoverObjectKey || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestStoreSetContentManualCoverMapsNoRowsToConflict(t *testing.T) {
	now := time.Now().UTC()
	db := openClassroomQueryDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		return classroomRows(contentColumns, nil), nil
	})
	_, err := NewStore(db).SetContentManualCover(context.Background(), 1, "x", now, nil)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestStoreSaveUploadTaskPersistsSchemaCleanupValueCleaned(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	db := openClassroomQueryDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		if !strings.Contains(query, "UPDATE classroom_upload_tasks") {
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
		if len(args) != 17 || args[13].Value != "cleaned" {
			t.Fatalf("cleanup args=%+v", args)
		}
		return classroomRows([]string{"created_at", "updated_at"}, [][]driver.Value{{now, now.Add(time.Second)}}), nil
	})
	task := UploadTask{ID: 1, ContentID: 7, CreatorID: 42, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 10, Checksum: "crc64:123", PartSize: 10, MaxParts: 1, Status: UploadAborted, ExpiresAt: now.Add(time.Hour), AttemptCount: 1, CleanupStatus: "cleaned"}
	got, err := NewStore(db).SaveUploadTask(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if got.CleanupStatus != "cleaned" {
		t.Fatalf("got %+v", got)
	}
}

func TestUploadTaskCleanupStatusMatchesSchemaConstraint(t *testing.T) {
	base := UploadTask{ContentID: 1, CreatorID: 1, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 1, Checksum: "crc64:1", PartSize: 1, MaxParts: 1, Status: UploadAborted, ExpiresAt: time.Now().Add(time.Hour), AttemptCount: 1}
	base.CleanupStatus = "clean"
	if err := base.Validate(); err == nil {
		t.Fatal("legacy clean value accepted")
	}
	base.CleanupStatus = "cleaned"
	if err := base.Validate(); err != nil {
		t.Fatalf("cleaned rejected: %v", err)
	}
}

func TestStoreSaveUploadTaskRejectsCleanupOutsideSchemaConstraint(t *testing.T) {
	task := UploadTask{ID: 1, ContentID: 1, CreatorID: 1, OSSUploadID: "u", ObjectKey: "k", ExpectedSize: 1, Checksum: "crc64:1", PartSize: 1, MaxParts: 1, Status: UploadAborted, ExpiresAt: time.Now().Add(time.Hour), AttemptCount: 1, CleanupStatus: "clean"}
	if _, err := NewStore(nil).SaveUploadTask(context.Background(), task); err == nil {
		t.Fatal("expected store validation before SQL")
	}
}

func TestStoreCoverSettingsCASesGeneratedMediaKeyAndPreservesContentLifecycle(t *testing.T) {
	now := time.Date(2026, 7, 28, 13, 0, 0, 0, time.UTC)
	mediaID := int64(9)
	content := Content{ID: 7, Title: "课件", ContentType: ContentVideo, MediaAssetID: &mediaID, Status: ContentPublished, AccessLevel: AccessPublic, ManualCoverObjectKey: "manual.webp", CoverAspectRatio: CoverAspectRatio16x9, UpdatedAt: now}
	media := MediaAsset{ID: mediaID, ObjectKey: "video.mp4", CoverObjectKey: "old.jpg", ContentType: ContentVideo, StorageStatus: MediaReady}
	updated := content
	updated.CoverAspectRatio = CoverAspectRatio9x16
	updated.UpdatedAt = now.Add(time.Second)
	state := &classroomTxState{}
	db := openClassroomTxDB(t, state, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(query, "FROM classroom_contents WHERE id"):
			return classroomRows(contentColumns, [][]driver.Value{contentValues(content)}), nil
		case strings.Contains(query, "FROM classroom_media_assets"):
			return classroomRows(mediaColumns, [][]driver.Value{mediaValues(media)}), nil
		case strings.Contains(query, "UPDATE classroom_media_assets"):
			if !strings.Contains(query, "WHERE id=$2 AND cover_object_key=$3") || len(args) != 3 || args[2].Value != "old.jpg" {
				t.Fatalf("generated cover CAS query=%s args=%+v", query, args)
			}
			return classroomRows([]string{"id"}, [][]driver.Value{{mediaID}}), nil
		case strings.Contains(query, "UPDATE classroom_contents"):
			return classroomRows(contentColumns, [][]driver.Value{contentValues(updated)}), nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
	})
	got, err := NewStore(db).SetContentCoverSettings(context.Background(), content.ID, CoverAspectRatio9x16, now, nil, &mediaID, "old.jpg", "new.jpg")
	if err != nil || got.Status != ContentPublished || got.ManualCoverObjectKey != "manual.webp" || got.CoverAspectRatio != CoverAspectRatio9x16 || !state.committed {
		t.Fatalf("got=%+v err=%v tx=%+v", got, err, state)
	}
}
