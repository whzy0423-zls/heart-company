# Plan Management Actions Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose reliable edit and publish/unpublish controls on the production App plan management page.

**Architecture:** Refresh access codes when the page mounts, derive the action column from current write access, and reuse the existing full-plan PUT endpoint for availability changes. Keep backend authorization and the edit drawer contract unchanged.

**Tech Stack:** Vue 3, TypeScript, Ant Design Vue, Vitest, Go backend, Docker Compose, Nginx.

---

## Chunk 1: Page Behavior

### Task 1: Permission Refresh And Visible Actions

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/app/plan-management.vue`
- Modify: `nx-backend/apps/web-antd/src/views/app/plan-management.test.ts`

- [ ] Add failing tests for `getAccessCodesApi`, textual edit action, publish/unpublish actions, confirmation, and per-row loading state.
- [ ] Run `pnpm exec vitest run apps/web-antd/src/views/app/plan-management.test.ts --dom` and verify RED.
- [ ] Refresh access codes before loading plans, following the existing story-management pattern.
- [ ] Render a 170px action column only for writers; otherwise show an explicit read-only notice above the table and omit the column.
- [ ] Implement `toggleAvailability(plan)` with `Modal.confirm`, a copied payload, `enabled` inversion, success/error feedback, and per-row loading.
- [ ] Re-run the focused test and commit.

### Task 2: Typecheck And Build

**Files:**
- Verify only.

- [ ] Run the focused Vitest file.
- [ ] Run `pnpm --filter @vben/web-antd typecheck`.
- [ ] Run `pnpm run build:antd`.
- [ ] Confirm the generated plan-management chunk contains the new controls.

## Chunk 2: Production Delivery

### Task 3: Merge And Deploy

**Files:**
- Integrate the feature commit with remote `main` without discarding unrelated working-tree changes.

- [ ] Push the focused commit to remote `main` after reconciling the existing asset-recovery commit.
- [ ] On production, preserve current operational overrides and cherry-pick/pull only the verified commits.
- [ ] Build and recreate `admin` without restarting unrelated services.
- [ ] Verify HTML no-cache, hashed assets, container health, and browser console.
- [ ] Confirm all four plan rows show Edit and the correct publish/unpublish action for the admin account.
