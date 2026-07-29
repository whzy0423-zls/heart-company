# Profile Logo and Teacher Portrait Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Use the existing Nine-Type logo for profile avatar fallbacks and replace the unreadable homepage teacher poster thumbnail with a clear portrait plus full-screen detail preview.

**Architecture:** Extend the normalized expert view model with separate portrait and detail image URLs while preserving the legacy `image` field as the detail-image source. Keep media URL normalization centralized in `resolveContentAsset()`, use `/assets/teacher.jpg` for the compact portrait and `/assets/teacher-poster.jpg` for the full preview, and keep all existing hero copy/actions unchanged.

**Tech Stack:** uni-app + Vue 3, WeChat mini-program `<image>`, `uni.previewImage`, Node assertion contract tests, existing PostCSS-compatible asset resolver.

---

## Chunk 1: Expert view-model contract

### Task 1: Add failing normalization tests

**Files:**
- Modify: `src/utils/personalExpertHome.test.mjs`
- Modify: `src/utils/personalExpertHome.js`

- [ ] **Step 1: Extend the fixture with portrait/detail fields and legacy poster compatibility.**
  Assert that `portraitImage` prefers `portraitImage`, falls back to `/assets/teacher.jpg`, and `detailImage` prefers `detailImage`, then legacy `image`, then `/assets/teacher-poster.jpg`.

- [ ] **Step 2: Run the focused test to verify it fails.**

  Run: `node src/utils/personalExpertHome.test.mjs`
  Expected: FAIL because `expertHero` does not yet expose `portraitImage` or `detailImage`.

- [ ] **Step 3: Implement the smallest model change.**
  Add a `firstAsset()` helper that tries each candidate through `resolveContentAsset()` and returns the first valid URL. Preserve `expertHero.image` as an alias of `detailImage` for existing consumers.

- [ ] **Step 4: Run the focused test.**

  Run: `node src/utils/personalExpertHome.test.mjs`
  Expected: PASS.

- [ ] **Step 5: Commit the model change.**

  ```bash
  git add src/utils/personalExpertHome.js src/utils/personalExpertHome.test.mjs
  git commit -m "feat: separate expert portrait and detail images"
  ```

## Chunk 2: Profile logo fallbacks

### Task 2: Add failing profile Logo contract tests

**Files:**
- Create: `src/pages/profile/profile.logo.test.mjs`
- Modify: `src/pages/profile/profile.vue`

- [ ] **Step 1: Write source-contract tests.**
  Assert that the logged-out profile Hero renders an `<image>` using `/static/wheel.png`, and that the logged-in avatar branch uses the same Logo when the user has no avatar or `userAvatarFailed` is true.

- [ ] **Step 2: Run the focused test to verify it fails.**

  Run: `node src/pages/profile/profile.logo.test.mjs`
  Expected: FAIL because the current template renders a text “九” placeholder.

- [ ] **Step 3: Implement the Logo fallback.**
  Add a small reusable `profile-logo` image block in both logged-out and logged-in fallback branches. Use `mode="aspectFit"`, `aria-label="九型 Logo"`, and keep the existing frame dimensions and gold/navy styling.

- [ ] **Step 4: Run the focused test.**

  Run: `node src/pages/profile/profile.logo.test.mjs`
  Expected: PASS.

- [ ] **Step 5: Commit the profile change.**

  ```bash
  git add src/pages/profile/profile.vue src/pages/profile/profile.logo.test.mjs
  git commit -m "feat: use Nine-Type logo for profile avatar fallback"
  ```

## Chunk 3: Homepage portrait and detail preview

### Task 3: Add failing homepage behavior tests

**Files:**
- Modify: `src/pages/index/index.test.mjs`
- Modify: `src/pages/index/index.vue`

- [ ] **Step 1: Add template/behavior assertions.**
  Assert that the teacher frame binds `view.expertHero.portraitImage`, has a distinct image error state, exposes a “查看完整导师介绍” affordance, and calls `uni.previewImage` with `view.expertHero.detailImage` only when that URL exists.

- [ ] **Step 2: Run the focused test to verify it fails.**

  Run: `node src/pages/index/index.test.mjs`
  Expected: FAIL because the template currently binds `view.expertHero.image` directly and has no preview action.

- [ ] **Step 3: Implement portrait/detail rendering.**
  Add `teacherPortraitFailed` state and a `previewTeacherDetail()` handler. Replace the compact frame source with `portraitImage`; show a branded “九” fallback when the portrait fails; add a bottom hint and `hover-class`; call `uni.previewImage({ current: detailImage, urls: [detailImage] })` when a valid detail image is present.

- [ ] **Step 4: Update scoped styles.**
  Keep the existing navy/gold frame but make the portrait the visual focus, add a readable bottom overlay hint, and preserve responsive behavior on narrow screens.

- [ ] **Step 5: Run the focused test.**

  Run: `node src/pages/index/index.test.mjs`
  Expected: PASS.

- [ ] **Step 6: Commit the homepage change.**

  ```bash
  git add src/pages/index/index.vue src/pages/index/index.test.mjs
  git commit -m "feat: add clear teacher portrait preview"
  ```

## Chunk 4: Full verification and WeChat preview

### Task 4: Run regression, build, and dev preview

**Files:**
- Modify: `package.json` only if the new profile contract test needs to be included in `test:config`.

- [ ] **Step 1: Add the profile contract test to `test:config`.**

- [ ] **Step 2: Run all tests.**

  Run: `npm run test:config`
  Expected: all existing and new contract tests pass.

- [ ] **Step 3: Build the WeChat target.**

  Run: `npm run build:mp-weixin`
  Expected: build succeeds and generated WXSS contains no `var(--*)` expressions.

- [ ] **Step 4: Restart the development watcher.**

  Run: `npm run dev:mp-weixin`
  Expected: `DONE Build complete. Watching for changes...`.

- [ ] **Step 5: Verify in the developer tool.**
  Check the profile page in logged-out state, logged-in/no-avatar state, homepage teacher portrait, and the full-screen poster preview. Confirm no image-load error is emitted for the portrait or Logo.

- [ ] **Step 6: Run final repository checks.**

  ```bash
  git diff --check -- .
  git status --short
  ```

  Expected: no whitespace errors and only intentional committed changes.
