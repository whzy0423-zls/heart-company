# App Release Metadata Design

## Goal

When an administrator uploads an Android APK, automatically extract and persist the app's Chinese display name, package name, version name, version code, and launcher icon. Show that metadata in both the current published release card and the release history table.

## Architecture

APK metadata extraction remains inside `apprelease.APKInspector`, alongside the existing manifest and certificate inspection. The inspector resolves the Chinese (`zh-CN`) application label first, then the default label, and finally the package name. It decodes the launcher icon and normalizes it to PNG bytes.

`apprelease.Service` persists text metadata with the release record and stores the normalized icon next to the APK in managed app-release storage. The database stores only the icon's managed relative path, not image bytes or a data URL.

The server exposes the protected route `GET|HEAD /api/app-release-icons/{id}` before the existing app-release mutation catch-all. It requires `Website:AppReleases:List`, allowing draft and archived icons to be shown only to authorized administrators. The frontend reuses the existing bearer-authenticated protected-image resolver, extended to recognize this path. The endpoint never exposes an APK or accepts a filesystem path.

## Data Model and Compatibility

Add backward-compatible columns to `app_releases`:

- `app_name`: extracted localized or fallback display name.
- `package_name`: manifest package identifier.
- `icon_path`: managed relative path for the normalized PNG icon.

Each column is `TEXT NOT NULL DEFAULT ''`. The schema includes idempotent `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` statements so existing production databases migrate at startup without producing `NULL` values that would break the existing direct string scanners. Existing rows remain valid with empty metadata and display a placeholder. Newly uploaded releases receive complete metadata when available.

## Extraction and Fallbacks

Required metadata remains package name, version name, and positive version code. Failure to read those values continues to reject the APK.

Optional presentation metadata is best effort:

1. Try the `zh-CN` application label.
2. Fall back to the APK's default label.
3. Fall back to the package name.
4. Try to decode a raster PNG or JPEG launcher icon and encode it as PNG.
5. Adaptive/vector XML icons are not rendered in this iteration; they use the placeholder unless the APK also resolves a raster launcher resource.
6. Reject optional icon processing when the source exceeds 8 MiB, dimensions exceed 2048×2048, or total pixels exceed 4 million. The normalized PNG output is capped at 8 MiB.
7. If icon resolution, format, limit validation, decoding, or encoding fails, accept the APK and leave `icon_path` empty so the UI shows a placeholder.

Signature and expected-package validation remain unchanged.

## API and UI

Release JSON adds `appName`, `packageName`, and `iconUrl`. `iconUrl` is `/api/app-release-icons/{id}` when an icon exists and empty otherwise.

The current published release card shows:

- launcher icon or placeholder;
- Chinese/default app name;
- package name;
- `versionName` and `versionCode`;
- APK file name and size.

The history table groups the icon, app name, and package name in an “应用” column while retaining the existing version, file, status, time, notes, and actions columns.

## Error Handling and Security

- Optional label/icon extraction errors do not fail a valid upload.
- Icon keys are derived from the committed APK key by replacing `.apk` with `.png`. `FileStore` writes PNGs atomically through a temporary file, caps them at 8 MiB, and only resolves/removes audited `.apk` and `.png` keys beneath `android/`.
- Icon persistence is best effort. A valid APK remains uploadable when the optional icon cannot be saved.
- If the icon is saved but database creation fails, service rollback removes both APK and PNG. Database reference enumeration and orphan auditing include both `file_path` and `icon_path` so maintenance treats them as one release artifact set.
- The protected icon response supports GET and HEAD, uses `image/png`, `X-Content-Type-Options: nosniff`, the release SHA-256 plus icon metadata for its ETag, and returns `404` for absent rows/files.
- Missing icon files return `404` and the UI falls back to a placeholder.
- APK upload size, signature, package identity, and version validation are unchanged.

## Testing

- APK inspector tests cover localized/default label fallback and icon PNG generation.
- Service/store tests cover metadata persistence and cleanup when database creation fails.
- Schema tests cover backward-compatible columns.
- Server tests cover icon routing, content type, missing records, and path safety.
- Frontend API/component tests cover the new fields and both current/history presentation.
- Existing app-release upload, publish, archive, proxy, and download tests remain green.
