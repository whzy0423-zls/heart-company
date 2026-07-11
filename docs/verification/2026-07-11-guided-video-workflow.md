# Guided Video Workflow Verification

Date: 2026-07-11

Branch: `feature/guided-video-workbench`

Implementation range: `69d2ca9..9216539`

## Evidence Matrix

| Requirement | Authoritative evidence | Command/assertion | Exit/result | Artifact |
| --- | --- | --- | --- | --- |
| Real five-step panels and default route | Vue source contracts and browser navigation | Vitest `workflow.source.test.ts`; Playwright complete five-step scenario | PASS | `apps/web-antd/src/views/video/projects/workflow.vue` |
| One bottom primary action per step | Source contract plus rendered `data-primary-action` | Vitest source contract; responsive Playwright checks | PASS | Desktop/tablet/mobile screenshots |
| Novice defaults with advanced route preserved | Router and production entry tests | Full frontend Vitest suite | 49 files, 223 tests passed | `apps/web-antd/src/router/routes/modules/video.ts` |
| Exact ready/stale/failed generation scope | Server readiness tests and captured batch shot IDs | Go full suite; Playwright asserts exact `['1','2']` eligible scope | PASS | Complete five-step scenario |
| Paid submission idempotency and ambiguous outcome safety | Go state-machine/gateway tests; intercepted browser fixture | `go test ./... -count=1`; Playwright asserts no real gateway and one mocked paid submission | PASS | Recovery screenshot |
| Restart recovery for accepted/reconciled submissions | Safe `activeSubmission` workflow metadata and resumed polling | Playwright accepted/reconciled scenarios | PASS | Browser trace available on failure only |
| Unknown outcome is reconcile-only | Recovery UI restores request key/task ID and never repeats generation POST | Playwright unknown outcome scenario | PASS | `artifacts/guided-workflow-recovery.png` |
| Legacy project and partial import recovery | Recommended legacy step, created/existing/failed result and retry | Playwright legacy/partial-import scenario | PASS | Repeatable fixture in E2E spec |
| Explicit version selection | Generated versions do not mutate selection; drawer selection is required | Playwright complete scenario and Go selection tests | PASS | Complete five-step scenario |
| Drawer and dirty-dialog focus | Drawer returns focus to the shot trigger; dirty dialog owns focus and Escape cancels | Playwright complete and dirty-navigation scenarios | PASS | Repeatable focus assertions |
| Poll timeout and manual refresh | Thirty poll attempts transition to a visible manual-refresh control | Playwright timeout scenario with controlled clock | PASS | Repeatable fixture in E2E spec |
| Compose failure/retry and stale final result | Async job failure preserves settings; retry completes; old result remains previewable | Playwright compose failure and stale-final scenarios | PASS | Repeatable fixture in E2E spec |
| Current compose input hash | Server compose snapshot/hash tests and current result fixture | Go full suite; Playwright waits for `当前成片` | PASS | Complete five-step scenario |
| Responsive desktop/tablet/mobile layouts | No horizontal page overflow, 44px CTA, sticky footer space, mobile shot select, one `aria-current`, reduced motion | Playwright 1440x900, 768x1024, 375x812 scenarios | PASS | Three responsive screenshots |
| Production type and bundle validity | Vue TypeScript and Vite production build | `pnpm --filter @vben/web-antd run typecheck`; `pnpm --filter @vben/web-antd run build` | Exit 0; only third-party `@vueuse/core` pure annotation warnings | `apps/web-antd/dist.zip` (ignored build output) |

## Verification Commands

```text
cd nx-backend/apps/server
go test ./... -count=1
Result: exit 0; all Go packages passed.

cd nx-backend
pnpm exec vitest run apps/web-antd/src
Result: exit 0; 49 files and 223 tests passed.

pnpm exec playwright test guided-video-workflow.spec.ts \
  --config apps/web-antd/playwright.guided.config.ts \
  --project=chromium --reporter=line
Result: exit 0; 12 tests passed in 52.6 seconds.

pnpm --filter @vben/web-antd run typecheck
Result: exit 0.

pnpm --filter @vben/web-antd run build
Result: exit 0; 8050 modules transformed and production ZIP created.
```

## Browser Evidence

- Desktop 1440x900: `/Users/wohenzaiyi/Desktop/nine-xing/artifacts/guided-workflow-desktop.png`
- Tablet 768x1024: `/Users/wohenzaiyi/Desktop/nine-xing/artifacts/guided-workflow-tablet.png`
- Mobile 375x812: `/Users/wohenzaiyi/Desktop/nine-xing/artifacts/guided-workflow-mobile.png`
- Recovery state: `/Users/wohenzaiyi/Desktop/nine-xing/artifacts/guided-workflow-recovery.png`
- Chromium assertions collect page errors and failed requests. The complete scenario finished with both arrays empty and with no request to a real paid gateway.
- Visual review of the refreshed desktop and mobile screenshots found no horizontal clipping or footer overlap after the footer changed to sticky document layout.

## In-App Browser Limitation

The required Browser skill was loaded and the app was started at `http://127.0.0.1:4317`. Browser runtime discovery returned `No browser is available`; the prescribed troubleshooting call then returned an empty browser list (`[]`). No in-app Browser URL, viewport, console, or focus result is claimed. The managed Vite process used for this attempt was stopped. Repeatable Chromium Playwright coverage remains the browser authority for this environment.

## Repository Audit

```text
git diff --stat 69d2ca9..9216539
Result: 46 files changed, 7088 insertions, 433 deletions.

git diff --check 69d2ca9..9216539
Result: exit 0, no whitespace errors.

git diff --cached --name-only | rg '^artifacts/'
Result: no matches; screenshots remain outside the feature worktree and are not committed.
```
