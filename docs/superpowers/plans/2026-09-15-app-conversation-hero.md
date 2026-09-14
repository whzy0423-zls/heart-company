# App Conversation Hero Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the static hero screenshot with a realistic animated growth conversation and add a persistent iOS unavailable notice.

**Architecture:** Add one presentational React component with fixed anonymous content and no runtime data dependency. Integrate it into the existing page and download section, then extend the page stylesheet with scoped responsive and reduced-motion rules.

**Tech Stack:** React 18, CSS, Node test runner, Vite

---

## Chunk 1: Conversation Preview

### Task 1: Lock the hero and iOS behavior with tests

**Files:**
- Modify: `website-react/src/pages/app-download-page.test.mjs`
- Modify: `website-react/src/components/AppDownloadSection.test.mjs`

- [ ] Add assertions for a dedicated conversation preview, anonymous example copy, insight action, and no static hero screenshot.
- [ ] Add an assertion for the exact persistent iOS availability message.
- [ ] Run both targeted test files and confirm they fail for the missing behavior.

### Task 2: Implement the conversation preview

**Files:**
- Create: `website-react/src/components/AppConversationPreview.jsx`
- Modify: `website-react/src/pages/AppDownload.jsx`
- Modify: `website-react/src/index.css`

- [ ] Build the semantic preview header, user message, AI response, action callout, and composer.
- [ ] Replace the hero image with the component.
- [ ] Add scoped visual styling, subtle animation, and reduced-motion fallbacks.
- [ ] Run the page test and confirm it passes.

### Task 3: Add the persistent iOS status

**Files:**
- Modify: `website-react/src/components/AppDownloadSection.jsx`
- Modify: `website-react/src/index.css`

- [ ] Render "当前 iOS 版本暂不支持，敬请期待" below the main action area.
- [ ] Style it as secondary platform status without competing with Android download.
- [ ] Run the component test and confirm it passes.

## Chunk 2: Verification And Release

### Task 4: Verify responsive quality

- [ ] Run `npm test`.
- [ ] Run `npm run build`.
- [ ] Inspect desktop and mobile browser screenshots and verify no horizontal overflow.
- [ ] Commit and fast-forward push the implementation.
- [ ] Deploy only the website container and verify the public page and assets.

