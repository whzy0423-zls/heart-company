# Infinite Canvas Theme Sync Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the infinite canvas visually inherit the current admin theme in light and dark modes.

**Architecture:** The Vue host serializes semantic design tokens into a versioned `postMessage`. The React iframe validates and applies those tokens, while Canvas and Ant Design consume the synchronized semantic colors.

**Tech Stack:** Vue 3, React 19, TypeScript, CSS custom properties, Vitest, Ant Design.

---

### Task 1: Define and test the iframe theme protocol

**Files:**
- Create: `nx-backend/apps/infinite-canvas/src/lib/admin-theme-bridge.test.ts`
- Create: `nx-backend/apps/infinite-canvas/src/lib/admin-theme-bridge.ts`

- [ ] Write failing tests for validation, HSL normalization and DOM application.
- [ ] Run the focused test and confirm the expected failure.
- [ ] Implement the minimal bridge.
- [ ] Run the focused test and confirm it passes.

### Task 2: Send the admin theme from the Vue host

**Files:**
- Create: `nx-backend/apps/web-antd/src/utils/infinite-canvas-theme.test.ts`
- Create: `nx-backend/apps/web-antd/src/utils/infinite-canvas-theme.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/infinite-canvas.vue`

- [ ] Write failing tests for computed-token extraction and message creation.
- [ ] Run the focused test and confirm the expected failure.
- [ ] Implement extraction, iframe delivery and mutation observation.
- [ ] Run the focused test and confirm it passes.

### Task 3: Consume semantic colors throughout the canvas

**Files:**
- Modify: `nx-backend/apps/infinite-canvas/src/lib/canvas-theme.ts`
- Modify: `nx-backend/apps/infinite-canvas/src/styles/globals.css`
- Modify: `nx-backend/apps/infinite-canvas/src/lib/app-theme.ts`
- Modify: `nx-backend/apps/infinite-canvas/src/components/layout/app-providers.tsx`
- Modify: `nx-backend/apps/infinite-canvas/src/stores/use-theme-store.ts`

- [ ] Replace stone colors with semantic variables.
- [ ] Make Ant Design consume synchronized primary/radius values.
- [ ] Keep standalone defaults aligned with the admin blue theme.

### Task 4: Verify behavior

- [ ] Run focused and regression unit tests.
- [ ] Run React and Vue typechecks.
- [ ] Build the infinite canvas into the admin public directory.
- [ ] Refresh the local admin page and verify visual consistency in the browser.
