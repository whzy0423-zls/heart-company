# Mini Program Home Carousel Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin-managed image carousel at the top of the mini-program home page.

**Architecture:** Store carousel data inside `home.miniappCarousel` in the existing site-config document so the existing upload persistence and public asset rewriting remain reusable. Add a dedicated admin menu/page for editing only this field, then normalize and render the public data in the mini program with a native `swiper`.

**Tech Stack:** Go HTTP server and seed database, Vue 3 + Ant Design Vue admin, uni-app Vue 3 mini program, Node/Vitest/Go tests.

---

## Chunk 1: Admin menu and carousel editor

### Task 1: Seed the new menu tree

**Files:**
- Modify: `nx-backend/apps/server/internal/db/db.go`
- Modify: `nx-backend/apps/server/internal/db/menu_test.go`

- [x] Add a failing test asserting a top-level `小程序管理` catalog and a child `首页管理` menu with component `/miniapp/home`, icon metadata, stable IDs, and `Website:Write` permission.
- [x] Run `go test ./internal/db` from `nx-backend/apps/server` and confirm the new test fails.
- [x] Add the two menu seeds without changing existing website menu IDs.
- [x] Run the database package tests and confirm they pass.

### Task 2: Build the admin carousel editor

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/miniapp/home.vue`
- Create: `nx-backend/apps/web-antd/src/views/miniapp/home.test.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/site-config.ts`

- [x] Add a failing source/component test covering configuration initialization, image upload component use, add/delete, enable switch, move up/down, and save wiring.
- [x] Run the focused Vitest test and confirm it fails because the page is missing.
- [x] Add typed carousel interfaces to `SiteConfig` and implement the editor using `ImagePathInput`, `Switch`, and ordered cards.
- [x] Run the focused test and admin typecheck/test commands.

### Task 3: Prove carousel uploads are publicly consumable

**Files:**
- Modify: `nx-backend/apps/server/internal/server/server_unit_test.go`

- [x] Add a server test with `home.miniappCarousel.items[].image` referencing `/api/upload-assets/:id`.
- [x] Assert `/api/public/site-config` rewrites the private reference to `/api/public/site-assets/:id`, never returns the private URL, and permits an unauthenticated GET of the referenced image bytes.
- [x] Run the focused Go test; the existing recursive public mechanism already satisfied the new contract.
- [x] Reuse the existing recursive public-site asset mechanism; no production change was required.
- [x] Run the focused server tests and confirm they pass.

---

## Chunk 2: Mini-program data normalization and carousel UI

### Task 4: Normalize carousel configuration and asset URLs

**Files:**
- Create: `miniapp/src/utils/homeCarousel.js`
- Create: `miniapp/src/utils/homeCarousel.test.mjs`
- Modify: `miniapp/package.json`

- [x] Write failing tests for missing config, disabled/empty item filtering, preserved order, interval bounds, and API-relative public asset URL resolution.
- [x] Run `node src/utils/homeCarousel.test.mjs` and confirm it fails because the utility is missing.
- [x] Implement the smallest pure utility functions needed by the home page.
- [x] Add the test to `test:config` and confirm the focused test passes.

### Task 5: Render and refresh the top carousel

**Files:**
- Modify: `miniapp/src/pages/index/index.vue`
- Modify: `miniapp/scripts/ui-compat.test.mjs`

- [x] Add failing source-level assertions that the carousel is the first home content block, uses native `swiper`, autoplay, circular looping, indicator dots, and `aspectFill` images.
- [x] Run `node scripts/ui-compat.test.mjs` and confirm it fails for the missing carousel.
- [x] Load cached site config immediately, refresh it on mount, render enabled slides above the navigation row, and remove failed slides without breaking the page.
- [x] Add responsive carousel styling consistent with the existing rounded iOS cards.
- [x] Run the focused compatibility and utility tests.

---

## Chunk 3: Integration verification

### Task 6: Verify the end-to-end feature

**Files:**
- Modify only if verification reveals a defect in the files above.

- [x] Run `go test ./internal/db ./internal/server/...` from `nx-backend/apps/server`.
- [x] Run the focused admin tests and `pnpm --filter @vben/web-antd typecheck` from `nx-backend`.
- [ ] Run `npm run test:config` and `npm run build:mp-weixin` from `miniapp`. (`build:mp-weixin` passes; the aggregate test is blocked by the pre-existing `.env.production.example` assertion.)
- [x] Run the admin production build.
- [ ] Refresh WeChat Developer Tools and confirm the mini program starts without a carousel when no images are configured.
- [ ] Start the local admin/backend stack if available and upload at least two carousel images.
- [ ] Reorder the two images, disable one, save, refresh the admin page, and confirm the persisted order and enabled state.
- [ ] Confirm the mini program displays only the enabled image after the disabled-state save.
- [ ] Re-enable the second image, save, and confirm both images appear in the saved order; wait longer than the configured interval and confirm the carousel advances automatically and loops back.
- [ ] Delete one image, save, refresh both admin and mini program, and confirm the deletion is reflected.
