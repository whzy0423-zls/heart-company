# Native miniapp share cover fix — 2026-10-02

The native friend/Moments preview displayed a broken image even though the home page and `wx.getImageInfo` could read the bundled JPEG. Reproducing with an equivalent page-relative package path still failed; the same native friend dialog displayed an existing public HTTPS photo correctly. The failure is in the share dialog's package-resource loading context, not the JPEG bytes or share title. The previous asset-decode check did not verify this context.

`utils/share.js` now resolves supported bundled share covers through a generated public asset manifest. Both native share callbacks receive stable HTTPS URLs. Known teacher portraits use the composed teacher/brand card; course, video and nine-type covers keep their corresponding images. Unknown local assets, unsupported formats and signed URLs use the published fallback. Public remote PNG/JPG metadata remains supported. Page rendering and full-size image previews keep their original sources.

`scripts/publish-share-assets.mjs` copies selected existing public images to `website-react/public/assets/miniapp-share/` and generates `src/data/shareAssets.js`. Content hashes in filenames prevent stale-cache collisions; previous files are retained for old share cards. `--check` validates the source bytes, image signatures, immutable output and deterministic manifest without writing files. Both production prebuild hooks and the configuration/sharing suites run this check. Publish new cover assets before distributing the client that references them.

## Verification

- Added a regression that initially failed on `/static/share/studio.jpg` where a public HTTPS share URL was required. It now passes for both channels, all nine types/default, course/video covers and unpublished local-image fallback.
- Full `npm run test:config`, `npm run build:mp-weixin`, and `node scripts/share-policy.test.mjs --compiled` passed. The 19 page share/privacy policies remain in place.
- All 37 deployed images passed anonymous HTTPS GET, MIME, complete byte and SHA-256 comparisons (1,198,627 bytes total).
- Reopened the final compiled project in Developer Tools Nightly 2.02.2606032 / WeChatLib 3.17.2. The original friend-share dialog now visibly renders the composed teacher photo and nine-type logo, using `studio-36c3f8a46b86.jpg`.
- Opened the native right-top Moments menu and its “分享到朋友圈预览” dialog. The thumbnail visibly renders the same teacher/brand cover beside the correct title.
- No share message was sent and no WeChat public release was published. Device-to-device receipt remains part of normal release acceptance.

## Static asset deployment

The live website received only the 37 new public cover files. Its 599 existing files were compared with the prior image and preserved. The resulting 636-file public tree matches a derivative image built from the exact live website image, with only the cover directory appended. The website container and API were not restarted; the website root and `/api/app/health` both returned HTTP 200 afterward.

Persistent image: `heart-company-website:miniapp-share-af86bc7f6446`.

Image ID: `sha256:c52f9f592e7bdfbe29ad111481c4a2ef260ff8ab66c3e791345b23fcad9d474b`.

Both currently used Compose overlay paths select this image for future website recreations; effective configuration comparisons exclude only `website.image` and verify all other service settings are unchanged. The live container still reports the prior image ID because files were added without recreation; its public contents match the new image.

Deployment evidence, previous overlays, file hashes and pre-release rollback materials: `/opt/heart-company/.deploy/miniapp-share-20261002-af86bc7f6446/`. Preserve published hash-named covers when rolling back a distributed client, so previously sent cards keep working. No database changes were made.

Default public cover: https://xn--9iq9az5uo8fz16d.com/assets/miniapp-share/studio-36c3f8a46b86.jpg
