# 官网 App 下载与版本管理 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为官网提供可信的 Android APK 下载区，并让管理员可在后台上传、发布和下架版本，官网自动展示当前正式版本。

**Architecture:** Go 服务新增独立 `apprelease` 领域包，负责 APK 元数据/签名校验、流式文件存储和 PostgreSQL 发布状态；HTTP 层只做权限、协议和错误映射。Vue 管理端通过独立 API/页面管理版本，React 官网通过公开元数据接口渲染下载区；APK 始终保存在已有 `/data/uploads` 持久卷，不进入 Git、数据库字节列或官网构建产物。

**Tech Stack:** Go 1.22、PostgreSQL、`github.com/avast/apkverifier`、`github.com/shogo82148/androidbinary`、Vue 3 + Ant Design Vue、React + Vite、Node `node:test`、Vitest、nginx、Docker Compose。

**Approved spec:** `docs/superpowers/specs/2026-07-20-website-app-download-design.md`

---

## File map

### Backend domain and persistence

- Create `nx-backend/apps/server/internal/apprelease/model.go`: release/status structs and typed domain errors.
- Create `nx-backend/apps/server/internal/apprelease/store.go`: PostgreSQL CRUD, latest lookup, archive, and serialized publish transaction.
- Create `nx-backend/apps/server/internal/apprelease/store_test.go`: database behavior and concurrent publish tests.
- Create `nx-backend/apps/server/internal/apprelease/files.go`: bounded streaming save, SHA-256, atomic rename, safe path resolution, startup cleanup/audit.
- Create `nx-backend/apps/server/internal/apprelease/files_test.go`: file lifecycle and path-boundary tests.
- Create `nx-backend/apps/server/internal/apprelease/apk.go`: APK Manifest/signature inspection and package/certificate validation.
- Create `nx-backend/apps/server/internal/apprelease/apk_test.go`: signed fixture, package, version, and certificate tests.
- Create `nx-backend/apps/server/internal/apprelease/testdata/signed-minimal.apk`: tiny, non-production, signed APK fixture used only by Go tests.
- Create `nx-backend/apps/server/internal/apprelease/testdata/unsigned-minimal.apk`: matching unsigned fixture used to prove signature rejection.
- Create `nx-backend/apps/server/internal/apprelease/testdata/README.md`: exact disposable fixture-generation commands and expected manifest/certificate metadata.
- Create `nx-backend/apps/server/internal/apprelease/service.go`: upload/publish/archive orchestration and cleanup compensation.
- Create `nx-backend/apps/server/internal/apprelease/service_test.go`: orchestration and fail-closed publication tests.
- Modify `nx-backend/apps/server/internal/db/schema.sql`: add `app_releases` table and constraints.
- Create `nx-backend/apps/server/internal/db/schema_app_release_test.go`: schema constraint contract tests.
- Modify `nx-backend/apps/server/internal/db/db.go`: seed App release list/write menu permissions.
- Modify `nx-backend/apps/server/internal/db/menu_test.go`: assert menu hierarchy and permission codes.
- Modify `nx-backend/apps/server/internal/config/env.go`: add package name and release certificate config.
- Modify `nx-backend/apps/server/internal/config/env_test.go`: defaults and certificate normalization tests.
- Modify `nx-backend/apps/server/go.mod` and `go.sum`: pure-Go APK parsing/signature dependencies.

### Backend HTTP and deployment plumbing

- Create `nx-backend/apps/server/internal/server/app_releases.go`: admin/public handlers and download protocol.
- Create `nx-backend/apps/server/internal/server/app_releases_test.go`: permissions, status mapping, upload, metadata, redirect, HEAD, Range, ETag, and file-loss tests.
- Create `nx-backend/apps/server/internal/server/app_release_proxy_test.go`: nginx upload/download contract tests.
- Modify `nx-backend/apps/server/internal/server/server.go`: initialize service, run startup maintenance, and register routes.
- Modify `nx-backend/apps/server/cmd/server/main.go`: permit large streaming uploads/downloads without the existing short global deadlines cutting them off.
- Modify `nx-backend/scripts/deploy/nginx.conf`: 301 MiB upload override and unbuffered download routes.
- Modify `website-react/nginx.conf`: unbuffered public APK download routes.

### Admin UI

- Create `nx-backend/apps/web-antd/src/api/core/app-release.ts`: typed list/upload/publish/archive client.
- Create `nx-backend/apps/web-antd/src/api/core/app-release.test.ts`: request contract tests.
- Modify `nx-backend/apps/web-antd/src/api/core/index.ts`: export the new API.
- Create `nx-backend/apps/web-antd/src/views/site-config/app-releases.vue`: current version summary, upload modal/progress, history table, publish/archive actions.
- Create `nx-backend/apps/web-antd/src/views/site-config/app-releases.test.ts`: page source/behavior contract tests.
- Create `nx-backend/apps/web-antd/src/views/site-config/app-release-view.ts`: pure file/status/action rules for focused tests.
- Create `nx-backend/apps/web-antd/src/views/site-config/app-release-view.test.ts`: focused rule tests.
- Modify `nx-backend/apps/web-antd/src/test-utils/antd-stubs.ts`: add Upload/Progress and confirm-capable Modal stubs used by mounted page tests.
- Modify `nx-backend/apps/web-antd/src/api/core/site-config.ts`: type `home.appDownload`.
- Create `nx-backend/apps/web-antd/src/api/core/site-config.typecheck.ts`: compile-time assertion that `appDownload` is not `any`.
- Modify `nx-backend/apps/web-antd/src/views/site-config/home.vue`: editable download-section copy and list fields.
- Create `nx-backend/apps/web-antd/src/views/site-config/home-app-download.test.ts`: editor contract tests.

### Public website and backward compatibility

- Modify `nx-backend/apps/server/internal/siteconfig/site_config.go`: merge missing `home.appDownload` defaults into old stored configs.
- Modify/create matching `internal/siteconfig` tests: prove old configs retain existing values and receive only missing defaults.
- Create `website-react/src/api/appRelease.js`: fetch latest public metadata.
- Create `website-react/src/api/appRelease.test.mjs`: success/unavailable/error response tests.
- Create `website-react/src/utils/appDownloadDevice.js`: Android/iOS/iPadOS/desktop classification and formatting helpers.
- Create `website-react/src/utils/appDownloadDevice.test.mjs`: device and formatting tests.
- Create `website-react/src/utils/appDownloadViewModel.js`: pure loading/error/device/action state derivation for Node tests.
- Create `website-react/src/utils/appDownloadViewModel.test.mjs`: executable state matrix tests without JSX tooling.
- Create `website-react/src/components/AppDownloadSection.jsx`: accessible branded download section with QR and resilient states.
- Create `website-react/src/components/AppDownloadSection.test.mjs`: rendered/source contract tests for states and QR target.
- Create `website-react/src/pages/home-app-download.test.mjs`: section ordering and entry-point tests.
- Modify `website-react/src/pages/Home.jsx`: place the section immediately after Hero.
- Modify `website-react/src/components/Layout.jsx`: respect reduced-motion for hash scrolling.
- Modify `website-react/src/index.css`: responsive warm-gold styling, 44px targets, wrapping digest, reduced-motion and no horizontal overflow.
- Modify `shared/site-config.json`: navigation, Hero CTA, and `home.appDownload` copy.

### Operations and documentation

- Modify `.env.example` and `nx-backend/apps/server/.env.example`: document release package and certificate fingerprint.
- Modify `docker-compose.yml`: pass release env vars while reusing the existing uploads volume.
- Modify `nx-backend/apps/server/Dockerfile` and/or `docker-entrypoint.sh`: ensure `app-releases` directory exists and is writable by the non-root service user.
- Create `nx-backend/apps/server/internal/server/app_release_deployment_test.go`: env/Compose/Dockerfile/documentation deployment contracts.
- Modify `DEPLOY.md`, `README.md`, and `nx-backend/apps/server/README.md`: release workflow, proxy limits, persistent files, backups, formal signing, and troubleshooting.

---

## Chunk 1: Database, configuration, APK inspection, and file storage

### Task 1: Add schema, permissions, and release configuration

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Create: `nx-backend/apps/server/internal/db/schema_app_release_test.go`
- Modify: `nx-backend/apps/server/internal/db/db.go`
- Modify: `nx-backend/apps/server/internal/db/menu_test.go`
- Modify: `nx-backend/apps/server/internal/config/env.go`
- Modify: `nx-backend/apps/server/internal/config/env_test.go`
- Modify: `nx-backend/apps/server/go.mod`
- Modify: `nx-backend/apps/server/go.sum`

- [ ] **Step 1: Add the existing-style Go test assertion dependency**

Run from the Go module before writing the new tests:

```bash
cd nx-backend/apps/server
go get github.com/stretchr/testify@v1.10.0
```

This is test-only usage; production code must not import it.

- [ ] **Step 2: Write failing schema source-contract tests**

Follow the existing `schema_*_test.go` pattern so RED is reproducible without PostgreSQL:

```go
func TestSchemaDefinesAppReleaseConstraints(t *testing.T) {
    raw, err := os.ReadFile("schema.sql")
    if err != nil {
        t.Fatal(err)
    }
    schema := string(raw)
    required := []string{
        "CREATE TABLE IF NOT EXISTS app_releases",
        "CHECK (platform IN ('android'))",
        "CHECK (status IN ('draft', 'published', 'archived'))",
        "CHECK (version_code > 0)",
        "UNIQUE(platform, version_code)",
        "ON app_releases(platform) WHERE status = 'published'",
    }
    for _, fragment := range required {
        if !strings.Contains(schema, fragment) {
            t.Fatalf("schema must contain %q", fragment)
        }
    }
}
```

The PostgreSQL behavior tests that actually insert conflicting rows belong in Task 4, where the store test helper will use `testutil.ValidateIsolatedPostgresDSN`, `db.Open`, `TRUNCATE app_releases RESTART IDENTITY`, and per-test cleanup. Do not reference a nonexistent test database helper.

- [ ] **Step 3: Run the schema test and verify RED**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/db -run TestSchemaDefinesAppReleaseConstraints -count=1
```

Expected: FAIL with the first missing schema fragment.

- [ ] **Step 4: Add failing menu tests**

Assert fixed seed entries:

```go
seedMenu{ID: 315, PID: 300, Name: "WebsiteAppReleases", Path: "/website/app-releases", Component: "/site-config/app-releases", AuthCode: "Website:AppReleases:List", Type: "menu", Title: "App 版本"}
seedMenu{ID: 316, PID: 315, Name: "WebsiteAppReleasesWrite", AuthCode: "Website:AppReleases:Write", Type: "button"}
```

- [ ] **Step 5: Run menu tests and verify RED**

Run: `go test ./internal/db -run 'Test.*Menu' -count=1`

Expected: FAIL because IDs 315/316 are absent.

- [ ] **Step 6: Add failing config tests**

Define the desired API in tests:

```go
func TestLoadAppReleaseDefaults(t *testing.T) {
    t.Setenv("APP_RELEASE_PACKAGE_NAME", "")
    t.Setenv("APP_RELEASE_CERT_SHA256", "")
    env := Load()
    require.Equal(t, "com.xinzhili.nine_xing_app", env.AppRelease.PackageName)
    require.Empty(t, env.AppRelease.CertificateSHA256)
}

func TestAppReleaseConfigExpectedCertificateNormalizesSHA256(t *testing.T) {
    cfg := AppReleaseConfig{CertificateSHA256: "AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA"}
    got, err := cfg.ExpectedCertificateSHA256()
    require.NoError(t, err)
    require.Equal(t, strings.Repeat("aa", 32), got)
}

func TestAppReleaseConfigExpectedCertificateRejectsMalformedValue(t *testing.T) {
    cfg := AppReleaseConfig{CertificateSHA256: "not-a-sha256"}
    _, err := cfg.ExpectedCertificateSHA256()
    require.ErrorIs(t, err, ErrInvalidAppReleaseCertificate)
}

func TestAppReleaseConfigExpectedCertificateRequiresConfiguration(t *testing.T) {
    _, err := (AppReleaseConfig{}).ExpectedCertificateSHA256()
    require.ErrorIs(t, err, ErrAppReleaseCertificateNotConfigured)
}
```

Keep these tests in the existing `package config`; `AppReleaseConfig` and its errors are therefore unqualified as shown. These two typed errors have one owner only: `internal/config`. `Load()` remains non-failing and stores the trimmed environment value in `Env.AppRelease.CertificateSHA256`. Publication obtains the fingerprint through an injected provider that calls `ExpectedCertificateSHA256()`; service code converts either config error to the single domain error `apprelease.ErrPublishCertificateUnavailable`. Uploading a draft never calls the provider.

- [ ] **Step 7: Run config tests and verify RED**

Run: `go test ./internal/config -run 'TestLoadAppRelease|TestAppReleaseConfigExpectedCertificate' -count=1`

Expected: FAIL because `Env.AppRelease`, `ExpectedCertificateSHA256`, and the typed errors do not exist.

- [ ] **Step 8: Implement schema and indexes**

Add idempotent SQL:

```sql
CREATE TABLE IF NOT EXISTS app_releases (
  id BIGSERIAL PRIMARY KEY,
  platform TEXT NOT NULL CHECK (platform IN ('android')),
  version_name TEXT NOT NULL,
  version_code BIGINT NOT NULL CHECK (version_code > 0),
  release_notes TEXT NOT NULL DEFAULT '',
  file_name TEXT NOT NULL,
  file_path TEXT NOT NULL,
  file_size BIGINT NOT NULL CHECK (file_size > 0),
  sha256 TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('draft', 'published', 'archived')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  UNIQUE(platform, version_code)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_app_releases_one_published_platform
  ON app_releases(platform) WHERE status = 'published';
```

- [ ] **Step 9: Implement menu seeds and configuration**

Add:

```go
type AppReleaseConfig struct {
    PackageName       string
    CertificateSHA256 string
}
```

`ExpectedCertificateSHA256()` trims whitespace, removes `:`, lowercases, and validates exactly 64 hexadecimal characters. Empty returns `config.ErrAppReleaseCertificateNotConfigured`; malformed non-empty input returns `config.ErrInvalidAppReleaseCertificate`. Do not define errors with those names in `apprelease`, reuse `AppVersion`, reject process startup, or change `UPLOAD_MAX_MB`.

- [ ] **Step 10: Run focused tests and verify GREEN**

Run:

```bash
go test ./internal/config ./internal/db -count=1
```

Expected: PASS (database-specific tests may report SKIP only when no test database is configured).

- [ ] **Step 11: Commit**

```bash
git add nx-backend/apps/server/internal/config nx-backend/apps/server/internal/db nx-backend/apps/server/go.mod nx-backend/apps/server/go.sum
git commit -m "feat: add app release schema and permissions"
```

### Task 2: Implement bounded file storage and lifecycle maintenance

**Files:**
- Create: `nx-backend/apps/server/internal/apprelease/model.go`
- Create: `nx-backend/apps/server/internal/apprelease/files.go`
- Create: `nx-backend/apps/server/internal/apprelease/files_test.go`

- [ ] **Step 1: Write failing staging tests**

Define `FileStore` behavior with real temporary directories:

```go
func TestFileStoreStageStreamsAndHashesAPK(t *testing.T) {
    root := t.TempDir()
    store, err := NewFileStore(root, 16)
    require.NoError(t, err)
    staged, err := store.Stage("release.apk", strings.NewReader("signed-apk-bytes"))
    require.NoError(t, err)
    require.EqualValues(t, len("signed-apk-bytes"), staged.Size)
    require.Equal(t, sha256Hex([]byte("signed-apk-bytes")), staged.SHA256)
    require.True(t, strings.HasPrefix(filepath.Base(staged.TempPath), ".tmp-"))
    require.FileExists(t, staged.TempPath)
}

func TestFileStoreStageRejectsMoreThanLimitAndCleansTemp(t *testing.T) {
    root := t.TempDir()
    store, err := NewFileStore(root, 8)
    require.NoError(t, err)
    _, err = store.Stage("release.apk", strings.NewReader("123456789"))
    require.ErrorIs(t, err, ErrFileTooLarge)
    matches, globErr := filepath.Glob(filepath.Join(root, ".tmp-*"))
    require.NoError(t, globErr)
    require.Empty(t, matches)
}

func TestFileStoreStageReaderErrorCleansTemp(t *testing.T) {
    root := t.TempDir()
    store, err := NewFileStore(root, 64)
    require.NoError(t, err)
    _, err = store.Stage("release.apk", &failingReader{Payload: []byte("partial"), Err: io.ErrUnexpectedEOF})
    require.ErrorIs(t, err, io.ErrUnexpectedEOF)
    matches, globErr := filepath.Glob(filepath.Join(root, ".tmp-*"))
    require.NoError(t, globErr)
    require.Empty(t, matches)
}

func TestFileStoreStageRejectsNonAPKExtension(t *testing.T) {
    store, err := NewFileStore(t.TempDir(), 32)
    require.NoError(t, err)
    _, err = store.Stage("release.zip", strings.NewReader("bytes"))
    require.ErrorIs(t, err, ErrInvalidExtension)
}
```

Use a test-only small limit passed to the constructor so the size-boundary test runs quickly.

- [ ] **Step 2: Run file save tests and verify RED**

Run: `go test ./internal/apprelease -run 'TestFileStoreStage' -count=1`

Expected: FAIL because the package/API does not exist.

- [ ] **Step 3: Write failing path-boundary and startup-maintenance tests**

Cover:

```go
func TestFileStoreCommitUsesManifestVersionThenAtomicallyRenames(t *testing.T) {
    store, err := NewFileStore(t.TempDir(), 64)
    require.NoError(t, err)
    staged, err := store.Stage("release.apk", strings.NewReader("bytes"))
    require.NoError(t, err)
    saved, err := store.Commit(staged, "android", 123)
    require.NoError(t, err)
    require.Contains(t, saved.Key, "android/123-")
    require.NoFileExists(t, staged.TempPath)
    require.FileExists(t, saved.Path)
}

func TestFileStoreResolveRejectsTraversal(t *testing.T) {
    store, err := NewFileStore(t.TempDir(), 64)
    require.NoError(t, err)
    _, err = store.Resolve("../outside.apk")
    require.ErrorIs(t, err, ErrUnsafePath)
}

func TestFileStoreResolveRejectsSymlinkEscape(t *testing.T) {
    root, outside := t.TempDir(), t.TempDir()
    require.NoError(t, os.Symlink(outside, filepath.Join(root, "android")))
    store, err := NewFileStore(root, 64)
    require.NoError(t, err)
    _, err = store.Resolve("android/escape.apk")
    require.ErrorIs(t, err, ErrUnsafePath)
}

func TestFileStoreCleanupStaleTempsKeepsRecentFiles(t *testing.T) {
    store, err := NewFileStore(t.TempDir(), 64)
    require.NoError(t, err)
    old := filepath.Join(store.Root(), ".tmp-old")
    recent := filepath.Join(store.Root(), ".tmp-recent")
    require.NoError(t, os.WriteFile(old, []byte("old"), 0o600))
    require.NoError(t, os.WriteFile(recent, []byte("recent"), 0o600))
    now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
    require.NoError(t, os.Chtimes(old, now.Add(-25*time.Hour), now.Add(-25*time.Hour)))
    require.NoError(t, os.Chtimes(recent, now.Add(-23*time.Hour), now.Add(-23*time.Hour)))
    require.NoError(t, store.CleanupStaleTemps(now, 24*time.Hour))
    require.NoFileExists(t, old)
    require.FileExists(t, recent)
}

func TestFileStoreAuditReportsOrphansWithoutDeletingThem(t *testing.T) {
    store, err := NewFileStore(t.TempDir(), 64)
    require.NoError(t, err)
    orphan := filepath.Join(store.Root(), "android", "123-orphan.apk")
    require.NoError(t, os.MkdirAll(filepath.Dir(orphan), 0o750))
    require.NoError(t, os.WriteFile(orphan, []byte("apk"), 0o640))
    got, err := store.AuditOrphans(map[string]struct{}{})
    require.NoError(t, err)
    require.Equal(t, []string{"android/123-orphan.apk"}, got)
    require.FileExists(t, orphan)
}
```

- [ ] **Step 4: Run boundary tests and verify RED**

Run: `go test ./internal/apprelease -run 'TestFileStoreCommit|TestFileStoreResolve|TestFileStoreCleanup|TestFileStoreAudit' -count=1`

Expected: FAIL because safe resolution and maintenance are missing.

- [ ] **Step 5: Implement focused models and a two-phase `FileStore`**

`Stage` writes only a `.tmp-*` file because Manifest/signature validation has not happened yet:

```go
limited := io.LimitReader(src, maxBytes+1)
hash := sha256.New()
n, copyErr := io.Copy(io.MultiWriter(tmp, hash), limited)
if copyErr != nil {
    _ = tmp.Close()
    _ = os.Remove(tmpName)
    return StagedFile{}, fmt.Errorf("stream staged APK: %w", copyErr)
}
if n > maxBytes {
    _ = tmp.Close()
    _ = os.Remove(tmpName)
    return StagedFile{}, ErrFileTooLarge
}
if err := tmp.Sync(); err != nil {
    _ = tmp.Close()
    _ = os.Remove(tmpName)
    return StagedFile{}, fmt.Errorf("sync staged APK: %w", err)
}
if err := tmp.Close(); err != nil {
    _ = os.Remove(tmpName)
    return StagedFile{}, fmt.Errorf("close staged APK: %w", err)
}
```

Return `StagedFile{TempPath, OriginalName, Size, SHA256}`. Only after APK inspection returns the Manifest version may `Commit(staged, platform, versionCode)` create a server-generated key `android/<versionCode>-<random>.apk` and atomically rename the staged file. Create the root with `0750`, files with `0640`, and close/remove on every error path. `Discard(staged)` removes validation failures; `Remove(saved.Key)` compensates database failures. Resolve keys using `filepath.Clean`, `filepath.Rel`, and evaluated parent symlinks so the final path cannot leave `<UPLOAD_DIR>/app-releases`.

- [ ] **Step 6: Implement maintenance**

`CleanupStaleTemps(now, 24*time.Hour)` removes only `.tmp-*` files older than the cutoff. `AuditOrphans(referenced map[string]struct{})` returns/logs unreferenced final APKs without deleting them.

- [ ] **Step 7: Run focused and race tests and verify GREEN**

Run:

```bash
go test ./internal/apprelease -run 'TestFileStore' -count=1
go test -race ./internal/apprelease -run 'TestFileStore' -count=1
```

Expected: PASS, with no leftover temp files.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/apprelease/model.go nx-backend/apps/server/internal/apprelease/files.go nx-backend/apps/server/internal/apprelease/files_test.go
git commit -m "feat: add app release file storage"
```

### Task 3: Inspect APK Manifest and signing certificate with pure Go

**Files:**
- Create: `nx-backend/apps/server/internal/apprelease/apk.go`
- Create: `nx-backend/apps/server/internal/apprelease/apk_test.go`
- Create: `nx-backend/apps/server/internal/apprelease/testdata/signed-minimal.apk`
- Create: `nx-backend/apps/server/internal/apprelease/testdata/unsigned-minimal.apk`
- Create: `nx-backend/apps/server/internal/apprelease/testdata/README.md`
- Modify: `nx-backend/apps/server/go.mod`
- Modify: `nx-backend/apps/server/go.sum`

- [ ] **Step 1: Add a tiny signed APK fixture and failing extraction test**

The fixture must be a minimal test-only APK, signed with a disposable certificate and containing:

```text
package=com.xinzhili.nine_xing_app
versionName=1.2.3
versionCode=123
```

Do not use or generate a production keystore. Generate only this disposable fixture, commit the APK but delete the temporary keystore. Put these exact commands in `testdata/README.md` (adjust only the installed Android build-tools version):

```bash
tmp="$(mktemp -d)"
cat > "$tmp/AndroidManifest.xml" <<'EOF'
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
  package="com.xinzhili.nine_xing_app"
  android:versionCode="123"
  android:versionName="1.2.3">
  <uses-sdk android:minSdkVersion="23" android:targetSdkVersion="35" />
  <application android:label="九型芯之力测试包" android:hasCode="false" />
</manifest>
EOF
build_tools="$(find "$ANDROID_HOME/build-tools" -mindepth 1 -maxdepth 1 -type d | sort -V | tail -1)"
"$build_tools/aapt2" link \
  -I "$ANDROID_HOME/platforms/android-35/android.jar" \
  --manifest "$tmp/AndroidManifest.xml" \
  -o "$tmp/unsigned.apk"
keytool -genkeypair -noprompt \
  -keystore "$tmp/fixture.jks" -storepass fixturepass -keypass fixturepass \
  -alias fixture -keyalg RSA -keysize 2048 -validity 3650 \
  -dname "CN=Nine Xing Test Fixture,O=Local Tests,C=CN"
"$build_tools/zipalign" -f 4 "$tmp/unsigned.apk" "$tmp/aligned.apk"
cp "$tmp/aligned.apk" \
  nx-backend/apps/server/internal/apprelease/testdata/unsigned-minimal.apk
"$build_tools/apksigner" sign \
  --ks "$tmp/fixture.jks" --ks-pass pass:fixturepass --key-pass pass:fixturepass \
  --out nx-backend/apps/server/internal/apprelease/testdata/signed-minimal.apk \
  "$tmp/aligned.apk"
"$build_tools/apksigner" verify --verbose --print-certs \
  nx-backend/apps/server/internal/apprelease/testdata/signed-minimal.apk
rm -rf "$tmp"
```

Write:

```go
func TestAPKInspectorExtractsManifestAndCertificate(t *testing.T) {
    info, err := NewAPKInspector().Inspect("testdata/signed-minimal.apk")
    require.NoError(t, err)
    require.Equal(t, "com.xinzhili.nine_xing_app", info.PackageName)
    require.Equal(t, "1.2.3", info.VersionName)
    require.EqualValues(t, 123, info.VersionCode)
    require.Regexp(t, `^[0-9a-f]{64}$`, info.CertificateSHA256)
}
```

- [ ] **Step 2: Add dependencies and run extraction test to verify RED**

Run:

```bash
cd nx-backend/apps/server
go get github.com/avast/apkverifier@v0.0.0-20260710162049-d0e1a791cd5a
go get github.com/shogo82148/androidbinary@v1.0.5
go test ./internal/apprelease -run TestAPKInspectorExtracts -count=1
```

Expected: FAIL because `APKInspector` is absent.

- [ ] **Step 3: Add failing validation tests**

Cover invalid ZIP/APK, unsigned APK, package mismatch, and configured certificate mismatch. Manifest-empty/non-positive cases use an injected fake manifest reader so tests do not need several binary fixtures:

```go
func TestAPKInspectorRejectsInvalidArchive(t *testing.T) {
    path := filepath.Join(t.TempDir(), "invalid.apk")
    require.NoError(t, os.WriteFile(path, []byte("not a zip archive"), 0o600))
    _, err := NewAPKInspector().Inspect(path)
    require.ErrorIs(t, err, ErrInvalidAPK)
}

func TestAPKInspectorRejectsUnsignedAPK(t *testing.T) {
    _, err := NewAPKInspector().Inspect("testdata/unsigned-minimal.apk")
    require.ErrorIs(t, err, ErrUnsignedAPK)
}

func TestValidateUploadAPKRejectsWrongPackage(t *testing.T) {
    info := APKInfo{PackageName: "example.wrong", VersionName: "1.2.3", VersionCode: 123, CertificateSHA256: strings.Repeat("a", 64)}
    err := ValidateUploadAPK(info, "com.xinzhili.nine_xing_app")
    require.ErrorIs(t, err, ErrPackageMismatch)
}

func TestValidateUploadAPKAllowsDraftWithoutConfiguredCertificate(t *testing.T) {
    info := APKInfo{PackageName: "com.xinzhili.nine_xing_app", VersionName: "1.2.3", VersionCode: 123, CertificateSHA256: strings.Repeat("a", 64)}
    require.NoError(t, ValidateUploadAPK(info, "com.xinzhili.nine_xing_app"))
}

func TestValidatePublishAPKRejectsMismatchedCertificate(t *testing.T) {
    info := APKInfo{PackageName: "com.xinzhili.nine_xing_app", VersionName: "1.2.3", VersionCode: 123, CertificateSHA256: strings.Repeat("a", 64)}
    require.ErrorIs(t, ValidatePublishAPK(info, strings.Repeat("b", 64)), ErrCertificateMismatch)
}

func TestValidateUploadAPKRejectsMissingVersionMetadata(t *testing.T) {
    base := APKInfo{PackageName: "com.xinzhili.nine_xing_app", VersionName: "1.2.3", VersionCode: 123, CertificateSHA256: strings.Repeat("a", 64)}
    missingName := base
    missingName.VersionName = ""
    require.ErrorIs(t, ValidateUploadAPK(missingName, base.PackageName), ErrInvalidVersion)
    missingCode := base
    missingCode.VersionCode = 0
    require.ErrorIs(t, ValidateUploadAPK(missingCode, base.PackageName), ErrInvalidVersion)
}
```

- [ ] **Step 4: Run validation tests and verify RED**

Run: `go test ./internal/apprelease -run 'TestAPKInspector|TestValidate(Upload|Publish)APK' -count=1`

Expected: FAIL for the missing inspection/validation functions.

- [ ] **Step 5: Implement APK inspection**

Use `androidbinary.OpenFile` for `AndroidManifest.xml` and `apkverifier.Verify` plus `apkverifier.PickBestApkCert` for signature verification. Compute the selected signer certificate SHA-256 over `cert.Raw`, lowercase hex. Return typed errors that preserve a safe administrator-facing reason without leaking filesystem paths.

- [ ] **Step 6: Implement upload and publish validation helpers**

Upload validation requires a valid signed APK and expected package. `ValidatePublishAPK` receives a non-empty, already-normalized expected certificate and checks exact match; missing/malformed configuration is owned by `config.AppReleaseConfig.ExpectedCertificateSHA256()` and converted by `Service.Publish` to `ErrPublishCertificateUnavailable`. Manifest values are the only source for `version_name` and `version_code`.

- [ ] **Step 7: Run tests and verify GREEN**

Run:

```bash
go test ./internal/apprelease -run 'TestAPKInspector|TestValidate(Upload|Publish)APK' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/apprelease nx-backend/apps/server/go.mod nx-backend/apps/server/go.sum
git commit -m "feat: validate signed android releases"
```

---

## Chunk 2: Release state, HTTP APIs, and download protocol

### Task 4: Implement PostgreSQL release store and serialized publication

**Files:**
- Create: `nx-backend/apps/server/internal/apprelease/store.go`
- Create: `nx-backend/apps/server/internal/apprelease/store_test.go`

- [ ] **Step 1: Write failing CRUD/list tests**

Test `CreateDraft`, paginated `List`, `FindByID`, `LatestPublished`, `Archive`, and `ReferencedKeys`. Order history by `created_at DESC, id DESC`; `ListResult` contains a separate `Current *Release` so the published summary is available even when it is outside the requested history page, plus total storage bytes. `file_available` is derived by service/handler rather than stored in the table. The test helper must call `testutil.ValidateIsolatedPostgresDSN`, open with `db.Open`, and execute `TRUNCATE app_releases RESTART IDENTITY` before/after each test.

- [ ] **Step 2: Run store tests and verify RED**

Prepare and select an isolated local database, then run from the Go module:

```bash
docker rm -f nine-xing-apprelease-test-db 2>/dev/null || true
docker run --name nine-xing-apprelease-test-db --rm -d \
  -e POSTGRES_USER=nx \
  -e POSTGRES_PASSWORD=nx_test_password \
  -e POSTGRES_DB=nx_admin_test \
  -p 55432:5432 \
  --health-cmd='pg_isready -U nx -d nx_admin_test' \
  --health-interval=1s --health-timeout=3s --health-retries=30 \
  postgres:16-alpine
until [ "$(docker inspect -f '{{.State.Health.Status}}' nine-xing-apprelease-test-db)" = healthy ]; do sleep 1; done
cd nx-backend/apps/server
export TEST_DATABASE_URL='postgres://nx:nx_test_password@localhost:55432/nx_admin_test?sslmode=disable'
go test ./internal/apprelease -run 'TestStore' -count=1
```

Expected: FAIL because the store is missing.

- [ ] **Step 3: Write failing publish transaction tests**

Cover publishing a draft, republishing an archived version for rollback, archiving the previous published row, preserving old publication when the transaction fails, and two simultaneous publish attempts.

```go
func TestStorePublishArchivesPreviousVersionAtomically(t *testing.T) {
    database := openAppReleaseTestDB(t)
    store := NewStore(database)
    old := insertRelease(t, database, Release{Platform: "android", VersionName: "1.0.0", VersionCode: 100, Status: StatusPublished})
    target := insertRelease(t, database, Release{Platform: "android", VersionName: "1.1.0", VersionCode: 110, Status: StatusDraft})

    published, err := store.Publish(context.Background(), target.ID, "android")
    require.NoError(t, err)
    require.Equal(t, target.ID, published.ID)
    require.Equal(t, StatusPublished, releaseStatus(t, database, target.ID))
    require.Equal(t, StatusArchived, releaseStatus(t, database, old.ID))
}

func TestStorePublishAllowsArchivedVersionRollback(t *testing.T) {
    database := openAppReleaseTestDB(t)
    store := NewStore(database)
    old := insertRelease(t, database, Release{Platform: "android", VersionName: "1.0.0", VersionCode: 100, Status: StatusArchived})
    current := insertRelease(t, database, Release{Platform: "android", VersionName: "1.1.0", VersionCode: 110, Status: StatusPublished})

    _, err := store.Publish(context.Background(), old.ID, "android")
    require.NoError(t, err)
    require.Equal(t, StatusPublished, releaseStatus(t, database, old.ID))
    require.Equal(t, StatusArchived, releaseStatus(t, database, current.ID))
}

func TestStoreConcurrentPublishLeavesExactlyOnePublished(t *testing.T) {
    database := openAppReleaseTestDB(t)
    store := NewStore(database)
    first := insertRelease(t, database, Release{Platform: "android", VersionName: "1.0.0", VersionCode: 100, Status: StatusDraft})
    second := insertRelease(t, database, Release{Platform: "android", VersionName: "1.1.0", VersionCode: 110, Status: StatusDraft})

    var group errgroup.Group
    group.Go(func() error { _, err := store.Publish(context.Background(), first.ID, "android"); return err })
    group.Go(func() error { _, err := store.Publish(context.Background(), second.ID, "android"); return err })
    require.NoError(t, group.Wait())
    require.Equal(t, 1, publishedCount(t, database, "android"))
}
```

Define `openAppReleaseTestDB`, `insertRelease`, `releaseStatus`, and `publishedCount` in the test file using direct SQL. `insertRelease` must supply non-empty file fields and a 64-character SHA so it exercises the real schema rather than mocks.

For the rollback test, install a temporary PostgreSQL `BEFORE UPDATE` trigger that raises an exception only when the selected target changes to `published`. This deterministically fails after the old row is updated to `archived`; after `Publish` returns, assert the transaction rolled back and the old row is still `published`. Drop the trigger/function in `t.Cleanup`.

- [ ] **Step 4: Run publish tests and verify RED**

Run:

```bash
cd nx-backend/apps/server
export TEST_DATABASE_URL='postgres://nx:nx_test_password@localhost:55432/nx_admin_test?sslmode=disable'
go test -race ./internal/apprelease -run 'TestStore.*Publish|TestStoreConcurrentPublish' -count=1
```

Expected: FAIL because publish serialization is absent.

- [ ] **Step 5: Implement store queries and transaction**

Inside `Publish(ctx, id, platform)`:

```sql
SELECT pg_advisory_xact_lock(hashtextextended($1, 0));
SELECT id, status FROM app_releases WHERE id=$2 AND platform=$1 FOR UPDATE;
UPDATE app_releases SET status='archived' WHERE platform=$1 AND status='published' AND id<>$2;
UPDATE app_releases
   SET status='published', published_at=now()
 WHERE id=$2 AND platform=$1 AND status IN ('draft', 'archived');
```

If the target is already the current `published` row, return it idempotently. Otherwise require target status `draft` or `archived`, then archive the current row and publish the target. Check `RowsAffected()==1` for the target update; any zero-row update, trigger error, unique conflict, or commit error rolls back the entire transaction so the old release remains published. Translate invalid state/unique failures into typed domain conflicts.

- [ ] **Step 6: Run store tests repeatedly and verify GREEN**

Run:

```bash
cd nx-backend/apps/server
export TEST_DATABASE_URL='postgres://nx:nx_test_password@localhost:55432/nx_admin_test?sslmode=disable'
go test -race ./internal/apprelease -run 'TestStore' -count=1
go test -race ./internal/apprelease -run TestStoreConcurrentPublish -count=20
```

Expected: PASS with exactly one published row on every iteration.

- [ ] **Step 7: Commit**

```bash
git add nx-backend/apps/server/internal/apprelease/store.go nx-backend/apps/server/internal/apprelease/store_test.go
git commit -m "feat: persist app release lifecycle"
```

### Task 5: Orchestrate uploads, publication, archive, and startup maintenance

**Files:**
- Create: `nx-backend/apps/server/internal/apprelease/service.go`
- Create: `nx-backend/apps/server/internal/apprelease/service_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: Write failing service tests**

Use injected inspector/store/file-store interfaces to verify:

- upload saves to a temp/final file, inspects APK, derives Manifest version, creates a draft, and removes the final file if DB insertion fails;
- invalid APK or duplicate version leaves no final/temp file;
- publish re-inspects the stored file, fails when certificate config is missing/mismatched, and never changes current published status on failure;
- archive changes status but retains the file;
- startup removes stale temps and reports, but never deletes, orphan final files.

The service exposes a two-phase upload boundary so the HTTP multipart parser can support arbitrary field order without buffering the APK:

```go
StageAPK(originalName string, src io.Reader) (StagedFile, error)
CreateDraftFromStaged(ctx context.Context, staged StagedFile, releaseNotes string) (Release, error)
DiscardStaged(staged StagedFile) error
```

`CreateDraftFromStaged` performs inspect/validate, then `Commit`, then DB insert; validation failures discard only the temp file, and DB failures remove the committed final file.

- [ ] **Step 2: Run service tests and verify RED**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/apprelease -run 'TestService|TestStartupMaintenance' -count=1
```

Expected: FAIL because `Service` does not exist.

- [ ] **Step 3: Implement minimal orchestration**

Expose focused methods:

```go
StageAPK(originalName string, src io.Reader) (StagedFile, error)
CreateDraftFromStaged(ctx context.Context, staged StagedFile, releaseNotes string) (Release, error)
DiscardStaged(staged StagedFile) error
Publish(ctx context.Context, id int64) (Release, error)
Archive(ctx context.Context, id int64) (Release, error)
List(ctx context.Context, page, pageSize int) (ListResult, error)
Latest(ctx context.Context, platform string) (Release, error)
OpenPublished(ctx context.Context, id int64) (Release, *os.File, error)
Maintain(ctx context.Context, now time.Time) error
```

Keep the 300 MiB APK constant in this package and expose a 301 MiB multipart request constant to the HTTP layer.

- [ ] **Step 4: Wire service initialization without registering routes yet**

Build the file root from `filepath.Join(env.UploadDir, "app-releases")`, inject config/package/certificate, and run maintenance after database/schema initialization. Maintenance errors should be logged and should not make the whole API unavailable unless the root cannot be created/written.

- [ ] **Step 5: Run tests and verify GREEN**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/apprelease ./internal/server -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add nx-backend/apps/server/internal/apprelease nx-backend/apps/server/internal/server/server.go
git commit -m "feat: orchestrate app release publication"
```

### Task 6: Add admin and public HTTP APIs with resumable downloads

**Files:**
- Create: `nx-backend/apps/server/internal/server/app_releases.go`
- Create: `nx-backend/apps/server/internal/server/app_releases_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/cmd/server/main.go`

- [ ] **Step 1: Write failing route and permission tests**

Assert:

- list requires `Website:AppReleases:List`;
- upload/publish/archive require `Website:AppReleases:Write`;
- public latest/download routes require no token;
- only `GET|HEAD` is accepted for public download endpoints.

- [ ] **Step 2: Run permission tests and verify RED**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/server -run 'TestAppRelease.*Permission|TestPublicAppRelease.*Method' -count=1
```

Expected: FAIL/404 because routes are absent.

- [ ] **Step 3: Write failing upload/admin response tests**

Cover multipart streaming, arbitrary `release_notes`/`file` part order, `.apk` validation, duplicate/missing file parts, bounded small text fields, request limit 301 MiB, 413 mapping, invalid APK 400, duplicate 409, certificate publication 503/409, and the exact list response keys below. Confirm the handler never calls `ParseMultipartForm` or `io.ReadAll` for the APK.

```json
{
  "current": null,
  "items": [],
  "page": 1,
  "pageSize": 20,
  "total": 0,
  "totalFileSize": 0
}
```

When rows exist, every serialized item uses the exact camelCase keys `id`, `platform`, `versionName`, `versionCode`, `releaseNotes`, `fileName`, `fileSize`, `sha256`, `status`, `fileAvailable`, `createdAt`, and `publishedAt`; never expose `filePath`.

- [ ] **Step 4: Run admin API tests and verify RED**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/server -run 'TestAppReleaseUpload|TestAppReleaseList|TestAppReleasePublish|TestAppReleaseArchive' -count=1
```

Expected: FAIL because handlers are absent.

- [ ] **Step 5: Write failing public metadata and download protocol tests**

Cover exact behavior:

```text
GET latest no published       -> 200 {available:false}, Cache-Control:no-cache
GET latest published          -> 200 metadata, immutable download URL, no file_path
GET latest missing file       -> 503
GET latest download           -> 302 Location:/api/public/app-releases/{id}/download
HEAD latest download          -> 302 same Location/no response body
GET/HEAD latest no published  -> 404
GET/HEAD draft ID             -> 404
GET/HEAD archived ID          -> 410
GET/HEAD published missing    -> 503
GET published                 -> 200 APK headers/body
HEAD published                -> 200 same headers/no body
Range bytes=0-3               -> 206 + Content-Range
Invalid/unsatisfiable Range   -> 416 + Content-Range: bytes */{size}
If-None-Match matching SHA    -> 304
```

Verify `Content-Type`, safe quoted `Content-Disposition`, `nosniff`, SHA ETag, and `private, max-age=300, must-revalidate`. Latest metadata and both GET/HEAD 302 responses use `Cache-Control: no-cache`; HEAD responses have no entity body.

- [ ] **Step 6: Run protocol tests and verify RED**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/server -run 'TestPublicAppRelease' -count=1
```

Expected: FAIL because protocol handlers are absent.

- [ ] **Step 7: Implement handlers and error mapping**

Use `http.MaxBytesReader` and `r.MultipartReader()`. Iterate parts once; read `release_notes` through an independent 64 KiB `LimitReader`, accept exactly one `file` part, and pass that part directly to `Service.StageAPK` while the request is being read. Support either field order by holding only the small text value plus `StagedFile` metadata. Reject duplicate file parts/unknown oversized fields, call `DiscardStaged` on every later parse/validation failure, and never create Go/system multipart temp files. After all parts are parsed, call `CreateDraftFromStaged`.

Use `http.ServeContent` for immutable downloads; pre-handle exact ETag matches before `ServeContent` and ensure archived/draft status is checked before opening the file. Let `ServeContent` produce standards-compliant `206` and `416` responses.

Latest metadata response shape:

```json
{
  "available": true,
  "platform": "android",
  "versionName": "1.2.3",
  "versionCode": 123,
  "publishedAt": "2026-07-20T12:00:00Z",
  "fileSize": 12345678,
  "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "releaseNotes": "优化首次安装流程",
  "downloadUrl": "/api/public/app-releases/42/download"
}
```

- [ ] **Step 8: Write failing real-server deadline tests**

Start an `httptest.NewUnstartedServer`, set `ReadTimeout`/`WriteTimeout` to 20ms, and send an upload body through `io.Pipe` with chunks delayed by 40ms. The release handler must succeed only after it clears/extends the connection read deadline. Add a deadline-aware response-writer test for a delayed streamed download and assert the helper clears the write deadline. Run:

```bash
cd nx-backend/apps/server
go test ./internal/server -run 'TestAppRelease.*Deadline' -count=1
```

Expected: FAIL because release handlers do not yet adjust deadlines.

- [ ] **Step 9: Register routes and handle deadlines**

Register the six specified endpoints in `routes()`. At the beginning of upload/download handlers, use `http.NewResponseController(w).SetReadDeadline(time.Time{})` and/or `SetWriteDeadline(time.Time{})`; ignore only `http.ErrNotSupported`, but surface/log other errors. Keep the global 20s/120s timeouts for unrelated APIs.

- [ ] **Step 10: Run focused and full Go tests and verify GREEN**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/apprelease ./internal/config ./internal/db ./internal/server -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add nx-backend/apps/server/internal/server nx-backend/apps/server/cmd/server/main.go
git commit -m "feat: expose app release management and downloads"
```

### Task 7: Configure nginx streaming boundaries

**Files:**
- Create: `nx-backend/apps/server/internal/server/app_release_proxy_test.go`
- Modify: `nx-backend/scripts/deploy/nginx.conf`
- Modify: `website-react/nginx.conf`

- [ ] **Step 1: Write failing nginx contract tests**

Read both configs and assert:

- admin exact `location = /api/app-releases/upload` has `client_max_body_size 301m` and `proxy_request_buffering off`;
- global `client_max_body_size 50m` remains unchanged;
- both configs have focused latest/id download locations with `proxy_buffering off` and `proxy_cache off`;
- website config does not expose upload override.

- [ ] **Step 2: Run proxy tests and verify RED**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/server -run TestAppReleaseProxyConfiguration -count=1
```

Expected: FAIL because focused locations are absent.

- [ ] **Step 3: Add focused nginx locations**

Place them before generic `/api/` blocks and preserve existing forwarded headers. Use `proxy_http_version 1.1`, clear request buffering for upload, and disable response buffering/cache for APK download. Do not relax other API limits.

- [ ] **Step 4: Run tests and nginx syntax checks and verify GREEN**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/server -run TestAppReleaseProxyConfiguration -count=1
cd ../../..
docker run --rm --add-host server:127.0.0.1 -v "$PWD/nx-backend/scripts/deploy/nginx.conf:/etc/nginx/nginx.conf:ro" nginx:alpine nginx -t -c /etc/nginx/nginx.conf
docker run --rm --add-host server:127.0.0.1 -v "$PWD/website-react/nginx.conf:/etc/nginx/nginx.conf:ro" nginx:alpine nginx -t -c /etc/nginx/nginx.conf
```

Expected: PASS and both `nginx -t` commands report syntax successful.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/server/app_release_proxy_test.go nx-backend/scripts/deploy/nginx.conf website-react/nginx.conf
git commit -m "feat: stream app release uploads and downloads"
```

---

## Chunk 3: Admin version management and editable download copy

### Task 8: Add the typed admin API client

**Files:**
- Create: `nx-backend/apps/web-antd/src/api/core/app-release.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/app-release.test.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/index.ts`

- [ ] **Step 1: Write failing API contract tests**

Mock `requestClient` and assert list query params, multipart upload with `release_notes`, upload progress callback, and publish/archive URLs/methods.

The desired API is exact:

```ts
export interface AppReleaseListParams { page: number; pageSize: number }
export function getAppReleaseListApi(params: AppReleaseListParams): Promise<AppReleaseListResult> {
  return requestClient.get('/app-releases/list', { params });
}
export function uploadAppReleaseApi(file: File, releaseNotes: string, onUploadProgress?: (event: AxiosProgressEvent) => void): Promise<AppRelease> {
  return requestClient.upload('/app-releases/upload', { file, release_notes: releaseNotes }, { onUploadProgress });
}
export function publishAppReleaseApi(id: number): Promise<AppRelease> {
  return requestClient.post(`/app-releases/${id}/publish`);
}
export function archiveAppReleaseApi(id: number): Promise<AppRelease> {
  return requestClient.post(`/app-releases/${id}/archive`);
}
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
cd nx-backend
pnpm vitest run apps/web-antd/src/api/core/app-release.test.ts
```

Expected: FAIL because the module is absent.

- [ ] **Step 3: Implement the typed client**

Define:

```ts
export type AppReleaseStatus = 'archived' | 'draft' | 'published';
export interface AppRelease { id: number; platform: 'android'; versionName: string; versionCode: number; releaseNotes: string; fileName: string; fileSize: number; sha256: string; status: AppReleaseStatus; fileAvailable: boolean; createdAt: string; publishedAt: null | string; }
export interface AppReleaseListResult { current: AppRelease | null; items: AppRelease[]; page: number; pageSize: number; total: number; totalFileSize: number; }
```

Use `requestClient.upload` for multipart so `onUploadProgress` is preserved. Task 6 must serialize list JSON with the exact camelCase keys `current`, `items`, `page`, `pageSize`, `total`, and `totalFileSize`.

- [ ] **Step 4: Run test and typecheck and verify GREEN**

Run:

```bash
cd nx-backend
pnpm vitest run apps/web-antd/src/api/core/app-release.test.ts
pnpm --filter @vben/web-antd typecheck
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/api/core/app-release.ts nx-backend/apps/web-antd/src/api/core/app-release.test.ts nx-backend/apps/web-antd/src/api/core/index.ts
git commit -m "feat: add app release admin api client"
```

### Task 9: Build the App version management page

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/site-config/app-releases.vue`
- Create: `nx-backend/apps/web-antd/src/views/site-config/app-releases.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/site-config/app-release-view.ts`
- Create: `nx-backend/apps/web-antd/src/views/site-config/app-release-view.test.ts`
- Modify: `nx-backend/apps/web-antd/src/test-utils/antd-stubs.ts`

- [ ] **Step 1: Write failing pure file/status rule tests**

Define and test:

```ts
validateAPKFile(file: Pick<File, 'name' | 'size'>): null | string
canPublishRelease(release: AppRelease): boolean
canArchiveRelease(release: AppRelease): boolean
releaseStatusLabel(status: AppReleaseStatus): string
formatReleaseFileSize(bytes: number): string
```

Assert case-insensitive `.apk`, exact 300 MiB acceptance, `300 MiB + 1 byte` rejection, draft/archived publication only when `fileAvailable`, published archive eligibility, Chinese labels, and readable byte formatting.

- [ ] **Step 2: Run rule tests and verify RED**

Run:

```bash
cd nx-backend
pnpm vitest run apps/web-antd/src/views/site-config/app-release-view.test.ts
```

Expected: FAIL because the rule module is absent.

- [ ] **Step 3: Implement the minimal pure rules and verify GREEN**

Run the same command and expect PASS.

- [ ] **Step 4: Write failing mounted list/summary tests**

Mount the real Vue component with `#/test-utils/vue-mount`, mock `#/api`, and assert:

- loading and inline retry behavior;
- `current` summary is independent of the visible pagination page;
- total file count/storage;
- every required detail is rendered: version name/code, file name/size, SHA-256, upload time, publish time, update notes, status, and file availability;
- visible guidance states “最大 300 MiB” and “仅上传正式签名 APK”.

- [ ] **Step 5: Run list/summary tests and verify RED**

Run: `cd nx-backend && pnpm vitest run --dom apps/web-antd/src/views/site-config/app-releases.test.ts`

Expected: FAIL because the page and required Ant stubs are absent.

- [ ] **Step 6: Add test stubs and implement list/summary GREEN**

Extend `antd-stubs.ts` with a behavior-capable `Upload` stub that renders `<input type="file">`; on input change it calls the provided `beforeUpload(file)`, then emits Ant-compatible `change` payload `{ file: { name, originFileObj: file, size, type } }` when selection is retained. This lets Happy DOM tests install a `File`, trigger change, assert exact-limit validation, and prove API upload is not called on selection. Add a `Progress` stub exposing `percent` and `Modal.confirm` that tests can spy on; keep the existing modal component behavior. Stub `@vben/common-ui` `Page` separately in the page test because `Page` is not an Ant Design component. Implement list loading, current summary, details, pagination, total storage, missing-file warning, and retry; rerun Step 5 and expect PASS.

- [ ] **Step 7: Write failing permission and upload-flow tests**

Assert:

- `Website:AppReleases:Write` gates upload/publish/archive actions;
- file picker accepts `.apk` only and rejects more than 300 MiB before upload;
- release notes persist when validation/upload fails;
- upload progress is visible and double submit is disabled;
- selected file is not automatically uploaded;
- successful upload renders the Manifest-derived version name/code read-only and reloads history.

- [ ] **Step 8: Run upload-flow tests and verify RED**

Run: `cd nx-backend && pnpm vitest run --dom apps/web-antd/src/views/site-config/app-releases.test.ts`

Expected: FAIL on missing upload/permission behavior.

- [ ] **Step 9: Implement upload/permission GREEN**

Use Ant Design Vue `Page`, `Card`, `Table`, `Upload`, `Modal`, `Progress`, `Tag`, `Alert`, and `Descriptions`. Use `beforeUpload={() => false}` so selection never auto-uploads. Validate `file.name.toLowerCase().endsWith('.apk')` and `file.size <= 300 * 1024 * 1024`; retain the selected file and release notes when upload fails. Keep Manifest-derived version fields out of the upload form; show them only after the server returns the created draft. Make archive visually destructive and publish the single primary action per eligible row. Disable all async mutation buttons while their request is active and show a recoverable inline list error.

Rerun:

```bash
cd nx-backend
pnpm vitest run --dom apps/web-antd/src/views/site-config/app-releases.test.ts
```

Expected: PASS for the permission/upload-flow cases before writing action tests.

- [ ] **Step 10: Write failing publish/archive action tests**

Assert draft/archived rows with an available file call publish after `Modal.confirm`, published rows call archive after a destructive confirmation, missing-file rows cannot publish, each success reloads the list, and failure leaves current data visible.

- [ ] **Step 11: Run action tests and verify RED**

Run: `cd nx-backend && pnpm vitest run --dom apps/web-antd/src/views/site-config/app-releases.test.ts`

Expected: FAIL on missing action behavior.

- [ ] **Step 12: Implement action GREEN**

Use the pure eligibility helpers for rendering and handlers, then rerun Step 11 and expect PASS.

- [ ] **Step 13: Run all page tests, typecheck, and targeted build**

Run:

```bash
cd nx-backend
pnpm vitest run --dom apps/web-antd/src/views/site-config/app-release-view.test.ts apps/web-antd/src/views/site-config/app-releases.test.ts
pnpm --filter @vben/web-antd typecheck
pnpm run build:antd
```

Expected: PASS.

- [ ] **Step 14: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/site-config/app-release-view.ts nx-backend/apps/web-antd/src/views/site-config/app-release-view.test.ts nx-backend/apps/web-antd/src/views/site-config/app-releases.vue nx-backend/apps/web-antd/src/views/site-config/app-releases.test.ts nx-backend/apps/web-antd/src/test-utils/antd-stubs.ts
git commit -m "feat: manage app releases in admin"
```

---

## Chunk 4: Public website, device routing, QR, and old-config compatibility

### Task 10: Add default `home.appDownload` and backfill existing stored site configs

**Files:**
- Modify: `nx-backend/apps/server/internal/siteconfig/site_config.go`
- Modify/Create: `nx-backend/apps/server/internal/siteconfig/site_config_test.go`
- Modify: `shared/site-config.json`

- [ ] **Step 1: Write failing default and backward-compatibility tests**

Add `TestSharedConfigDefinesAppDownloadDefaults`, then a pure `TestMergeMissingMapDefaultsForAppDownload`: an old `home` map without `appDownload` receives the default object, a customized Hero title remains unchanged, partial `appDownload` customizations win, and explicit empty strings/arrays remain empty. Add `TestReadStoreBackfillsAppDownload`, using `TEST_DATABASE_URL`, that inserts old JSON into `site_configs`, calls real `ReadStore`, and observes the same result. Validate the DSN with `testutil.ValidateIsolatedPostgresDSN`, use the standalone `nine-xing-apprelease-test-db` from Task 4, and clean only the `site_configs/default` test row.

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
cd nx-backend/apps/server
export TEST_DATABASE_URL='postgres://nx:nx_test_password@localhost:55432/nx_admin_test?sslmode=disable'
go test ./internal/siteconfig -run 'TestSharedConfigDefinesAppDownloadDefaults|TestMergeMissingMapDefaultsForAppDownload|TestReadStoreBackfillsAppDownload' -count=1
```

Expected: FAIL because the merge helper/default section does not exist. The integration case may SKIP without `TEST_DATABASE_URL`, but the pure test must fail.

- [ ] **Step 3: Implement recursive default merge for this section**

Add the complete default copy object to `shared/site-config.json` before any admin editor consumes it. In `ReadStore`, read the provided default config path, deep-clone `home.appDownload`, and recursively fill only absent keys in stored config. Preserve explicit empty strings/empty arrays if the administrator intentionally saved them. Keep the existing customer-service QR compatibility logic intact; do not cache globally by one path because tests and deployments may supply different config paths.

- [ ] **Step 4: Run tests and verify GREEN**

Run:

```bash
cd nx-backend/apps/server
export TEST_DATABASE_URL='postgres://nx:nx_test_password@localhost:55432/nx_admin_test?sslmode=disable'
go test ./internal/siteconfig -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/siteconfig shared/site-config.json
git commit -m "fix: backfill app download site config"
```

### Task 11: Make download-section copy editable in site settings

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/site-config.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/site-config.typecheck.ts`
- Modify: `nx-backend/apps/web-antd/src/views/site-config/home.vue`
- Create: `nx-backend/apps/web-antd/src/views/site-config/home-app-download.test.ts`

- [ ] **Step 1: Write the failing compile-time type assertion**

Create a type-only fixture using an `IsAny<T>` conditional type and assert `SiteConfig['home']['appDownload']` is not `any` and is assignable to `AppDownloadSiteConfig`. Run:

```bash
cd nx-backend
pnpm --filter @vben/web-antd typecheck
```

Expected: FAIL because `AppDownloadSiteConfig`/the typed property do not exist.

- [ ] **Step 2: Add explicit types and verify typecheck GREEN**

Define `AppDownloadSiteConfig`, then type `home` as `Record<string, any> & { appDownload: AppDownloadSiteConfig }`. Rerun typecheck and expect PASS.

- [ ] **Step 3: Write failing mounted editor tests**

Mount `home.vue` with a mocked config and assert controls for `eyebrow`, `title`, `lead`, `features[]`, `installSteps[]`, `androidButtonText`, `iosComingSoonText`, `unavailableText`, and `retryText`. Verify newline editing maps back to arrays without changing unrelated Hero fields. The shared default/backfill from Task 10 guarantees the object exists; still initialize from the bundled default defensively if a malformed test payload omits it.

- [ ] **Step 4: Run editor test and verify RED**

Run: `cd nx-backend && pnpm vitest run --dom apps/web-antd/src/views/site-config/home-app-download.test.ts`

Expected: FAIL because controls are absent.

- [ ] **Step 5: Implement editor controls and verify GREEN**

Use computed textarea getters/setters plus `linesToArray` for array fields and visible helper copy; do not expose version number, SHA, file size, release date, or release notes because those come from the release API.

- [ ] **Step 6: Run editor test, typecheck, and final admin build**

```bash
cd nx-backend
pnpm vitest run --dom apps/web-antd/src/views/site-config/home-app-download.test.ts
pnpm --filter @vben/web-antd typecheck
pnpm run build:antd
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add nx-backend/apps/web-antd/src/api/core/site-config.ts nx-backend/apps/web-antd/src/api/core/site-config.typecheck.ts nx-backend/apps/web-antd/src/views/site-config/home.vue nx-backend/apps/web-antd/src/views/site-config/home-app-download.test.ts
git commit -m "feat: edit app download website copy"
```

### Task 12: Add public metadata client and device/format helpers

**Files:**
- Create: `website-react/src/api/appRelease.js`
- Create: `website-react/src/api/appRelease.test.mjs`
- Create: `website-react/src/utils/appDownloadDevice.js`
- Create: `website-react/src/utils/appDownloadDevice.test.mjs`

- [ ] **Step 1: Write failing device tests**

Assert Android, iPhone/iPad/iPod, iPadOS desktop mode (`platform === 'MacIntel' && maxTouchPoints > 1`), desktop, and unknown fallback. Keep navigator reads inside a function for Node tests.

- [ ] **Step 2: Run and verify RED**

Run: `cd website-react && node --test src/utils/appDownloadDevice.test.mjs`

Expected: FAIL because the module is absent.

- [ ] **Step 3: Write failing metadata/format tests**

Cover API success, `{available:false}`, non-2xx/error envelope, abort/network failure, MiB formatting, local date formatting, and SHA grouping/wrapping-safe output. The client signature is:

```js
getLatestAppRelease({ apiBase, fetchImpl, signal } = {})
buildLatestAppReleaseDownloadURL(apiBase)
```

`apiBase` defaults safely with `import.meta.env?.VITE_API_BASE_URL || '/api'` so Node can import the module. Throw `AppReleaseAPIError` with `status` and a safe message for non-2xx responses; preserve `AbortError` unchanged. Assert a 503 retains `status === 503` and the download URL builder works for both `/api` and `https://api.example.com/api`.

- [ ] **Step 4: Run and verify RED**

Run:

```bash
cd website-react
node --test src/api/appRelease.test.mjs src/utils/appDownloadDevice.test.mjs
```

Expected: FAIL because the API and helpers are absent.

- [ ] **Step 5: Implement pure helpers and API client**

Follow existing response envelope handling but use the Node-safe injectable signature above. Export a pure `detectAppDownloadDevice({userAgent, platform, maxTouchPoints})` plus formatting helpers. Unknown values return `desktop`; do not auto-download on module import or page load. The component must obtain its clickable latest-download URL from `buildLatestAppReleaseDownloadURL`, never a separately hardcoded `/api` path.

- [ ] **Step 6: Run tests and verify GREEN**

Run:

```bash
cd website-react
node --test src/api/appRelease.test.mjs src/utils/appDownloadDevice.test.mjs
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add website-react/src/api/appRelease.js website-react/src/api/appRelease.test.mjs website-react/src/utils/appDownloadDevice.js website-react/src/utils/appDownloadDevice.test.mjs
git commit -m "feat: load public app release metadata"
```

### Task 13: Build and integrate the branded App download section

**Files:**
- Create: `website-react/src/components/AppDownloadSection.jsx`
- Create: `website-react/src/components/AppDownloadSection.test.mjs`
- Create: `website-react/src/pages/home-app-download.test.mjs`
- Create: `website-react/src/utils/appDownloadViewModel.js`
- Create: `website-react/src/utils/appDownloadViewModel.test.mjs`
- Modify: `website-react/src/pages/Home.jsx`
- Modify: `website-react/src/components/Layout.jsx`
- Modify: `website-react/src/index.css`
- Modify: `shared/site-config.json`

- [ ] **Step 1: Write failing placement and entry-point tests**

Assert:

- `<AppDownloadSection />` appears immediately after the Hero closing section and before `teacherTeaser`;
- `navigation.main` and `navigation.drawer` contain `下载 App` to `/#download-app`;
- Hero contains an anchor action to `#download-app`.

- [ ] **Step 2: Run placement tests and verify RED**

Run: `cd website-react && node --test src/pages/home-app-download.test.mjs`

Expected: FAIL because the section/config entries are absent.

- [ ] **Step 3: Write failing pure state-matrix tests**

In `appDownloadViewModel.test.mjs`, cover loading, published, unavailable, 503 missing-file, ordinary network error with retry, Android direct link, iOS disabled coming-soon state, desktop QR, and unknown-device desktop behavior. The view model receives `{config, device, error, loading, release, downloadURL, pageURL}` and returns semantic labels/flags without reading globals. Assert the QR payload is `${origin}/#download-app`, not a third-party URL, and all published metadata fields are exposed.

- [ ] **Step 4: Run component tests and verify RED**

Run:

```bash
cd website-react
node --test src/utils/appDownloadViewModel.test.mjs
```

Expected: FAIL because the pure view-model module is absent.

- [ ] **Step 5: Implement the view model and add failing component/CSS/Layout source contracts**

Implement the pure state module and verify its tests GREEN. Then create `AppDownloadSection.test.mjs` as an executable source/config/CSS contract test (matching the repository's existing Node-test convention). It must assert the component consumes the view model, uses `QRCode.toDataURL`, catches QR failure and renders fallback text, uses the API download URL builder, passes an `AbortSignal`, ignores abort on unmount, and renders all required fields: App name/introduction, version, publish time, file size, update notes, SHA-256, and installation steps. Assert a 503-specific “安装包暂时不可用” message and a retry action.

Add CSS/Layout RED assertions for:

- download CTA `min-height: 44px` and visible `:focus-visible`;
- SHA `overflow-wrap: anywhere` and section `max-width: 100%` without horizontal overflow;
- `#download-app`/section `scroll-margin-top` at least the sticky 70px navigation plus spacing;
- mobile single-column media query;
- `prefers-reduced-motion` animation/transition reduction;
- `Layout.jsx` checks `matchMedia('(prefers-reduced-motion: reduce)')` and uses `behavior: 'auto'` instead of smooth when requested.

Run:

```bash
cd website-react
node --test src/components/AppDownloadSection.test.mjs
```

Expected: FAIL because component/style/Layout contracts are absent.

- [ ] **Step 6: Implement `AppDownloadSection`**

Use `QRCode.toDataURL` lazily in `useEffect`; abort metadata fetch on unmount. Render stable reserved cards to avoid layout shift. Desktop shows Android CTA + QR; Android uses the URL returned by `buildLatestAppReleaseDownloadURL`; iOS shows a real disabled button with configured text. Error state includes a retry button and `aria-live="polite"`; status 503 uses the dedicated package-unavailable text.

- [ ] **Step 7: Integrate approved brand styling and reduced-motion scrolling**

Follow the existing warm/organic brand rather than the generic blue/orange palette returned by the UI search. Use the current CSS variables, official `/assets/logo.svg`, soft rounded surfaces, restrained gold glow, clear hierarchy, and one primary Android CTA. Requirements:

- 44px minimum touch targets and visible keyboard focus;
- digest uses `overflow-wrap:anywhere`/monospace without horizontal scroll;
- mobile single column at 375px, tablet/desktop balanced two-column layout;
- only transform/opacity micro-interactions in 150–300ms;
- `prefers-reduced-motion: reduce` disables nonessential motion;
- loading/error/unavailable states retain the section footprint;
- no emojis as structural icons.

Update `Layout.jsx` so hash scrolling chooses `auto` when reduced motion is requested and `smooth` otherwise. Add `scroll-margin-top` on the download section so the sticky navigation never covers its heading.

- [ ] **Step 8: Add navigation entry points**

Reuse the `home.appDownload` product copy and installation instructions added in Task 10; do not duplicate or hardcode version metadata. Add download links to both navigation collections and Hero actions.

- [ ] **Step 9: Run website tests/build and verify GREEN**

Run:

```bash
cd website-react
npm test
npm run build
```

Expected: all tests PASS and Vite build exits 0.

- [ ] **Step 10: Perform responsive browser QA**

Start the built preview from `website-react`:

```bash
cd website-react
npm run preview -- --host 127.0.0.1 --port 4173
```

Open `http://127.0.0.1:4173/#download-app` with the browser tool and inspect 375px, 768px, 1024px, and 1440px widths. Verify no horizontal scrolling (`document.documentElement.scrollWidth === document.documentElement.clientWidth`), hash navigation lands below the fixed header, iOS/Android branches are testable by device emulation/injected detector inputs, focus states are visible, QR has stable dimensions, all required metadata is readable, 503/retry can be simulated, and reduced-motion mode uses non-animated hash scrolling.

- [ ] **Step 11: Commit**

```bash
git add shared/site-config.json website-react/src/api/appRelease.js website-react/src/components/AppDownloadSection.jsx website-react/src/components/AppDownloadSection.test.mjs website-react/src/components/Layout.jsx website-react/src/pages/Home.jsx website-react/src/pages/home-app-download.test.mjs website-react/src/utils/appDownloadViewModel.js website-react/src/utils/appDownloadViewModel.test.mjs website-react/src/index.css
git commit -m "feat: add branded app download section"
```

---

## Chunk 5: Runtime configuration, operations documentation, and full verification

### Task 14: Wire runtime env, persistent directory, and deployment documentation

**Files:**
- Modify: `.env.example`
- Modify: `nx-backend/apps/server/.env.example`
- Modify: `docker-compose.yml`
- Modify: `nx-backend/apps/server/Dockerfile`
- Modify: `nx-backend/apps/server/docker-entrypoint.sh` if runtime volume ownership requires it
- Modify: `DEPLOY.md`
- Modify: `README.md`
- Modify: `nx-backend/apps/server/README.md`
- Create: `nx-backend/apps/server/internal/server/app_release_deployment_test.go`

- [ ] **Step 1: Add failing configuration contract test/check**

Create `app_release_deployment_test.go`. From the `internal/server` package directory it reads `../../../../../.env.example`, `../../.env.example`, `../../../../../docker-compose.yml`, `../../Dockerfile`, `../../../../../DEPLOY.md`, `../../../../../README.md`, and `../../README.md`. Assert both env names are documented/passed, Compose still mounts `uploads:/data/uploads`, Dockerfile creates/chowns `/data/uploads/app-releases` before `USER app`, docs mention 301 MiB outer-proxy handling and uploads backup/restore, and no document suggests committing a keystore.

- [ ] **Step 2: Run and verify RED**

Run: `cd nx-backend/apps/server && go test ./internal/server -run TestAppReleaseDeploymentConfiguration -count=1`

Expected: FAIL because runtime/documentation wiring is absent.

- [ ] **Step 3: Update env and container wiring**

Pass:

```yaml
APP_RELEASE_PACKAGE_NAME: ${APP_RELEASE_PACKAGE_NAME:-com.xinzhili.nine_xing_app}
APP_RELEASE_CERT_SHA256: ${APP_RELEASE_CERT_SHA256:-}
```

Reuse the current uploads volume. In the image, create `/data/uploads/app-releases` and `chown -R app:app /data/uploads` before `USER app`; a newly created named volume will inherit the prepared mountpoint contents/ownership. Do not make entrypoint run as root. For pre-existing volumes, document a one-time explicit maintenance command using `docker compose run --rm --user root server chown -R app:app /data/uploads`, to be run only after backup. Do not add a keystore or certificate private key.

- [ ] **Step 4: Document the complete release workflow**

Document:

- build a formally signed Flutter release APK outside this feature;
- obtain the signing certificate SHA-256 and configure it (fingerprint is public metadata, not the keystore/private key);
- upload draft, verify extracted Manifest fields, publish, and archive;
- no configured formal fingerprint means drafts are allowed but publication fails closed;
- nginx/宝塔/CDN outer proxy must permit 301 MiB and disable request buffering only for upload;
- APK download proxy must not buffer/cache the whole response;
- backup/restore and monitor `/data/uploads/app-releases`;
- archived files remain on disk until explicit operations cleanup;
- troubleshoot 413, 400 invalid APK, 409 duplicate/mismatch, 503 fingerprint/file missing, and Range/HEAD failures.

Include a concrete consistency-safe backup/restore runbook:

1. Pause release mutations by stopping `admin`, `website`, and `server`; leave `db` running.
2. In the same maintenance window, create a PostgreSQL custom-format dump and a tar archive of `/data/uploads/app-releases` from the mounted `uploads` volume.
3. Record both backup SHA-256 values and the currently published release metadata.
4. To restore, keep public services stopped, restore PostgreSQL and the uploads tar as one matched backup set, then run a root maintenance container to `chown -R app:app /data/uploads`.
5. Start `server`, then `admin`/`website`; fetch latest metadata, download the immutable APK, and compare its SHA-256 to both the API value and the restored database row before reopening publication.

Document concrete commands based on:

```bash
docker compose stop admin website server
mkdir -p backups
docker compose exec -T db sh -lc 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > backups/nx-admin.dump
docker compose run --rm --no-deps --user root -v "$PWD/backups:/backup" server sh -c 'tar -C /data/uploads -czf /backup/app-releases.tgz app-releases'
sha256sum backups/nx-admin.dump backups/app-releases.tgz > backups/SHA256SUMS

docker compose exec -T db sh -lc 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists' < backups/nx-admin.dump
docker compose run --rm --no-deps --user root -v "$PWD/backups:/backup:ro" server sh -c 'rm -rf /data/uploads/app-releases && tar -C /data/uploads -xzf /backup/app-releases.tgz && chown -R app:app /data/uploads'
docker compose up -d server admin website
curl -fsS 'https://example.com/api/public/app-release/latest?platform=android'
curl -fL 'https://example.com/api/public/app-release/download?platform=android' -o restored.apk
sha256sum restored.apk
```

Also include exact outer nginx/宝塔 route examples, preserving existing generic limits:

```nginx
location = /api/app-releases/upload {
  client_max_body_size 301m;
  proxy_http_version 1.1;
  proxy_set_header Connection "";
  proxy_request_buffering off;
  proxy_pass http://127.0.0.1:8080;
}
location = /api/public/app-release/download {
  proxy_buffering off;
  proxy_cache off;
  proxy_pass http://127.0.0.1:8000;
}
location ~ ^/api/public/app-releases/[0-9]+/download$ {
  proxy_buffering off;
  proxy_cache off;
  proxy_pass http://127.0.0.1:8000;
}
```

For the formal signing fingerprint, document this command against the final Release APK:

```bash
"$ANDROID_HOME/build-tools/35.0.0/apksigner" verify --verbose --print-certs app-release.apk
```

Copy only `Signer #1 certificate SHA-256 digest` into `APP_RELEASE_CERT_SHA256`. Never generate, copy to the server, or commit the keystore/private key as part of this workflow.

- [ ] **Step 5: Run config checks and verify GREEN**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/server -run TestAppReleaseDeploymentConfiguration -count=1
cd ../../..
APP_ENV=production JWT_SECRET=12345678901234567890123456789012 ADMIN_PASSWORD=a-strong-admin-password POSTGRES_PASSWORD=a-strong-database-password docker compose config --quiet
```

Expected: PASS and Compose exits 0.

- [ ] **Step 6: Commit**

```bash
git add .env.example docker-compose.yml DEPLOY.md README.md nx-backend/apps/server/.env.example nx-backend/apps/server/Dockerfile nx-backend/apps/server/README.md nx-backend/apps/server/internal/server/app_release_deployment_test.go
git commit -m "docs: document app release operations"
```

### Task 15: Full verification, security review, and branch handoff

**Files:**
- Modify only files required by issues found during verification/review.

- [ ] **Step 1: Verify the complete Go backend**

Run:

```bash
docker rm -f nine-xing-apprelease-test-db 2>/dev/null || true
docker run --name nine-xing-apprelease-test-db --rm -d \
  -e POSTGRES_USER=nx \
  -e POSTGRES_PASSWORD=nx_test_password \
  -e POSTGRES_DB=nx_admin_test \
  -p 55432:5432 \
  --health-cmd='pg_isready -U nx -d nx_admin_test' \
  --health-interval=1s --health-timeout=3s --health-retries=30 \
  postgres:16-alpine
until [ "$(docker inspect -f '{{.State.Health.Status}}' nine-xing-apprelease-test-db)" = healthy ]; do sleep 1; done
cd nx-backend/apps/server
export TEST_DATABASE_URL='postgres://nx:nx_test_password@localhost:55432/nx_admin_test?sslmode=disable'
go test ./... -count=1
go test -race ./internal/apprelease ./internal/config ./internal/db ./internal/server ./internal/siteconfig -count=1
go test -race ./internal/apprelease -run TestStoreConcurrentPublish -count=20
```

Expected: PASS with no database integration skips. Stop/remove the named test container after all final checks, not between commands that reuse it.

- [ ] **Step 2: Verify admin and website**

Run:

```bash
cd nx-backend
pnpm test:unit
pnpm --filter @vben/web-antd typecheck
pnpm run build:antd

cd ../website-react
npm test
npm run build
```

Expected: PASS with zero test failures and exit code 0 for both builds.

- [ ] **Step 3: Verify Compose/nginx and APK protocol**

From the repository root run:

```bash
APP_ENV=production JWT_SECRET=12345678901234567890123456789012 ADMIN_PASSWORD=a-strong-admin-password POSTGRES_PASSWORD=a-strong-database-password docker compose config --quiet
APP_ENV=production JWT_SECRET=12345678901234567890123456789012 ADMIN_PASSWORD=a-strong-admin-password POSTGRES_PASSWORD=a-strong-database-password docker compose build server admin website
docker run --rm --add-host server:127.0.0.1 -v "$PWD/nx-backend/scripts/deploy/nginx.conf:/etc/nginx/nginx.conf:ro" nginx:alpine nginx -t -c /etc/nginx/nginx.conf
docker run --rm --add-host server:127.0.0.1 -v "$PWD/website-react/nginx.conf:/etc/nginx/nginx.conf:ro" nginx:alpine nginx -t -c /etc/nginx/nginx.conf
cd nx-backend/apps/server
go test ./internal/server -run 'TestPublicAppRelease|TestAppRelease.*Deadline|TestAppReleaseProxyConfiguration|TestAppReleaseDeploymentConfiguration' -count=1
```

Expected: Compose config/build and nginx syntax pass; automated real-handler tests verify latest metadata, 302 GET/HEAD, GET, HEAD, `Range: bytes=0-1023`, invalid Range 416, matching `If-None-Match` 304, archive 410, draft 404, no-current 404, and missing-file 503. If a deployed environment is available, additionally repeat these cases with `curl`; do not weaken automated coverage if no live environment exists.

- [ ] **Step 4: Run security and requirements checklist**

Confirm:

- `git -C "$(git rev-parse --show-toplevel)" ls-files '*.apk'` lists only `signed-minimal.apk` and `unsigned-minimal.apk` under `internal/apprelease/testdata`;
- `git -C "$(git rev-parse --show-toplevel)" ls-files | rg -i '\.(jks|keystore|p12|pfx|key)$|(^|/)key\.properties$'` returns no production signing material;
- the public disposable password `fixturepass` appears only in `internal/apprelease/testdata/README.md`; no production signing password is present;
- no API response exposes `file_path` or absolute disk paths;
- path traversal/symlink escape and oversized upload tests pass;
- current published version survives failed competing publication;
- no draft can be downloaded by guessed ID;
- website has a recoverable error state and no auto-download on desktop/unknown devices;
- existing generic upload remains 20 MiB and generic nginx limit remains 50 MiB.

- [ ] **Step 5: Request final spec and code-quality review**

Provide reviewers the approved spec, this plan, `BASE_SHA=261c59f`, final `HEAD_SHA`, and the full diff. Fix every Critical/Important issue, rerun affected tests, and re-request review until approved.

- [ ] **Step 6: Run a fresh final verification after all review fixes**

Recreate/confirm the isolated database, then run the complete commands again rather than referring to earlier output:

```bash
docker rm -f nine-xing-apprelease-test-db 2>/dev/null || true
docker run --name nine-xing-apprelease-test-db --rm -d \
  -e POSTGRES_USER=nx \
  -e POSTGRES_PASSWORD=nx_test_password \
  -e POSTGRES_DB=nx_admin_test \
  -p 55432:5432 \
  --health-cmd='pg_isready -U nx -d nx_admin_test' \
  --health-interval=1s --health-timeout=3s --health-retries=30 \
  postgres:16-alpine
until [ "$(docker inspect -f '{{.State.Health.Status}}' nine-xing-apprelease-test-db)" = healthy ]; do sleep 1; done

cd nx-backend/apps/server
export TEST_DATABASE_URL='postgres://nx:nx_test_password@localhost:55432/nx_admin_test?sslmode=disable'
go test ./... -count=1
go test -race ./internal/apprelease ./internal/config ./internal/db ./internal/server ./internal/siteconfig -count=1
go test -race ./internal/apprelease -run TestStoreConcurrentPublish -count=20

cd ../../../nx-backend
pnpm test:unit
pnpm --filter @vben/web-antd typecheck
pnpm run build:antd

cd ../website-react
npm test
npm run build

cd ..
APP_ENV=production JWT_SECRET=12345678901234567890123456789012 ADMIN_PASSWORD=a-strong-admin-password POSTGRES_PASSWORD=a-strong-database-password docker compose config --quiet
APP_ENV=production JWT_SECRET=12345678901234567890123456789012 ADMIN_PASSWORD=a-strong-admin-password POSTGRES_PASSWORD=a-strong-database-password docker compose build server admin website
docker run --rm --add-host server:127.0.0.1 -v "$PWD/nx-backend/scripts/deploy/nginx.conf:/etc/nginx/nginx.conf:ro" nginx:alpine nginx -t -c /etc/nginx/nginx.conf
docker run --rm --add-host server:127.0.0.1 -v "$PWD/website-react/nginx.conf:/etc/nginx/nginx.conf:ro" nginx:alpine nginx -t -c /etc/nginx/nginx.conf
docker rm -f nine-xing-apprelease-test-db
```

Record exact commands, pass counts, and build exit codes for the final handoff.

- [ ] **Step 7: Commit review fixes and report branch state**

If review or final verification changed files, first inspect `git diff --check` and `git diff --stat`, then commit the scoped fixes:

```bash
git add -A
git commit -m "fix: address app release review"
```

If there are no post-review changes, do not create an empty commit. Finally run:

```bash
git status --short
git log --oneline --decorate -15
```

Expected: clean worktree on `feature/website-app-download`. Then use `superpowers:finishing-a-development-branch` and present the required integration options; do not merge or push without the user choosing.
