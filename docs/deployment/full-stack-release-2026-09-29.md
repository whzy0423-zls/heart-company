# Full-Stack Production Release, September 29, 2026

## Source And Deployment

- App main/build revision: `6c09646e08240cbe339953e497dad6c1e23ec998`.
- Backend initial static release: `0e1cd1260b1f3eb8d4e61b0b5988e879d2774c71`.
- Backend server/knowledge runtime revision: `e6b78b1fc1e78248e9074a3019ad9b4088fc564e`.
- Both repositories were pushed normally to main; no history rewrite.
- App version: `1.1.26+192`, officially published as release 38.

Production uses immutable artifacts from tested local source. Its existing
dirty source checkout, production overlays, volumes, environment, certificates,
website verification files and unrelated services were retained. Release Compose
overrides are under `/opt/heart-company/.deploy/full-stack-20260929` and layer
over existing base/operational/import configuration. Only affected services were
recreated. The gateway retrieval allowance is 15,000 ms; it is not a latency SLA.

| Service | Tag | Image ID |
| --- | --- | --- |
| server | full-stack-20260929-e6b78b1 | `59c78338ab1f1283f3e4e9460d99ac2d0cc95bc8261f3deac9412aadf3e61f50` |
| knowledge-service | full-stack-20260929-e6b78b1 | `52d9306845a5c0d114bf81e0dfb25888dd90ab1f349d2be53dc0eeab930c9792` |
| admin | full-stack-20260929-0e1cd12 | `897ce4558fc2006c0f11f945df461495cf337fa076bfef5d050021d23b0dbe94` |
| website | full-stack-20260929-0e1cd12 | `309fb41eb26588c116514e399a35b85dc4c6c2b9869d2502e45014f6dca4f0b9` |
| reading | full-stack-20260929-0e1cd12 | `007e0eb8f0189dd2653e7636fecd24cb67d80b8ba3db5b9c5e405debfc099ee0` |

Image names use the `heart-company-` prefix. Server binary SHA-256:
`365dcffe1d355a13f42f29e8d53ed933d1a3a87fd11f541c5e7823598318b5c0`.
The three modified Python source hashes match local tested files and image files.
Budget artifact archive SHA-256:
`298021bd727f84c7ced01d760815660755e9b01ae1120cb7e5df6c4f1d96b9b0`.

## Verification

- Flutter 3.41.7: 2,553 full tests including 15 goldens; 2,538 CI non-golden tests.
- App format: 637 files, no change. Analysis: zero issues. Release contracts: 33 passed.
- Go final full PostgreSQL/race: 5,490 passed, zero failures/data races; 74 packages passed.
- Go vet, native full build and Linux amd64 build passed.
- Python final isolated PostgreSQL suite: 76 passed.
- Admin/workspace: 804; canvas: 95; website: 80; motion website: 65;
  reading: 8; optimized assets: 1; miniapp: 31 sequential script groups.
- Frontend typechecks and production builds passed; deployed reading base is `/read/`.
- Production App health, knowledge readiness, authenticated admin/menu/catalog/body preview passed.
- Real browser admin login, book-directory search/body preview and website desktop/mobile passed;
  zero page errors or mobile horizontal overflow. Browser screenshots are in local staging.

Post-budget deployment retrieval: exact App-shaped top-8 request took 3.742 s,
returned eight results/four managed documents. Read-only Go-equivalent fallback
SQL took 0.342 s/six rows. Hybrid top-30 `创伤后成长` took 2.17 s, returned
five managed documents, all enabled. Enabled/disabled/private scope checks passed.
The exact-vector plan uses the valid non-null partial index without changing
deterministic ranking. SQL/HTTP budgets and cold-query findings are documented
in `sqlite-public-knowledge-query-budget.md`; timings are smoke measurements.

Corpus reverified: 3,645,921 managed documents, zero missing search caches,
4,756 sources/30 primary categories, 455 completed batches, 711 enabled sources.
Every source activation equals the pre-deploy backup. Legacy counts remain
public 83,072, Enneagram 186, skill 1,626. All document indexes are ready/valid,
the temporary index is absent, table/TOAST options are inherited defaults.
Global max_wal_size=1024 MB and checkpoint_timeout=300 s, with durable fsync and
full_page_writes enabled. No bulk document embedding calls were made.

## APK Publication

Protected self-hosted workflow run: `36488082941`, pinned to the App revision
above. Full quality gates execute before production signing. Initial HTTPS Git
transport stalled; SSH reaches the same repository/ref, so only the temporary
checkout's local transport configuration was adjusted. Source/SHA checks remain.

The protected workflow completed successfully; its fresh quality gates passed
2,553 full and 2,538 non-golden tests. GitHub draft archive ID `398659089`
contains `app-release.apk`; the uploaded asset digest matches the signed local
artifact, server staging file, published metadata and full website download.

Official release 38 was published at `2026-09-28T22:09:12.574639Z` (UTC):
package `com.xinzhili.nine_xing_app`, version `1.1.26+192`, 208,245,675 bytes.
SHA-256: `2085e16c1fe6a49c400e0fc327c1567aa4a327797cfe1acdf8d3995b25fbe81e`.
The APK signature matches the existing production certificate. Package/version,
ZIP integrity, production API origin, permissions, exported components, backup
policy and billing links were verified. Non-forced policy remains minimum code
0 and rollout 100%.

Official website: `https://xn--9iq9az5uo8fz16d.com/app`. Public latest and the
legacy redirect point to `/api/public/app-releases/38/download`. Desktop/mobile
browser tests assert version 1.1.26 and release 38 download links, with no page
errors or horizontal overflow. A 32-byte Range returned HTTP 206 and correct
Content-Range/ETag. The full public download required resume after one network
interruption; its final bytes are identical to the original signed artifact,
with independently verified signature and SHA-256. This is not a guarantee
of uninterrupted slow-network transfer.

The previous official release is ID 37, `1.1.25+187`, 208,212,551 bytes,
SHA-256 `95bb9d72f3563e7ab631afe1b6ee3b6571c8324d8d560b4c510e5e53de06a298`.
Its freshly downloaded bytes match. Production signing certificate SHA-256:
`5ce7f31b79c719533d7db8d34bc630e23ffcbe9670e0f5a0cae95c93f091f505`.
The existing publish transaction archives the previous release. Consequently,
its public download endpoint returns HTTP 410, not HTTP 200 after publication.
Authenticated release listing confirms old record 37 remains archived with
`fileAvailable=true` and unchanged metadata/hash, and new record 38 is published
with its file available. Preserve this existing behavior; backend re-publication
of an archived version is the rollback path, not simultaneous public releases.

Local signed artifact and reports:
`/Users/wohenzaiyi/Downloads/nine-xing-releases/1.1.26+192/`.

## Gaps And Recovery

One Go live-provider performance baseline was skipped due missing baseline flags.
The admin configured E2E directory is missing; actual production browser smoke
was run separately. No Android device/system image is available, so installation
and launch smoke are not executed. GitHub-hosted run `36482917596` executed no
steps because of account payment/spending restrictions, not a failed code test.

Rollback branches: `codex/pre-release-20260929` in both repositories.
Previous production images: `pre-release-20260929`; rollback Compose retained.
Root-private backups include validated application/schema/catalog dump (65 MB),
live legacy archive (454 MB) and source/config overlay archive (2 MB), plus the
validated 518 MB pre-import dump. All 455 wire archive hashes match manifest
`55830d54028a4a066cd32443254e78a0516c907322fed73afe56cdff1edd4285`.
Use code-only rollback or selective tables/batch replay. Do not restore a whole
database over later user activity. Keep the valid non-null vector index.
The ephemeral runner exited successfully and removed its credentials and
registration; the GitHub runner list is empty. Signing files, persisted checkout
HTTP authorization and runner temporary files are absent. Temporary SSH master
and admin tunnel sessions were closed; port 18089 no longer listens. Existing
local development fixtures and unrelated services were not removed.

The final repository commit adds documentation only; deployed runtime revision
remains `e6b78b1fc1e78248e9074a3019ad9b4088fc564e`.
