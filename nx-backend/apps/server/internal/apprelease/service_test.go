package apprelease

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestServiceCreateDraftPersistsMetadataAndIcon(t *testing.T) {
	files, err := NewFileStore(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	store := &stubReleaseStore{}
	service := &Service{
		store: store,
		files: files,
		inspector: stubAPKInspector{info: APKInfo{
			PackageName:       "com.example.ninexing",
			VersionName:       "1.2.3",
			VersionCode:       123,
			CertificateSHA256: strings.Repeat("a", 64),
			AppName:           "九星",
			IconPNG:           testPNG(t),
		}},
		packageName: "com.example.ninexing",
	}
	staged, err := service.StageAPK("nine-xing.apk", strings.NewReader("apk bytes"))
	if err != nil {
		t.Fatal(err)
	}

	created, err := service.CreateDraftFromStaged(context.Background(), staged, "notes")
	if err != nil {
		t.Fatal(err)
	}
	if store.created.AppName != "九星" || store.created.PackageName != "com.example.ninexing" {
		t.Fatalf("CreateDraft input metadata = (%q, %q), want extracted values", store.created.AppName, store.created.PackageName)
	}
	wantIconKey := strings.TrimSuffix(store.created.FilePath, ".apk") + ".png"
	if store.created.IconPath != wantIconKey {
		t.Fatalf("CreateDraft input IconPath = %q, want %q", store.created.IconPath, wantIconKey)
	}
	if created.AppName != store.created.AppName || created.PackageName != store.created.PackageName || created.IconPath != wantIconKey {
		t.Fatalf("created release metadata = %+v, want persisted metadata", created)
	}
	if created.IconURL != "/api/app-release-icons/42" || !created.FileAvailable {
		t.Fatalf("created release enrichment = iconURL %q fileAvailable %v", created.IconURL, created.FileAvailable)
	}
	iconPath, err := files.Resolve(wantIconKey)
	if err != nil {
		t.Fatal(err)
	}
	assertRegularFile(t, iconPath)
}

func TestServiceCreateDraftMetadataSurvivesBestEffortIconFailure(t *testing.T) {
	files, err := NewFileStore(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	store := &stubReleaseStore{}
	service := &Service{
		store:       store,
		files:       files,
		inspector:   stubAPKInspector{info: APKInfo{PackageName: "com.example.ninexing", VersionName: "1.2.3", VersionCode: 123, AppName: "九星", IconPNG: []byte("invalid")}},
		packageName: "com.example.ninexing",
	}
	staged, err := service.StageAPK("nine-xing.apk", strings.NewReader("apk bytes"))
	if err != nil {
		t.Fatal(err)
	}

	created, err := service.CreateDraftFromStaged(context.Background(), staged, "")
	if err != nil {
		t.Fatalf("CreateDraftFromStaged() error = %v, want valid APK accepted without icon", err)
	}
	if store.created.AppName != "九星" || store.created.PackageName != "com.example.ninexing" || store.created.IconPath != "" {
		t.Fatalf("CreateDraft input = %+v, want text metadata and empty icon path", store.created)
	}
	if created.IconURL != "" {
		t.Fatalf("IconURL = %q, want empty", created.IconURL)
	}
}

func TestServiceCreateDraftRollbackRemovesAPKAndIcon(t *testing.T) {
	files, err := NewFileStore(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("create failed")
	store := &stubReleaseStore{createErr: wantErr}
	service := &Service{
		store:       store,
		files:       files,
		inspector:   stubAPKInspector{info: APKInfo{PackageName: "com.example.ninexing", VersionName: "1.2.3", VersionCode: 123, AppName: "九星", IconPNG: testPNG(t)}},
		packageName: "com.example.ninexing",
	}
	staged, err := service.StageAPK("nine-xing.apk", strings.NewReader("apk bytes"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.CreateDraftFromStaged(context.Background(), staged, ""); !errors.Is(err, wantErr) {
		t.Fatalf("CreateDraftFromStaged() error = %v, want %v", err, wantErr)
	}
	for _, key := range []string{store.created.FilePath, store.created.IconPath} {
		if key == "" {
			t.Fatalf("rollback key is empty: %+v", store.created)
		}
		path, err := files.Resolve(key)
		if err != nil {
			t.Fatal(err)
		}
		assertNotExists(t, path)
	}
}

func TestServiceCreateDraftRollbackSurfacesCleanupErrorsAndAttemptsBothRemovals(t *testing.T) {
	storeErr := errors.New("create failed")
	iconRemoveErr := errors.New("icon remove failed")
	apkRemoveErr := errors.New("apk remove failed")
	files := &rollbackFailureFileStore{
		FileStore:  &FileStore{},
		saved:      SavedFile{Key: "android/123-release.apk", OriginalName: "release.apk", Size: 9, SHA256: strings.Repeat("a", 64)},
		iconKey:    "android/123-release.png",
		removeErrs: map[string]error{"android/123-release.png": iconRemoveErr, "android/123-release.apk": apkRemoveErr},
	}
	service := &Service{
		store:       &stubReleaseStore{createErr: storeErr},
		files:       files,
		inspector:   stubAPKInspector{info: APKInfo{PackageName: "com.example.ninexing", VersionName: "1.2.3", VersionCode: 123, IconPNG: testPNG(t)}},
		packageName: "com.example.ninexing",
	}

	_, err := service.CreateDraftFromStaged(context.Background(), StagedFile{}, "")
	for _, want := range []error{storeErr, iconRemoveErr, apkRemoveErr} {
		if !errors.Is(err, want) {
			t.Fatalf("CreateDraftFromStaged() error = %v, want joined error containing %v", err, want)
		}
	}
	wantAttempts := []string{files.iconKey, files.saved.Key}
	if len(files.removeAttempts) != len(wantAttempts) {
		t.Fatalf("Remove attempts = %q, want %q", files.removeAttempts, wantAttempts)
	}
	for i := range wantAttempts {
		if files.removeAttempts[i] != wantAttempts[i] {
			t.Fatalf("Remove attempts = %q, want %q", files.removeAttempts, wantAttempts)
		}
	}
}

func TestServiceEnrichesEveryReleaseResponseConsistently(t *testing.T) {
	files, release := managedReleaseFixture(t)
	store := &stubReleaseStore{release: release, list: ListResult{Current: releasePtr(release), Items: []Release{release}}}
	service := &Service{
		store:       store,
		files:       files,
		inspector:   stubAPKInspector{info: APKInfo{PackageName: release.PackageName, VersionName: release.VersionName, VersionCode: release.VersionCode, CertificateSHA256: strings.Repeat("a", 64)}},
		packageName: release.PackageName,
		certificate: func() (string, error) { return strings.Repeat("a", 64), nil },
	}
	assertEnriched := func(t *testing.T, got Release) {
		t.Helper()
		if !got.FileAvailable || got.IconURL != "/api/app-release-icons/7" {
			t.Fatalf("release enrichment = fileAvailable %v iconURL %q, want true and protected URL", got.FileAvailable, got.IconURL)
		}
	}

	published, err := service.Publish(context.Background(), release.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertEnriched(t, published)
	archived, err := service.Archive(context.Background(), release.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertEnriched(t, archived)
	latest, err := service.Latest(context.Background(), "android")
	if err != nil {
		t.Fatal(err)
	}
	assertEnriched(t, latest)
	listed, err := service.List(context.Background(), 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	assertEnriched(t, *listed.Current)
	assertEnriched(t, listed.Items[0])
	opened, file, err := service.Open(context.Background(), release.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	assertEnriched(t, opened)
}

func TestServiceOpenIconOpensRegularManagedPNG(t *testing.T) {
	files, release := managedReleaseFixture(t)
	service := &Service{store: &stubReleaseStore{release: release}, files: files}

	gotRelease, file, err := service.OpenIcon(context.Background(), release.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if gotRelease.IconURL != "/api/app-release-icons/7" {
		t.Fatalf("IconURL = %q, want protected URL", gotRelease.IconURL)
	}
	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, testPNG(t)) {
		t.Fatalf("OpenIcon() bytes differ: got %d", len(got))
	}
}

func TestServiceOpenIconReturnsNotFoundForMissingMetadataOrFile(t *testing.T) {
	files, release := managedReleaseFixture(t)
	store := &stubReleaseStore{release: release}
	service := &Service{store: store, files: files}

	store.release.IconPath = ""
	if _, _, err := service.OpenIcon(context.Background(), release.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("OpenIcon(empty metadata) error = %v, want ErrNotFound", err)
	}
	store.release = release
	if err := files.Remove(release.IconPath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.OpenIcon(context.Background(), release.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("OpenIcon(missing file) error = %v, want ErrNotFound", err)
	}
}

type stubAPKInspector struct {
	info APKInfo
	err  error
}

type rollbackFailureFileStore struct {
	*FileStore
	saved          SavedFile
	iconKey        string
	removeErrs     map[string]error
	removeAttempts []string
}

func (s *rollbackFailureFileStore) Commit(StagedFile, string, int64) (SavedFile, error) {
	return s.saved, nil
}

func (s *rollbackFailureFileStore) SaveIcon(string, []byte) (string, error) {
	return s.iconKey, nil
}

func (s *rollbackFailureFileStore) Remove(key string) error {
	s.removeAttempts = append(s.removeAttempts, key)
	return s.removeErrs[key]
}

func (s stubAPKInspector) Inspect(string) (APKInfo, error) { return s.info, s.err }

type stubReleaseStore struct {
	created   Release
	createErr error
	release   Release
	list      ListResult
}

func (s *stubReleaseStore) CreateDraft(_ context.Context, input Release) (Release, error) {
	s.created = input
	if s.createErr != nil {
		return Release{}, s.createErr
	}
	input.ID = 42
	s.release = input
	return input, nil
}

func (s *stubReleaseStore) FindByID(context.Context, int64) (Release, error) {
	if s.release.ID == 0 {
		return Release{}, ErrNotFound
	}
	return s.release, nil
}

func (s *stubReleaseStore) Publish(context.Context, int64, string) (Release, error) {
	return s.release, nil
}

func (s *stubReleaseStore) Archive(context.Context, int64, string) (Release, error) {
	return s.release, nil
}

func (s *stubReleaseStore) List(context.Context, int, int) (ListResult, error) {
	return s.list, nil
}

func (s *stubReleaseStore) LatestPublished(context.Context, string) (Release, error) {
	return s.release, nil
}

func (s *stubReleaseStore) ReferencedKeys(context.Context) (map[string]struct{}, error) {
	return map[string]struct{}{}, nil
}

func managedReleaseFixture(t *testing.T) (*FileStore, Release) {
	t.Helper()
	files, err := NewFileStore(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := files.Stage("release.apk", strings.NewReader("apk bytes"))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := files.Commit(staged, "android", 123)
	if err != nil {
		t.Fatal(err)
	}
	iconKey, err := files.SaveIcon(saved.Key, testPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	return files, Release{
		ID:          7,
		Platform:    "android",
		AppName:     "九星",
		PackageName: "com.example.ninexing",
		IconPath:    iconKey,
		VersionName: "1.2.3",
		VersionCode: 123,
		FileName:    saved.OriginalName,
		FilePath:    saved.Key,
		FileSize:    saved.Size,
		SHA256:      saved.SHA256,
	}
}

func releasePtr(release Release) *Release { return &release }
