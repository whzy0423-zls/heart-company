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
	if !CanTransitionContent(ContentDraft, ContentProcessing) || !CanTransitionContent(ContentProcessing, ContentReady) || !CanTransitionContent(ContentReady, ContentPublished) || !CanTransitionContent(ContentPublished, ContentOffline) || !CanTransitionContent(ContentFailed, ContentDraft) {
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
	db := openClassroomQueryDB(t, func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(query, "FROM classroom_series WHERE id") {
			return classroomRows(seriesColumns, [][]driver.Value{seriesValues(series)}), nil
		}
		if strings.Contains(query, "UPDATE classroom_series") {
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

var contentColumns = []string{"id", "series_id", "show_as_standalone", "title", "description", "content_type", "media_asset_id", "cover_url", "duration_seconds", "teacher_key", "teacher_name_snapshot", "recorded_at", "badge", "tags", "episode_no", "sort_order", "status", "playback_blocked", "access_level", "price_cents", "published_at", "created_by", "updated_by", "created_at", "updated_at"}

func contentValues(c Content) []driver.Value {
	return []driver.Value{c.ID, nil, c.ShowAsStandalone, c.Title, c.Description, string(c.ContentType), nil, c.CoverURL, int64(c.DurationSeconds), c.TeacherKey, c.TeacherNameSnapshot, nil, c.Badge, []byte(`[]`), int64(c.EpisodeNo), int64(c.SortOrder), string(c.Status), c.PlaybackBlocked, string(c.AccessLevel), int64(c.PriceCents), nil, nil, nil, c.CreatedAt, c.UpdatedAt}
}
