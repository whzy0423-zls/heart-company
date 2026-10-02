# Course Registration Switch Implementation Plan

**Goal:** Provide one persistent administration switch that hides course sales and retains the real booking form.

**Architecture:** Reuse site configuration at `home.miniappCourses.enabled`, defaulting to true. Frontend pages consume a shared predicate; backend checks block new course purchases while retaining existing receipts and paid access.

**Tech Stack:** Vue / uni-app, Vue / Ant Design administration, Go / PostgreSQL.

- [x] Add admin switch, boolean type and normalization; verify save/reload and preservation of course data with `courses.test.ts`.
- [x] Add backend predicate and new-booking/payment guards; run siteconfig and server regressions including PostgreSQL.
- [x] Add miniapp predicate, form-only booking mode, home recommendation gating and old detail-link redirection; refresh settings on show and guard stale drafts/payments.
- [x] Verify pending-order actions and retain paid order/course navigation.
- [x] Run `npm run test:config`, production miniapp build, administration tests/build and backend relevant checks.
- [x] Verify both switch modes in a non-production fixture, deploy tested admin/server artifacts using existing release overrides, and rebuild the local WeChat project. Preserve live switch values and existing orders.
- [x] Resolve the observed existing 2 MiB upload limit by excluding redundant hosted covers, verify all public JPEGs and preserve local original portrait/canvas bytes. Add production package budget verification.
