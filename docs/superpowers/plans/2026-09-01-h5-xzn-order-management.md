# H5 XZN Order Management Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect the personality-test H5 to XZN payments and manage all resulting orders from the existing admin system.

**Architecture:** PostgreSQL is the source of truth. The Go server owns product pricing, XZN signing, order transitions, callbacks, queries, and refunds; the H5 only starts and polls orders, while the Vue admin operates the same records.

**Tech Stack:** Go, PostgreSQL, Vue 3, Ant Design Vue, Node.js H5, XZN form-urlencoded API.

---

## Chunk 1: Backend Order Core

### Task 1: Add payment order schema and store

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Create: `nx-backend/apps/server/internal/xznpay/order_store.go`
- Test: `nx-backend/apps/server/internal/xznpay/order_store_test.go`

- [ ] Write PostgreSQL store tests for create, lookup, pagination, and idempotent state transitions.
- [ ] Add `xzn_payment_orders` schema and indexes.
- [ ] Implement the store and run `go test ./internal/xznpay`.

### Task 2: Complete XZN client operations

**Files:**
- Modify: `nx-backend/apps/server/internal/xznpay/client.go`
- Test: `nx-backend/apps/server/internal/xznpay/client_test.go`

- [ ] Add deterministic signing tests and mocked create/query/refund responses.
- [ ] Implement structured create, query, refund, response validation, and safe pay URL validation.
- [ ] Run focused tests.

### Task 3: Add public order and callback APIs

**Files:**
- Modify: `nx-backend/apps/server/internal/server/xzn_payment_handlers.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Test: `nx-backend/apps/server/internal/server/xzn_payment_handlers_test.go`

- [ ] Test fixed server-side pricing and channel allowlist.
- [ ] Test callback PID/amount/order validation and idempotency.
- [ ] Implement create/status/callback handlers.
- [ ] Run server tests.

## Chunk 2: Admin Order Operations

### Task 4: Add admin list, detail, query, and refund APIs

**Files:**
- Modify: `nx-backend/apps/server/internal/server/xzn_payment_handlers.go`
- Test: `nx-backend/apps/server/internal/server/xzn_payment_handlers_test.go`

- [ ] Test permission-protected pagination and filters.
- [ ] Test active query state synchronization and full refund.
- [ ] Implement admin handlers and audit entries.

### Task 5: Add admin order menu and page

**Files:**
- Modify: `nx-backend/apps/server/internal/db/db.go`
- Modify: `nx-backend/apps/web-antd/src/router/routes/modules/payment.ts`
- Create: `nx-backend/apps/web-antd/src/views/third-party-payment/xzn-orders.vue`
- Create: `nx-backend/apps/web-antd/src/api/core/xzn-payment.ts`

- [ ] Add order-management child menu and permission binding.
- [ ] Implement typed API client.
- [ ] Build filters, table, detail drawer, query, and refund confirmation.
- [ ] Run typecheck and focused frontend tests.

## Chunk 3: H5 Integration

### Task 6: Import and adapt H5 source

**Files:**
- Create: `personality-test-mvp/` from the supplied archive.
- Modify: `personality-test-mvp/server.mjs`
- Modify: `personality-test-mvp/app.js`
- Modify: `personality-test-mvp/paid-plan.js`
- Test: `personality-test-mvp/tests/*.mjs`

- [ ] Import source without secrets or generated order data.
- [ ] Replace local XZN credentials/order writes with Go payment API calls.
- [ ] Redirect only to validated `payUrl` and poll order status after return.
- [ ] Preserve paid-plan entitlement behavior and run `npm test`.

### Task 7: Historical sample migration and verification

**Files:**
- Create: `nx-backend/apps/server/cmd/import-xzn-orders/main.go`
- Test: `nx-backend/apps/server/cmd/import-xzn-orders/main_test.go`

- [ ] Implement dry-run and idempotent import for legacy `orders.json`.
- [ ] Verify Go tests, H5 tests, frontend typecheck, and `git diff --check`.
- [ ] Document deployment order, callback URL, rollback, and secret rotation.
