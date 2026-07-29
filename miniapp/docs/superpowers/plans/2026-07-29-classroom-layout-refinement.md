# Classroom Layout Refinement Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refine the teacher classroom page layout so course content appears earlier, progress is easier to scan, and every course card has one clear action.

**Architecture:** Keep all existing classroom state, API, payment, resume, and navigation logic unchanged. Modify only the classroom template/source-contract tests and scoped styles to compact the header/progress/tabs and remove the duplicate cover CTA from course cards.

**Tech Stack:** uni-app, Vue 3, WeChat mini-program native buttons/images, Node source-contract/runtime tests, scoped CSS/WXSS compatibility pipeline.

---

## Chunk 1: Classroom information hierarchy

### Task 1: Add failing layout contract tests

**Files:**
- Modify: `src/pages/classroom/classroom.test.mjs`

- [ ] Assert the hero uses the concise title “视频与音频课件”, the concise lead, and exactly two meta tags: “独立课件” and “系列课程”.
- [ ] Assert the hero padding/title/lead/tag spacing is compact.
- [ ] Assert continue-learning keeps one-row heading/action, progress semantics, and reduced minimum height.
- [ ] Assert tabs keep two 88rpx native buttons with a compact shell and clear active state.
- [ ] Assert the cover overlay contains tags and a play icon but no duplicated action label.
- [ ] Assert each course card retains only one action button in its body/footer.
- [ ] Run `node src/pages/classroom/classroom.test.mjs` and confirm RED.

### Task 2: Implement the compact layout

**Files:**
- Modify: `src/pages/classroom/classroom.vue`

- [ ] Replace the hero copy with the concise title/lead and two meta tags.
- [ ] Reduce hero padding, type scale, and chip spacing without changing brand tokens.
- [ ] Compact continue-learning padding, progress spacing, and title layout while preserving the full clickable button.
- [ ] Compact the tabs and strengthen the selected visual state.
- [ ] Remove `classroom-card__play-text`; keep a centered decorative play/expand glyph only.
- [ ] Reorder the course body into category, title, summary, facts, and one action.
- [ ] Reduce cover/body vertical spacing while retaining all ratio classes and 88rpx actions.
- [ ] Run focused, UI compatibility, and WeChat style tests.
- [ ] Commit `feat: refine classroom content layout`.

## Chunk 2: Verification

### Task 3: Run complete verification

- [ ] Run `npm run test:config`.
- [ ] Run `npm run build:mp-weixin`.
- [ ] Confirm generated WXSS contains zero `var(--*)`.
- [ ] Confirm generated classroom WXML contains one visible course action label per card and no `classroom-card__play-text`.
- [ ] Confirm the existing `npm run dev:mp-weixin` watcher rebuilt `dist/dev/mp-weixin/pages/classroom`.
- [ ] Run `git diff --check -- miniapp` and `git status --short -- miniapp`.
