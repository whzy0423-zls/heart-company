# 小程序课程配置与报名支付 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the existing three registration courses into a configurable miniapp management page, support server-owned prices, and let the miniapp create and confirm paid course-registration orders through the existing WeChat payment wrapper.

**Architecture:** Keep media classroom products and their entitlement flow unchanged. Add a separate `home.miniappCourses` configuration and `course_booking` order product for registration courses; the server derives the course snapshot and amount from persisted site config, while the client submits only `courseId` and `bookingId`. Put the new editor under the existing Miniapp Management catalog and keep the old website course editor compatible.

**Tech Stack:** Go 1.22 HTTP handlers and PostgreSQL store, Vue 3 + TypeScript + Ant Design Vue admin, uni-app/Vue miniapp, existing `createWechatPaymentController` and WeChat Pay v3 callback.

---

## Chunk 1: Backend course configuration and payment contract

**Files:**
- Modify: `nx-backend/apps/server/internal/db/db.go`
- Modify: `nx-backend/apps/server/internal/db/menu_test.go`
- Modify: `nx-backend/apps/server/internal/siteconfig/*` (normalization/validation for `home.miniappCourses`)
- Modify: `nx-backend/apps/server/internal/server/miniapp_handlers.go` and booking service files
- Modify: `nx-backend/apps/server/internal/miniapp/*` booking/order store files
- Create or modify: backend course booking handler and tests

- [ ] **Step 1: Write failing Go tests** for menu placement, config fallback from `home.courses`, course validation, booking DTO snapshots, owner checks, stale-price rejection, duplicate pending order reuse, paid callback idempotency, and paid booking visibility after course unpublishing.
- [ ] **Step 2: Run the focused Go tests** and confirm each new behavior fails before implementation.
- [ ] **Step 3: Add `home.miniappCourses` normalization** with stable IDs, explicit-empty preservation, defaults of `paymentMode=consult` and `priceCents=0`, and bounds/paid-price validation. Preserve unknown site-config fields during whole-config updates.
- [ ] **Step 4: Extend booking input/output** with `courseId`; keep title, price, mode, and payment status server-owned snapshots. Reject disabled/missing courses and return course metadata in booking lists/details.
- [ ] **Step 5: Add `course_booking` order endpoints** for create, status, and development payment. Accept only booking ID, require current user ownership and a paid course, preserve immutable amount/title snapshots, and handle changed prices with an explicit re-confirmation error.
- [ ] **Step 6: Update payment settlement** so the existing verified callback marks the registration payment paid in the same transaction, records the course booking product, and never grants classroom entitlements.
- [ ] **Step 7: Run focused Go tests and `gofmt`**, then commit the backend chunk with a scoped message.

## Chunk 2: Admin miniapp course editor

**Files:**
- Modify: `nx-backend/apps/server/internal/db/db.go` and `nx-backend/apps/server/internal/db/menu_test.go`
- Modify: `nx-backend/apps/web-antd/src/api/core/site-config.ts`
- Create: `nx-backend/apps/web-antd/src/views/miniapp/courses.vue`
- Create: `nx-backend/apps/web-antd/src/views/miniapp/courses.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/customer/miniapp-orders.vue` and related product labels if needed

- [ ] **Step 1: Write failing Vue tests** for default course hydration, add/edit/remove/reorder behavior, 元↔分 conversion, `consult`/`paid` validation, and the `小程序管理 → 课程配置` menu contract.
- [ ] **Step 2: Run focused Vitest tests** and confirm the new editor behavior fails.
- [ ] **Step 3: Add the menu entry** `/miniapp/courses` under `MiniappManage`, while retaining `/website/courses` compatibility.
- [ ] **Step 4: Implement `MiniappCoursesConfig` types and a focused normalization/editor model**, including the three existing courses as fallback and no demo price promotion into production.
- [ ] **Step 5: Implement the editor UI** with course copy, cover, schedule, bullets, outline, enabled switch, payment mode, and yuan input converted to integer cents. Save through the existing whole site-config editor and preserve unknown fields.
- [ ] **Step 6: Add course booking product labels/amount columns** to the existing admin order view if its API already exposes them; do not change unrelated order products.
- [ ] **Step 7: Run focused Vitest and `vue-tsc` checks**, then commit the admin chunk.

## Chunk 3: Miniapp registration checkout

**Files:**
- Modify: `miniapp/src/api/index.js`
- Modify: `miniapp/src/utils/teacherCourseware.js` or a new course normalization utility
- Modify: `miniapp/src/pages/booking/booking.vue`
- Modify: `miniapp/src/pages/course-detail/course-detail.vue`
- Create or modify: focused miniapp course/payment tests

- [ ] **Step 1: Write failing miniapp tests** for course list fallback, disabled-course filtering, selecting a paid course, booking submission with `courseId`, order creation with only `bookingId`, payment status confirmation, and consult-mode fallback.
- [ ] **Step 2: Run the focused miniapp tests** and confirm they fail.
- [ ] **Step 3: Add API wrappers** for course-booking order creation, status, and dev payment. Keep request amounts out of all client payloads.
- [ ] **Step 4: Load normalized `home.miniappCourses` in the booking page**, show configured price/mode, attach the selected stable course ID, and preserve existing draft/intent behavior.
- [ ] **Step 5: After a paid booking is created, run `createWechatPaymentController` with the booking order adapters and render pending/success/cancelled/timeout states. Refresh booking records only after server-confirmed paid status.
- [ ] **Step 6: Keep unbound/consult courses on the existing intent/booking path** and make course detail use the same paid registration entry when the configured course is paid.
- [ ] **Step 7: Run `npm run test:config`, miniapp build, and focused course/payment tests; commit the miniapp chunk.

## Chunk 4: Integration verification

**Files:**
- Modify: `miniapp/docs/miniapp-payment-api.md` with the new course-booking endpoint mapping
- Modify: `docs/superpowers/specs/2026-09-30-miniapp-course-config-payment-design.md` only if implementation decisions changed

- [ ] **Step 1: Run backend focused tests and the complete server test package(s).**
- [ ] **Step 2: Run admin focused tests and typecheck.**
- [ ] **Step 3: Run miniapp `npm run test:config` and `npm run build:mp-weixin`.**
- [ ] **Step 4: Run `git diff --check`, inspect `git status`, and verify no client request includes a price or payment signature field.
- [ ] **Step 5: Open the built miniapp in the WeChat Developer Tools and verify consult and paid course paths, including price display and server-confirmed success.
- [ ] **Step 6: Commit documentation and integration verification updates.
