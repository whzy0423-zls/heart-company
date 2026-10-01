# Purchased course access — 2026-10-02

The paid order's course action opened the public sales page, which still displayed the purchase button. Paid orders now open a private My Course page with current class arrangements and the historical purchase receipt. Payment success uses the same primary destination, and the public introduction checks ownership before offering checkout.

The new authenticated `GET /api/miniapp/course/enrollment` endpoint accepts exactly one booking ID or course ID. It joins the requesting user's course bookings and orders, grants ownership only for a persisted paid course order, and reuses strict WeChat reconciliation for unresolved payment records. URL flags and booking status do not grant access. Historical title and amount come from the order; the schedule, location, duration, outline and notice come from existing database-backed course configuration.

Paid students retain access when the catalog entry is disabled or removed. Disabled arrangements are hidden from non-owners. Deleted courses retain their historical receipt, and missing arrangements display pending-announcement text. There is no database migration or invented video entitlement: course bookings are not currently linked to classroom video products.

The new page preserves the existing cream/brown appearance, keeps the top of the original cover visible, and previews full images. Private data is cleared on hide, unload and session changes; page sharing is hidden through the supported WeChat runtime API.

Order/course/receipt navigation reuses an existing page instead of repeatedly filling the WeChat page stack. Course and receipt reuse matches the booking ID. Existing pages revalidate the backend and session on show; 20-round-trip regressions, direct entry and different-booking tests cover this behavior.

Verification before deployment:

- Full Go package tests passed.
- PostgreSQL race tests passed for enrollment, payment reconciliation and checkout, including identity isolation, pending and refunded records, old purchases, historical pricing and disabled/deleted courses.
- The full miniapp configuration/behavior suite and production WeChat build passed. The bundle contains the new route and page artifacts, uses the production API origin and retains the supported payment runtime.

The prepared rollback image is `heart-company-server:before-owned-course-20261002`; configuration snapshots and the live read-only verifier are in `/opt/heart-company/backups/owned-course-20261002`. Only the server service is rebuilt/recreated. No new payment or refund is initiated by this verification.

The screenshot's order-list HTTP 502 occurred during the preceding server replacement. Subsequent production requests returned 200 and contained the real paid order. A temporary 502/503/504 now displays a retry message without claiming that payment records were lost.

This delivery updates the production API and the local WeChat developer-tool bundle. WeChat store publication is a separate release action.

Production verification:

- Backend revision `92a84c3d0c83eb09f56f923269c9ad907d357c53` was built and deployed; build and server-only recreation both exited 0. The running image is `sha256:36898744d71c5fff586b2d111b5dbf0f1a2e65ec72baa3d4310437b4b955c411`.
- Public-domain health returned HTTP 200. Authenticated enrollment requests by booking and by course confirmed the incident's paid purchase for one cent. Unauthenticated access returned HTTP 401.
- The real order and booking remain paid, with exactly one administration payment notification. Other Compose services remain running; configuration, volumes and certificate mounts were preserved.
- After the final miniapp suite and build passed, WeChat developer tools reopened the production bundle. Clicking the paid order's “查看课程” opened `pages/my-course/my-course`, displaying “个人成长 · 关系疗愈”, “已报名”, the real ¥0.01 receipt and database-backed course content. Unconfigured arrangements displayed “待通知”. The “返回我的订单” action returned to the existing order list, and reopening the paid course worked.
- The public course introduction's ownership CTA is covered by the behavioral suite, including paid, unpaid, loading, uncertain status, guest login and account-switch scenarios.
