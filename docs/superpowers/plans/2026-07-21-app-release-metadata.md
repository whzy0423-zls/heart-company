# App Release Metadata Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract an APK's localized app name, package name, version metadata, and launcher icon during upload, persist them safely, and show them in the current and historical App release admin UI.

**Architecture:** Extend the existing APK inspector with best-effort presentation metadata, persist text fields in PostgreSQL and normalized PNG icons beside managed APK files, expose icons through a permission-protected endpoint, and reuse the admin's authenticated image-preview resolver. Required APK identity/version/signature validation stays unchanged; optional label/icon failures fall back safely.

**Tech Stack:** Go 1.x, `archive/zip`, `image/png`, `github.com/shogo82148/androidbinary`, PostgreSQL, Vue 3, TypeScript, Ant Design Vue, Vitest.

---

## Chunk 1: Backend metadata and persistence

### Task 1: Extract localized labels and bounded raster icons

**Files:**
- Modify: `nx-backend/apps/server/internal/apprelease/apk.go`
- Modify: `nx-backend/apps/server/internal/apprelease/apk_test.go`

- [ ] **Step 1: Write failing inspector tests**

Add tests that assert `APKInfo` contains `AppName` and optional `IconPNG`, that label fallback returns the package name when localized/default labels are absent, and that icon normalization rejects dimensions above 2048×2048 or more than 4 million pixels.

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/apprelease -run 'TestAPKInspector|TestNormalizeAPKIcon|TestResolveAPKAppName' -count=1
```

Expected: FAIL because the new metadata fields/helpers do not exist.

- [ ] **Step 3: Implement minimal bounded extraction**

Extend `APKInfo` with:

```go
AppName string
IconPNG []byte
```

Use `apk.Label` with `androidbinary.ResTableConfig{Language: {'z','h'}, Country: {'C','N'}}`, then an empty default resource configuration, then `PackageName`. Resolve the launcher path with `parsed.Manifest().App.Icon.WithResTableConfig(&androidbinary.ResTableConfig{}).String()`. If resolution remains a resource ID or selects an XML/adaptive resource, return no icon. Otherwise open the same APK with `archive/zip.OpenReader`, locate the exact cleaned resource entry, accept only `.png`, `.jpg`, or `.jpeg`, check `FileHeader.UncompressedSize64 <= 8 MiB`, read through an 8 MiB limited reader, validate `image.DecodeConfig` dimensions/pixels before full decode, encode to PNG, and cap output at 8 MiB. Treat every optional label/icon error as a fallback rather than an invalid APK.

Use small synthetic ZIP fixtures in tests for bounded PNG/JPEG extraction and XML/adaptive fallback. Keep signature verification tests on the existing signed fixture; presentation helper tests must not depend on files inside the Go module cache.

- [ ] **Step 4: Run the focused tests and verify GREEN**

Run the command from Step 2. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/apprelease/apk.go nx-backend/apps/server/internal/apprelease/apk_test.go
git commit -m "feat: extract app release presentation metadata"
```

### Task 2: Persist release metadata with backward-compatible schema

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Modify: `nx-backend/apps/server/internal/db/schema_app_release_test.go`
- Modify: `nx-backend/apps/server/internal/apprelease/model.go`
- Modify: `nx-backend/apps/server/internal/apprelease/store.go`
- Modify: `nx-backend/apps/server/internal/apprelease/store_test.go`

- [ ] **Step 1: Write failing schema and store tests**

Require these columns in both fresh schema and idempotent migration statements:

```sql
app_name TEXT NOT NULL DEFAULT ''
package_name TEXT NOT NULL DEFAULT ''
icon_path TEXT NOT NULL DEFAULT ''
```

Extend store tests so create, list, find, publish, and archive round-trip the three fields without breaking existing rows.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd nx-backend/apps/server
go test ./internal/db ./internal/apprelease -run 'TestSchemaDefinesAppRelease|TestStore' -count=1
```

Expected: FAIL because schema/model/store do not contain the fields.

- [ ] **Step 3: Implement schema/model/store changes**

Add `AppName`, `PackageName`, and `IconPath` to `Release` with JSON names `appName`, `packageName`, and no direct JSON exposure for the storage key. Update `releaseColumns`, insert parameters, and `scanRelease`. Add `IconURL string json:"iconUrl"` as an API-facing computed field.

- [ ] **Step 4: Run tests and verify GREEN**

Run the command from Step 2. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/db/schema.sql nx-backend/apps/server/internal/db/schema_app_release_test.go nx-backend/apps/server/internal/apprelease/model.go nx-backend/apps/server/internal/apprelease/store.go nx-backend/apps/server/internal/apprelease/store_test.go
git commit -m "feat: persist app release metadata"
```

### Task 3: Store PNG icons atomically and maintain their lifecycle

**Files:**
- Modify: `nx-backend/apps/server/internal/apprelease/files.go`
- Modify: `nx-backend/apps/server/internal/apprelease/files_test.go`
- Modify: `nx-backend/apps/server/internal/apprelease/service.go`
- Create: `nx-backend/apps/server/internal/apprelease/service_test.go`

- [ ] **Step 1: Write failing file lifecycle tests**

Cover derived `.png` keys, atomic save, 8 MiB limit, unsafe paths, PNG removal, APK/PNG orphan auditing, referenced icon keys, and cleanup of both artifacts when database creation fails.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd nx-backend/apps/server
go test ./internal/apprelease -run 'TestFileStore.*Icon|TestFileStoreAuditOrphans|TestService.*Metadata|TestService.*Rollback|TestService.*OpenIcon' -count=1
```

Expected: FAIL because icon storage/lifecycle methods do not exist.

- [ ] **Step 3: Implement icon storage and service integration**

Add a `SaveIcon(apkKey string, pngData []byte) (string, error)` operation that derives the PNG key from a validated managed APK key, writes a temporary sibling file, syncs, renames, and syncs the directory. Update remove/resolve/audit allowlists to accept only `.apk` and `.png` under `android/`.

In `CreateDraftFromStaged`, commit the APK, best-effort save `APKInfo.IconPNG`, populate metadata fields, and remove both artifacts on store failure. Update `ReferencedKeys` to include non-empty `icon_path`.

Add one service-level `enrichRelease` helper that checks icon availability and sets `/api/app-release-icons/{id}`. Apply it consistently to releases returned by create/upload, publish, archive, list/current, latest, find/open, and any other release-returning service method so API responses never disagree about `iconUrl`.

Add `OpenIcon(ctx context.Context, id int64) (Release, *os.File, error)` to find the release, require a non-empty managed `IconPath`, resolve it through `FileStore`, open only a regular `.png`, and return `ErrNotFound` for absent metadata/files. Test this method directly; HTTP handlers must not reach into store/file internals.

- [ ] **Step 4: Run tests and verify GREEN**

Run the command from Step 2. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/apprelease/files.go nx-backend/apps/server/internal/apprelease/files_test.go nx-backend/apps/server/internal/apprelease/service.go nx-backend/apps/server/internal/apprelease/service_test.go
git commit -m "feat: manage app release icons"
```

## Chunk 2: Icon API and admin presentation

### Task 4: Serve protected release icons

**Files:**
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/app_releases.go`
- Modify: `nx-backend/apps/server/internal/server/app_releases_test.go` or create focused `app_release_icon_test.go`

- [ ] **Step 1: Write failing route and response tests**

Test `GET` and `HEAD /api/app-release-icons/{id}`, list-permission enforcement, PNG content type, nosniff header, ETag/304 behavior, missing metadata/file as 404, invalid IDs, and confirmation that the mutation catch-all is not invoked. Stub/inject the service `OpenIcon` seam used by server tests rather than accessing storage directly.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd nx-backend/apps/server
go test ./internal/server -run 'TestAppReleaseIcon' -count=1
```

Expected: FAIL because the route and handler do not exist.

- [ ] **Step 3: Implement the route and handler**

Register `/api/app-release-icons/` before `/api/app-releases/`, wrap it with GET/HEAD and `Website:AppReleases:List`, parse only a positive numeric ID, call `appReleases.OpenIcon`, and serve fixed `image/png` content with safe cache headers and an ETag derived from the release hash plus icon file metadata.

- [ ] **Step 4: Run tests and verify GREEN**

Run the command from Step 2. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/server/server.go nx-backend/apps/server/internal/server/app_releases.go nx-backend/apps/server/internal/server/app_release_icon_test.go
git commit -m "feat: serve protected app release icons"
```

### Task 5: Load protected icons in the admin client

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/app-release.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/app-release.test.ts`
- Modify: `nx-backend/apps/web-antd/src/utils/upload-asset-preview.ts`
- Modify: `nx-backend/apps/web-antd/src/utils/upload-asset-preview.test.ts`

- [ ] **Step 1: Write failing frontend contract tests**

Extend `AppRelease` with `appName`, `packageName`, and `iconUrl`. Require the protected preview utility to recognize `/api/app-release-icons/{id}` and fetch it with the bearer token.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd nx-backend
pnpm vitest run --dom apps/web-antd/src/api/core/app-release.test.ts apps/web-antd/src/utils/upload-asset-preview.test.ts
```

Expected: FAIL because the icon path is not classified as protected.

- [ ] **Step 3: Implement the API type and protected path recognition**

Add the three response fields to `AppRelease` and extend `isProtectedUploadAssetPath` with `/api/app-release-icons/`. Preserve the previously added 30-minute upload timeout.

- [ ] **Step 4: Run tests and verify GREEN**

Run the command from Step 2. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/api/core/app-release.ts nx-backend/apps/web-antd/src/api/core/app-release.test.ts nx-backend/apps/web-antd/src/utils/upload-asset-preview.ts nx-backend/apps/web-antd/src/utils/upload-asset-preview.test.ts
git commit -m "feat: load protected app release icons"
```

### Task 6: Display metadata in current and historical releases

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/site-config/app-releases.vue`
- Create: `nx-backend/apps/web-antd/src/views/site-config/app-release-icon.vue`
- Create: `nx-backend/apps/web-antd/src/views/site-config/app-release-icon.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/site-config/app-releases.test.ts`

- [ ] **Step 1: Write a failing presentation test**

Mount a focused `AppReleaseIcon` component with the existing Vue test mount helpers. Mock `fetch`, the access token, `URL.createObjectURL`, and `URL.revokeObjectURL`; assert a successful protected fetch renders the blob image and a rejected/404 fetch renders the placeholder. Mount the page with mocked list API data containing distinct current/history records and assert both app names, package names, version name/code values, and two icon instances are rendered.

- [ ] **Step 2: Run the test and verify RED**

```bash
cd nx-backend
pnpm vitest run --dom apps/web-antd/src/views/site-config/app-release-icon.test.ts apps/web-antd/src/views/site-config/app-releases.test.ts
```

Expected: FAIL because the fields and icon presentation are absent.

- [ ] **Step 3: Implement the UI**

Implement focused `AppReleaseIcon` with `useUploadAssetPreviewUrl(iconUrl, token)` and an Ant Design `Avatar`/image fallback so each mounted icon has deterministic success/failure behavior and revokes object URLs. Add an “应用” column containing this component, display name, and package name. Update the current release card with the same component and metadata while preserving file size, status, release notes, and actions.

- [ ] **Step 4: Run focused frontend tests and verify GREEN**

```bash
cd nx-backend
pnpm vitest run --dom apps/web-antd/src/views/site-config/app-release-icon.test.ts apps/web-antd/src/views/site-config/app-releases.test.ts apps/web-antd/src/api/core/app-release.test.ts apps/web-antd/src/utils/upload-asset-preview.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/site-config/app-release-icon.vue nx-backend/apps/web-antd/src/views/site-config/app-release-icon.test.ts nx-backend/apps/web-antd/src/views/site-config/app-releases.vue nx-backend/apps/web-antd/src/views/site-config/app-releases.test.ts
git commit -m "feat: show app metadata in release management"
```

## Chunk 3: End-to-end verification

### Task 7: Verify the complete release workflow

**Files:**
- Modify only if verification exposes a defect in files already listed above.

- [ ] **Step 1: Run all app-release backend tests**

```bash
cd nx-backend/apps/server
go test ./internal/apprelease ./internal/db ./internal/server -count=1
```

Expected: PASS.

- [ ] **Step 2: Run focused frontend tests**

```bash
cd nx-backend
pnpm vitest run --dom apps/web-antd/src/api/core/app-release.test.ts apps/web-antd/src/utils/upload-asset-preview.test.ts apps/web-antd/src/views/site-config/app-release-icon.test.ts apps/web-antd/src/views/site-config/app-releases.test.ts
```

Expected: PASS.

- [ ] **Step 3: Run frontend typecheck and production build**

```bash
cd nx-backend
pnpm --filter @vben/web-antd typecheck
pnpm --filter @vben/web-antd build
```

Expected: both exit 0. Existing dependency annotation warnings are acceptable; type or build errors are not.

- [ ] **Step 4: Run formatting/diff checks**

```bash
gofmt -w nx-backend/apps/server/internal/apprelease/*.go nx-backend/apps/server/internal/server/app_release*.go
git diff --check
git status --short
```

Expected: no whitespace errors; status contains only intended changes.

- [ ] **Step 5: Commit any verification-only corrections**

Only if Step 1-4 required corrections:

```bash
git add <corrected-files>
git commit -m "fix: finalize app release metadata workflow"
```
