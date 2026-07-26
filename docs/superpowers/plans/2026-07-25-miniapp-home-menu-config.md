# Miniapp Home Module Configuration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow administrators to configure all purple miniapp home modules while preserving fixed navigation targets, module order, and curated icon/theme choices.

**Architecture:** Store normalized content under `home.miniappHome` in the existing site-config document and keep `home.miniappCarousel` unchanged. A focused miniapp normalizer supplies backward-compatible defaults, while the admin page edits the same contract through collapsible sections and fixed entry keys.

**Tech Stack:** Vue 3, uni-app, WeChat Mini Program, Ant Design Vue, Vitest, Node assertion tests, Go site-config API.

---

## Chunk 1: Configuration Contracts

### Task 1: Define admin configuration types

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/site-config.ts`
- Test: `nx-backend/apps/web-antd/src/views/miniapp/home.test.ts`

- [ ] **Step 1: Write a failing source-contract test**

Add assertions that `MiniappHomeConfig`, `MiniappHomeEntry`, icon keys, theme keys, and the optional `home.miniappHome` field are available to the admin page.

- [ ] **Step 2: Run the focused test and verify failure**

Run: `pnpm exec vitest run apps/web-antd/src/views/miniapp/home.test.ts`

Expected: FAIL because the new home configuration types do not exist.

- [ ] **Step 3: Add the minimal typed contract**

Define literal unions for entry keys, icon keys, and theme keys. Define brand, hero, entries section, entry item, and growth interfaces. Add `miniappHome?: MiniappHomeConfig` beside `miniappCarousel`.

- [ ] **Step 4: Run the focused test**

Run: `pnpm exec vitest run apps/web-antd/src/views/miniapp/home.test.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/api/core/site-config.ts nx-backend/apps/web-antd/src/views/miniapp/home.test.ts
git commit -m "feat: define miniapp home configuration contract"
```

### Task 2: Add miniapp configuration normalization

**Files:**
- Create: `miniapp/src/utils/homeMenu.js`
- Create: `miniapp/src/utils/homeMenu.test.mjs`
- Modify: `miniapp/package.json`

- [ ] **Step 1: Write failing default and malformed-config tests**

Cover current purple copy defaults, missing sections, empty strings, invalid booleans, duplicate/unknown entry keys, missing fixed entries, invalid icon/theme keys, configured order, and all entries disabled.

- [ ] **Step 2: Run the new test and verify failure**

Run: `node src/utils/homeMenu.test.mjs`

Expected: FAIL because `homeMenu.js` does not exist.

- [ ] **Step 3: Implement immutable normalization**

Export constants for fixed entry behavior metadata and allowed icons/themes. Export `normalizeMiniappHome(config)` returning complete brand, hero, entries section, and growth values without mutating the API response.

- [ ] **Step 4: Run focused and full miniapp tests**

Run: `node src/utils/homeMenu.test.mjs && npm run test:config`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add miniapp/src/utils/homeMenu.js miniapp/src/utils/homeMenu.test.mjs miniapp/package.json
git commit -m "feat: normalize configurable miniapp home modules"
```

## Chunk 2: Admin Editor

### Task 3: Extend admin normalization without losing existing home data

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/miniapp/home.vue`
- Modify: `nx-backend/apps/web-antd/src/views/miniapp/home.test.ts`

- [ ] **Step 1: Write failing normalization tests**

Assert that missing `miniappHome` receives purple defaults; malformed sections recover independently; all four fixed entry keys are present once; existing entry order is preserved; unknown entries are dropped; and unrelated `home` fields plus `miniappCarousel` retain object identity.

- [ ] **Step 2: Run the focused test and verify failure**

Run: `pnpm exec vitest run apps/web-antd/src/views/miniapp/home.test.ts`

Expected: FAIL because only carousel normalization exists.

- [ ] **Step 3: Implement `ensureMiniappHome`**

Add exported normalization helpers with fixed defaults and enum allowlists. Keep `ensureCarousel` intact and invoke both normalizers from the existing config watcher.

- [ ] **Step 4: Run the focused test**

Run: `pnpm exec vitest run apps/web-antd/src/views/miniapp/home.test.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/miniapp/home.vue nx-backend/apps/web-antd/src/views/miniapp/home.test.ts
git commit -m "feat: initialize configurable miniapp home modules"
```

### Task 4: Build the single-page collapsible editor

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/miniapp/home.vue`
- Modify: `nx-backend/apps/web-antd/src/views/miniapp/home.test.ts`

- [ ] **Step 1: Write failing interaction tests**

Mount the page and assert sections appear in fixed order: carousel, brand, hero, entries, growth. Test copy editing, section enable switches, preset icon/theme selects, fixed destination labels, entry up/down ordering, entry visibility, and one save call preserving unrelated config.

- [ ] **Step 2: Run the focused test and verify failure**

Run: `pnpm exec vitest run apps/web-antd/src/views/miniapp/home.test.ts`

Expected: FAIL because the editor only renders carousel controls.

- [ ] **Step 3: Implement collapse panels and controls**

Use Ant Design Vue `Collapse`, `Form`, `Input`, `Select`, `Switch`, `Card`, and `Button`. Use curated arrays for icon/theme options and display fixed route descriptions as read-only text.

- [ ] **Step 4: Add responsive editor styling**

Keep controls usable at narrow widths, make entry action buttons wrap, and avoid introducing a second save mechanism.

- [ ] **Step 5: Run admin tests**

Run: `pnpm exec vitest run apps/web-antd/src/views/miniapp/home.test.ts`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/miniapp/home.vue nx-backend/apps/web-antd/src/views/miniapp/home.test.ts
git commit -m "feat: edit miniapp home modules in admin"
```

## Chunk 3: Purple Home Rendering

### Task 5: Render configured section copy and visibility

**Files:**
- Modify: `miniapp/src/pages/index/index.vue`
- Modify: `miniapp/scripts/ui-compat.test.mjs`

- [ ] **Step 1: Write failing page-contract tests**

Require the page to normalize `miniappHome` alongside carousel data, bind brand/hero/entries/growth text, conditionally render each section, and retain current purple defaults when configuration is missing.

- [ ] **Step 2: Run the UI test and verify failure**

Run: `node scripts/ui-compat.test.mjs`

Expected: FAIL because homepage copy and section visibility are hard-coded.

- [ ] **Step 3: Integrate normalized home state**

Create one reactive normalized home value. Apply cached and refreshed site config to both carousel and home modules. Replace hard-coded copy with bindings and use `v-if` for section visibility without changing fixed module order.

- [ ] **Step 4: Run focused tests**

Run: `node src/utils/homeMenu.test.mjs && node scripts/ui-compat.test.mjs`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add miniapp/src/pages/index/index.vue miniapp/scripts/ui-compat.test.mjs
git commit -m "feat: render configurable miniapp home sections"
```

### Task 6: Render ordered entries with fixed navigation

**Files:**
- Modify: `miniapp/src/pages/index/index.vue`
- Modify: `miniapp/src/utils/homeMenu.js`
- Modify: `miniapp/src/utils/homeMenu.test.mjs`
- Modify: `miniapp/scripts/ui-compat.test.mjs`

- [ ] **Step 1: Write failing entry behavior tests**

Require a `v-for` over normalized enabled entries, stable keys, theme/icon modifier classes, exact fixed navigation mapping for all four keys, and no configurable URL fields.

- [ ] **Step 2: Run tests and verify failure**

Run: `node src/utils/homeMenu.test.mjs && node scripts/ui-compat.test.mjs`

Expected: FAIL because four cards and handlers are hard-coded.

- [ ] **Step 3: Implement fixed entry activation**

Add a single `activateHomeEntry(key)` dispatcher using the design mapping. Render ordered entries with curated class names and retain keyboard, pressed, and accessible-name behavior.

- [ ] **Step 4: Add preset icon and theme styles**

Map the six icon keys to CSS-only shapes and five theme keys to curated gradients/tints. Unknown values must already be normalized before rendering.

- [ ] **Step 5: Run full miniapp tests**

Run: `npm run test:config`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add miniapp/src/pages/index/index.vue miniapp/src/utils/homeMenu.js miniapp/src/utils/homeMenu.test.mjs miniapp/scripts/ui-compat.test.mjs
git commit -m "feat: configure ordered miniapp home entries"
```

## Chunk 4: End-to-End Verification

### Task 7: Verify API preservation and complete regression suite

**Files:**
- Modify if required: `nx-backend/apps/server/internal/server/server_unit_test.go`
- Modify if required: `docs/superpowers/specs/2026-07-25-miniapp-home-menu-config-design.md`

- [ ] **Step 1: Add a server regression test if the existing generic JSON coverage is insufficient**

Ensure a `home.miniappHome` payload survives authenticated update and public read while OSS image URLs and `miniappCarousel` remain unchanged.

- [ ] **Step 2: Run backend and admin suites**

Run: `go test ./internal/db ./internal/server` from `nx-backend/apps/server`.

Run: `pnpm exec vitest run apps/web-antd/src/views/miniapp/home.test.ts` from `nx-backend`.

Expected: PASS.

- [ ] **Step 3: Run full miniapp suite**

Run: `npm run test:config` from `miniapp`.

Expected: PASS.

- [ ] **Step 4: Run formatting and conflict checks**

Run: `git diff --check && ! rg -n '^(<<<<<<<|=======|>>>>>>>)' miniapp nx-backend`

Expected: zero output and exit status 0.

- [ ] **Step 5: Compile the WeChat development output**

Run: `VITE_API_BASE=http://127.0.0.1:5320/api npm run dev:mp-weixin`

Expected: `DONE Build complete. Watching for changes...`; inspect `dist/dev/mp-weixin/pages/index/index.wxml` for the configurable carousel and entry loop.

- [ ] **Step 6: Commit final verification changes**

```bash
git add nx-backend/apps/server/internal/server/server_unit_test.go docs/superpowers/specs/2026-07-25-miniapp-home-menu-config-design.md
git commit -m "test: verify configurable miniapp home flow"
```
