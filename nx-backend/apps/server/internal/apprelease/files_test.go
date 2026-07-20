package apprelease

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileStoreStageStreamsAndHashesAPK(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app-releases")
	store, err := NewFileStore(root, 16)
	if err != nil {
		t.Fatal(err)
	}

	const payload = "signed-apk-bytes"
	staged, err := store.Stage("release.apk", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}

	if staged.OriginalName() != "release.apk" {
		t.Fatalf("OriginalName = %q, want release.apk", staged.OriginalName())
	}
	if staged.Size() != int64(len(payload)) {
		t.Fatalf("Size = %d, want %d", staged.Size(), len(payload))
	}
	if staged.SHA256() != sha256Hex([]byte(payload)) {
		t.Fatalf("SHA256 = %q, want %q", staged.SHA256(), sha256Hex([]byte(payload)))
	}
	if !strings.HasPrefix(filepath.Base(staged.Path()), ".tmp-") {
		t.Fatalf("Path = %q, want .tmp-* basename", staged.Path())
	}
	assertRegularFile(t, staged.Path())
	assertFileContents(t, staged.Path(), payload)
	assertFileMode(t, staged.Path(), 0o640)
}

func TestFileStoreStageRejectsMoreThanLimitAndCleansTemp(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root, 8)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Stage("release.apk", strings.NewReader("123456789"))
	if !isError(err, ErrFileTooLarge) {
		t.Fatalf("Stage() error = %v, want ErrFileTooLarge", err)
	}
	assertNoTemps(t, root)
}

func TestFileStoreStageAcceptsExactlyLimit(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}

	staged, err := store.Stage("release.apk", strings.NewReader("12345678"))
	if err != nil {
		t.Fatal(err)
	}
	if staged.Size() != 8 {
		t.Fatalf("Size = %d, want 8", staged.Size())
	}
}

func TestFileStoreStageReaderErrorCleansTemp(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root, 64)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Stage("release.apk", &failingReader{
		Payload: []byte("partial"),
		Err:     io.ErrUnexpectedEOF,
	})
	if !isError(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Stage() error = %v, want io.ErrUnexpectedEOF", err)
	}
	assertNoTemps(t, root)
}

func TestFileStoreStageRejectsNonAPKExtensionWithoutCreatingTemp(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root, 32)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"release.zip", "release.apk.zip", "release"} {
		t.Run(name, func(t *testing.T) {
			_, err := store.Stage(name, strings.NewReader("bytes"))
			if !isError(err, ErrInvalidExtension) {
				t.Fatalf("Stage(%q) error = %v, want ErrInvalidExtension", name, err)
			}
			assertNoTemps(t, root)
		})
	}
}

func TestFileStoreStageAcceptsCaseInsensitiveAPKExtension(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 32)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"release.APK", "release.ApK"} {
		t.Run(name, func(t *testing.T) {
			staged, err := store.Stage(name, strings.NewReader("bytes"))
			if err != nil {
				t.Fatalf("Stage(%q) error = %v, want success", name, err)
			}
			if staged.OriginalName() != name {
				t.Fatalf("OriginalName = %q, want %q", staged.OriginalName(), name)
			}
		})
	}
}

func TestFileStoreCommitUsesManifestVersionThenAtomicallyRenames(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.Stage("release.apk", strings.NewReader("bytes"))
	if err != nil {
		t.Fatal(err)
	}

	saved, err := store.Commit(staged, "android", 123)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(saved.Key, "android/123-") || !strings.HasSuffix(saved.Key, ".apk") {
		t.Fatalf("Key = %q, want android/123-<random>.apk", saved.Key)
	}
	if saved.Path != filepath.Join(store.Root(), filepath.FromSlash(saved.Key)) {
		t.Fatalf("Path = %q, want path for key %q", saved.Path, saved.Key)
	}
	if saved.OriginalName != staged.OriginalName() || saved.Size != staged.Size() || saved.SHA256 != staged.SHA256() {
		t.Fatalf("saved metadata = %+v, staged metadata = %+v", saved, staged)
	}
	assertNotExists(t, staged.Path())
	assertRegularFile(t, saved.Path)
	assertFileContents(t, saved.Path, "bytes")
	assertFileMode(t, saved.Path, 0o640)
}

func TestFileStoreCommitRejectsAnythingExceptOwnedTempFiles(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root, 64)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "unregistered root temp", path: filepath.Join(root, ".tmp-fake")},
		{name: "ordinary file", path: filepath.Join(root, "upload.apk")},
		{name: "outside temp", path: filepath.Join(t.TempDir(), ".tmp-outside")},
		{name: "nested temp", path: filepath.Join(root, "nested", ".tmp-nested")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(test.path), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(test.path, []byte("do-not-move"), 0o640); err != nil {
				t.Fatal(err)
			}
			payload := []byte("do-not-move")
			forged := StagedFile{
				id:           "forged-stage-id",
				path:         test.path,
				originalName: "forged.apk",
				size:         int64(len(payload)),
				sha256:       sha256Hex(payload),
			}
			_, err := store.Commit(forged, "android", 123)
			if !isError(err, ErrUnsafePath) {
				t.Fatalf("Commit() error = %v, want ErrUnsafePath", err)
			}
			assertFileContents(t, test.path, "do-not-move")
			assertNoFinalAPKs(t, root)
		})
	}
}

func TestFileStoreCommitRejectsChangedStagedFileWithoutCreatingFinalFile(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.Stage("release.apk", strings.NewReader("original"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Discard(staged) })
	if err := os.WriteFile(staged.Path(), []byte("tampered"), 0o640); err != nil {
		t.Fatal(err)
	}

	_, err = store.Commit(staged, "android", 123)
	if !isError(err, ErrStagedFileChanged) {
		t.Fatalf("Commit() error = %v, want ErrStagedFileChanged", err)
	}
	assertRegularFile(t, staged.Path())
	assertNoFinalAPKs(t, store.Root())
}

func TestFileStoreCommitRejectsReplacedStagedFileEvenWhenContentsMatch(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.Stage("release.apk", strings.NewReader("same-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(staged.Path()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged.Path(), []byte("same-bytes"), 0o640); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(staged.Path()) })

	_, err = store.Commit(staged, "android", 123)
	if !isError(err, ErrStagedFileChanged) {
		t.Fatalf("Commit() error = %v, want ErrStagedFileChanged", err)
	}
	assertNoFinalAPKs(t, store.Root())
}

func TestFileStoreCommitUsesRegisteredMetadataInsteadOfHandleFields(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.Stage("release.apk", strings.NewReader("bytes"))
	if err != nil {
		t.Fatal(err)
	}
	forged := staged
	forged.path = filepath.Join(store.Root(), ".tmp-fake")
	forged.originalName = "forged.apk"
	forged.size = 999
	forged.sha256 = strings.Repeat("f", 64)

	saved, err := store.Commit(forged, "android", 123)
	if err != nil {
		t.Fatal(err)
	}
	if saved.OriginalName != "release.apk" || saved.Size != 5 || saved.SHA256 != sha256Hex([]byte("bytes")) {
		t.Fatalf("Commit() trusted forged handle metadata: %+v", saved)
	}
}

func TestFileStoreCommitRejectsAndroidDirectoryReplacedBySymlink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	store, err := NewFileStore(root, 64)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.Stage("release.apk", strings.NewReader("bytes"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Discard(staged) })
	androidDir := filepath.Join(root, "android")
	if err := os.Mkdir(androidDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(androidDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, androidDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	info, err := os.Lstat(androidDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%q mode = %v, want symlink", androidDir, info.Mode())
	}

	_, err = store.Commit(staged, "android", 123)
	if !isError(err, ErrUnsafePath) {
		t.Fatalf("Commit() error = %v, want ErrUnsafePath", err)
	}
	info, err = os.Lstat(androidDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("Commit replaced symlink %q with mode %v", androidDir, info.Mode())
	}
	assertNoFinalAPKs(t, outside)
}

func TestFileStoreCommitRejectsUnsupportedPlatformAndInvalidVersion(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.Stage("release.apk", strings.NewReader("bytes"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Commit(staged, "ios", 123); !isError(err, ErrUnsupportedPlatform) {
		t.Fatalf("Commit(ios) error = %v, want ErrUnsupportedPlatform", err)
	}
	assertRegularFile(t, staged.Path())
	if _, err := store.Commit(staged, "android", 0); !isError(err, ErrInvalidVersion) {
		t.Fatalf("Commit(version 0) error = %v, want ErrInvalidVersion", err)
	}
	assertRegularFile(t, staged.Path())
}

func TestFileStoreDiscardAndRemoveCleanTheirOwnedFiles(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}

	discarded, err := store.Stage("discard.apk", strings.NewReader("discard"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Discard(discarded); err != nil {
		t.Fatal(err)
	}
	assertNotExists(t, discarded.Path())
	if err := store.Discard(discarded); err != nil {
		t.Fatalf("second Discard() = %v, want idempotent success", err)
	}

	staged, err := store.Stage("remove.apk", strings.NewReader("remove"))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Commit(staged, "android", 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(saved.Key); err != nil {
		t.Fatal(err)
	}
	assertNotExists(t, saved.Path)
	if err := store.Remove(saved.Key); err != nil {
		t.Fatalf("second Remove() = %v, want idempotent success", err)
	}
}

func TestFileStoreResolveRejectsAbsoluteAndTraversalPaths(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{
		"",
		"/tmp/outside.apk",
		"../outside.apk",
		"android/../../outside.apk",
		"android/../outside.apk",
		`C:\outside.apk`,
		`\\server\share\outside.apk`,
	} {
		t.Run(key, func(t *testing.T) {
			if _, err := store.Resolve(key); !isError(err, ErrUnsafePath) {
				t.Fatalf("Resolve(%q) error = %v, want ErrUnsafePath", key, err)
			}
		})
	}
}

func TestFileStoreResolveRejectsParentSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "android")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store, err := NewFileStore(root, 64)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Resolve("android/escape.apk"); !isError(err, ErrUnsafePath) {
		t.Fatalf("Resolve() error = %v, want ErrUnsafePath", err)
	}
}

func TestFileStoreResolveRejectsFileSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "android"), 0o750); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outside, "outside.apk")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "android", "linked.apk")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store, err := NewFileStore(root, 64)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Resolve("android/linked.apk"); !isError(err, ErrUnsafePath) {
		t.Fatalf("Resolve() error = %v, want ErrUnsafePath", err)
	}
}

func TestFileStoreResolveReturnsSafePath(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.Stage("release.apk", strings.NewReader("bytes"))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Commit(staged, "android", 123)
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := store.Resolve(saved.Key)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != saved.Path {
		t.Fatalf("Resolve(%q) = %q, want %q", saved.Key, resolved, saved.Path)
	}
}

func TestFileStoreCleanupStaleTempsOnlyDeletesFilesOlderThanCutoff(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	old := writeAgedFile(t, filepath.Join(store.Root(), ".tmp-old"), now.Add(-25*time.Hour))
	recent := writeAgedFile(t, filepath.Join(store.Root(), ".tmp-recent"), now.Add(-23*time.Hour))
	atCutoff := writeAgedFile(t, filepath.Join(store.Root(), ".tmp-at-cutoff"), now.Add(-24*time.Hour))
	nonTemp := writeAgedFile(t, filepath.Join(store.Root(), "keep.apk"), now.Add(-48*time.Hour))
	tempDir := filepath.Join(store.Root(), ".tmp-directory")
	if err := os.Mkdir(tempDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tempDir, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	if err := store.CleanupStaleTemps(now, 24*time.Hour); err != nil {
		t.Fatal(err)
	}

	assertNotExists(t, old)
	assertRegularFile(t, recent)
	assertRegularFile(t, atCutoff)
	assertRegularFile(t, nonTemp)
	if info, err := os.Lstat(tempDir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("temp directory should remain: info=%v err=%v", info, err)
	}
}

func TestFileStoreAuditReportsSortedOrphansWithoutDeletingOrFollowingSymlinks(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	androidDir := filepath.Join(store.Root(), "android")
	if err := os.MkdirAll(androidDir, 0o750); err != nil {
		t.Fatal(err)
	}
	orphanB := filepath.Join(androidDir, "200-b.apk")
	orphanA := filepath.Join(androidDir, "100-a.apk")
	referenced := filepath.Join(androidDir, "150-referenced.apk")
	for _, path := range []string{orphanB, orphanA, referenced} {
		if err := os.WriteFile(path, []byte("apk"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(androidDir, "ignore.txt"), []byte("text"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root(), ".tmp-ignore"), []byte("temp"), 0o640); err != nil {
		t.Fatal(err)
	}

	outside := t.TempDir()
	outsideAPK := filepath.Join(outside, "outside.apk")
	if err := os.WriteFile(outsideAPK, []byte("outside"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideAPK, filepath.Join(androidDir, "300-linked.apk")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	outsideDir := filepath.Join(outside, "nested")
	if err := os.Mkdir(outsideDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "400-hidden.apk"), []byte("hidden"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(androidDir, "linked-directory")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := store.AuditOrphans(map[string]struct{}{
		"android/150-referenced.apk": {},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"android/100-a.apk", "android/200-b.apk"}
	if len(got) != len(want) {
		t.Fatalf("AuditOrphans() = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AuditOrphans() = %q, want %q", got, want)
		}
	}
	for _, path := range []string{orphanA, orphanB, referenced, outsideAPK} {
		assertRegularFile(t, path)
	}
}

type failingReader struct {
	Payload []byte
	Err     error
	done    bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.Err
	}
	r.done = true
	n := copy(p, r.Payload)
	return n, r.Err
}

func sha256Hex(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func isError(got, want error) bool {
	for got != nil {
		if got == want {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		wrapped, ok := got.(unwrapper)
		if !ok {
			return false
		}
		got = wrapped.Unwrap()
	}
	return false
}

func assertNoTemps(t *testing.T, root string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, ".tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %q", matches)
	}
}

func assertNoFinalAPKs(t *testing.T, root string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "android", "*.apk"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("final APK files unexpectedly created: %q", matches)
	}
}

func assertRegularFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %q: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		t.Fatalf("%q mode = %v, want regular file", path, info.Mode())
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%q still exists or stat failed: %v", path, err)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("contents of %q = %q, want %q", path, got, want)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%q is a symlink", path)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode of %q = %o, want %o", path, got, want)
	}
}

func writeAgedFile(t *testing.T, path string, modified time.Time) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	return path
}
