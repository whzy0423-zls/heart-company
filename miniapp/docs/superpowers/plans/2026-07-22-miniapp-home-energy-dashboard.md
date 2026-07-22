# Miniapp Home Energy Dashboard Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the existing miniapp homepage as a modern, colorful energy dashboard while preserving all current routes, content emphasis, and H5/WeChat compatibility.

**Architecture:** Keep the homepage as one focused Vue single-file component because its state and interactions are small and page-specific. Replace the old repeated glass-card layout with four explicit sections—brand header, immersive hero, four energy entry cards, and a growth insight card—while retaining shared safe-area and button primitives from `apple-mobile.css`.

**Tech Stack:** uni-app, Vue 3 `<script setup>`, scoped CSS/rpx, Node static compatibility tests, Vite H5 and mp-weixin builds.

---

## Chunk 1: Homepage redesign and verification

### Task 1: Lock the new homepage behavior with static compatibility tests

**Files:**
- Modify: `scripts/ui-compat.test.mjs`
- Reference: `docs/superpowers/specs/2026-07-22-miniapp-home-energy-dashboard-design.md`

- [ ] **Step 1: Replace old homepage-specific selectors with assertions for the approved structure**

Update the homepage block so it requires:

```js
const indexPage = readFileSync('src/pages/index/index.vue', 'utf8')
const homeFeatureCards = (indexPage.match(/<view\b[^>]*>/g) || []).filter((tag) => {
  const classMatch = tag.match(/class=["']([^"']+)["']/)
  return classMatch?.[1].split(/\s+/).includes('energy-card')
})

assert.equal(homeFeatureCards.length, 4, 'home page should expose four energy dashboard entries')
for (const card of homeFeatureCards) {
  assert.match(card, /aria-label=["'][^"']+["']/, `home energy card should expose an accessibility label: ${card}`)
  assert.match(card, /role=["']button["']/, `home energy card should use button semantics: ${card}`)
  assert.match(card, /hover-class=["']energy-card--pressed["']/, `home energy card should expose press feedback: ${card}`)
}

assert.match(indexPage, /function goProfile\(\)/, 'home page should expose the growth archive/profile route')
assert.match(indexPage, /uni\.switchTab\(\{ url: '\/pages\/profile\/profile' \}\)/, 'growth archive should use switchTab')
assert.match(indexPage, /class=["'][^"']*home-nav__profile[^"']*["'][^>]*role=["']button["'][^>]*aria-label=["']打开我的成长档案["']/, 'home profile control should expose button semantics and an accessible name')
assert.match(indexPage, /class=["'][^"']*growth-card[^"']*["'][^>]*role=["']button["'][^>]*aria-label=["']打开老师课程与成长内容["']/, 'growth content should expose button semantics and an accessible name')
assert.match(indexPage, /class=["'][^"']*hero__wheel[^"']*["'][^>]*@error=["']hideWheel["']/, 'hero wheel should expose an image failure fallback')
assert.match(indexPage, /v-if=["']wheelVisible["']/, 'hero wheel should be removable after an image failure')
assert.match(indexPage, /class=["'][^"']*hero__wheel[^"']*["'][^>]*lazy-load/, 'hero wheel should retain lazy loading for project compatibility')
assert.match(indexPage, /\.hero__wheel\s*\{[\s\S]*width:\s*\d+rpx[\s\S]*height:\s*\d+rpx/, 'hero wheel should reserve fixed dimensions')
assert.match(indexPage, /\.hero__wheel-fallback\s*\{[\s\S]*width:\s*\d+rpx[\s\S]*height:\s*\d+rpx/, 'wheel fallback should reserve fixed dimensions')
assert.match(indexPage, /@media \(prefers-reduced-motion: reduce\)/, 'home page should disable decorative motion when reduced motion is requested')
assert.match(indexPage, /\.energy-card\s*\{[\s\S]*min-height:\s*(?:1[7-9]\d|[2-9]\d\d)rpx/, 'energy cards should keep a touch-friendly height')
assert.match(indexPage, /\.home-nav__profile\s*\{[\s\S]*min-width:\s*88rpx[\s\S]*min-height:\s*88rpx/, 'home profile control should keep an 88rpx touch target')
assert.match(indexPage, /老师|导师/, 'home page should emphasize teacher guidance')
assert.match(indexPage, /课件|课程/, 'home page should emphasize courseware and courses')
assert.doesNotMatch(indexPage, /AI 对话/, 'home page must not expose an AI chat entry')
```

Remove assertions tied specifically to `.grid__item`, `aria-pressed="false"`, and `grid__item--hover`. Keep the existing project-wide shared style, safe-area, teacher/course, and no-AI checks.

- [ ] **Step 2: Run the focused compatibility test and verify it fails**

Run:

```bash
node scripts/ui-compat.test.mjs
```

Expected: FAIL because `index.vue` does not yet contain four `.energy-card` entries and the wheel fallback.

- [ ] **Step 3: Commit the failing test**

```bash
git add scripts/ui-compat.test.mjs
git commit -m "test: define modern home dashboard contract"
```

### Task 2: Implement homepage structure and navigation

**Files:**
- Modify: `src/pages/index/index.vue:1-103`
- Test: `scripts/ui-compat.test.mjs`

- [ ] **Step 1: Extend the page state and existing navigation helpers**

Use a boolean ref for the wheel fallback and preserve existing route helpers:

```js
const total = ref(QUESTIONS.length)
const wheelVisible = ref(true)

function startTest() {
  uni.navigateTo({ url: '/pages/test/test' })
}

function goLearn() {
  uni.switchTab({ url: '/pages/learn/learn' })
}

function goRelation() {
  uni.navigateTo({ url: '/pages/relation/relation' })
}

function goProfile() {
  uni.switchTab({ url: '/pages/profile/profile' })
}

function hideWheel() {
  wheelVisible.value = false
}
```

- [ ] **Step 2: Replace the template with the approved four-section layout**

Keep the root classes `wrap home page-stack ios-page ios-safe-bottom`. Implement:

1. `.home-nav` with brand text and an accessible profile button-like view.
2. `.hero.card.ios-card` with kicker, concise title, supporting copy, one `.hero__cta`, decorative shapes, fixed-size lazy-loaded image, and `.hero__wheel-fallback`.
3. `.energy-grid` with exactly four `.energy-card` views for test, relation, learn, and profile. Each card must include `role="button"`, `aria-label`, `hover-class="energy-card--pressed"`, a CSS geometric icon, title, and one-line description.
4. `.growth-card` with teacher/course wording, `role="button"`, `aria-label="打开老师课程与成长内容"`, `hover-class="growth-card--pressed"`, and `@click="goLearn"`.

Use this exact accessibility and fallback branching shape:

```vue
<view class="home-nav">
  <view class="home-nav__copy"><text class="home-nav__brand">九型芯之力</text><text class="home-nav__tagline">认识自己，也读懂关系</text></view>
  <view class="home-nav__profile" role="button" aria-label="打开我的成长档案" hover-class="home-nav__profile--pressed" @click="goProfile">
    <view class="profile-icon"><view class="profile-icon__head"></view><view class="profile-icon__body"></view></view>
  </view>
</view>

<view class="hero card ios-card">
  <view class="hero__orb hero__orb--one"></view><view class="hero__orb hero__orb--two"></view>
  <view class="hero__copy">
    <text class="hero__kicker">老师导学 · 课件配套 · {{ total }} 题自测</text>
    <text class="hero__title">解锁你的\n人格能量</text>
    <text class="hero__lead">从核心动机出发，看见优势、压力与关系模式。</text>
    <button class="hero__cta ios-button" hover-class="hero__cta--pressed" @click="startTest">开始探索</button>
  </view>
  <view class="hero__visual">
    <image v-if="wheelVisible" class="hero__wheel" src="/static/wheel.png" mode="aspectFit" lazy-load @error="hideWheel" />
    <view v-else class="hero__wheel-fallback"><text>9</text></view>
  </view>
</view>
```

Each of the four energy cards follows this exact opening-tag contract, with the correct handler and modifier:

```vue
<view class="energy-card energy-card--test" role="button" aria-label="开始九型人格测试" hover-class="energy-card--pressed" @click="startTest">
  <view class="energy-card__icon energy-card__icon--test"><view class="energy-icon__ring"></view><view class="energy-icon__dot"></view></view>
  <text class="energy-card__title">人格测试</text><text class="energy-card__desc">找到你的核心动机</text>
</view>
```

Add the remaining three cards and the growth content with these exact contracts:

```vue
<view class="energy-card energy-card--relation" role="button" aria-label="打开九型关系合盘" hover-class="energy-card--pressed" @click="goRelation">
  <view class="energy-card__icon energy-card__icon--relation"><view class="energy-icon__person energy-icon__person--left"></view><view class="energy-icon__person energy-icon__person--right"></view></view>
  <text class="energy-card__title">关系合盘</text><text class="energy-card__desc">看见彼此的互动模式</text>
</view>
<view class="energy-card energy-card--learn" role="button" aria-label="打开老师课程与课件" hover-class="energy-card--pressed" @click="goLearn">
  <view class="energy-card__icon energy-card__icon--learn"><view class="energy-icon__line"></view><view class="energy-icon__line"></view><view class="energy-icon__line"></view></view>
  <text class="energy-card__title">老师课程</text><text class="energy-card__desc">跟着课件系统学习</text>
</view>
<view class="energy-card energy-card--profile" role="button" aria-label="打开我的成长档案" hover-class="energy-card--pressed" @click="goProfile">
  <view class="energy-card__icon energy-card__icon--profile"><view class="energy-icon__profile-head"></view><view class="energy-icon__profile-body"></view></view>
  <text class="energy-card__title">成长档案</text><text class="energy-card__desc">记录你的探索轨迹</text>
</view>
<view class="growth-card" role="button" aria-label="打开老师课程与成长内容" hover-class="growth-card--pressed" @click="goLearn">
  <view class="growth-card__visual"><view class="growth-card__spark"></view></view>
  <view class="growth-card__copy"><text class="growth-card__eyebrow">今日成长 · 老师导学</text><text class="growth-card__title">先觉察动机，再改变反应</text><text class="growth-card__desc">进入课程与课件，建立你的九型成长地图</text></view>
  <text class="growth-card__arrow">›</text>
</view>
```

Use only existing routes and `/static/wheel.png`. Do not introduce external assets, data requests, emoji icons, or new business behavior.

- [ ] **Step 3: Add baseline scoped styles required by the compatibility contract**

Before running the test, add these concrete baseline declarations to the scoped style block:

```css
.home-nav__profile {
  min-width: 88rpx;
  min-height: 88rpx;
}
.hero__wheel {
  width: 300rpx;
  height: 300rpx;
}
.hero__wheel-fallback {
  width: 300rpx;
  height: 300rpx;
}
.energy-card {
  min-height: 196rpx;
  transition: opacity .18s ease, transform .18s ease;
}
.energy-card--pressed,
.growth-card--pressed,
.home-nav__profile--pressed,
.hero__cta--pressed {
  opacity: .84;
  transform: scale(.98);
}
@media (prefers-reduced-motion: reduce) {
  .hero__visual,
  .hero__orb,
  .energy-card,
  .growth-card {
    animation: none;
    transition: none;
  }
}
```

- [ ] **Step 4: Run the focused compatibility test**

Run:

```bash
node scripts/ui-compat.test.mjs
```

Expected: `ui compatibility tests passed`. Task 3 enhances these passing styles without changing the contract.

- [ ] **Step 5: Commit the structural implementation**

```bash
git add src/pages/index/index.vue
git commit -m "feat: restructure miniapp home dashboard"
```

### Task 3: Implement the colorful energy visual system

**Files:**
- Modify: `src/pages/index/index.vue:105-end`
- Test: `scripts/ui-compat.test.mjs`

- [ ] **Step 1: Replace the baseline scoped block with the concrete approved CSS system**

Implement these exact layout, color, size, typography, fallback, and interaction declarations; icon-specific pseudo-elements may be appended immediately after this block using the dimensions described in Step 2:

```css
.home { --home-blue:#2563eb; --home-purple:#7c3aed; gap:24rpx; position:relative; }
.home::before { content:""; position:absolute; top:40rpx; right:-120rpx; width:360rpx; height:360rpx; border-radius:50%; background:rgba(124,58,237,.10); pointer-events:none; }
.home-nav { position:relative; z-index:1; display:flex; align-items:center; justify-content:space-between; gap:20rpx; }
.home-nav__copy { display:flex; flex-direction:column; gap:4rpx; }
.home-nav__brand { color:#0f172a; font-size:30rpx; font-weight:900; }
.home-nav__tagline { color:#64748b; font-size:21rpx; }
.home-nav__profile { min-width:88rpx; min-height:88rpx; border-radius:28rpx; display:flex; align-items:center; justify-content:center; background:rgba(255,255,255,.82); border:2rpx solid rgba(255,255,255,.96); box-shadow:0 16rpx 34rpx -26rpx rgba(37,99,235,.55); transition:opacity .18s ease,transform .18s ease; }
.hero { min-height:520rpx; padding:38rpx 32rpx; display:flex; overflow:hidden; background:linear-gradient(145deg,#1d4ed8 0%,#4f46e5 48%,#7c3aed 100%); border:0; border-radius:38rpx; box-shadow:0 34rpx 72rpx -38rpx rgba(67,56,202,.72); }
.hero__copy { position:relative; z-index:3; width:62%; display:flex; flex-direction:column; align-items:flex-start; }
.hero__kicker { color:#dbeafe; font-size:21rpx; font-weight:800; }
.hero__title { margin-top:18rpx; color:#fff; font-size:58rpx; line-height:1.08; font-weight:900; white-space:pre-line; }
.hero__lead { margin-top:18rpx; color:rgba(255,255,255,.82); font-size:25rpx; line-height:1.58; }
.hero__cta { min-width:190rpx; min-height:88rpx; margin-top:28rpx; padding:0 28rpx; border:0; border-radius:999rpx; background:#fff; color:#3730a3; font-size:27rpx; font-weight:900; box-shadow:0 18rpx 38rpx -22rpx rgba(15,23,42,.65); transition:opacity .18s ease,transform .18s ease; }
.hero__visual { position:absolute; z-index:2; right:-24rpx; bottom:10rpx; width:330rpx; height:330rpx; display:flex; align-items:center; justify-content:center; animation:hero-float 5s ease-in-out infinite; }
.hero__wheel { position:relative; z-index:2; width:300rpx; height:300rpx; filter:drop-shadow(0 26rpx 28rpx rgba(15,23,42,.28)); }
.hero__wheel-fallback { z-index:2; width:300rpx; height:300rpx; display:flex; align-items:center; justify-content:center; border:2rpx solid rgba(255,255,255,.7); border-radius:50%; color:#fff; font-size:88rpx; font-weight:300; background:rgba(255,255,255,.10); }
.hero__orb { position:absolute; border-radius:50%; pointer-events:none; }
.hero__orb--one { right:-90rpx; top:-120rpx; width:330rpx; height:330rpx; background:rgba(56,189,248,.24); }
.hero__orb--two { left:38%; bottom:-150rpx; width:280rpx; height:280rpx; background:rgba(236,72,153,.18); }
.section-heading { display:flex; align-items:flex-end; justify-content:space-between; padding:0 4rpx; }
.section-heading__title { color:#0f172a; font-size:34rpx; font-weight:900; }
.section-heading__meta { color:#64748b; font-size:22rpx; }
.energy-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:18rpx; }
.energy-card { position:relative; min-width:0; min-height:196rpx; padding:26rpx; border-radius:30rpx; display:flex; flex-direction:column; align-items:flex-start; justify-content:flex-end; overflow:hidden; color:#fff; box-shadow:0 24rpx 48rpx -34rpx rgba(15,23,42,.62); transition:opacity .18s ease,transform .18s ease; }
.energy-card::after { content:""; position:absolute; right:-30rpx; bottom:-48rpx; width:150rpx; height:150rpx; border:2rpx solid rgba(255,255,255,.18); border-radius:44rpx; transform:rotate(24deg); }
.energy-card--test { background:linear-gradient(145deg,#2563eb,#38bdf8); }
.energy-card--relation { background:linear-gradient(145deg,#7c3aed,#ec4899); }
.energy-card--learn { background:linear-gradient(145deg,#0f766e,#34d399); }
.energy-card--profile { background:linear-gradient(145deg,#ea580c,#fb7185); }
.energy-card__icon { position:absolute; top:22rpx; right:22rpx; width:72rpx; height:72rpx; border-radius:24rpx; background:rgba(255,255,255,.16); border:2rpx solid rgba(255,255,255,.26); }
.energy-card__title { position:relative; z-index:2; font-size:29rpx; font-weight:900; }
.energy-card__desc { position:relative; z-index:2; margin-top:8rpx; color:rgba(255,255,255,.80); font-size:21rpx; line-height:1.45; }
.growth-card { min-height:150rpx; padding:22rpx; display:flex; align-items:center; gap:20rpx; border-radius:30rpx; background:rgba(255,255,255,.84); border:2rpx solid rgba(255,255,255,.96); box-shadow:0 22rpx 48rpx -38rpx rgba(15,23,42,.50); transition:opacity .18s ease,transform .18s ease; }
.growth-card__visual { flex:0 0 112rpx; height:112rpx; border-radius:26rpx; background:linear-gradient(145deg,#7c3aed,#2563eb 55%,#38bdf8); position:relative; overflow:hidden; }
.growth-card__copy { min-width:0; flex:1; display:flex; flex-direction:column; gap:6rpx; }
.growth-card__eyebrow { color:#4f46e5; font-size:20rpx; font-weight:900; }
.growth-card__title { color:#0f172a; font-size:27rpx; font-weight:900; line-height:1.3; }
.growth-card__desc { color:#64748b; font-size:21rpx; line-height:1.45; }
.growth-card__arrow { color:#4f46e5; font-size:46rpx; font-weight:300; }
.energy-card--pressed,.growth-card--pressed,.home-nav__profile--pressed,.hero__cta--pressed { opacity:.84; transform:scale(.98); }
@keyframes hero-float { 0%,100%{transform:translateY(0)} 50%{transform:translateY(-10rpx)} }
@media (min-width:768px) { .hero{padding:46rpx 40rpx}.energy-card{padding:34rpx} }
@media (max-width:360px) { .hero__copy{width:68%}.hero__title{font-size:52rpx}.hero__visual{right:-68rpx}.energy-card{padding:22rpx}.energy-card__desc{font-size:20rpx} }
@media (prefers-reduced-motion:reduce) { .hero__visual,.hero__orb,.energy-card,.growth-card{animation:none;transition:none} }
```

- [ ] **Step 2: Add the exact CSS geometry for the five local icons**

Create the profile control silhouette with a `20rpx` circular head and `38rpx × 22rpx` rounded body. Inside energy cards, create: test with `42rpx` and `16rpx` concentric rings; relation with two overlapping `25rpx` bordered circles; learn with three centered `38rpx × 3rpx` lines separated by `10rpx`; profile with the same head/body silhouette; growth with two rotated translucent squares. All icon shapes use `rgba(255,255,255,.82)` borders/backgrounds and absolute centering.

- [ ] **Step 3: Verify the hero has one visually dominant CTA and every secondary surface has press feedback**

Confirm only `.hero__cta` uses a button element and solid high-contrast surface. The profile control, four energy cards, and growth card remain secondary button-like views with their exact `hover-class` values.

- [ ] **Step 4: Verify responsive and fallback layouts in source**

Confirm the grid is always two columns, the 768px rule only increases padding, the 360px rule prevents hero overlap, and `.hero__wheel`/`.hero__wheel-fallback` remain the same `300rpx` square.

- [ ] **Step 5: Run the compatibility test and verify it passes**

Run:

```bash
node scripts/ui-compat.test.mjs
```

Expected: `ui compatibility tests passed`.

- [ ] **Step 6: Commit the visual implementation**

```bash
git add src/pages/index/index.vue
git commit -m "style: add colorful energy home experience"
```

### Task 4: Full verification and visual QA

**Files:**
- Verify: `src/pages/index/index.vue`
- Verify: `scripts/ui-compat.test.mjs`

- [ ] **Step 1: Run the full configuration and unit test suite**

Run:

```bash
npm run test:config
```

Expected: all Node test files complete successfully, ending with their pass messages.

- [ ] **Step 2: Build H5**

Run:

```bash
npm run build:h5
```

Expected: Vite/uni-app production build exits with code 0 and writes `dist/build/h5`.

- [ ] **Step 3: Build the WeChat miniapp**

Run:

```bash
npm run build:mp-weixin
```

Expected: uni-app production build exits with code 0 and writes `dist/build/mp-weixin`.

- [ ] **Step 4: Inspect the H5 homepage at mobile widths**

Run:

```bash
npm run dev:h5 -- --host 127.0.0.1
```

Expected: the dev server prints a local H5 URL such as `http://127.0.0.1:5173/`. Open that URL with the available browser inspection tool and inspect at 375px and 390px widths. Confirm:

- no horizontal scroll;
- the hero CTA remains readable and unobstructed;
- the wheel or its fallback keeps the hero height stable;
- the grid remains two columns with comfortable gaps;
- the bottom growth card and tabBar safe area are fully visible;
- all seven clickable surfaces behave correctly: top profile → profile, hero CTA → test, four energy cards → test/relation/learn/profile, and growth card → learn.

- [ ] **Step 5: Review the final diff and commit any QA-only correction**

If QA changes are required, rerun `npm run test:config`, `npm run build:h5`, and `npm run build:mp-weixin`, then commit them:

```bash
git add src/pages/index/index.vue scripts/ui-compat.test.mjs
git commit -m "fix: polish miniapp home visual QA"
git diff --check
git status --short
```

Expected: no whitespace errors; only the user's pre-existing `.env.development` change may remain outside the task commits.
