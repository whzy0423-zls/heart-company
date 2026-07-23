# Profile and Booking Detail Navigation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move WeChat profile editing out of the “我的” overview and add appointment list/detail navigation with complete booking data.

**Architecture:** Keep `profile.vue` as an authenticated overview, create focused profile-edit, booking-records, and booking-detail pages, and add two pure utility modules for display formatting and token-bound in-memory selection. Reuse existing user and booking APIs; do not change backend contracts.

**Tech Stack:** uni-app Vue 3, JavaScript ES modules, Node assertion tests, H5 and mp-weixin builds.

---

## Chunk 1: Pure booking boundaries

### Task 1: Booking display helpers

**Files:**
- Create: `src/utils/bookingDisplay.js`
- Create: `src/utils/bookingDisplay.test.mjs`

- [ ] Write failing tests for numeric ID normalization; number/string equivalence; whitespace; letters, decimals and symbols; malformed URI inputs such as `%` and `%E0%A4%A`; type/status labels; empty values; normal phone masking; short phone preservation; and already-masked phone preservation.
- [ ] Run `node src/utils/bookingDisplay.test.mjs` and verify RED because the module is missing.
- [ ] Implement `normalizeBookingId`, `bookingKindLabel`, `bookingStatusLabel`, `bookingValue`, and `maskBookingPhone` minimally.
- [ ] Run the test and verify GREEN.
- [ ] Commit with `feat: add booking display helpers`.

### Task 2: Token-bound booking page session

**Files:**
- Create: `src/utils/bookingSession.js`
- Create: `src/utils/bookingSession.test.mjs`

- [ ] Write failing tests for set/read, numeric/string ID equivalence, token mismatch clearing, ID mismatch clearing, invalid record rejection, and explicit clearing.
- [ ] Run `node src/utils/bookingSession.test.mjs` and verify RED.
- [ ] Implement a module-memory `{ ownerToken, record }` session using `normalizeBookingId`.
- [ ] Run both booking utility tests and verify GREEN.
- [ ] Commit with `feat: add booking detail session`.

## Chunk 2: Independent pages

### Task 3: Personal profile page

**Files:**
- Create: `src/pages/profile-edit/profile-edit.vue`
- Modify: `src/pages.json`
- Modify: `scripts/ui-compat.test.mjs`

- [ ] Add failing UI contract assertions for the new route, user-info loading, `chooseAvatar` and nickname input on WeChat, H5 disabled guidance without WeChat handlers, PUT save, and auth redirect via `switchTab`.
- [ ] Run `node scripts/ui-compat.test.mjs` and verify RED.
- [ ] Add the route and implement the page by extracting the existing profile sync/edit behavior with three independent states: `profileLoading`, `profileSyncing`, and `profileSaving` (or exact equivalents). Each state prevents duplicate work; unload/session generation invalidates stale results; authentication failure clears token and performs one Toast plus one `switchTab`.
- [ ] Keep H5 nickname save available for an existing token while disabling one-click sync and avatar selection.
- [ ] Run UI compatibility and auth/profile utility tests; verify GREEN.
- [ ] Commit with `feat: move profile editing to dedicated page`.

### Task 4: Appointment records page

**Files:**
- Create: `src/pages/booking-records/booking-records.vue`
- Modify: `src/pages.json`
- Modify: `scripts/ui-compat.test.mjs`

- [ ] Add failing assertions for the route, authenticated list API, loading/error/empty states, retry button, masked phone, Chinese labels, token-bound session set, and navigation to detail. Cover no token and 401/403 clearing auth plus booking session, one Toast/navigation, token change invalidating old responses, and retry never setting a session or navigating.
- [ ] Run UI compatibility and verify RED.
- [ ] Implement list loading with single auth redirect, stale-response guard, session clearing on auth/token changes, accessible cards, empty-state switchTab to booking, and retry isolated from navigation.
- [ ] Verify the API order is preserved without client sorting. Every appointment card uses separately focusable navigation content and controls; H5 has `role="button"`, WeChat has `aria-role="button"`, both have `tabindex="0"`, Enter/Space activation, and any retry control is a separate native button with propagation stopped.
- [ ] Run UI compatibility and booking utility tests; verify GREEN.
- [ ] Commit with `feat: add appointment records page`.

### Task 5: Appointment detail page

**Files:**
- Create: `src/pages/booking-detail/booking-detail.vue`
- Modify: `src/pages.json`
- Modify: `scripts/ui-compat.test.mjs`

- [ ] Add failing assertions for route ID normalization; malformed URI sequences (`%`, `%E0%A4%A`), blank, alphabetic, decimal and symbolic IDs making zero API calls; number/string comparison; token/session validation; recent-50 list recovery; all detail fields; retry/not-found states; auth redirect once; and onUnload session clearing.
- [ ] Run UI compatibility and verify RED.
- [ ] Implement route decoding inside try/catch, cached selection first, list recovery second, and explicit missing/error states without adding a backend endpoint. No token, 401/403, token change, ID mismatch, and unload all clear the booking session; stale responses cannot restore cleared sensitive data.
- [ ] Run UI compatibility and booking utility tests; verify GREEN.
- [ ] Commit with `feat: add appointment detail page`.

## Chunk 3: Overview integration and verification

### Task 6: Simplify the “我的” overview

**Files:**
- Modify: `src/pages/profile/profile.vue`
- Modify: `scripts/ui-compat.test.mjs`

- [ ] Add failing assertions that the identity block navigates to profile-edit, the embedded editor is absent, the booking shell uses a separate navigation body and retry button, retry stops propagation, and booking navigation remains available in loading/error/empty states. Verify the first API item is used as the recent summary without sorting, and the navigation body/retry are separately focusable with H5 `role`, WeChat `aria-role`, tabindex, and Enter/Space behavior.
- [ ] Run UI compatibility and verify RED.
- [ ] Remove profile draft/sync/save state and handlers from the overview; add accessible identity navigation and appointment summary navigation using the first API item.
- [ ] Ensure logout clears the booking session and existing profile race guards remain intact.
- [ ] Run UI compatibility, auth, WeChat profile, booking display, booking session, and list preview tests; verify GREEN.
- [ ] Commit with `feat: connect profile and appointment detail navigation`.

### Task 7: Full integration verification

**Files:**
- Modify only if verification exposes a defect.

- [ ] Run `node src/utils/bookingDisplay.test.mjs`.
- [ ] Run `node src/utils/bookingSession.test.mjs`.
- [ ] Run `node scripts/ui-compat.test.mjs`.
- [ ] Run `node src/utils/auth.test.mjs`, `node src/utils/wechatProfile.test.mjs`, and `node src/utils/listPreview.test.mjs`.
- [ ] Run `npm run build:h5`.
- [ ] Run `npm run build:mp-weixin`.
- [ ] Run `npm run test:config` fresh and record the existing `scripts/project-config.test.mjs:36` failure. Then run the booking tests, UI compatibility, auth, WeChat profile, and list-preview tests independently because the aggregate command stops early.
- [ ] Verify MP page artifacts exist. Run the following; every command must exit 0:

  ```bash
  test -f dist/build/mp-weixin/pages/profile-edit/profile-edit.wxml
  test -f dist/build/mp-weixin/pages/booking-records/booking-records.wxml
  test -f dist/build/mp-weixin/pages/booking-detail/booking-detail.wxml
  ```

- [ ] Verify MP capability and navigation contracts. Run the following; every positive `rg -q` must exit 0:

  ```bash
  rg -q '<button[^>]*open-type="chooseAvatar"[^>]*bindchooseavatar' dist/build/mp-weixin/pages/profile-edit/profile-edit.wxml
  rg -q '<input[^>]*type="nickname"[^>]*bindinput' dist/build/mp-weixin/pages/profile-edit/profile-edit.wxml
  rg -q 'class="[^"]*profile-hero__identity-action[^"]*"[^>]*bindtap' dist/build/mp-weixin/pages/profile/profile.wxml
  rg -q 'class="[^"]*booking-summary__open[^"]*"[^>]*bindtap' dist/build/mp-weixin/pages/profile/profile.wxml
  rg -q 'class="[^"]*booking-record__open[^"]*"[^>]*bindtap' dist/build/mp-weixin/pages/booking-records/booking-records.wxml
  rg -q '预约编号' dist/build/mp-weixin/pages/booking-detail/booking-detail.wxml
  ```

- [ ] Locate the three H5 page chunks and verify they exist. Run the following; the three `test -n` commands must exit 0:

  ```bash
  profile_edit_h5=$(find dist/build/h5/assets -maxdepth 1 -type f -name 'pages-profile-edit-profile-edit.*.js' -print -quit)
  booking_records_h5=$(find dist/build/h5/assets -maxdepth 1 -type f -name 'pages-booking-records-booking-records.*.js' -print -quit)
  booking_detail_h5=$(find dist/build/h5/assets -maxdepth 1 -type f -name 'pages-booking-detail-booking-detail.*.js' -print -quit)
  test -n "$profile_edit_h5"
  test -n "$booking_records_h5"
  test -n "$booking_detail_h5"
  ```

- [ ] Verify the H5-only personal-profile boundary. The positive check and each `! rg -q` negative check must exit 0:

  ```bash
  rg -q '请在微信小程序内同步微信资料' "$profile_edit_h5"
  ! rg -q 'getUserProfile|chooseAvatar|chooseavatar|syncWechatProfile' "$profile_edit_h5"
  ```
- [ ] Run `git diff --check` and confirm the worktree contains only intended changes.
- [ ] Request final spec and code-quality review; fix all Critical/Important findings.
- [ ] Commit any verification fixes, then report the known aggregate `test:config` baseline separately without claiming the entire suite is green.
