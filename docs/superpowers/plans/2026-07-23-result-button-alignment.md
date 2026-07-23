# Result Button Alignment Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Vertically center every result-page button label while preserving the existing design.

**Architecture:** Keep the correction local to the result page. Extend the existing source-level UI compatibility test so the alignment contract cannot regress, then add the minimal CSS declarations required to override WeChat native button metrics.

**Tech Stack:** Vue 3, uni-app, scoped CSS, Node.js `assert` tests, WeChat mini-program build.

---

## Chunk 1: Result page alignment regression

### Task 1: Add a failing style contract

**Files:**
- Modify: `miniapp/scripts/ui-compat.test.mjs`
- Test: `miniapp/scripts/ui-compat.test.mjs`

- [x] Read `miniapp/src/pages/result/result.vue` and extract the standalone style bodies for `.report__cta`, `.report__secondary`, and `.result-actions button`.
- [x] Assert that each relevant rule uses flex layout, centers items on both axes, and removes vertical padding.
- [x] Run `node scripts/ui-compat.test.mjs` and confirm it fails because the result-page rules do not yet provide the alignment contract.

### Task 2: Apply the minimal CSS fix

**Files:**
- Modify: `miniapp/src/pages/result/result.vue`

- [x] Add `padding: 0 24rpx`, `display: flex`, `align-items: center`, `justify-content: center`, and `line-height: 1.2` to the report buttons and result action buttons.
- [x] Run `node scripts/ui-compat.test.mjs` and confirm it passes.
- [ ] Run `npm run test:config` and confirm the full compatibility suite passes. (Blocked by the pre-existing `.env.production.example` production API assertion.)
- [x] Run `npm run build:mp-weixin` and confirm the WeChat build completes.
- [x] Reopen or refresh the latest build in WeChat Developer Tools.
