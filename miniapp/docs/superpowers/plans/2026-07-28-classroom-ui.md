# Classroom UI Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the miniapp teacher classroom entry and course browsing feel like a polished video/audio content platform while preserving existing data and backend behavior.

**Architecture:** Keep the existing UniApp/Vue page structure and API calls. Add test-first static contracts for new UI hooks, then update `index.vue`, `learn.vue`, `classroom.vue`, and `classroom-detail.vue` styles/templates without changing purchase, progress, or media-fetching logic.

**Tech Stack:** UniApp + Vue 3 single-file components, Node `.mjs` regression tests, WeChat Mini Program build output.

---

## Chunk 1: UI Contracts

### Task 1: Add regression assertions for the new classroom entry and platform card structure

**Files:**
- Create: `src/pages/index/index.test.mjs`
- Modify: `src/pages/learn/learn.content-state.test.mjs`
- Modify: `src/pages/classroom/classroom.test.mjs`
- Modify: `src/pages/classroom-detail/classroom-detail.test.mjs`
- Modify: `package.json`

- [ ] Add failing assertions for `classroom-spotlight` on the home page, including route to `/pages/classroom/classroom?tab=series` and 88rpx CTA touch target.
- [ ] Add failing assertions for `classroom-entry__hero` and clickable preview items on the learn page.
- [ ] Add failing assertions for content-platform list structure: full-width cover shell, overlay chips/play CTA, and vertical card body.
- [ ] Add failing assertions for detail cover shell and unified media panel style.
- [ ] Run the focused tests and confirm they fail before production changes.

## Chunk 2: Home + Learn Entrances

### Task 2: Implement the visible classroom entry surfaces

**Files:**
- Modify: `src/pages/index/index.vue`
- Modify: `src/pages/learn/learn.vue`

- [ ] Add `goClassroom()` route helper on the home page.
- [ ] Add a `classroom-spotlight` card below hero and before the function grid.
- [ ] Style the spotlight to match the existing home palette while reading as video/audio content.
- [ ] Upgrade the learn page classroom entry header to a content banner.
- [ ] Make preview items tap into the classroom page with clear visual affordance.
- [ ] Run focused tests for index and learn.

## Chunk 3: Classroom List

### Task 3: Convert classroom cards to A-style content platform cards

**Files:**
- Modify: `src/pages/classroom/classroom.vue`

- [ ] Restructure card template so cover is full-width and metadata overlays the cover.
- [ ] Keep existing purchase button `.series-buy` and event `.stop` behavior.
- [ ] Add a clear play/expand affordance without changing route helpers.
- [ ] Refresh tabs, empty/error states, selected series state, and lesson rows visually.
- [ ] Run `node src/pages/classroom/classroom.test.mjs`.

## Chunk 4: Detail Page

### Task 4: Align detail page cover/player/access/progress presentation

**Files:**
- Modify: `src/pages/classroom-detail/classroom-detail.vue`

- [ ] Add a large cover shell with overlay title/meta.
- [ ] Keep existing video/audio player and progress/purchase state logic untouched.
- [ ] Apply unified player panel spacing, colors, and CTA styling.
- [ ] Run `node src/pages/classroom-detail/classroom-detail.test.mjs`.

## Chunk 5: Verification

### Task 5: Verify build and output

**Files:**
- Build output: `dist/build/mp-weixin`

- [ ] Run all focused classroom/index/learn tests.
- [ ] Run `npm run test:config`.
- [ ] Run `npm run build:mp-weixin`.
- [ ] Verify `dist/build/mp-weixin` exists and contains classroom pages.
