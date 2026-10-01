# Course payment status recovery — 2026-10-02

The purchaser completed a real CNY 0.01 payment, but the course status endpoint kept returning the local pending record. A repeat checkout reused the paid merchant order and converted WeChat's rejection into HTTP 502. The client had no dedicated result screen and treated a short confirmation timeout as a payment failure.

The incident payment was verified with WeChat's transaction query, including a separately verified response signature. The merchant, app, merchant order number and amount matched the stored snapshot. The existing idempotent settlement routine restored the order and booking to paid, wrote one administration payment notification, and closed the previous unpaid price snapshot remotely before marking it closed locally. No new charge or refund was initiated.

Nginx logs contain the checkout and six subsequent status queries, but no payment notification request during the incident. The configured public callback endpoint is reachable and rejects unsigned requests with HTTP 400. The reason WeChat's callback did not arrive remains unconfirmed; signature validation was not weakened.

Implementation:

- Course status and checkout query WeChat for unresolved orders and validate merchant/app/order/amount/transaction before applying the existing settlement transaction. Queries have bounded timeouts, a short cache and in-flight sharing.
- Already paid checkouts return the original purchase without payment parameters, including after catalog changes. `ORDERPAID` forces reconciliation; uncertain results return pending/retrying without another cashier.
- A course-specific controller recovers on native return, page resume or a missing-callback timeout. It does not change report or classroom checkout behavior.
- A dedicated payment result page displays server-confirmed success, pending confirmation or closed state. It supports refresh and My Orders, protects session/lifecycle changes and does not trust status/amount from page parameters.
- `paidAt` is the database settlement confirmation time, so the result page labels it as confirmation time.

Validation:

- `npm run test:config` passed, including course checkout, order continuation, unknown native outcomes and result-page behavior.
- `npm run build:mp-weixin` passed; the compiled app registers the result route and contains its four page artifacts.
- `go test ./...` passed.
- PostgreSQL integration with the race detector covers lost callbacks, duplicate settlement/notifications, repeated checkout, user isolation, field mismatches, provider outages, concurrent queries, ORDERPAID races and historical price snapshots.

Production incident backups are under `/opt/heart-company/backups/payment-status-20261002`: verified transaction evidence, the small business-table backup, original environment/Compose override and server image ID. The rollback image is `heart-company-server:before-payment-reconcile-20261002`. No schema change is required.

This is a production API update and a local WeChat developer-tool bundle. It does not publish a new version to the WeChat store.
