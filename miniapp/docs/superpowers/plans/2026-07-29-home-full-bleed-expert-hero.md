# Home Full-Bleed Expert Hero Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the homepage split text/portrait hero with a full-width teacher portrait card that keeps only compact identity and conversion actions.

**Architecture:** Keep the existing `expertHero` view-model and preview handler unchanged. Refactor only the homepage template and scoped styles so the native portrait button becomes the full hero background, the long `lead` copy is not rendered, and the classroom/detail actions remain discoverable in a bottom gradient overlay.

**Tech Stack:** uni-app, Vue 3, WeChat mini-program native buttons/images, Node source-contract tests, scoped CSS/WXSS compatibility pipeline.

---

## Chunk 1: Full-bleed hero contract and implementation

### Task 1: Replace the split hero with a full-bleed portrait card

**Files:**
- Modify: `src/pages/index/index.test.mjs`
- Modify: `src/pages/index/index.vue`

- [ ] **Step 1: Write failing source-contract tests.**

Assert that:

- `.expert-hero` contains the native portrait preview button as its full visual surface.
- The former `.expert-hero__copy` and `view.expertHero.lead` are absent from the hero template.
- The bottom overlay still renders `view.expertHero.eyebrow`, `view.expertHero.title`, an “进入老师课堂” action, and the “查看完整导师介绍” affordance.
- The portrait still binds `portraitImage` identity and the preview handler still uses `detailImage`.
- The full-bleed card has a stable height, `aspectFill`, gold border, deep-blue gradient overlay, narrow-screen rules, and native 88rpx touch targets.

- [ ] **Step 2: Run the focused test and confirm RED.**

Run:

```bash
node src/pages/index/index.test.mjs
```

Expected: FAIL because the current hero still renders `.expert-hero__copy` and uses a narrow portrait.

- [ ] **Step 3: Implement the minimal template refactor.**

Use one full-width `expert-hero__portrait` button for image/detail preview. Move compact identity into its bottom overlay. Keep “进入老师课堂” as a separate native button positioned in the overlay without nesting it inside the preview button; use sibling overlay/action structure or a wrapper so the template has no nested interactive controls.

- [ ] **Step 4: Implement the scoped styles.**

Make the portrait fill the card (`width: 100%`, approximately `600rpx` high), preserve rounded corners/gold border, use `aspectFill`, and add a deep-blue bottom gradient for readable identity/actions. Remove obsolete split-layout rules and keep the ≤380px layout usable.

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
