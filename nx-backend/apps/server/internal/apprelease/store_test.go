package apprelease

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/testutil"
)

func TestStoreReleaseMetadataJSONContract(t *testing.T) {
	raw, err := json.Marshal(Release{
		AppName:     "九星",
		PackageName: "com.example.ninexing",
		IconPath:    "private/icon.png",
		IconURL:     "https://cdn.example.com/icon.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["appName"] != "九星" || payload["packageName"] != "com.example.ninexing" || payload["iconUrl"] != "https://cdn.example.com/icon.png" {
		t.Fatalf("metadata JSON = %s, want public metadata fields", raw)
	}
	if _, exists := payload["IconPath"]; exists {
		t.Fatalf("metadata JSON = %s, must not expose IconPath", raw)
	}
	if _, exists := payload["iconPath"]; exists {
		t.Fatalf("metadata JSON = %s, must not expose iconPath", raw)
	}
}

func TestStoreCreateDraftAndReadLifecycle(t *testing.T) {
	database := openAppReleaseTestDB(t)
	store := NewStore(database)
	ctx := context.Background()

	created, err := store.CreateDraft(ctx, Release{
		Platform:     "android",
		AppName:      "九星",
		PackageName:  "com.example.ninexing",
		IconPath:     "android/icons/nine-xing.png",
		VersionName:  "1.2.3",
		VersionCode:  123,
		ReleaseNotes: "Fix startup reliability.",
		FileName:     "nine-xing.apk",
		FilePath:     "android/123-release.apk",
		FileSize:     8192,
		SHA256:       repeatedSHA("1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID <= 0 || created.Status != StatusDraft || created.PublishedAt != nil {
		t.Fatalf("unexpected created release: %+v", created)
	}
	if created.CreatedAt.IsZero() {
		t.Fatal("created release should include createdAt")
	}
	if created.AppName != "九星" || created.PackageName != "com.example.ninexing" || created.IconPath != "android/icons/nine-xing.png" {
		t.Fatalf("CreateDraft() metadata = (%q, %q, %q), want persisted metadata", created.AppName, created.PackageName, created.IconPath)
	}

	found, err := store.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.FilePath != created.FilePath || found.VersionCode != created.VersionCode || found.SHA256 != created.SHA256 ||
		found.AppName != created.AppName || found.PackageName != created.PackageName || found.IconPath != created.IconPath {
		t.Fatalf("FindByID() = %+v, want persisted release %+v", found, created)
	}

	keys, err := store.ReferencedKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := keys[created.FilePath]; !ok || len(keys) != 1 {
		t.Fatalf("ReferencedKeys() = %+v, want only %q", keys, created.FilePath)
	}

	if _, err := store.LatestPublished(ctx, "android"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LatestPublished() error = %v, want ErrNotFound", err)
	}
	if _, err := store.FindByID(ctx, created.ID+999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FindByID() error = %v, want ErrNotFound", err)
	}
}

func TestStoreReadsDefaultMetadataForExistingRelease(t *testing.T) {
	database := openAppReleaseTestDB(t)
	store := NewStore(database)

	var id int64
	err := database.QueryRow(`
		INSERT INTO app_releases
		(platform, version_name, version_code, release_notes, file_name, file_path, file_size, sha256, status)
		VALUES ('android','1.0.1',101,'legacy','legacy.apk','android/legacy.apk',1024,$1,'draft')
		RETURNING id`, repeatedSHA("a")).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}

	found, err := store.FindByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if found.AppName != "" || found.PackageName != "" || found.IconPath != "" {
		t.Fatalf("legacy metadata = (%q, %q, %q), want empty defaults", found.AppName, found.PackageName, found.IconPath)
	}
}

func TestStoreCreateDraftRejectsDuplicatePlatformVersion(t *testing.T) {
	database := openAppReleaseTestDB(t)
	store := NewStore(database)
	ctx := context.Background()
	input := releaseFixture(100, StatusDraft)

	if _, err := store.CreateDraft(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.FilePath = "android/100-duplicate.apk"
	if _, err := store.CreateDraft(ctx, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("CreateDraft() duplicate error = %v, want ErrConflict", err)
	}
}

func TestStoreListReturnsPageCurrentAndTotalStorage(t *testing.T) {
	database := openAppReleaseTestDB(t)
	store := NewStore(database)
	ctx := context.Background()

	old := insertRelease(t, database, releaseFixture(100, StatusArchived))
	current := insertRelease(t, database, releaseFixture(200, StatusPublished))
	newest := insertRelease(t, database, releaseFixture(300, StatusDraft))
	setCreatedAt(t, database, old.ID, time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC))
	setCreatedAt(t, database, current.ID, time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC))
	setCreatedAt(t, database, newest.ID, time.Date(2026, 7, 20, 1, 0, 0, 0, time.UTC))

	result, err := store.List(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Page != 1 || result.PageSize != 1 || result.Total != 3 {
		t.Fatalf("unexpected pagination: %+v", result)
	}
	if len(result.Items) != 1 || result.Items[0].ID != newest.ID {
		t.Fatalf("Items = %+v, want newest release %d", result.Items, newest.ID)
	}
	assertReleaseMetadata(t, result.Items[0], newest)
	if result.Current == nil || result.Current.ID != current.ID {
		t.Fatalf("Current = %+v, want published release %d", result.Current, current.ID)
	}
	assertReleaseMetadata(t, *result.Current, current)
	wantSize := old.FileSize + current.FileSize + newest.FileSize
	if result.TotalFileSize != wantSize {
		t.Fatalf("TotalFileSize = %d, want %d", result.TotalFileSize, wantSize)
	}

	second, err := store.List(ctx, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != current.ID {
		t.Fatalf("second page Items = %+v, want release %d", second.Items, current.ID)
	}
}

func TestStoreArchivePublishedReleaseKeepsRecord(t *testing.T) {
	database := openAppReleaseTestDB(t)
	store := NewStore(database)
	current := insertRelease(t, database, releaseFixture(100, StatusPublished))

	archived, err := store.Archive(context.Background(), current.ID, "android")
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != StatusArchived || archived.PublishedAt == nil {
		t.Fatalf("Archive() = %+v, want archived release retaining publishedAt", archived)
	}
	assertReleaseMetadata(t, archived, current)
	if status := releaseStatus(t, database, current.ID); status != StatusArchived {
		t.Fatalf("stored status = %q, want archived", status)
	}
	if _, err := store.LatestPublished(context.Background(), "android"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LatestPublished() after archive error = %v, want ErrNotFound", err)
	}
}

func TestStoreArchiveRejectsDraftAndMissingRelease(t *testing.T) {
	database := openAppReleaseTestDB(t)
	store := NewStore(database)
	draft := insertRelease(t, database, releaseFixture(100, StatusDraft))

	if _, err := store.Archive(context.Background(), draft.ID, "android"); !errors.Is(err, ErrConflict) {
		t.Fatalf("Archive(draft) error = %v, want ErrConflict", err)
	}
	if _, err := store.Archive(context.Background(), draft.ID+999, "android"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Archive(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStorePublishArchivesPreviousVersionAtomically(t *testing.T) {
	database := openAppReleaseTestDB(t)
	store := NewStore(database)
	old := insertRelease(t, database, releaseFixture(100, StatusPublished))
	target := insertRelease(t, database, releaseFixture(110, StatusDraft))

	published, err := store.Publish(context.Background(), target.ID, "android")
	if err != nil {
		t.Fatal(err)
	}
	if published.ID != target.ID || published.Status != StatusPublished || published.PublishedAt == nil {
		t.Fatalf("Publish() = %+v, want target published", published)
	}
	assertReleaseMetadata(t, published, target)
	if got := releaseStatus(t, database, target.ID); got != StatusPublished {
		t.Fatalf("target status = %q, want published", got)
	}
	if got := releaseStatus(t, database, old.ID); got != StatusArchived {
		t.Fatalf("old status = %q, want archived", got)
	}
}

func TestStorePublishAllowsArchivedVersionRollback(t *testing.T) {
	database := openAppReleaseTestDB(t)
	store := NewStore(database)
	old := insertRelease(t, database, releaseFixture(100, StatusArchived))
	current := insertRelease(t, database, releaseFixture(110, StatusPublished))

	if _, err := store.Publish(context.Background(), old.ID, "android"); err != nil {
		t.Fatal(err)
	}
	if got := releaseStatus(t, database, old.ID); got != StatusPublished {
		t.Fatalf("rollback target status = %q, want published", got)
	}
	if got := releaseStatus(t, database, current.ID); got != StatusArchived {
		t.Fatalf("current status = %q, want archived", got)
	}
}

func TestStorePublishCurrentReleaseIsIdempotent(t *testing.T) {
	database := openAppReleaseTestDB(t)
	store := NewStore(database)
	current := insertRelease(t, database, releaseFixture(100, StatusPublished))

	first, err := store.FindByID(context.Background(), current.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Publish(context.Background(), current.ID, "android")
	if err != nil {
		t.Fatal(err)
	}
	if second.PublishedAt == nil || first.PublishedAt == nil || !second.PublishedAt.Equal(*first.PublishedAt) {
		t.Fatalf("idempotent Publish() changed publishedAt: before=%v after=%v", first.PublishedAt, second.PublishedAt)
	}
}

func TestStorePublishFailureRollsBackArchivedCurrent(t *testing.T) {
	database := openAppReleaseTestDB(t)
	store := NewStore(database)
	current := insertRelease(t, database, releaseFixture(100, StatusPublished))
	target := insertRelease(t, database, releaseFixture(110, StatusDraft))
	installPublishFailureTrigger(t, database, target.ID)

	if _, err := store.Publish(context.Background(), target.ID, "android"); err == nil {
		t.Fatal("Publish() error = nil, want trigger failure")
	}
	if got := releaseStatus(t, database, current.ID); got != StatusPublished {
		t.Fatalf("current status after rollback = %q, want published", got)
	}
	if got := releaseStatus(t, database, target.ID); got != StatusDraft {
		t.Fatalf("target status after rollback = %q, want draft", got)
	}
}

func TestStoreConcurrentPublishLeavesExactlyOnePublished(t *testing.T) {
	database := openAppReleaseTestDB(t)
	store := NewStore(database)
	first := insertRelease(t, database, releaseFixture(100, StatusDraft))
	second := insertRelease(t, database, releaseFixture(110, StatusDraft))

	start := make(chan struct{})
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for _, id := range []int64{first.ID, second.ID} {
		group.Add(1)
		go func(id int64) {
			defer group.Done()
			<-start
			_, err := store.Publish(context.Background(), id, "android")
			errs <- err
		}(id)
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Publish() error = %v", err)
		}
	}
	if got := publishedCount(t, database, "android"); got != 1 {
		t.Fatalf("published count = %d, want 1", got)
	}
}

func openAppReleaseTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run app release store integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database, err := db.Open(ctx, dsn, "apprelease_store_admin", "123456")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if _, err := database.ExecContext(ctx, `TRUNCATE app_releases RESTART IDENTITY`); err != nil {
		_ = database.Close()
		t.Fatalf("truncate app releases: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`TRUNCATE app_releases RESTART IDENTITY`)
		_ = database.Close()
	})
	return database
}

func releaseFixture(versionCode int64, status Status) Release {
	publishedAt := (*time.Time)(nil)
	if status == StatusPublished {
		now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
		publishedAt = &now
	}
	return Release{
		Platform:     "android",
		AppName:      fmt.Sprintf("Nine Xing %d", versionCode),
		PackageName:  "com.example.ninexing",
		IconPath:     fmt.Sprintf("android/icons/%d.png", versionCode),
		VersionName:  fmt.Sprintf("1.0.%d", versionCode),
		VersionCode:  versionCode,
		ReleaseNotes: fmt.Sprintf("release %d", versionCode),
		FileName:     fmt.Sprintf("release-%d.apk", versionCode),
		FilePath:     fmt.Sprintf("android/%d-fixture.apk", versionCode),
		FileSize:     versionCode * 10,
		SHA256:       repeatedSHA(fmt.Sprintf("%x", versionCode%16)),
		Status:       status,
		PublishedAt:  publishedAt,
	}
}

func repeatedSHA(value string) string {
	if value == "" {
		value = "0"
	}
	return string(makeRepeatedByte(value[0], 64))
}

func makeRepeatedByte(value byte, count int) []byte {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return result
}

func insertRelease(t *testing.T, database *sql.DB, release Release) Release {
	t.Helper()
	var publishedAt any
	if release.PublishedAt != nil {
		publishedAt = *release.PublishedAt
	}
	err := database.QueryRow(`
		INSERT INTO app_releases
		(platform, app_name, package_name, icon_path, version_name, version_code, release_notes, file_name, file_path, file_size, sha256, status, published_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, created_at`,
		release.Platform, release.AppName, release.PackageName, release.IconPath, release.VersionName, release.VersionCode,
		release.ReleaseNotes, release.FileName, release.FilePath, release.FileSize, release.SHA256, release.Status, publishedAt,
	).Scan(&release.ID, &release.CreatedAt)
	if err != nil {
		t.Fatalf("insert release: %v", err)
	}
	return release
}

func assertReleaseMetadata(t *testing.T, got, want Release) {
	t.Helper()
	if got.AppName != want.AppName || got.PackageName != want.PackageName || got.IconPath != want.IconPath {
		t.Fatalf("release metadata = (%q, %q, %q), want (%q, %q, %q)",
			got.AppName, got.PackageName, got.IconPath, want.AppName, want.PackageName, want.IconPath)
	}
}

func setCreatedAt(t *testing.T, database *sql.DB, id int64, value time.Time) {
	t.Helper()
	if _, err := database.Exec(`UPDATE app_releases SET created_at=$1 WHERE id=$2`, value, id); err != nil {
		t.Fatalf("set created_at: %v", err)
	}
}

func releaseStatus(t *testing.T, database *sql.DB, id int64) Status {
	t.Helper()
	var status Status
	if err := database.QueryRow(`SELECT status FROM app_releases WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatalf("query release status: %v", err)
	}
	return status
}

func publishedCount(t *testing.T, database *sql.DB, platform string) int {
	t.Helper()
	var count int
	if err := database.QueryRow(`SELECT count(*) FROM app_releases WHERE platform=$1 AND status='published'`, platform).Scan(&count); err != nil {
		t.Fatalf("count published releases: %v", err)
	}
	return count
}

func installPublishFailureTrigger(t *testing.T, database *sql.DB, targetID int64) {
	t.Helper()
	functionName := fmt.Sprintf("fail_app_release_publish_%d", targetID)
	triggerName := fmt.Sprintf("fail_app_release_publish_trigger_%d", targetID)
	if _, err := database.Exec(fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.id = %d AND NEW.status = 'published' THEN
		    RAISE EXCEPTION 'forced app release publish failure';
		  END IF;
		  RETURN NEW;
		END $$;
		CREATE TRIGGER %s BEFORE UPDATE ON app_releases
		FOR EACH ROW EXECUTE FUNCTION %s();`, functionName, targetID, triggerName, functionName)); err != nil {
		t.Fatalf("install publish failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON app_releases`, triggerName))
		_, _ = database.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
	})
}
