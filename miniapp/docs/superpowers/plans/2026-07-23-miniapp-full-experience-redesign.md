# Miniapp Full Experience Redesign Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the approved modern energy homepage into a cohesive, accessible redesign of the test, result, relation, learning, booking, and profile pages without changing backend contracts or core business behavior.

**Architecture:** Add a small shared visual foundation to `apple-mobile.css`, then keep each page's identity and layout in its existing scoped Vue styles. Introduce one pure report-display-state utility so the result page's price/unlock UI is explicit and testable; all other business logic remains in the existing page files.

**Tech Stack:** uni-app, Vue 3 `<script setup>`, scoped CSS/rpx, Node assertion tests, Vite H5 and mp-weixin builds.

---

## Chunk 1: Shared system and core journey pages

### Task 1: Add the shared page visual foundation

**Files:**
- Modify: `scripts/ui-compat.test.mjs`
- Modify: `src/styles/apple-mobile.css`
- Reference: `docs/superpowers/specs/2026-07-23-miniapp-full-experience-redesign-design.md`

- [ ] **Step 1: Add failing assertions for shared tokens and primitives**

Require these tokens in `apple-mobile.css`:

```js
for (const token of [
  '--nx-page-bg', '--nx-surface', '--nx-surface-soft', '--nx-line',
  '--nx-blue', '--nx-purple', '--nx-pink', '--nx-teal', '--nx-green',
  '--nx-orange', '--nx-danger',
]) {
  assert.match(appleMobileStyle, new RegExp(token), `shared styles should define ${token}`)
}
for (const className of ['.nx-page-hero', '.nx-section-head', '.nx-panel', '.nx-state', '.nx-tag', '.nx-focusable']) {
  assert.match(appleMobileStyle, new RegExp(className.replace('.', '\\.') + '\\s*\\{'), `shared styles should define ${className}`)
}
assert.match(appleMobileStyle, /\.nx-focusable:focus\s*\{[\s\S]*(?:outline|box-shadow)/, 'shared focusable controls should have visible focus')
assert.match(appleMobileStyle, /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.nx-focusable/, 'shared focusable motion should respect reduced motion')
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `node scripts/ui-compat.test.mjs`

Expected: FAIL on the first missing shared token or class, not a syntax error.

- [ ] **Step 3: Implement exact shared tokens and primitives**

Append to `apple-mobile.css`:

```css
:root {
  --nx-page-bg: #f3f6fc;
  --nx-surface: #ffffff;
  --nx-surface-soft: #f8faff;
  --nx-line: rgba(51, 65, 85, .10);
  --nx-blue: #2563eb;
  --nx-purple: #7c3aed;
  --nx-pink: #c0267e;
  --nx-teal: #0f766e;
  --nx-green: #15803d;
  --nx-orange: #c2410c;
  --nx-danger: #b42318;
}
.nx-page-hero { position:relative; overflow:hidden; border-radius:38rpx; padding:38rpx 32rpx; box-sizing:border-box; }
.nx-section-head { display:flex; flex-direction:column; gap:8rpx; padding:0 4rpx; }
.nx-panel { border-radius:30rpx; padding:30rpx; background:rgba(255,255,255,.88); border:2rpx solid rgba(255,255,255,.96); box-sizing:border-box; }
.nx-state { min-height:112rpx; display:flex; align-items:center; justify-content:center; gap:16rpx; color:#64748b; font-size:25rpx; line-height:1.55; }
.nx-tag { min-height:44rpx; display:inline-flex; align-items:center; padding:0 16rpx; border-radius:999rpx; font-size:21rpx; font-weight:800; }
.nx-focusable { transition:opacity .18s ease, transform .18s ease, box-shadow .18s ease; }
.nx-focusable:focus { outline:4rpx solid rgba(37,99,235,.34); outline-offset:4rpx; }
.nx-focusable--pressed { opacity:.84; transform:scale(.98); }
@media (prefers-reduced-motion: reduce) { .nx-focusable { animation:none; transition:none; } }
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run: `node scripts/ui-compat.test.mjs`

Expected: `ui compatibility tests passed`.

- [ ] **Step 5: Commit**

```bash
git add scripts/ui-compat.test.mjs src/styles/apple-mobile.css
git commit -m "style: add shared miniapp experience tokens"
```

### Task 2: Redesign the test page

**Files:**
- Modify: `scripts/ui-compat.test.mjs`
- Modify: `src/pages/test/test.vue`

- [ ] **Step 1: Add failing test-page structure assertions**

Require the existing behavioral functions (`answerLocked`, `clearAdvanceTimer`, `onUnload`) plus:

```js
assert.match(testPage, /class=["'][^"']*test-hero[^"']*nx-page-hero/, 'test page should use the blue-purple hero')
assert.match(testPage, /class=["'][^"']*gender__card[^"']*nx-focusable/, 'gender choices should use shared focus behavior')
assert.match(testPage, /const total = QUESTIONS\.length/, 'test page should expose a stable total question count')
assert.match(testPage, /class=["'][^"']*quiz__progress-meta[^"']*["'][^>]*:aria-label=["']`第 \$\{step \+ 1\} 题，共 \$\{total\} 题`["']/, 'quiz should expose question and total progress text')
assert.match(testPage, /<button\b[^>]*class=["']quiz__opt nx-focusable["'][^>]*>/, 'quiz choices should use focusable option panels')
assert.match(testPage, /\.quiz__opt\.on\s*\{[\s\S]*(?:border|box-shadow)/, 'selected answer should have non-color-only emphasis')
```

- [ ] **Step 2: Run `node scripts/ui-compat.test.mjs` and verify RED**

Expected: FAIL because `test-hero` and progress metadata are absent.

- [ ] **Step 3: Implement the approved template structure**

Keep all current stage, scoring, locking, timer and navigation logic. Add `const total = QUESTIONS.length`, then replace only template/style structure:

- root unchanged: `wrap test page-stack ios-page ios-safe-bottom`;
- gender stage contains `.test-hero.nx-page-hero`, title, explanation, and two native button cards;
- quiz stage contains `.quiz-shell`, `.quiz__progress-meta` with `第 {{ step + 1 }} 题 / 共 {{ total }} 题`, exact `:aria-label="`第 ${step + 1} 题，共 ${total} 题`"`, existing progress bar, question, options and previous button;
- option opening tag uses `class="quiz__opt nx-focusable"` and preserves existing `:class`, `:disabled`, `:aria-label` and answer handler.

Use `button` for gender/options/back. Do not convert to clickable views.

- [ ] **Step 4: Apply exact visual rules**

- page ambient blue-purple glow;
- hero gradient `#1d4ed8 → #6d28d9`, white copy, 34–38rpx radius;
- gender cards two columns, min-height 230rpx, male `#155e75 → #1d4ed8`, female `#7e22ce → #be185d`;
- quiz shell uses a light surface without the old generic `.card` appearance;
- options min-height 112rpx, 24rpx radius, selected state uses `4rpx` blue-purple border plus inset check ring;
- all secondary text `#64748b` or darker;
- `@media (max-width:360px)` reduces question size, not touch height.

- [ ] **Step 5: Run test and builds**

Run:

```bash
node scripts/ui-compat.test.mjs
npm run build:h5
npm run build:mp-weixin
```

Expected: all exit 0, apart from existing non-blocking warnings.

- [ ] **Step 6: Commit**

```bash
git add scripts/ui-compat.test.mjs src/pages/test/test.vue
git commit -m "style: redesign miniapp test journey"
```

### Task 3: Redesign relation selection and results

**Files:**
- Modify: `scripts/ui-compat.test.mjs`
- Modify: `src/pages/relation/relation.vue`

- [ ] **Step 1: Add failing relation visual-contract assertions**

Add exact assertions:

```js
assert.match(relationPage, /class=["'][^"']*relation-hero[^"']*nx-page-hero/, 'relation pick stage should use a themed hero')
assert.equal((relationPage.match(/class=["'][^"']*type-picker[^"']*["']/g) || []).length, 2, 'relation should render two type pickers')
assert.match(relationPage, /<button\b[\s\S]*class=["']type-chip nx-focusable["'][\s\S]*:aria-label=/, 'relation type chips should keep native button labels')
assert.match(relationPage, /:aria-pressed=["'](?:myType|taType) === t\.id["']/, 'relation type chips should expose selected state')
assert.match(relationPage, /class=["']type-chip__selected["']>已选</, 'selected chip should include a text marker')
assert.match(relationPage, /const myAvatarFailed = ref\(false\)/, 'relation should track my avatar failure')
assert.match(relationPage, /const taAvatarFailed = ref\(false\)/, 'relation should track TA avatar failure')
assert.match(relationPage, /class=["']pair__avatar-fallback["']/, 'relation should render fixed avatar fallbacks')
assert.match(relationPage, /class=["']pair-connection["']/, 'relation result should render the connection visual')
for (const modifier of ['insight--bond', 'insight--friction', 'insight--tip']) {
  assert.match(relationPage, new RegExp(modifier), `relation result should include ${modifier}`)
}
```

Keep every existing invalid-query, redirect, validation, 88rpx, gap, lazy-image and press-feedback assertion.

- [ ] **Step 2: Run focused test and verify RED**

Run: `node scripts/ui-compat.test.mjs`

Expected: FAIL on missing relation hero/insight classes.

- [ ] **Step 3: Implement selection template**

- purple-pink hero with title and explanation;
- two `.type-picker.nx-panel` groups;
- nine native button chips remain 88rpx minimum, include number and abbreviated name;
- selected chip renders `.type-chip__selected` text `已选` in addition to border/color;
- each chip keeps `:aria-label`, `:aria-pressed`, `hover-class`, and `@click` on the same native button;
- retain one existing primary analyze button.

- [ ] **Step 4: Implement result template and styles**

- add `const myAvatarFailed = ref(false)` and `const taAvatarFailed = ref(false)`;
- `analyze()` resets both flags before entering result; `reset()` also resets both;
- add `onMyAvatarError()` / `onTaAvatarError()` handlers that set their flag;
- `.pair.nx-page-hero` contains two fixed-size lazy images with `v-if="!myAvatarFailed"` / `v-if="!taAvatarFailed"`, `@error` handlers, and `v-else` `.pair__avatar-fallback` using the type number; image and fallback use the same width/height;
- center `.pair-connection` contains score and label;
- bond/friction/tip use separate insight panels and existing analysis strings;
- drives use a two-column `.drive-pair`;
- preserve reset, disclaimer and redirecting state.

Use purple/pink for connection, orange/coral for friction, teal/green for advice. Keep normal text contrast >=4.5:1.

- [ ] **Step 5: Run the focused test**

```bash
node scripts/ui-compat.test.mjs
```

Expected: `ui compatibility tests passed`.

- [ ] **Step 6: Build both platforms**

```bash
npm run build:h5
npm run build:mp-weixin
```

Expected: both exit 0; only existing circular-dependency/update warnings.

- [ ] **Step 7: Commit**

```bash
git add scripts/ui-compat.test.mjs src/pages/relation/relation.vue
git commit -m "style: redesign miniapp relation experience"
```

## Chunk 2: Result identity and report state

### Task 4: Extract and test the report display state

**Files:**
- Create: `src/utils/reportDisplayState.js`
- Create: `src/utils/reportDisplayState.test.mjs`
- Modify: `package.json`

- [ ] **Step 1: Write the failing pure-state tests**

Test exact cases:

```js
assert.deepEqual(reportDisplayState({ recordId: '', loading: false, error: '', unlocked: false, priceCents: null }), { key: 'needs-save', priceCents: null })
assert.equal(reportDisplayState({ recordId: 'r1', loading: true, error: '', unlocked: false, priceCents: null }).key, 'status-loading')
assert.equal(reportDisplayState({ recordId: 'r1', loading: false, error: '失败', unlocked: false, priceCents: null }).key, 'status-error')
assert.deepEqual(reportDisplayState({ recordId: 'r1', loading: false, error: '', unlocked: false, priceCents: 990 }), { key: 'ready', priceCents: 990 })
assert.equal(reportDisplayState({ recordId: 'r1', loading: false, error: '', unlocked: false, priceCents: 0 }).key, 'status-error')
assert.equal(reportDisplayState({ recordId: 'r1', loading: false, error: '', unlocked: true, priceCents: null }).key, 'unlocked')
assert.equal(reportDisplayState({ recordId: '', loading: true, error: '失败', unlocked: true, priceCents: null }).key, 'unlocked')
```

The last assertion locks the priority: unlocked does not require price.

- [ ] **Step 2: Run test and verify RED**

Run: `node src/utils/reportDisplayState.test.mjs`

Expected: module-not-found or missing export.

- [ ] **Step 3: Implement the pure state function**

```js
export function reportDisplayState({ recordId, loading, error, unlocked, priceCents }) {
  if (unlocked) return { key: 'unlocked', priceCents: null }
  if (!recordId) return { key: 'needs-save', priceCents: null }
  if (loading) return { key: 'status-loading', priceCents: null }
  if (error) return { key: 'status-error', priceCents: null }
  if (Number.isFinite(priceCents) && priceCents > 0) return { key: 'ready', priceCents }
  return { key: 'status-error', priceCents: null }
}
```

- [ ] **Step 4: Add the test to `test:config`**

Insert the new test immediately after `resultPersona.test.mjs` in `package.json`.

- [ ] **Step 5: Run the pure test and verify GREEN**

Run: `node src/utils/reportDisplayState.test.mjs`

Expected: `report display state tests passed` and exit 0.

- [ ] **Step 6: Commit**

```bash
git add src/utils/reportDisplayState.js src/utils/reportDisplayState.test.mjs package.json
git commit -m "test: define result report display states"
```

### Task 5: Redesign result content and integrate report states

**Files:**
- Modify: `scripts/ui-compat.test.mjs`
- Modify: `src/pages/result/result.vue`
- Test: `src/utils/reportDisplayState.test.mjs`

- [ ] **Step 1: Add failing result-page contract assertions**

Add concrete assertions:

```js
assert.match(resultPage, /import \{ reportDisplayState \} from ['"]\.\.\/\.\.\/utils\/reportDisplayState['"]/, 'result page should use the pure report state')
assert.match(resultPage, /const reportPriceCents = ref\(null\)/, 'report price should not be prefilled')
assert.match(resultPage, /const reportStatusLoading = ref\(false\)/, 'result page should track report status loading')
assert.match(resultPage, /const reportStatusError = ref\(['"]['"]\)/, 'result page should track report status errors')
assert.match(resultPage, /const reportState = computed\(/, 'result page should derive the five-state report display')
for (const state of ['needs-save', 'status-loading', 'status-error', 'ready']) {
  assert.match(resultPage, new RegExp(`reportState\\.key === ['"]${state}['"]`), `result page should render ${state}`)
}
for (const className of ['result-hero', 'drive-grid', 'center-panel', 'direction-grid', 'report-panel']) {
  assert.match(resultPage, new RegExp(className), `result page should include ${className}`)
}
assert.match(resultPage, /const avatarFailed = ref\(false\)/, 'result avatar should have a failure flag')
assert.match(resultPage, /class=["'][^"']*result-hero__avatar--fallback/, 'result avatar should have a fixed fallback')
assert.match(resultPage, /#ifdef H5[\s\S]*disabled[^>]*>请在微信小程序内登录后保存<\/button>/, 'H5 save should be disabled guidance')
assert.match(resultPage, /#ifdef H5[\s\S]*disabled[^>]*>请在微信小程序内完成存档与支付<\/button>/, 'H5 payment should be disabled guidance')
assert.match(resultPage, /#ifdef MP-WEIXIN[\s\S]*open-type=["']share["']/, 'share should remain MP-only')
assert.doesNotMatch(resultPage, /const reportPriceCents = ref\(990\)/, 'result page must not show a default price')
```

Preserve existing poster conditional compilation, report retry, cached-result validation, payment and error-normalization assertions.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
node src/utils/reportDisplayState.test.mjs
node scripts/ui-compat.test.mjs
```

Expected: pure utility passes; UI test fails on result integration.

- [ ] **Step 3: Integrate report status refs and computed state**

Change script state to:

```js
const reportPriceCents = ref(null)
const reportStatusLoading = ref(false)
const reportStatusError = ref('')
const reportState = computed(() => reportDisplayState({
  recordId: recordId.value,
  loading: reportStatusLoading.value,
  error: reportStatusError.value,
  unlocked: reportUnlocked.value,
  priceCents: reportPriceCents.value,
}))
```

`refreshReportStatus()` must set loading true/error empty; call the existing API; set unlocked first; only assign price when finite and positive; set `reportStatusError` on failure or invalid price while locked; load content when unlocked; clear loading in `finally`. Saved-record retries remain available in MP when status fails.

`reportPriceYuan` returns an empty string unless the value is finite and positive.

- [ ] **Step 4: Implement identity, drive, center, and direction template sections**

Use this hierarchy before the report panel:

```vue
<view class="result-hero nx-page-hero" :class="'result-hero--' + info.color">
  <view class="result-hero__avatar-wrap">
    <image v-if="!avatarFailed" class="result-hero__avatar" :src="`/static/avatars/${result.type}.png`" mode="aspectFill" lazy-load @error="avatarFailed = true" />
    <view v-else class="result-hero__avatar result-hero__avatar--fallback">{{ result.type }}</view>
  </view>
  <text class="result-hero__title">{{ r.title }}</text>
  <text class="result-hero__meta">{{ info.en }} · {{ info.keywords }}</text>
  <text class="result-hero__summary">{{ r.summary }}</text>
  <view class="result-hero__persona">{{ persona }}</view>
</view>
<view class="drive-grid">
  <view class="drive-card drive-card--fear"><text class="drive-card__label">基本恐惧</text><text class="drive-card__text">{{ info.fear }}</text></view>
  <view class="drive-card drive-card--desire"><text class="drive-card__label">核心欲望</text><text class="drive-card__text">{{ info.desire }}</text></view>
</view>
<view class="center-panel nx-panel">
  <view class="nx-section-head"><text class="sec-title">你的三中心分布</text><text class="section-note">能量会在不同中心间流动</text></view>
  <view v-for="c in result.centers" :key="c.key" class="bar">
    <text class="bar__name">{{ c.name }}</text><view class="bar__track"><view class="bar__fill" :class="'bar__fill--' + c.key" :style="{ width: c.pct + '%' }" /></view><text class="bar__pct">{{ c.pct }}%</text>
  </view>
</view>
<view v-if="secondInfo" class="secondary-panel nx-panel">
  <text class="sec-title">{{ wing ? '你的侧翼倾向' : '你的副型倾向' }}</text>
  <text class="sec-txt">主型 {{ result.type }} 号 {{ info.name }}，副型 {{ result.second }} 号 {{ secondInfo.name }} 特质也很突出，让你更立体。</text>
  <text class="sec-kw">{{ secondInfo.keywords }}</text>
</view>
<view class="direction-grid">
  <view class="direction-card direction-card--stress"><text class="direction-card__label">压力下</text><text class="direction-card__type">{{ info.stress }} 号 · {{ stressInfo.name }}</text></view>
  <view class="direction-card direction-card--growth"><text class="direction-card__label">成长时</text><text class="direction-card__type">{{ info.growth }} 号 · {{ growthInfo.name }}</text></view>
</view>
<view class="growth-insight nx-panel"><text class="sec-title">成长建议</text><text class="sec-txt">{{ r.growth }}</text></view>
```

Add `const avatarFailed = ref(false)`. Preserve all current data bindings and center bar widths.

- [ ] **Step 5: Implement the five mutually exclusive report branches**

- hero uses type color class derived from existing `info.color` and fixed avatar fallback;
- fear/desire `.drive-grid`;
- centers `.center-panel.nx-panel`;
- wing/secondary and stress/growth use different layouts;
- report branches exactly by `reportState.key`:
  - `needs-save`: explanatory copy; MP save button; H5 disabled save guidance;
  - `status-loading`: local loading text with `aria-live="polite"`;
  - `status-error`: retry `refreshReportStatus` in MP; H5 guidance if relevant;
  - `ready`: MP price/unlock button; H5 disabled payment guidance;
  - `unlocked`: existing content loading/error/content/view behavior.

Remove the existing standalone save/archive primary button from the later action list. Save appears only inside `needs-save`; unlock appears only in `ready`. In `status-loading`, `status-error`, and `unlocked`, no second archive primary button is rendered.

Use exact conditional blocks:

```vue
<view class="report-panel">
  <view class="report-panel__head"><text class="report-panel__title">AI 深度性格报告</text><text v-if="reportState.key === 'unlocked'" class="report-panel__badge">已解锁</text></view>
  <template v-if="reportState.key === 'needs-save'">
    <text class="report-panel__intro">先保存基础结果，再查询你的专属报告价格。</text>
    <!-- #ifdef H5 -->
    <button class="report-panel__cta report-panel__cta--disabled ios-button" disabled>请在微信小程序内登录后保存</button>
    <!-- #endif -->
    <!-- #ifndef H5 -->
    <button class="report-panel__cta ios-button" :loading="saving" :disabled="saving" @click="saveRecord">{{ saving ? '正在存档' : '存入档案并查看价格' }}</button>
    <!-- #endif -->
  </template>
  <view v-else-if="reportState.key === 'status-loading'" class="report-status" aria-live="polite">正在查询报告状态…</view>
  <template v-else-if="reportState.key === 'status-error'">
    <text class="report-status report-status--error">{{ reportStatusError || '报告价格获取失败' }}</text>
    <!-- #ifndef H5 -->
    <button class="report-panel__secondary ios-button" :disabled="reportStatusLoading" @click="refreshReportStatus">重新查询</button>
    <!-- #endif -->
  </template>
  <template v-else-if="reportState.key === 'ready'">
    <text class="report-panel__price">￥{{ reportPriceYuan }}</text><text class="report-panel__intro">生成专属性格画像、成长盲点、人际与职业建议。</text>
    <!-- #ifdef H5 -->
    <button class="report-panel__cta report-panel__cta--disabled ios-button" disabled>请在微信小程序内完成存档与支付</button>
    <!-- #endif -->
    <!-- #ifndef H5 -->
    <button class="report-panel__cta ios-button" :loading="paying" :disabled="paying" @click="unlockReport">￥{{ reportPriceYuan }} 解锁深度报告</button>
    <!-- #endif -->
  </template>
  <template v-else>
    <view v-if="reportLoading" class="report-status" aria-live="polite">报告生成中，请稍候…</view>
    <view v-else-if="reportError" class="report-status report-status--error"><text>{{ reportError }}</text><button class="report-panel__secondary ios-button" :disabled="reportLoading" @click="loadReportContent">重试</button></view>
    <text v-else-if="reportContent" class="report-panel__content">{{ reportContent }}</text>
    <button v-else class="report-panel__secondary ios-button" @click="loadReportContent">查看报告</button>
  </template>
</view>
```

MP `needs-save` binds `saveRecord`; H5 renders the disabled save guidance without `@click`. MP `ready` binds `unlockReport`; H5 renders disabled payment guidance. MP status-error retry binds `refreshReportStatus` whenever `recordId` exists.

- [ ] **Step 6: Rebuild the remaining action hierarchy and platform guards**

Move share button into `#ifdef MP-WEIXIN`. Keep poster blocks and canvas logic unchanged. After the report panel, render exactly:

```vue
<view class="result-actions">
  <!-- #ifdef MP-WEIXIN -->
  <view class="result-actions__share"><button class="btn-ghost ios-button" open-type="share">分享好友</button><button class="btn-ghost ios-button" @click="makePoster">生成海报</button></view>
  <!-- #endif -->
  <!-- #ifdef H5 -->
  <button class="btn-ghost ios-button" disabled>小程序内生成海报</button>
  <!-- #endif -->
  <button class="btn-soft ios-button" @click="goRelation">和 TA 合盘 · 看关系</button>
  <button class="btn-ghost ios-button" @click="goBooking">预约深入解读</button>
  <button class="result-actions__restart" @click="restart">重新测试</button>
</view>
```

There is no archive or unlock button in `result-actions`.

- [ ] **Step 7: Style hero and interpretation sections**

- `.result-hero`: min-height 520rpx, 38rpx radius, 184rpx avatar/fallback, white text; define existing `info.color` modifier classes using blue/purple/orange/teal/coral families;
- `.drive-grid` and `.direction-grid`: two equal columns, gap >=16rpx, min-width 0;
- fear uses orange/coral tint, desire blue/purple, stress orange, growth teal;
- `.center-panel` uses light surface; bar tracks 16rpx high and retain percentage labels;
- `.growth-insight` uses pale teal surface and dark text.

Use fixed dimensions for avatar/fallback. Map hero classes from existing `info.color`; define only those existing class names in scoped CSS. All report text on the dark panel must use white or >=90%-white.

- [ ] **Step 8: Style report, actions, and poster accessibility**

- `.report-panel`: background `linear-gradient(145deg,#111827,#312e81)`, radius 34rpx, padding 34rpx, white text;
- `.report-panel__cta`: solid white or accessible coral, min-height 88rpx; disabled state visibly distinct;
- `.report-panel__secondary`: transparent white border, min-height 88rpx;
- `.result-actions`: grouped vertical spacing; share row two equal columns; restart is low-emphasis native button with 88rpx target;
- poster close remains visible and receives `aria-label="关闭海报"`.

- [ ] **Step 9: Run pure and UI tests**

```bash
node src/utils/reportDisplayState.test.mjs
node scripts/ui-compat.test.mjs
```

Expected: `report display state tests passed` and `ui compatibility tests passed`.

- [ ] **Step 10: Build both platforms**

```bash
npm run build:h5
npm run build:mp-weixin
```

Expected: both exit 0; only documented existing warnings.

- [ ] **Step 11: Commit**

```bash
git add scripts/ui-compat.test.mjs src/pages/result/result.vue
git commit -m "feat: redesign result and report experience"
```

## Chunk 3: Learning, booking, and profile

### Task 6: Redesign the learning center

**Files:**
- Modify: `scripts/ui-compat.test.mjs`
- Modify: `src/pages/learn/learn.vue`

- [ ] **Step 1: Add failing learning-page assertions**

Add:

```js
assert.match(learnPage, /class=["'][^"']*learn-hero[^"']*nx-page-hero/, 'learn page should use a teal hero')
for (const className of ['teacher-media', 'course-media', 'quote-editorial', 'type-badge-grid']) {
  assert.match(learnPage, new RegExp(className), `learn page should include ${className}`)
}
assert.match(learnPage, /先完成测试，建立你的学习地图/, 'learn page should keep the fixed test CTA')
assert.match(learnPage, /const teacherImageErrors = ref\(\{\}\)/, 'learn page should track teacher image failures')
assert.match(learnPage, /const courseImageErrors = ref\(\{\}\)/, 'learn page should track course image failures')
assert.match(learnPage, /const typeImageErrors = ref\(\{\}\)/, 'learn page should track type image failures')
for (const fallback of ['teacher-media__fallback', 'course-media__fallback', 'type-badge__fallback']) {
  assert.match(learnPage, new RegExp(fallback), `learn page should render ${fallback}`)
}
assert.match(learnPage, /getStoredSiteConfig/, 'learn page should render cached content first')
assert.match(learnPage, /refreshSiteConfig/, 'learn page should refresh in background')
assert.match(learnPage, /silent/, 'cached refresh should stay non-blocking')
assert.match(learnPage, /catch \(e\)[\s\S]*if \(!silent\)[\s\S]*teachers\.value = normalizeTeachers\(\)/, 'only a non-silent failure should replace visible learning content')
assert.match(learnPage, /v-if=["']loading["']/, 'first load should show loading state')
assert.match(learnPage, /v-else-if=["']loadError["']/, 'first-load failure should render before content')
assert.match(learnPage, /@click=["']loadContent["']/, 'failure state should retry')
assert.match(learnPage, /quotes\.length === 0/, 'empty quotes should have an explicit state')
```

- [ ] **Step 2: Run UI test and verify RED**

Run: `node scripts/ui-compat.test.mjs`

Expected: FAIL on missing `learn-hero` or image-fallback state.

- [ ] **Step 3: Implement script fallback maps and the exact media template**

- teal-green hero with title and short lead;
- teacher cards: fixed 112rpx avatar area; on error show first character or `师`;
- course cards: fixed `220rpx × 150rpx` cover on mobile; fallback gradient displays badge/index;
- quotes use large opening mark and readable copy;
- types become two/three-column compact badges using existing `TYPES_INFO.color` class and fixed avatar fallback;
- bottom CTA keeps existing `goTest` route and exact approved text.

Add exact failure maps and setters:

```js
const teacherImageErrors = ref({})
const courseImageErrors = ref({})
const typeImageErrors = ref({})
function markTeacherImageError(key) { teacherImageErrors.value = { ...teacherImageErrors.value, [key]: true } }
function markCourseImageError(key) { courseImageErrors.value = { ...courseImageErrors.value, [key]: true } }
function markTypeImageError(id) { typeImageErrors.value = { ...typeImageErrors.value, [id]: true } }
```

Images use `v-if="!…Errors[key]"`, existing `lazy-load`, and `@error`; `v-else` fallbacks occupy the same dimensions. Type avatars must use `typeImageErrors[t.id]`, not rely on teacher/course state.

Use this hierarchy:

```vue
<view class="learn-hero nx-page-hero"><text class="learn-hero__eyebrow">学习中心</text><text class="learn-hero__title">跟着老师，把九型用进生活</text><text class="learn-hero__lead">从老师资料、课程课件到九型图鉴，建立自己的成长地图。</text></view>
<view class="learn-section nx-panel">
  <view class="nx-section-head"><text class="sec-title">老师资料</text><text class="section-note">跟着老师系统学习</text></view>
  <view v-if="loading" class="nx-state">老师资料加载中…</view>
  <view v-else-if="loadError" class="nx-state nx-state--error"><text>{{ loadError }}</text><button class="retry nx-focusable" hover-class="retry--hover" @click="loadContent">重新加载</button></view>
  <view v-for="teacher in teachers" :key="teacher.name" class="teacher-media">
    <image v-if="!teacherImageErrors[teacher.name]" class="teacher-media__avatar" :src="teacher.avatar" mode="aspectFill" lazy-load @error="markTeacherImageError(teacher.name)" />
    <view v-else class="teacher-media__avatar teacher-media__fallback">{{ teacher.name ? teacher.name.slice(0, 1) : '师' }}</view>
    <view class="teacher-media__body"><text class="teacher-media__name">{{ teacher.name }}</text><text class="teacher-media__title">{{ teacher.title }}</text><text class="teacher-media__bio">{{ teacher.bio }}</text><view class="teacher-media__tags"><text v-for="tag in teacher.tags" :key="tag" class="nx-tag teacher-media__tag">{{ tag }}</text></view></view>
  </view>
</view>
<view class="learn-section nx-panel">
  <view class="nx-section-head"><text class="sec-title">课程与课件</text><text class="section-note">循序渐进理解九型</text></view>
  <view v-if="loading" class="nx-state">课件内容加载中…</view>
  <view v-for="(c, i) in coursewareItems" :key="c.title + i" class="course-media">
    <image v-if="!courseImageErrors[c.title + i]" class="course-media__cover" :src="c.cover" mode="aspectFill" lazy-load @error="markCourseImageError(c.title + i)" />
    <view v-else class="course-media__cover course-media__fallback">{{ c.badge || (i + 1) }}</view>
    <view class="course-media__body"><view class="course-media__meta"><text class="nx-tag">{{ c.badge || (i + 1) }}</text><text v-if="c.duration">{{ c.duration }}</text></view><text class="course-media__title">{{ c.title }}</text><text class="course-media__desc">{{ c.description }}</text></view>
  </view>
</view>
<view class="learn-section nx-panel"><view class="nx-section-head"><text class="sec-title">老韩语录</text></view><view v-if="loading" class="nx-state">语录内容加载中…</view><view v-else-if="!loadError && quotes.length === 0" class="nx-state">语录内容即将上线</view><view v-for="quote in quotes" :key="quote" class="quote-editorial"><text class="quote-editorial__mark">“</text><text class="quote-editorial__text">{{ quote }}</text></view></view>
<view class="learn-section nx-panel"><view class="nx-section-head"><text class="sec-title">九种性格图鉴</text></view><view class="type-badge-grid"><view v-for="t in types" :key="t.id" class="type-badge" :class="'type-badge--' + t.color"><image v-if="!typeImageErrors[t.id]" class="type-badge__avatar" :src="`/static/avatars/${t.id}.png`" mode="aspectFill" lazy-load @error="markTypeImageError(t.id)" /><view v-else class="type-badge__avatar type-badge__fallback">{{ t.id }}</view><text class="type-badge__num">{{ t.id }}</text><text class="type-badge__name">{{ t.name }}</text><text class="type-badge__kw">{{ t.keywords }}</text></view></view></view>
<button class="btn-primary ios-button learn-cta" @click="goTest">先完成测试，建立你的学习地图</button>
```

Do not add click behavior to course cards or a tested-state branch.

- [ ] **Step 4: Apply concrete learning styles**

Use `.learn-hero` gradient `#0f766e → #15803d`, radius 38rpx, white text. `.learn-section` is a vertical flex container with 22rpx gap. `.teacher-media` is flex with 112rpx fixed avatar/fallback; `.course-media` is flex with `220rpx × 150rpx` fixed cover/fallback; both use 18rpx gap and `min-width:0` bodies. `.quote-editorial` uses pale green background, 30rpx padding and a 54rpx mark. `.type-badge-grid` is two columns below 768px and three columns at/above 768px; cards min-height 190rpx. Retry remains native button, 88rpx minimum. All secondary copy uses `#64748b` or darker.

- [ ] **Step 5: Run the focused test**

Run: `node scripts/ui-compat.test.mjs`

Expected: `ui compatibility tests passed`.

- [ ] **Step 6: Build H5**

Run: `npm run build:h5`

Expected: exit 0 with only documented warnings.

- [ ] **Step 7: Build mp-weixin**

Run: `npm run build:mp-weixin`

Expected: exit 0 with only documented warnings.

- [ ] **Step 8: Commit**

```bash
git add scripts/ui-compat.test.mjs src/pages/learn/learn.vue
git commit -m "style: redesign miniapp learning center"
```

### Task 7: Redesign booking with explicit H5 guard

**Files:**
- Modify: `scripts/ui-compat.test.mjs`
- Modify: `src/pages/booking/booking.vue`

- [ ] **Step 1: Add failing booking assertions**

```js
assert.match(bookingPage, /class=["'][^"']*booking-hero[^"']*nx-page-hero/, 'booking should use the orange-blue hero')
assert.equal((bookingPage.match(/class=["'][^"']*form-section[^"']*["']/g) || []).length, 3, 'booking should group fields into three sections')
assert.doesNotMatch(bookingPage, /⌄/, 'booking picker should use a CSS arrow')
assert.match(bookingPage, /#ifdef H5[\s\S]*<button[^>]*disabled[^>]*>请在微信小程序内提交预约<\/button>[\s\S]*#endif/, 'H5 should render disabled submit guidance')
assert.doesNotMatch(bookingPage.match(/#ifdef H5[\s\S]*?#endif/)?.[0] || '', /@click=["']submit["']/, 'H5 submit guard must not bind submit')
assert.match(bookingPage, /#ifndef H5[\s\S]*@click=["']submit["'][\s\S]*#endif/, 'miniapp submit should keep the handler')
for (const helper of ['loadBookingDraft', 'saveBookingDraft', 'clearBookingDraft', 'fieldErrors', 'aria-invalid']) {
  assert.match(bookingPage, new RegExp(helper), `booking should preserve ${helper}`)
}
```

- [ ] **Step 2: Run UI test and verify RED**

Run: `node scripts/ui-compat.test.mjs`

Expected: FAIL on missing `booking-hero` or H5 submit guard.

- [ ] **Step 3: Implement the exact grouped template**

- orange-blue hero;
- group 1: kind picker;
- group 2: contact name/phone;
- group 3: intent/time/message;
- picker arrow drawn using a bordered rotated pseudo-element;
- add helper text explaining local draft;
- use conditional submit buttons:

```vue
<!-- #ifdef H5 -->
<button class="booking-submit booking-submit--disabled ios-button" disabled>请在微信小程序内提交预约</button>
<!-- #endif -->
<!-- #ifndef H5 -->
<button class="btn-primary booking-submit ios-button" :loading="submitting" :disabled="submitting" @click="submit">提交预约</button>
<!-- #endif -->
```

Render the sections as:

```vue
<view class="booking-hero nx-page-hero"><text class="booking-hero__eyebrow">预约咨询</text><text class="booking-hero__title">让老师帮你找到合适的学习方式</text><text class="booking-hero__lead">资料会自动保存为本地草稿，提交后老师将尽快联系你。</text></view>
<view class="form-section nx-panel"><text class="form-section__title">预约类型</text><picker :range="kinds" range-key="label" :value="kindIndex" @change="onKindChange"><view class="picker field-control"><text>{{ kinds[kindIndex].label }}</text><view class="picker__arrow" aria-hidden="true"></view></view></picker></view>
<view class="form-section nx-panel"><text class="form-section__title">联系信息</text><view class="field"><text class="label">称呼</text><input class="input field-control" v-model="form.contactName" placeholder="怎么称呼你" :aria-invalid="!!fieldErrors.contactName" @input="clearFieldError('contactName')" /><text v-if="fieldErrors.contactName" class="field-error">{{ fieldErrors.contactName }}</text></view><view class="field"><text class="label">手机号</text><input class="input field-control" v-model="form.phone" type="number" maxlength="11" placeholder="方便老师联系" :aria-invalid="!!fieldErrors.phone" @input="clearFieldError('phone')" /><text v-if="fieldErrors.phone" class="field-error">{{ fieldErrors.phone }}</text></view></view>
<view class="form-section nx-panel"><text class="form-section__title">学习意向</text><view class="field"><text class="label">意向方向</text><input class="input field-control" v-model="form.intent" placeholder="如：亲子关系 / 个人成长 / 团队" /></view><view class="field"><text class="label">期望时间</text><input class="input field-control" v-model="form.preferredTime" placeholder="如：周末 / 工作日晚上" /></view><view class="field"><text class="label">留言</text><textarea class="textarea field-control" v-model="form.message" placeholder="想了解的问题（选填）" /></view><text class="draft-hint">填写内容会自动保存在当前设备</text></view>
```

- [ ] **Step 4: Apply concrete booking styles**

`.booking-hero` uses `linear-gradient(145deg,#c2410c,#2563eb)`, 38rpx radius and white text. `.form-section` is vertical flex with 20rpx gap and `.form-section__title` is 31rpx/900. Inputs remain >=88rpx; textarea 176rpx; errors use `--nx-danger`. `.picker__arrow` is `18rpx × 18rpx` with right/bottom `3rpx` blue borders rotated 45deg. `.booking-submit` is >=88rpx; disabled H5 style uses a light gray surface and `#64748b`.

- [ ] **Step 5: Run focused test**

Run: `node scripts/ui-compat.test.mjs`

Expected: `ui compatibility tests passed`.

- [ ] **Step 6: Build H5**

Run: `npm run build:h5`

Expected: exit 0 with only documented warnings.

- [ ] **Step 7: Build mp-weixin**

Run: `npm run build:mp-weixin`

Expected: exit 0 with only documented warnings.

- [ ] **Step 8: Commit**

```bash
git add scripts/ui-compat.test.mjs src/pages/booking/booking.vue
git commit -m "style: redesign miniapp booking flow"
```

### Task 8: Redesign profile states and summaries

**Files:**
- Modify: `scripts/ui-compat.test.mjs`
- Modify: `src/pages/profile/profile.vue`

- [ ] **Step 1: Add failing profile assertions**

```js
assert.match(profilePage, /class=["'][^"']*profile-hero[^"']*nx-page-hero/, 'profile should use the deep blue-purple hero')
assert.match(profilePage, /const recordCountLabel = computed\(/, 'profile should derive a loading-aware record count')
assert.match(profilePage, /const bookingCountLabel = computed\(/, 'profile should derive a loading-aware booking count')
assert.ok((profilePage.match(/class=["'][^"']*profile-stat[^"']*["']/g) || []).length >= 3, 'profile hero should show three stats')
assert.match(profilePage, /class=["']history-timeline["']/, 'profile histories should use a timeline container')
assert.match(profilePage, /class=["']history-item["']/, 'profile histories should use timeline items')
assert.match(profilePage, /const userAvatarFailed = ref\(false\)/, 'profile should track user avatar failure')
assert.match(profilePage, /const draftAvatarFailed = ref\(false\)/, 'profile should track draft avatar failure')
assert.doesNotMatch(profilePage, /管理档案/, 'profile must not invent a new archive-management action')
```

Keep all existing assertions for loading-before-empty, records/bookings error-before-empty, retries, stale load tickets, H5 disabled login, chooseAvatar, nickname, no phone authorization, and no AI/chat state.

- [ ] **Step 2: Run UI test and verify RED**

Run: `node scripts/ui-compat.test.mjs`

Expected: FAIL on missing profile hero or count labels.

- [ ] **Step 3: Add derived summary data and image fallbacks**

```js
const recordCount = computed(() => records.value.length)
const bookingCount = computed(() => bookings.value.length)
const recordCountLabel = computed(() => profileLoading.value ? '—' : String(recordCount.value))
const bookingCountLabel = computed(() => profileLoading.value ? '—' : String(bookingCount.value))
const userAvatarFailed = ref(false)
const draftAvatarFailed = ref(false)
```

Reset failure flags when drafts/user data change. Do not add API fields.

Implement resets inside existing functions:

```js
function syncDraftFromUser() {
  nicknameDraft.value = (user.value && user.value.nickname) || ''
  avatarDraft.value = (user.value && user.value.avatar) || ''
  userAvatarFailed.value = false
  draftAvatarFailed.value = false
}
function onChooseAvatar(e) {
  avatarDraft.value = e.detail && e.detail.avatarUrl ? e.detail.avatarUrl : ''
  draftAvatarFailed.value = false
}
function onUserAvatarError() { userAvatarFailed.value = true }
function onDraftAvatarError() { draftAvatarFailed.value = true }
```

Use `v-if="user && user.avatar && !userAvatarFailed"` and `v-if="avatarDraft && !draftAvatarFailed"`; fallbacks keep the existing 116rpx/avatar-picker dimensions.

- [ ] **Step 4: Implement logged-out and logged-in templates**

Use this complete structure, including the existing platform login blocks and all form bindings:

```vue
<view v-if="!logged" class="profile-hero profile-hero--login nx-page-hero">
  <view class="profile-hero__mark">九</view>
  <text class="profile-hero__eyebrow">个人档案</text>
  <text class="profile-hero__title">记录每一次自我看见</text>
  <text class="profile-hero__lead">登录后保存九型档案、测试历史和预约记录。</text>
  <!-- #ifdef H5 -->
  <button class="profile-login profile-login--disabled ios-button" disabled>请在微信小程序内登录</button>
  <text class="profile-hero__hint">H5 可浏览公开内容；保存档案和预约记录请打开微信小程序。</text>
  <!-- #endif -->
  <!-- #ifndef H5 -->
  <button class="profile-login ios-button" :loading="logging" :disabled="logging" @click="login">微信一键登录</button>
  <!-- #endif -->
</view>
<template v-else>
  <view class="profile-hero profile-hero--user nx-page-hero">
    <image v-if="user && user.avatar && !userAvatarFailed" class="profile-hero__avatar" :src="user.avatar" lazy-load @error="onUserAvatarError" /><view v-else class="profile-hero__avatar profile-hero__avatar--fallback">{{ (user && user.mainType) || '九' }}</view>
    <view class="profile-hero__identity"><text class="profile-hero__name">{{ (user && user.nickname) || '九型用户' }}</text><text class="profile-hero__type">{{ user && user.mainType ? typeName(user.mainType) : '已通过微信登录' }}</text></view>
    <view class="profile-stats"><view class="profile-stat"><text class="profile-stat__value">{{ (user && user.mainType) || '—' }}</text><text class="profile-stat__label">主型</text></view><view class="profile-stat"><text class="profile-stat__value">{{ recordCountLabel }}</text><text class="profile-stat__label">测试</text></view><view class="profile-stat"><text class="profile-stat__value">{{ bookingCountLabel }}</text><text class="profile-stat__label">预约</text></view></view>
  </view>
  <view class="profile-edit nx-panel">
    <view class="profile-edit__head"><text class="sec-title">微信资料</text><button class="mini-link" :loading="profileSaving" :disabled="profileSaving" @click="syncWechatProfile">一键同步</button></view>
    <view class="wechat-slot"><text class="wechat-slot__title">微信登录能力</text><text class="wechat-slot__desc">{{ wechatLoginReady.note }}</text></view>
    <view class="profile-edit__row">
      <button class="avatar-picker" open-type="chooseAvatar" @chooseavatar="onChooseAvatar"><image v-if="avatarDraft && !draftAvatarFailed" class="avatar-picker__img" :src="avatarDraft" mode="aspectFill" lazy-load @error="onDraftAvatarError" /><text v-else class="avatar-picker__ph">头像</text></button>
      <view class="nickname-field"><text class="nickname-field__label">昵称</text><input class="nickname-field__input" type="nickname" :value="nicknameDraft" placeholder="填写微信昵称" @input="onNicknameInput" @blur="onNicknameInput" /></view>
    </view>
    <button class="btn-primary profile-edit__save" :loading="profileSaving" :disabled="profileSaving" @click="saveProfile">保存资料</button>
  </view>
  <view class="history-section nx-panel">
    <text class="sec-title">我的测试历史</text>
    <view v-if="profileLoading" class="nx-state">正在同步测试历史…</view>
    <view v-else-if="recordsError" class="nx-state nx-state--error"><text>{{ recordsError }}</text><button class="sync-retry nx-focusable" @click="loadAll">重试</button></view>
    <view v-else-if="records.length === 0" class="nx-state">还没有记录，去测一测吧</view>
    <view v-else class="history-timeline"><view v-for="rec in visibleRecords" :key="rec.id" class="history-item"><view class="history-item__dot"></view><view class="history-item__body"><text class="history-item__main">{{ typeName(rec.resultType) }}</text><text class="history-item__meta">{{ rec.createTime }}</text></view></view></view>
    <text v-if="hiddenRecordCount" class="more-tip">还有 {{ hiddenRecordCount }} 条记录已收起</text>
  </view>
  <view class="history-section nx-panel">
    <text class="sec-title">我的预约</text>
    <view v-if="profileLoading" class="nx-state">正在同步预约记录…</view>
    <view v-else-if="bookingsError" class="nx-state nx-state--error"><text>{{ bookingsError }}</text><button class="sync-retry nx-focusable" @click="loadAll">重试</button></view>
    <view v-else-if="bookings.length === 0" class="nx-state">暂无预约</view>
    <view v-else class="history-timeline"><view v-for="b in visibleBookings" :key="b.id" class="history-item"><view class="history-item__dot"></view><view class="history-item__body"><text class="history-item__main">{{ b.intent || b.kind }}</text><text class="history-item__meta">{{ b.status }} · {{ b.createTime }}</text></view></view></view>
    <text v-if="hiddenBookingCount" class="more-tip">还有 {{ hiddenBookingCount }} 条预约已收起</text>
  </view>
  <button class="profile-logout ios-button" @click="logout">退出登录</button>
</template>
```

- [ ] **Step 5: Apply concrete profile styles**

`.profile-hero` uses `linear-gradient(145deg,#172554,#4338ca 56%,#7c3aed)`, radius 38rpx and white text. Avatar/fallback is 116rpx. `.profile-stats` is three columns with translucent white cells; values 34rpx/900. `.profile-edit` and `.history-section` use light surfaces. `.history-item` is flex with a 16rpx colored dot, `min-height:88rpx`, divider and `#64748b` metadata. `.profile-logout` is >=88rpx, transparent/light border, muted danger text, separated by 16rpx top margin.

- [ ] **Step 6: Run focused test**

Run: `node scripts/ui-compat.test.mjs`

Expected: `ui compatibility tests passed`.

- [ ] **Step 7: Build H5**

Run: `npm run build:h5`

Expected: exit 0 with only documented warnings.

- [ ] **Step 8: Build mp-weixin**

Run: `npm run build:mp-weixin`

Expected: exit 0 with only documented warnings.

- [ ] **Step 9: Commit**

```bash
git add scripts/ui-compat.test.mjs src/pages/profile/profile.vue
git commit -m "style: redesign miniapp growth profile"
```

## Chunk 4: Final integration and visual verification

### Task 9: Run complete integration verification

**Files:**
- Verify: all modified files
- Optional QA-only fix: page file or `scripts/ui-compat.test.mjs`

- [ ] **Step 1: Run task-focused tests**

```bash
node src/utils/reportDisplayState.test.mjs
node scripts/ui-compat.test.mjs
```

Expected: both exit 0.

- [ ] **Step 2: Run the full suite and record the known baseline**

Run: `npm run test:config`

Expected: current repository may stop only at `scripts/project-config.test.mjs:36` for `.env.production.example`. Confirm config/request/api tests before it pass. Do not modify `.env.development` or production example in this scope.

- [ ] **Step 3: Run every remaining test after the known early failure manually**

Run each command exactly:

```bash
node src/utils/reportDisplayState.test.mjs
node scripts/ui-compat.test.mjs
node src/utils/session.test.mjs
node src/utils/resultPersona.test.mjs
node src/utils/siteConfig.test.mjs
node src/utils/teacherCourseware.test.mjs
node src/utils/bookingDraft.test.mjs
node src/utils/clipboard.test.mjs
node src/utils/userMessage.test.mjs
node src/utils/wechatProfile.test.mjs
node src/utils/listPreview.test.mjs
node src/utils/push.test.mjs
node src/utils/auth.test.mjs
node src/utils/payment.test.mjs
node src/pages/learn.quote-card.test.mjs
```

Expected: every command exits 0. Record the full-suite `exit 1` separately from these passing follow-up checks; do not claim the full suite is green.

- [ ] **Step 4: Build both platforms**

```bash
npm run build:h5
npm run build:mp-weixin
```

Expected: both exit 0; only existing circular-dependency/update warnings.

- [ ] **Step 5: Prepare deterministic H5 visual states**

Use these exact routes and state preparations:

| Page | URL | Deterministic state |
|------|-----|---------------------|
| Test | `/#/pages/test/test` | Capture initial gender stage, then select either gender and capture the first-question stage. |
| Result | `/#/pages/result/result` | In DevTools console run `localStorage.setItem('nx_last_test_result', JSON.stringify({result:{type:2,second:3,score:{},centers:[{key:'heart',name:'情感中心',pct:45},{key:'head',name:'思维中心',pct:30},{key:'gut',name:'本能中心',pct:25}]},gender:'female'}))`, reload, and capture the visible H5 save/report/poster guards. The current session loader accepts this exact plain JSON string. |
| Relation | `/#/pages/relation/relation?type=2` | Capture the picker, select any available counterpart, tap generate, and capture the generated relationship result. |
| Learn | `/#/pages/learn/learn` | In DevTools console run `localStorage.setItem('nx_site_config_cache', JSON.stringify({ts:Date.now(),data:{teachers:[{name:'韩老师',title:'九型人格主讲老师',avatar:'/static/avatars/9.png',bio:'把九型知识落到关系沟通与日常成长。',tags:['九型入门','关系沟通']}],courseware:[{title:'九型人格入门课件',description:'建立九种核心动机的基础地图。',cover:'/static/wheel.png',badge:'入门',duration:'约 20 分钟'}],home:{quotes:{items:['看见模式，是改变发生的开始。']}}}}))`; enable Chrome DevTools Request Blocking for `*/public/site-config*`, reload, and capture the exact cached fixture. Disable request blocking after capture. Static tests cover the error branch. |
| Booking | `/#/pages/booking/booking` | In DevTools console run `localStorage.removeItem('nx_booking_draft')`, then `localStorage.setItem('nx_booking_draft', JSON.stringify({ts:Date.now(),data:{kind:'consult',contactName:'测试用户',phone:'13800138000',intent:'个人成长',preferredTime:'周末下午',message:'想了解适合我的学习路径'}}))`; reload and capture the populated draft, saved-draft hint, and disabled H5 submit. Static tests cover inline-error ordering because H5 submission is intentionally disabled. |
| Profile | `/#/pages/profile/profile` | In DevTools console run `localStorage.removeItem('nx_token')`, reload, and capture the deterministic H5 logged-out state. Verify logged-in-only structure through generated mp-weixin WXML in Step 7. |

Before visual review, confirm the result storage key is exactly `nx_last_test_result` from `src/utils/session.js`.

- [ ] **Step 6: Inspect every route at 360, 375, 390 and 800px widths**

Start:

```bash
npm run dev:h5 -- --host 127.0.0.1 --port 4173 --strictPort
```

Create:

- `docs/superpowers/qa/2026-07-23-full-experience/qa.md`
- screenshots under `docs/superpowers/qa/2026-07-23-full-experience/screenshots/`

At viewport widths `360`, `375`, `390`, and `800`, open these complete URLs:

```text
http://127.0.0.1:4173/#/pages/test/test
http://127.0.0.1:4173/#/pages/result/result
http://127.0.0.1:4173/#/pages/relation/relation?type=2
http://127.0.0.1:4173/#/pages/learn/learn
http://127.0.0.1:4173/#/pages/booking/booking
http://127.0.0.1:4173/#/pages/profile/profile
```

In `qa.md`, create one table row for each of the 24 route/width pairs using this fixed schema:

```markdown
| Page/state | Full URL | Viewport | No horizontal overflow | Hero clear | Primary CTA clear | Grid/cards readable | Safe bottom visible | Contrast/states readable | Screenshot |
```

For each row record:

- exact URL and viewport width;
- console result of `document.documentElement.scrollWidth === document.documentElement.clientWidth` (must be `true`);
- hero/header is not clipped or overlapped;
- one dominant primary CTA is visually clear;
- grids/cards remain readable and do not collapse awkwardly;
- safe-bottom spacing remains visible;
- disabled/error/help text has readable contrast;
- screenshot filename.

Use filenames `<page>-<state>-<width>.png`, for example `result-public-375.png`. This produces 24 base screenshots plus `test-question-375.png` and `relation-result-375.png`. Store all 26 PNGs in the screenshots directory and link them from `qa.md`. Stop the server afterward.

- [ ] **Step 7: Verify generated WeChat semantics**

Run:

```bash
rg -n 'aria-role="button"|aria-label=|aria-pressed=' src/pages/test/test.vue src/pages/relation/relation.vue src/pages/result/result.vue
rg -n 'open-type="share"' dist/build/mp-weixin/pages/result/result.wxml
rg -n '存入档案并查看价格' src/pages/result/result.vue
rg -n '@click="saveRecord"' src/pages/result/result.vue
rg -n '解锁深度报告' src/pages/result/result.vue
rg -n '@click="unlockReport"' src/pages/result/result.vue
test "$(rg -o '<button[^>]*class="[^"]*report-panel__cta[^"]*"[^>]*bindtap' dist/build/mp-weixin/pages/result/result.wxml | wc -l | tr -d ' ')" -eq 2
rg -n '<button[^>]*bindtap[^>]*>提交预约</button>' dist/build/mp-weixin/pages/booking/booking.wxml
rg -n 'open-type="chooseAvatar"[^>]*bindchooseavatar' dist/build/mp-weixin/pages/profile/profile.wxml
rg -n '<input[^>]*type="nickname"[^>]*bindinput' dist/build/mp-weixin/pages/profile/profile.wxml
! rg -n '请在微信小程序内提交预约|请在微信小程序内登录|请在微信小程序内完成存档与支付' dist/build/mp-weixin/pages/booking/booking.wxml dist/build/mp-weixin/pages/profile/profile.wxml dist/build/mp-weixin/pages/result/result.wxml
```

Expected:

- source accessibility contracts remain on the redesigned test/relation/result surfaces; native generated buttons/pickers provide the WeChat roles that the compiler owns;
- result source independently retains exact save/payment copy and handler bindings; generated output contains exactly two actionable `.report-panel__cta` buttons with `bindtap` plus the share button;
- booking output retains the exact `提交预约` button with `bindtap`;
- profile output retains WeChat avatar/nickname capabilities;
- the final explicit negative assertion exits 0 because H5-only guidance is absent from mp-weixin output.

- [ ] **Step 8: Apply any QA correction with regression coverage**

If a defect is found, add/adjust a failing static or pure test first, implement the minimal fix, rerun affected tests and both builds. Run `git diff --name-only` and stage each reported QA correction path explicitly; never stage `src/pages`, `src/styles`, or `src/utils` as a directory. Example, if the reported correction paths are exactly the result page, shared style and UI test:

```bash
git add scripts/ui-compat.test.mjs src/pages/result/result.vue src/styles/apple-mobile.css
git commit -m "fix: polish full miniapp visual QA"
```

If there are no QA code corrections, skip this correction commit.

- [ ] **Step 9: Final diff review**

```bash
git diff --check HEAD -- scripts/ui-compat.test.mjs src/styles/apple-mobile.css src/pages src/utils/reportDisplayState.js src/utils/reportDisplayState.test.mjs package.json
git status --short
```

Expected: task files have no whitespace errors; the user's pre-existing `.env.development` change may remain outside task commits.

- [ ] **Step 10: Commit auditable QA evidence**

```bash
test "$(find docs/superpowers/qa/2026-07-23-full-experience/screenshots -maxdepth 1 -type f -name '*.png' | wc -l | tr -d ' ')" -eq 26
git add docs/superpowers/qa/2026-07-23-full-experience/qa.md docs/superpowers/qa/2026-07-23-full-experience/screenshots/*.png
test "$(git diff --cached --name-only -- docs/superpowers/qa/2026-07-23-full-experience | wc -l | tr -d ' ')" -eq 27
git diff --cached --name-only
git commit -m "docs: record full miniapp visual QA"
```

Expected: the first assertion confirms exactly 26 PNGs; the staged-path assertion confirms exactly 27 QA artifacts (`qa.md` plus 26 PNGs); the staged list contains no source files or `.env.development`; then the QA evidence is committed.
