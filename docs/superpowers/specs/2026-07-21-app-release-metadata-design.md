# App Release Metadata Design

## Goal

When an administrator uploads an Android APK, automatically extract and persist the app's Chinese display name, package name, version name, version code, and launcher icon. Show that metadata in both the current published release card and the release history table.

## Architecture

APK metadata extraction remains inside `apprelease.APKInspector`, alongside the existing manifest and certificate inspection. The inspector resolves the Chinese (`zh-CN`) application label first, then the default label, and finally the package name. It decodes the launcher icon and normalizes it to PNG bytes.

`apprelease.Service` persists text metadata with the release record and stores the normalized icon next to the APK in managed app-release storage. The database stores only the icon's managed relative path, not image bytes or a data URL.

The server exposes a dedicated icon response for a release. App icons are non-sensitive APK metadata, so the endpoint may be loaded directly by an `<img>` element without requiring the admin bearer-token request client. The endpoint never exposes a draft APK or arbitrary filesystem paths.

## Data Model and Compatibility

Add nullable/default-compatible columns to `app_releases`:

- `app_name`: extracted localized or fallback display name.
- `package_name`: manifest package identifier.
- `icon_path`: managed relative path for the normalized PNG icon.

The schema includes idempotent `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` statements so existing production databases migrate at startup. Existing rows remain valid with empty metadata and display a placeholder. Newly uploaded releases receive complete metadata when available.

## Extraction and Fallbacks

Required metadata remains package name, version name, and positive version code. Failure to read those values continues to reject the APK.

Optional presentation metadata is best effort:

1. Try the `zh-CN` application label.
2. Fall back to the APK's default label.
3. Fall back to the package name.
4. Try to decode the launcher icon and encode it as PNG.
5. If icon decoding fails, accept the APK and leave `icon_path` empty so the UI shows a placeholder.

Signature and expected-package validation remain unchanged.

## API and UI

Release JSON adds `appName`, `packageName`, and `iconUrl`. `iconUrl` is empty when no extracted icon exists.

The current published release card shows:

- launcher icon or placeholder;
- Chinese/default app name;
- package name;
- `versionName` and `versionCode`;
- APK file name and size.

The history table groups the icon, app name, and package name in an “应用” column while retaining the existing version, file, status, time, notes, and actions columns.

## Error Handling and Security

- Optional label/icon extraction errors do not fail a valid upload.
- Icon paths are created and resolved through managed release storage; request paths never select filesystem paths directly.
- The icon response uses a fixed image content type, nosniff headers, and cache validation.
- Missing icon files return `404` and the UI falls back to a placeholder.
- APK upload size, signature, package identity, and version validation are unchanged.

## Testing

- APK inspector tests cover localized/default label fallback and icon PNG generation.
- Service/store tests cover metadata persistence and cleanup when database creation fails.
- Schema tests cover backward-compatible columns.
- Server tests cover icon routing, content type, missing records, and path safety.
- Frontend API/component tests cover the new fields and both current/history presentation.
- Existing app-release upload, publish, archive, proxy, and download tests remain green.
