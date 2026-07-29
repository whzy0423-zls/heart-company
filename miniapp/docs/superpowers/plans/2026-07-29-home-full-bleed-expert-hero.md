# Home Full-Bleed Expert Hero Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the homepage standalone portrait with the complete teacher detail poster, including its left-side introduction text, person, and bottom slogan, within the first screen.

**Architecture:** Keep the existing `expertHero` view-model and preview handler unchanged. Refactor only the homepage template and scoped styles so `detailImage` uses `aspectFit` inside an approximately `640 × 1140rpx` centered poster card after removing the homepage brand strip with no custom overlay. The poster itself opens the full-screen preview and the classroom action sits below it as a sibling native button.

**Tech Stack:** uni-app, Vue 3, WeChat mini-program native buttons/images, Node source-contract tests, scoped CSS/WXSS compatibility pipeline.

---

## Chunk 1: Full-bleed hero contract and implementation

### Task 1: Show the complete teacher detail poster in the first screen

**Files:**
- Modify: `src/pages/index/index.test.mjs`
- Modify: `src/pages/index/index.vue`

- [ ] **Step 1: Write failing source-contract tests.**

Assert that:

- The homepage no longer renders `.home-nav`; brand/profile framing remains on the profile page only.
- `.expert-hero` is a centered poster-format card near `640 × 1140rpx`, using most of the available page width with narrow side margins.
- The hero image binds `detailImage` for src/key/data-image with `mode="aspectFit"`; it does not bind `portraitImage`.
- The hero contains no custom eyebrow/title/lead/detail overlay, so the original poster text is unobstructed.
- The poster preview handler still uses `detailImage`; “进入老师课堂” is a separate native button below the poster.
- Gold/navy styling, narrow-screen rules, image-error isolation, and native 88rpx touch targets remain intact.

- [ ] **Step 2: Run the focused test and confirm RED.**

Run:

```bash
node src/pages/index/index.test.mjs
```

Expected: FAIL because the current hero still binds `portraitImage`, renders a detail affordance overlay, and keeps the classroom button inside the poster card.

- [ ] **Step 3: Implement the minimal template refactor.**

Bind the visible hero image to `detailImage`; remove the custom overlay entirely. Keep the poster preview button and classroom button as sibling native buttons, with the classroom button below the poster card.

- [ ] **Step 4: Implement the scoped styles.**

Center the poster at approximately `640rpx` wide and `1140rpx` high (bounded by `max-width: 100%`), preserve rounded corners/gold border, use `aspectFit`, and remove the bottom overlay gradient. Place the 88rpx classroom button below the poster; on ≤380px screens reduce the poster height proportionally so the complete image and button remain first-screen friendly.

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
