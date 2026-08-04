# Infinite Canvas Migration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the old video-generation admin pages with a single, trimmed Infinite Canvas application under the “视频生成 / 无限画布” menu.

**Architecture:** Keep the Vue/Vben admin and the React canvas isolated as sibling workspace apps. The Vue route embeds the canvas through a same-origin iframe, while the React app exposes only canvas project and editor routes and builds into the admin public directory.

**Tech Stack:** Vue 3, Vben Admin, React 19, Vite, Zustand, IndexedDB/localforage, Vitest.

---

## Chunk 1: Route and Vue integration

### Task 1: Replace old video routes

**Files:**
- Create: `nx-backend/apps/web-antd/src/router/routes/modules/video.test.ts`
- Modify: `nx-backend/apps/web-antd/src/router/routes/modules/video.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/infinite-canvas.vue`
- Delete: old files below `nx-backend/apps/web-antd/src/views/video/`

- [ ] Write a failing test asserting the menu title, only child route, path, and component.
- [ ] Run the focused Vitest test and confirm it fails against the old routes.
- [ ] Replace the route tree with `/video/infinite-canvas`.
- [ ] Add the iframe wrapper with loading, error, retry, and open-in-new-window controls.
- [ ] Remove obsolete video view files and rerun the route test.

## Chunk 2: React canvas application

### Task 2: Create the isolated workspace app

**Files:**
- Create: `nx-backend/apps/infinite-canvas/**`
- Create: `nx-backend/apps/infinite-canvas/UPSTREAM.md`
- Create: `nx-backend/apps/infinite-canvas/LICENSE`

- [ ] Copy the upstream web app without build artifacts or dependencies.
- [ ] Add a failing router test proving only canvas routes are exposed.
- [ ] Replace the router and app shell with canvas-only routes.
- [ ] Remove unrelated pages, navigation, account, Agent, WebDAV, update, analytics, advertising, and sponsor code.
- [ ] Remove unused dependencies after import validation.
- [ ] Configure `/infinite-canvas/` base and admin-public build output.
- [ ] Preserve AGPL license and document upstream revision/modifications.

## Chunk 3: Verification

### Task 3: Validate both applications

- [ ] Install workspace dependencies and update the lockfile.
- [ ] Run focused tests for route behavior.
- [ ] Run React typecheck and build.
- [ ] Run Vue typecheck and build.
- [ ] Start both development servers and verify HTTP responses for the admin route and canvas entry.
- [ ] Inspect `git diff`, confirm no backend API or `test` branch changes, and record results.
