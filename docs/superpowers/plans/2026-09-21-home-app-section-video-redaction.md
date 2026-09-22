# Homepage App Section And Install Video Redaction Implementation Plan

> **For agentic workers:** REQUIRED: Execute locally in the current task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the App introduction/download section from the homepage while preserving the dedicated `/app` page, and redact third-party video/player branding from the installation tutorial video and poster.

**Architecture:** Keep the shared App download component and styles because `/app` still consumes them. Remove only the homepage import/render path. Re-encode the existing bundled installation video with time-bounded blur regions, then regenerate the poster from the redacted output.

**Tech Stack:** React 18, Node test runner, Vite, FFmpeg.

---

## Chunk 1: Homepage scope

### Task 1: Remove the homepage-only App section

**Files:**
- Modify: `website-react/src/pages/home-app-download.test.mjs`
- Modify: `website-react/src/pages/Home.jsx`

- [x] Change the homepage contract test to require no `AppDownloadSection` import or render while preserving navigation and Hero links to `/app`.
- [x] Run the focused test and confirm it fails for the existing homepage.
- [x] Remove the homepage import and render.
- [x] Run the focused test and confirm it passes.

## Chunk 2: Media redaction

### Task 2: Redact the installation tutorial

**Files:**
- Modify: `website-react/public/assets/app/install-guide.mp4`
- Modify: `website-react/public/assets/app/install-guide-poster.webp`
- Modify: `website-react/src/pages/app-download-page.test.mjs`

- [x] Extract interval contact sheets and identify every frame range containing the third-party video cover, player name, or icon.
- [x] Add a media contract assertion that the bundled files remain valid and reference the redacted asset revision.
- [x] Re-encode the source using FFmpeg blur overlays limited to the identified regions and time ranges; preserve audio and dimensions.
- [x] Generate the poster from the redacted video.
- [x] Extract frames from the redacted output at interval boundaries and visually verify the sensitive regions stay covered.

## Chunk 3: Verification

### Task 3: Validate the website

- [x] Run `npm test`.
- [x] Run `npm run build`.
- [x] Serve `dist` locally and verify `/` excludes the App section while `/app` retains the dedicated page and redacted tutorial.
- [x] Inspect desktop and mobile screenshots and browser console errors.
