# Course pricing and checkout implementation plan

**Goal:** A course with no positive price opens teacher consultation/booking; a positively priced course opens WeChat checkout and persists an order visible to its owner and administrators.

**Architecture:** Keep the existing course booking payment product and callback settlement. Derive its payment mode from the server-owned price. Add direct course checkout and an authenticated, paginated order list; use the existing payment controller and admin order screen.

**Tech stack:** Go/PostgreSQL, Vue/uni-app, Vue/Ant Design.

- [x] Normalize server and admin course pricing: missing/null/zero amount means consultation, positive integer cents means paid; reject invalid monetary values.
- [x] Direct checkout accepts courseId, persists/reuses a booking and order for the authenticated user, and reads price/title from the database configuration. Preserve the bookingId flow.
- [x] Add authenticated GET /api/miniapp/orders with pagination and status filtering; enforce user isolation and return historical order amounts/titles.
- [x] Add direct checkout to course detail, consultation label/navigation, payment state handling, and My Orders navigation from profile and payment completion.
- [x] Build My Orders with persisted status, amounts, dates, order numbers, pagination, refresh and course payment retry.
- [x] Include course_booking in admin order filters/display.
- [x] Test price normalization, API contracts, direct checkout versus consultation, callback-authoritative success, duplicate submissions, user isolation and order persistence. Run miniapp configuration tests/build, affected admin tests/build and Go tests.
- [x] Publish the verified server/admin update using the existing deployment authorization; preserve production configuration, data, certificates and unrelated services. Prepare the updated WeChat developer-tool bundle; do not claim a WeChat store release or a real transaction without evidence.

Only server-confirmed payment is displayed as paid. Direct purchasers are associated with their real WeChat user; missing phone/contact details remain absent. Consultation remains a separate form submission. The paid order retains the original price and title even when the catalog changes later.

Validation (2026-10-01):

- Miniapp configuration/API/session/payment regression suite passed; new course checkout and order pagination/session/payment tests passed.
- WeChat mini-program production bundle compiled successfully at `miniapp/dist/build/mp-weixin`.
- Admin course editor/order component tests passed (8 tests across two suites after notification filter coverage); Vue typecheck passed; production admin build passed.
- `go test ./...` passed. Dedicated PostgreSQL checkout integration passed with the race detector, including 8 concurrent checkouts, isolation, idempotent settlement and notifications, price snapshot preservation and sibling order closure.
- Independent checkout review found no blocking issues; the order page was additionally guarded immediately before invoking the cashier after an account switch.
- No real WeChat charge was initiated during validation.

Production deployment preparation: existing Compose + override and payment certificate mounts verified; rollback images tagged `heart-company-{server,admin}:before-course-checkout-20261001`. Backup at `/opt/heart-company/backups/course-checkout-20261001` contains schema, application tables, configuration snapshots and previous revision. No schema migration is required.

Production result (verified 2026-10-02):

- Application revision `8e39db3dea64c80f5f5ef29827e174be4b58b8ff` deployed to `/opt/heart-company`; Compose build and server/admin recreation both exited 0, with the existing override, volumes and payment certificate mounts retained.
- Public `/api/app/health` and `/api/public/site-config` returned HTTP 200. Orders create/list/status return 401 without login, as expected.
- The running admin image includes updated course configuration and course-order UI bundles; the admin course route returned HTTP 200.
- Payment is enabled. The three existing courses currently have zero prices and remain consultation courses; no business price was invented or changed.
- Latest production miniapp bundle opened successfully in WeChat developer tools after project reload. This is a developer-tool build, not a WeChat public release. Real payment settlement still needs a user-run transaction.
