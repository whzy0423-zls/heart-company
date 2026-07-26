# Free Video Workflow Verification

Date: 2026-07-11

Branch: `feature/guided-video-workbench`

## Closure Result

The guided workflow now defaults to `demo`, renders valid local MP4 files with FFmpeg, requires explicit version selection, composes the selected local files, and exposes the final video through `/api/uploads/...`. No paid video-provider request was made.

Real HTTP closure result:

```text
zero-charge closure: project=1 shot=1 generation=1 job=1 provider_calls=0
```

The integration test used a uniquely named PostgreSQL database owned by the existing `nx` role, a temporary upload directory, a real `server.New` HTTP server, a tripwire `VIDEO_API_BASE`, the local object uploader, FFmpeg, and ffprobe. The database was dropped automatically after the test.

## Evidence Matrix

| Requirement | Evidence | Result |
| --- | --- | --- |
| Fail-closed default | Config matrix covers empty, malformed, incomplete paid, and acknowledged paid settings | PASS |
| No provider create call | Tripwire API base counter after real create/import/generate/select/compose/retrieve flow | `provider_calls=0` |
| Valid generated MP4 | Generated `/api/uploads/video/generated/...` response downloaded through HTTP and inspected with ffprobe | PASS |
| Explicit selection | Real `POST /api/video/shots-video-versions/set/:shot/:generation` response matched the generated version | PASS |
| Valid final MP4 | Final `/api/uploads/video/composed/...` response downloaded through HTTP and inspected with ffprobe | PASS |
| Browser safety signal | Playwright required the exact free-mode banner and rejected `/v1/videos` requests | 12/12 PASS |
| Backend regression | `go test ./... -count=1` | PASS |
| Frontend regression | Full Vitest source tree | 49 files, 223 tests PASS |
| Production validity | Vue typecheck and production build | PASS |

## Verification Commands

```text
cd nx-backend/apps/server
go test ./... -count=1
Result: exit 0; all Go packages passed. The database-backed closure test skips unless TEST_DATABASE_URL is set.

cd nx-backend
pnpm exec vitest run apps/web-antd/src
Result: exit 0; 49 files and 223 tests passed.

pnpm --filter @vben/web-antd run typecheck
Result: exit 0.

pnpm --filter @vben/web-antd run build
Result: exit 0; 8050 modules transformed and dist.zip created.

pnpm exec playwright test guided-video-workflow.spec.ts \
  --config apps/web-antd/playwright.guided.config.ts \
  --project=chromium --reporter=line
Result: exit 0; 12 tests passed in 51.1 seconds.
```

The real database-backed closure was run with a temporary database URL:

```text
TEST_DATABASE_URL=postgres://nx:***@localhost:5432/<temporary-db>?sslmode=disable \
  go test ./internal/server -run TestFreeVideoWorkflowRealHTTPClosure -count=1 -v
Result: PASS in 0.62s; provider_calls=0.
```

## Environment Notes

- FFmpeg and ffprobe were available from Homebrew and accepted both the shot MP4 and composed MP4.
- The installed Node version was `25.9.0`; the repository declares Node `^22.18.0 || ^24.0.0`. Typecheck, Vitest, Playwright, and build still passed.
- The production build emitted only the existing third-party `@vueuse/core` pure-annotation warnings.
