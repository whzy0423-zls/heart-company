# Miniapp Comprehensive Optimization Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge configurable homepage management and improve miniapp startup packaging, failure recovery, draft control, accessibility, and maintainability without changing the established visual style.

**Architecture:** Preserve current page contracts and introduce only focused boundaries: WeChat subpackages for non-tab pages, a dedicated result-poster utility, and explicit page-level state for cached learning content and restored booking drafts. Existing source-contract tests remain the UI regression layer, while new pure utility tests cover extracted behavior.

**Tech Stack:** Vue 3, uni-app, WeChat Mini Program, Node assertion tests, Vitest, Go.

---

## Chunk 1: Integration and Startup Package

### Task 1: Merge configurable homepage work and verify the combined baseline

**Files:**
- Merge branch: `feature/miniapp-home-menu-config`
- Verify: `miniapp/**`
- Verify: `nx-backend/apps/web-antd/src/views/miniapp/**`
- Verify: `nx-backend/apps/server/internal/{siteconfig,server}/**`

- [ ] **Step 1: Merge the homepage configuration branch**

Run from the optimization worktree root:

```bash
git merge --no-ff feature/miniapp-home-menu-config
```

Expected: merge commit with no unresolved conflicts. Preserve the optimization design document if the merge touches `docs/`.

- [ ] **Step 2: Run the combined focused suites**

```bash
cd miniapp && npm run test:config
cd ../nx-backend && pnpm exec vitest run apps/web-antd/src/views/miniapp/home.test.ts
cd apps/server && go test ./internal/siteconfig ./internal/db ./internal/server
```

Expected: PASS. The existing `MODULE_TYPELESS_PACKAGE_JSON` warning is recorded but does not fail the suite.

- [ ] **Step 3: Compile the merged WeChat output**

```bash
cd miniapp
VITE_API_BASE=http://127.0.0.1:5320/api npm run dev:mp-weixin
```

Expected: `DONE Build complete. Watching for changes...`; stop the watcher after verifying `dist/dev/mp-weixin/pages/index/index.wxml` contains the configurable entry loop.

### Task 2: Move non-tab pages into WeChat subpackages

**Files:**
- Modify: `miniapp/src/pages.json`
- Modify: `miniapp/scripts/project-config.test.mjs`
- Modify if required: `miniapp/scripts/ui-compat.test.mjs`

- [ ] **Step 1: Write failing package-layout assertions**

Require the main `pages` array to contain only:

```text
pages/index/index
pages/learn/learn
pages/booking/booking
pages/profile/profile
```

Require `subPackages` to register these existing URLs exactly once:

```text
pages/test/test
pages/result/result
pages/relation/relation
pages/profile-edit/profile-edit
pages/booking-records/booking-records
pages/booking-detail/booking-detail
```

Also assert every tabBar page remains in the main package and no page appears in both collections.

- [ ] **Step 2: Run the project-config test and verify RED**

```bash
cd miniapp
node scripts/project-config.test.mjs
```

Expected: FAIL because all pages are currently registered in the main package.

- [ ] **Step 3: Add minimal `subPackages` configuration**

Keep source files and route URLs unchanged. Group related detail pages under roots that preserve their full URL; do not move any tabBar page out of `pages`.

- [ ] **Step 4: Run focused and full tests**

```bash
node scripts/project-config.test.mjs
npm run test:config
```

Expected: PASS.

- [ ] **Step 5: Build and inspect generated package metadata**

```bash
VITE_API_BASE=http://127.0.0.1:5320/api npm run dev:mp-weixin
```

Expected: build succeeds; `dist/dev/mp-weixin/app.json` contains `subPackages`, main tabBar pages remain under `pages`, and all existing navigation URLs still resolve.

- [ ] **Step 6: Commit**

```bash
git add miniapp/src/pages.json miniapp/scripts/project-config.test.mjs miniapp/scripts/ui-compat.test.mjs
git commit -m "perf: split non-tab miniapp pages into subpackages"
```

## Chunk 2: Result and Learning Experience

### Task 3: Extract and harden result poster generation

**Files:**
- Create: `miniapp/src/utils/resultPoster.js`
- Create: `miniapp/src/utils/resultPoster.test.mjs`
- Modify: `miniapp/src/pages/result/result.vue`
- Modify: `miniapp/scripts/ui-compat.test.mjs`
- Modify: `miniapp/package.json`

- [ ] **Step 1: Write failing poster utility tests**

Cover:

- missing canvas node rejects with a stable error;
- avatar load failure rejects;
- successful drawing calls the runtime temp-file exporter and resolves its path;
- text wrapping never draws empty lines and respects maximum width;
- the utility does not call `uni.showToast` or mutate page refs.

- [ ] **Step 2: Write failing page-contract tests**

Require the result page to:

- import the poster utility rather than contain canvas drawing implementation;
- expose `posterError` and retry generation;
- render the modal with dialog semantics and `aria-modal="true"`;
- use `aria-live="polite"` for generating/error status;
- render the save action only when `posterUrl` exists and generation is complete.

- [ ] **Step 3: Run focused tests and verify RED**

```bash
node src/utils/resultPoster.test.mjs
node scripts/ui-compat.test.mjs
```

Expected: FAIL because the utility and explicit error state do not exist.

- [ ] **Step 4: Extract the poster utility**

Move canvas selection, DPR sizing, drawing, avatar loading, wrapping, and temp-file export behind one function such as:

```js
export async function createResultPoster({ instance, result, info, persona, runtime = uni })
```

Keep current colors, dimensions and copy. Reject errors to the caller.

- [ ] **Step 5: Implement resilient modal states**

`makePoster()` clears old error, opens the dialog and awaits the utility. Failure leaves the dialog open with retry controls. Closing the dialog must not leave a clickable save action without a generated path.

- [ ] **Step 6: Run focused and full tests**

```bash
node src/utils/resultPoster.test.mjs
node scripts/ui-compat.test.mjs
npm run test:config
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add miniapp/src/utils/resultPoster.js miniapp/src/utils/resultPoster.test.mjs miniapp/src/pages/result/result.vue miniapp/scripts/ui-compat.test.mjs miniapp/package.json
git commit -m "feat: make result poster generation recoverable"
```

### Task 4: Show non-blocking learning refresh status

**Files:**
- Modify: `miniapp/src/pages/learn/learn.vue`
- Create: `miniapp/src/pages/learn/learn.content-state.test.mjs`
- Modify: `miniapp/package.json`

- [ ] **Step 1: Write failing source and state tests**

Require cached content to remain visible when a silent refresh fails. Require a non-blocking refresh notice with retry, `aria-live="polite"`, and a disabled retry control while the request is active. Require successful refresh to clear the notice.

- [ ] **Step 2: Run the focused test and verify RED**

```bash
node src/pages/learn/learn.content-state.test.mjs
```

Expected: FAIL because silent refresh failures currently have no visible state.

- [ ] **Step 3: Implement minimal refresh-notice state**

Keep the existing blocking `loadError` path for first load without cache. Add a separate cached-refresh notice used only when content is already visible. Do not clear teachers, courseware or quotes on silent failure.

- [ ] **Step 4: Run focused and full tests**

```bash
node src/pages/learn/learn.content-state.test.mjs
npm run test:config
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add miniapp/src/pages/learn/learn.vue miniapp/src/pages/learn/learn.content-state.test.mjs miniapp/package.json
git commit -m "feat: surface cached learning refresh status"
```

## Chunk 3: Booking Draft Control and Final Audit

### Task 5: Make restored booking drafts explicit and clearable

**Files:**
- Modify: `miniapp/src/utils/bookingDraft.js`
- Modify: `miniapp/src/utils/bookingDraft.test.mjs`
- Modify: `miniapp/src/pages/booking/booking.vue`
- Modify: `miniapp/scripts/ui-compat.test.mjs`

- [ ] **Step 1: Write failing meaningful-draft tests**

Add a stored empty/default draft and require `loadBookingDraft()` to return `null`. Keep non-default kind or any non-whitespace field as meaningful.

- [ ] **Step 2: Write failing booking-page contract tests**

Require a restored-draft notice, a named clear action, and a clear handler that cancels pending writes before resetting type/form/errors and clearing storage.

- [ ] **Step 3: Run tests and verify RED**

```bash
node src/utils/bookingDraft.test.mjs
node scripts/ui-compat.test.mjs
```

Expected: FAIL because empty stored drafts are returned and the page has no explicit restore controls.

- [ ] **Step 4: Implement meaningful restore and clear flow**

Reuse `hasMeaningfulDraft()` in `loadBookingDraft()`. Track whether a draft was restored. The clear action must:

```text
cancel timer → clear storage → reset kind → reset form → reset errors → hide notice
```

Ensure the resulting empty-form watcher clears rather than recreates a draft.

- [ ] **Step 5: Run focused and full tests**

```bash
node src/utils/bookingDraft.test.mjs
node scripts/ui-compat.test.mjs
npm run test:config
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add miniapp/src/utils/bookingDraft.js miniapp/src/utils/bookingDraft.test.mjs miniapp/src/pages/booking/booking.vue miniapp/scripts/ui-compat.test.mjs
git commit -m "feat: make booking draft recovery controllable"
```

### Task 6: Complete cross-system verification

**Files:**
- Modify if required: `docs/superpowers/specs/2026-07-26-miniapp-comprehensive-optimization-design.md`

- [ ] **Step 1: Run miniapp regression suite**

```bash
cd miniapp
npm run test:config
```

Expected: PASS.

- [ ] **Step 2: Run homepage admin and backend suites**

```bash
cd nx-backend
pnpm exec vitest run apps/web-antd/src/views/miniapp/home.test.ts
pnpm exec oxfmt --check apps/web-antd/src/views/miniapp/home.vue apps/web-antd/src/views/miniapp/home.test.ts
cd apps/server
go test ./internal/siteconfig ./internal/db ./internal/server
```

Expected: PASS.

- [ ] **Step 3: Compile the final WeChat output**

```bash
cd miniapp
VITE_API_BASE=http://127.0.0.1:5320/api npm run dev:mp-weixin
```

Expected: `DONE Build complete`; inspect `app.json` for subpackages and `pages/index/index.wxml` for dynamic home entries.

- [ ] **Step 4: Run repository hygiene checks**

```bash
git diff --check
! rg -n '^(<<<<<<<|=======|>>>>>>>)' miniapp nx-backend docs
git status --short
```

Expected: no conflict markers or uncommitted implementation files.

- [ ] **Step 5: Commit verification documentation if changed**

```bash
git add docs/superpowers/specs/2026-07-26-miniapp-comprehensive-optimization-design.md
git commit -m "docs: record miniapp optimization verification"
```
