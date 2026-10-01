# Purchased course experience

**Goal:** A paid course opens its enrollment and class arrangements, and no longer presents a purchase CTA to its owner.

**Architecture:** Add an authenticated enrollment read endpoint backed by orders, bookings and existing database course configuration. Keep the public introduction page for discovery and add a private My Course page for confirmed students. Retain the existing warm white/brown visual style, image previews and server-authoritative payment flow. Work in the current main branch per the user's standing instruction.

**Stack:** Go/PostgreSQL and Vue/uni-app for WeChat.

Design:

- Orders: paid course primary action opens My Course by booking ID; pending course can still open its introduction and continue payment. Historical paid records remain accessible after catalog removal.
- Payment success: primary action opens My Course; My Orders is secondary. A pending payment never claims enrollment or grants access.
- My Course: compact cover, confirmed enrollment, historical purchased title, current class time/location/duration, syllabus and notice, then a booking/payment receipt. Missing arrangements say pending announcement. A configured customer-service QR is previewable; no invented contacts or lesson videos.
- Public course detail: authenticated enrollment lookup determines the CTA. Paid means “已报名 · 查看我的课程”. Loading/unknown state must not show a purchase prompt. Resume and account changes refresh ownership safely.
- The order-list 502 in the supplied screenshot occurred during the prior server replacement; production logs subsequently returned 200. Improve the temporary-service error copy without altering payment truth.

API: `GET /api/miniapp/course/enrollment` accepts exactly one `bookingId` or `courseId`, authenticated as miniapp. Response has `owned`, `bookingId`, `courseId`, `syncStatus`, `catalogAvailable`, `bookingStatus`, `order`, `course`, `customerServiceQr`. `order` contains historical title/amount/status/timestamps and no cashier parameters. Ownership requires a paid course order for the requesting user; pending reconciliation reuses the existing strict WeChat query. A foreign booking is 404. Disabled catalog items remain readable to owners; deleted items use historical title and blank arrangements.

- [x] Add the endpoint and PostgreSQL tests for user isolation, unpaid/uncertain state, delayed settlement and catalog edits/deletion.
- [x] Implement My Course, API wrapper and behavior tests for verified access, lifecycle/session guards, errors and QR/cover previews.
- [x] Update public course detail ownership/CTA behavior with regression tests.
- [x] Update order and payment-result navigation with regression tests.
- [x] Run relevant suites, the full miniapp configuration suite, PostgreSQL race tests and production miniapp build.
- [x] Deploy the verified server update with existing data/configuration preserved, refresh the developer-tool bundle and verify the actual paid course opens the new page.

The existing `course_booking` product has no linkage to video classroom entitlements. This change does not grant access to unrelated paid videos or fabricate lesson content. No database migration is necessary.

Review also caught repeated order/course/receipt navigation accumulating duplicate pages. The shared navigation helper now returns to existing matching routes; private course/receipt reuse requires the same booking ID. Regression tests include 20 round trips, direct entry, multiple stack levels and different bookings.

Production API revision: `92a84c3d0c83eb09f56f923269c9ad907d357c53`. Deployment and real-order verification are recorded in `docs/deployment/owned-course-2026-10-02.md`.
