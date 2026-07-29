# Home Full-Bleed Expert Hero Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the homepage cropped full-width hero with a centered portrait-format card that shows the complete teacher image within the first screen.

**Architecture:** Keep the existing `expertHero` view-model and preview handler unchanged. Refactor only the homepage template and scoped styles so the portrait uses `aspectFit` inside an approximately `600 × 1040rpx` centered card, all large identity/lead overlay copy is removed, and the classroom/detail actions remain discoverable in a thin bottom action layer.

**Tech Stack:** uni-app, Vue 3, WeChat mini-program native buttons/images, Node source-contract tests, scoped CSS/WXSS compatibility pipeline.

---

## Chunk 1: Full-bleed hero contract and implementation

### Task 1: Show the complete teacher portrait in a vertical first-screen card

**Files:**
- Modify: `src/pages/index/index.test.mjs`
- Modify: `src/pages/index/index.vue`

- [ ] **Step 1: Write failing source-contract tests.**

Assert that:

- `.expert-hero` is a centered portrait-format card near `600 × 1040rpx` that fits within a common phone first screen after the home header.
- The former `.expert-hero__copy`, `view.expertHero.lead`, `view.expertHero.eyebrow`, and `view.expertHero.title` are absent from the hero template.
- The portrait binds `portraitImage` identity with `mode="aspectFit"` so the complete image remains visible.
- The preview handler still uses `detailImage`; “进入老师课堂” remains a separate native button.
- The thin bottom action layer keeps gold/navy styling, narrow-screen rules, and native 88rpx touch targets without covering the teacher face/body.

- [ ] **Step 2: Run the focused test and confirm RED.**

Run:

```bash
node src/pages/index/index.test.mjs
```

Expected: FAIL because the current hero uses `aspectFill`, a wide 600rpx card, and large identity text inside the overlay.

- [ ] **Step 3: Implement the minimal template refactor.**

Keep the preview button and classroom button as sibling native buttons. Remove the eyebrow/title overlay, change the image to `aspectFit`, and keep only a compact “查看完整导师介绍” affordance plus the classroom action at the bottom.

- [ ] **Step 4: Implement the scoped styles.**

Center the card at approximately `600rpx` wide and `1040rpx` high (bounded by `max-width: 100%`), preserve rounded corners/gold border, use `aspectFit`, and keep a thin deep-blue bottom action gradient. On ≤380px screens retain the complete image and keep both actions clear without exceeding the first screen.

- [ ] **Step 5: Run focused and compatibility tests.**

```bash
node src/pages/index/index.test.mjs
node scripts/ui-compat.test.mjs
node scripts/wechat-style-compat.test.mjs
```

Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
git add src/pages/index/index.vue src/pages/index/index.test.mjs
git commit -m "feat: make teacher portrait the full home hero"
```

## Chunk 2: Final WeChat verification

### Task 2: Verify the new visual in generated mini-program output

**Files:**
- No source changes expected.

- [ ] **Step 1: Run all regression tests.**

```bash
npm run test:config
```

Expected: PASS.

- [ ] **Step 2: Build the WeChat target.**

```bash
npm run build:mp-weixin
```

Expected: `DONE Build complete.`

- [ ] **Step 3: Inspect generated output.**

Confirm generated WXML contains the full-bleed hero without the old lead copy, generated WXSS contains zero `var(--*)`, and `/assets/teacher.jpg` plus `/assets/teacher-poster.jpg` return HTTP 200.

- [ ] **Step 4: Confirm the existing development watcher is active.**

Do not start a duplicate watcher. Verify the running `npm run dev:mp-weixin`/`uni -p mp-weixin` process has compiled the latest source into `dist/dev/mp-weixin`.

- [ ] **Step 5: Run repository checks.**

```bash
git diff --check -- .
git status --short
```

Expected: clean.
