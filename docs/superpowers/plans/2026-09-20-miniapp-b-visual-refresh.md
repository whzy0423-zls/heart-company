# 小程序 B 方向视觉刷新 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将小程序首页落地为已确认的“雾青 × 麦穗金”视觉和内容层级，同时保持现有业务行为不变。

**Architecture:** 复用首页现有动态老师、课程、语录和导航逻辑，只调整模板信息层级与 scoped CSS。全局主题补充 B 方向语义变量，首页通过现有 classroomEnabled 控制课程入口。

**Tech Stack:** uni-app、Vue 3 `<script setup>`、微信小程序 WXSS、Node.js 测试脚本。

---

## Chunk 1: Theme Tokens

**Files:**
- Modify: `miniapp/src/styles/apple-mobile.css`
- Test: `miniapp/src/styles/personal-expert-theme.test.mjs`

- [ ] **Step 1: Extend the root theme tokens** with semantic mist, teal, gold and surface values required by the refreshed home page.
- [ ] **Step 2: Run the theme contract test** with `node src/styles/personal-expert-theme.test.mjs` and confirm the single `:root` contract remains valid.

## Chunk 2: Home Information Hierarchy

**Files:**
- Modify: `miniapp/src/pages/index/index.vue`
- Test: `miniapp/src/pages/index/index.test.mjs`

- [ ] **Step 1: Add a regression assertion** that the home template exposes the primary test CTA, four tool labels, teacher trust section, and daily guidance copy.
- [ ] **Step 2: Run the focused test** and confirm it fails before the template update if the contract is absent.
- [ ] **Step 3: Update the template** to present the B-direction hero, primary test action, four tool cards, teacher section, and daily guidance while preserving existing handlers and dynamic content.
- [ ] **Step 4: Replace home scoped styles** with the mist/teal/gold visual system, responsive two-column tool grid, stable touch targets, pressed states, and reduced-motion support.
- [ ] **Step 5: Run the focused home tests** and the existing content/config tests.

## Chunk 3: Build Verification

**Files:**
- Generated: `miniapp/dist/build/mp-weixin/`

- [ ] **Step 1: Run `npm run build:mp-weixin`** from `miniapp`.
- [ ] **Step 2: Verify `project.config.json`** contains AppID `wx7d12bddbec8e17f7` and the output directory is importable.
- [ ] **Step 3: Run the full relevant test command** and inspect git diff for unrelated changes.
