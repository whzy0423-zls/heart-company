# Manual Membership Administration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist time-bounded App memberships and let administrators confirm customer-service orders with correct renewal dates.

**Architecture:** Add nullable membership timestamps to users and activation-result timestamps to orders, centralize duration/renewal calculation in a small Go helper, and apply it transactionally from the admin confirmation endpoint. Extend the existing App order and App customer APIs/UI rather than adding a new administration area.

**Tech Stack:** Go, PostgreSQL, net/http, sqlmock-style local test drivers, Vue 3, TypeScript, Ant Design Vue, Vitest/typecheck.

---

## Chunk 1: Membership persistence and App APIs

### Task 1: Add compatible schema columns

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Test: `nx-backend/apps/server/internal/db/schema_test.go` or nearest schema assertion test

- [ ] Write a failing schema assertion for `member_started_at`, `member_expires_at`, `activation_at` and `membership_expires_at`.
- [ ] Add columns to fresh-table definitions plus `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` for existing deployments.
- [ ] Run the schema test and commit.

### Task 2: Centralize membership calculation

**Files:**
- Create: `nx-backend/apps/server/internal/server/app_membership.go`
- Create: `nx-backend/apps/server/internal/server/app_membership_test.go`

- [ ] Write table tests for 30/90/365 days, expired membership, active renewal, invalid plan and exact activation time.
- [ ] Verify failures, implement the pure calculator, rerun and commit.

### Task 3: Return real products, manual orders and active entitlements

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_billing.go`
- Modify: `nx-backend/apps/server/internal/server/app_billing_test.go`

- [ ] Write failing handler tests for three enabled manual products, no deep report, `pending_confirmation`, customer QR URL, and expired memberships becoming free.
- [ ] Verify failures.
- [ ] Implement the three-product responses, order metadata and timestamp-aware entitlement query.
- [ ] Run billing tests and commit.

## Chunk 2: Admin confirmation and customer visibility

### Task 4: Confirm orders transactionally and idempotently

**Files:**
- Modify: `nx-backend/apps/server/internal/server/admin_app_ops.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Test: `nx-backend/apps/server/internal/server/admin_app_ops_test.go`

- [ ] Write failing tests for activation-time validation, row locking, active renewal, expired renewal, paid-order idempotency and audit data.
- [ ] Verify failures.
- [ ] Accept `{ activationAt }`, calculate expiry in a transaction, update user/order atomically and return the resulting membership.
- [ ] Run tests and commit.

### Task 5: Expose membership fields in admin APIs

**Files:**
- Modify: `nx-backend/apps/server/internal/server/admin_app_orders.go`
- Modify: `nx-backend/apps/server/internal/appuser/store.go`
- Modify: `nx-backend/apps/server/internal/server/app_analytics.go`
- Test: corresponding Go tests

- [ ] Write failing tests for plan/start/expiry/remaining-day fields on orders and customers.
- [ ] Update focused queries and response structs while keeping old nullable rows compatible.
- [ ] Run affected Go tests and commit.

### Task 6: Upgrade App order and customer screens

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/app-order.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/app-ops.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/app-customer.ts`
- Modify: `nx-backend/apps/web-antd/src/views/customer/app-orders.vue`
- Modify: `nx-backend/apps/web-antd/src/views/customer/app-users.vue`
- Create: `nx-backend/apps/web-antd/src/views/customer/app-membership.ts`
- Test: `nx-backend/apps/web-antd/src/views/customer/app-membership.test.ts`

- [ ] Write failing pure-function tests for status labels, package labels, remaining days and confirmation payloads.
- [ ] Verify failures.
- [ ] Implement a date-time confirmation modal with visible labels, loading feedback and a confirmation summary.
- [ ] Show current membership timestamps in order/customer list and detail views; remove deep-report filters.
- [ ] Run tests/typecheck and commit.

## Chunk 3: Verification

### Task 7: Verify backend and admin

- [ ] Run `gofmt` on changed Go files.
- [ ] Run `go test ./...` from `nx-backend/apps/server`.
- [ ] Run the targeted frontend tests.
- [ ] Run `pnpm --filter @vben/web-antd typecheck` and `pnpm --filter @vben/web-antd build` from `nx-backend`.
- [ ] Smoke-test pending order creation, admin confirmation, early renewal, expiry display and QR reuse against the local database.
