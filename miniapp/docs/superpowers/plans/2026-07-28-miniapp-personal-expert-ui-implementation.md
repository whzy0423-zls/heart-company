# 小程序个人专家品牌全站视觉升级 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有小程序升级为统一、可信、适合个人专家面向企业客户推广的品牌界面，同时保留九型测试小游戏、课堂、预约、关系合盘和个人档案功能。

**Architecture:** 先以语义 Token、统一异步状态和导航资源建立全局基础，再通过纯函数归一化后台 `siteConfig` 为首页视图模型。页面改造按“首页与转化—课堂内容—测评与结果—档案记录”分批完成，现有 API、缓存、购买、登录和计分逻辑保持不变。

**Tech Stack:** uni-app、Vue 3、微信小程序、Node.js `assert` 源码/运行时测试、Python Pillow（仅生成 TabBar PNG）、现有 Go/Ant Design Vue 后台配置。

**Design spec:** `docs/superpowers/specs/2026-07-28-miniapp-personal-expert-ui-design.md`

**Safety baseline:** `miniapp-ui-baseline-before-training-refresh-20260728` → `b5b3780`

**Workspace rule:** 用户要求在当前目录持续运行 `npm run dev:mp-weixin` 并即时查看，因此本计划直接在当前工作区执行，不切换工作树。仓库中存在与本任务无关的后台暂存/未提交改动；禁止 `git add -A`，每次只提交计划列出的文件，并优先使用 `git commit --only`。

**Command cwd:** 除 Task 11 明确进入 `../nx-backend` 外，所有命令都在当前 `miniapp/` 目录执行；因此 Git pathspec 使用 `src/...`、`scripts/...`、`package.json`，不加 `miniapp/` 前缀。

**Dev watcher rule:** 执行开始时只启动一个 `npm run dev:mp-weixin` PTY/watch 会话并记录 session ID。后续任务修改后通过同一个会话轮询新输出，确认再次出现 `DONE Build complete. Watching for changes...`；禁止重复启动多个 watcher。若会话意外退出，先确认不存在旧进程，再重启一个。最终按用户需要保持该 watcher 运行以便即时预览。

### Task 0: 启动并记录唯一开发 watcher

**Files:**
- Verify: `dist/dev/mp-weixin/**`

- [ ] **Step 1: 启动测试环境 watcher**

Run in a persistent PTY: `npm run dev:mp-weixin`  
Expected: `DONE Build complete. Watching for changes...`

- [ ] **Step 2: 记录 session ID 并验证 API**

Run: `rg 'http://127.0.0.1:5320/api' dist/dev/mp-weixin/config.js`  
Expected: 命中本地测试 API。后续 Task 5–13 均复用此 watcher。

---

## Chunk 1: 设计系统、统一状态与导航

### Task 1: 建立语义设计 Token 与页面基础契约

**Files:**
- Modify: `src/styles/apple-mobile.css:1-309`
- Create: `src/styles/personal-expert-theme.test.mjs`
- Modify: `package.json:5-8`

- [ ] **Step 1: 编写失败的设计系统契约测试**

在 `src/styles/personal-expert-theme.test.mjs` 读取 `apple-mobile.css`，断言存在：

```js
for (const token of [
  '--nx-brand-900',
  '--nx-brand-700',
  '--nx-accent-gold',
  '--nx-page-bg',
  '--nx-surface',
  '--nx-text',
  '--nx-text-muted',
  '--nx-border',
  '--nx-danger',
  '--nx-success',
]) {
  assert.match(source, new RegExp(`${token}\\s*:`));
}
assert.match(source, /--nx-brand-900:\s*#202a37/i);
assert.match(source, /--nx-accent-gold:\s*#dfbc7f/i);
assert.doesNotMatch(source, /--nx-teal|--nx-green/);
```

并断言 `.nx-page-shell`、`.nx-card`、`.nx-button--primary`、`.nx-button--secondary`、`.nx-section-head` 的基础规则存在，主要按钮 `min-height: 88rpx`。

- [ ] **Step 2: 运行测试确认失败**

Run: `node src/styles/personal-expert-theme.test.mjs`  
Expected: FAIL，提示缺少新 Token 或基础类。

- [ ] **Step 3: 重整全局样式**

在 `apple-mobile.css` 中：

- 合并两套 `:root`。
- 建立设计文档约定的语义 Token。
- 将全局 page 背景改为暖灰/浅色，不使用大面积蓝橙径向背景。
- 新增 `.nx-page-shell`、`.nx-card`、`.nx-button`、`.nx-button--primary`、`.nx-button--secondary`、`.nx-button--text`、`.nx-section-head`、`.nx-eyebrow`、`.nx-title`、`.nx-body`。
- 保留旧 `.wrap/.card/.btn-primary` 兼容别名，逐页迁移期间不破坏页面。
- 统一安全区和底部 padding，避免 `.ios-page/.ios-safe-bottom/.page-stack` 三重叠加。
- 保留 `prefers-reduced-motion`。

- [ ] **Step 4: 把测试加入 `test:config`**

在 `package.json` 的 `test:config` 前半段加入：

```json
"node src/styles/personal-expert-theme.test.mjs"
```

- [ ] **Step 5: 运行定向测试并提交**

Run: `node src/styles/personal-expert-theme.test.mjs`  
Expected: PASS。此阶段不运行全量 `test:config`，因为 `scripts/ui-compat.test.mjs` 仍锁定旧 Token；全量迁移和 PASS 在 Task 12 完成。

Commit only:

```bash
git commit --only -m "feat: establish personal expert design tokens" -- \
  src/styles/apple-mobile.css \
  src/styles/personal-expert-theme.test.mjs \
  package.json
```

### Task 2: 创建统一异步状态组件

**Files:**
- Create: `src/components/NxAsyncState.vue`
- Create: `src/components/NxAsyncState.test.mjs`
- Modify: `package.json`

- [ ] **Step 1: 编写失败测试**

断言组件：

```js
assert.match(source, /defineProps/);
assert.match(source, /state:\s*\{[\s\S]*required:\s*true/);
assert.match(source, /defineEmits\(\[?["']action["']/);
assert.match(source, /@click="emit\('action'\)"/);
assert.match(source, /:disabled="busy"/);
for (const state of ['loading', 'stale', 'empty', 'error']) {
  assert.match(source, new RegExp(`nx-async-state--${state}`));
}
```

- [ ] **Step 2: 运行确认失败**

Run: `node src/components/NxAsyncState.test.mjs`  
Expected: FAIL，组件不存在。

- [ ] **Step 3: 实现组件**

组件 props：`state/title/description/actionText/busy`；`emit('action')`；Loading 使用 CSS 标记，其他状态显示标题、说明和可选按钮；`busy` 禁用按钮并显示“处理中…”。所有按钮使用统一触控高度。

- [ ] **Step 4: 运行测试并加入总测试**

Run: `node src/components/NxAsyncState.test.mjs`  
Expected: PASS。将 `node src/components/NxAsyncState.test.mjs` 加入 `package.json` 的 `test:config`。

- [ ] **Step 5: 提交**

```bash
git commit --only -m "feat: add unified miniapp async state" -- \
  src/components/NxAsyncState.vue \
  src/components/NxAsyncState.test.mjs \
  package.json
```

### Task 3: 更新底部导航和图标资源

**Files:**
- Modify: `src/pages.json:1-157`
- Create: `scripts/generate-tabbar-icons.py`
- Create: `src/static/tabbar/home.png`
- Create: `src/static/tabbar/home-active.png`
- Create: `src/static/tabbar/classroom.png`
- Create: `src/static/tabbar/classroom-active.png`
- Create: `src/static/tabbar/enterprise.png`
- Create: `src/static/tabbar/enterprise-active.png`
- Replace: `src/static/tabbar/profile.png`
- Replace: `src/static/tabbar/profile-active.png`
- Modify: `scripts/project-config.test.mjs`

- [ ] **Step 1: 扩展失败测试**

在 `scripts/project-config.test.mjs` 断言：

```js
assert.deepEqual(tabTexts, ['首页', '老师课堂', '企业服务', '我的']);
assert.equal(pages['pages/learn/learn'].navigationBarTitleText, '老师课堂');
assert.equal(pages['pages/booking/booking'].navigationBarTitleText, '企业服务');
assert.equal(config.tabBar.selectedColor.toLowerCase(), '#202a37');
```

并验证 8 个图标路径均存在且 PNG 尺寸为 `81×81`。

- [ ] **Step 2: 运行确认失败**

Run: `node scripts/project-config.test.mjs`  
Expected: FAIL，旧 Tab 文案或图标路径不匹配。

- [ ] **Step 3: 生成一致线宽图标**

用 Pillow 创建 81×81 RGBA PNG：

- 未选中：`#7A828C`
- 选中：`#202A37`
- 首页：房屋/工作室轮廓
- 课堂：播放窗口/书本组合
- 企业服务：公文包/建筑轮廓
- 我的：人物轮廓

生成脚本固定尺寸、颜色、线宽，方便后续重复生成。

- [ ] **Step 4: 更新 `pages.json`**

更新 Tab 文案、导航标题、图标路径、选中色和暖白背景。路由不变。

- [ ] **Step 5: 验证并提交**

Run: `python3 scripts/generate-tabbar-icons.py && node scripts/project-config.test.mjs`  
Expected: PASS，8 个 PNG 均为 81×81。

```bash
git commit --only -m "feat: align miniapp navigation with expert brand" -- \
  src/pages.json \
  scripts/generate-tabbar-icons.py \
  scripts/project-config.test.mjs \
  src/static/tabbar/home.png \
  src/static/tabbar/home-active.png \
  src/static/tabbar/classroom.png \
  src/static/tabbar/classroom-active.png \
  src/static/tabbar/enterprise.png \
  src/static/tabbar/enterprise-active.png \
  src/static/tabbar/profile.png \
  src/static/tabbar/profile-active.png
```

---

## Chunk 2: 首页、小游戏与企业预约转化

### Task 4: 建立首页个人专家视图模型

**Files:**
- Create: `src/utils/personalExpertHome.js`
- Create: `src/utils/personalExpertHome.test.mjs`
- Modify: `src/utils/homeMenu.js:1-160`
- Modify: `src/utils/homeMenu.test.mjs`
- Modify: `package.json`

- [ ] **Step 1: 编写视图模型失败测试**

覆盖：

- `teacherTeaser.image → fallbackImage → monogram` 数据顺序。
- 只返回有效 `home.hero.stats`，不伪造数字。
- `home.enterprise` 与企业课程项归一化。
- `home.game` 优先覆盖旧 `test` 文案。
- `test.enabled=false` 隐藏独立小游戏区。
- 通用宫格过滤 `test` 且保持其他项目后台顺序。
- 缺失案例时不产生演示客户案例。

示例：

```js
const view = normalizePersonalExpertHome(config);
assert.equal(view.game.title, '你的人设出厂设置');
assert.deepEqual(view.secondaryEntries.map((item) => item.key), ['relation', 'learn', 'profile']);
assert.deepEqual(view.proofStats, [{ value: '20', suffix: '+', label: '年一线导师经验' }]);
```

- [ ] **Step 2: 运行确认失败**

Run: `node src/utils/personalExpertHome.test.mjs`  
Expected: FAIL，模块不存在。

- [ ] **Step 3: 实现纯函数归一化**

导出：

```js
normalizePersonalExpertHome(config)
personalExpertGameSection(config, miniappHome)
personalExpertProofStats(config)
personalExpertServices(config)
```

所有文字 trim、数组限量、异常输入回退，不读取 `uni`，便于 Node 测试。

- [ ] **Step 4: 更新旧入口归一化**

`homeMenu.js` 保留旧行为和 API，同时提供 `testEntry` 与过滤后的 `navigationEntries`，旧消费者不被破坏。

- [ ] **Step 5: 测试并提交**

Run: `node src/utils/personalExpertHome.test.mjs && node src/utils/homeMenu.test.mjs`  
Expected: PASS。

- [ ] **Step 6: 加入总测试并限定提交**

将 `node src/utils/personalExpertHome.test.mjs` 加入 `package.json` 的 `test:config`。

```bash
git commit --only -m "feat: normalize personal expert home content" -- \
  src/utils/personalExpertHome.js \
  src/utils/personalExpertHome.test.mjs \
  src/utils/homeMenu.js \
  src/utils/homeMenu.test.mjs \
  package.json
```

### Task 4A: 前置实现一次性企业预约意图

**Files:**
- Create: `src/utils/bookingIntent.js`
- Create: `src/utils/bookingIntent.test.mjs`
- Modify: `package.json`

- [ ] **Step 1: 编写预约意图失败测试**

```js
setBookingIntent({ kind: 'enterprise', intentText: '团队沟通工作坊' });
assert.deepEqual(consumeBookingIntent(), {
  kind: 'enterprise',
  intentText: '团队沟通工作坊',
});
assert.equal(consumeBookingIntent(), null);
```

无效 kind 忽略；存储异常返回空，不影响进入页面。

- [ ] **Step 2: 运行确认失败**

Run: `node src/utils/bookingIntent.test.mjs`  
Expected: FAIL，模块不存在。

- [ ] **Step 3: 实现并测试**

使用独立 storage key 和一次性消费语义，不改动预约草稿 key。

Run: `node src/utils/bookingIntent.test.mjs`  
Expected: PASS。

- [ ] **Step 4: 加入总测试并提交**

将 `node src/utils/bookingIntent.test.mjs` 加入 `package.json` 的 `test:config`。

```bash
git commit --only -m "feat: add enterprise booking intent" -- \
  src/utils/bookingIntent.js \
  src/utils/bookingIntent.test.mjs \
  package.json
```

### Task 5: 重构首页为个人专家与企业信任中枢

**Files:**
- Modify: `src/pages/index/index.vue:1-945`
- Rewrite: `src/pages/index/index.test.mjs`
- Modify: `src/utils/homeCarousel.test.mjs`

- [ ] **Step 1: 更新首页失败测试**

断言页面按顺序包含：

```text
expert-hero
proof-stats
enterprise-services
test-game
classroom-preview
secondary-entries
enterprise-final-cta
```

并断言：

- 轮播不再强制首位。
- 老师图片有失败兜底。
- 主 CTA 调用企业预约意图。
- 小游戏区点击 `/pages/test/test`。
- 普通宫格不重复渲染 `test`。
- 课堂优先请求独立课件并最多展示 2 条。
- 课堂加载失败不阻塞首页其余内容。
- 首页导入并使用 `NxAsyncState`：课堂首次加载为 `loading`、成功无内容为 `empty`、首次失败无内容为 `error`、已有站点缓存但刷新失败为 `stale`。
- `NxAsyncState @action` 分别触发课堂重试或站点配置重试，`busy` 时禁止重复触发。
- 不出现 `80+`、`96%` 等未配置演示数字。

- [ ] **Step 2: 运行确认失败**

Run: `node src/pages/index/index.test.mjs`  
Expected: FAIL，旧首页结构不满足新顺序。

- [ ] **Step 3: 实现首页脚本数据流**

- 继续读取缓存 `siteConfig` 并静默刷新。
- 使用 `normalizePersonalExpertHome`。
- 调用 `listClassroomStandaloneApi({ limit: 2, offset: 0 })`。
- 用 `normalizeClassroomContent` 过滤无 ID 项，按接口顺序展示 2 条。
- 教师图、轮播图、课件封面分别维护失败集合。
- 使用 `NxAsyncState` 呈现课堂 loading/empty/error 和站点配置 stale；错误状态不替换已经可见的其他首页模块。
- 主 CTA 写入企业预约意图后切换 Tab。

- [ ] **Step 4: 实现确认的视觉结构**

使用石墨蓝 Hero、暖灰背景、香槟金按钮；真实老师图位于 Hero 右侧/下方；资历只显示后台有效项；企业服务读取配置；小游戏使用独立深色卡；课堂使用视频/音频媒体卡；轮播下移为专业证明内容。

- [ ] **Step 5: 运行首页和配置测试**

Run:

```bash
node src/pages/index/index.test.mjs
node src/utils/personalExpertHome.test.mjs
node src/utils/homeCarousel.test.mjs
```

Expected: PASS。

- [ ] **Step 6: 观察唯一 watcher 并提交**

轮询 Task 0 的 PTY session。  
Expected: 本轮修改后再次出现 `DONE Build complete. Watching for changes...`。

确认 `dist/dev/mp-weixin/config.js` 包含 `http://127.0.0.1:5320/api`，然后限定提交首页相关文件。

```bash
git commit --only -m "feat: rebuild miniapp home for expert brand" -- \
  src/pages/index/index.vue \
  src/pages/index/index.test.mjs \
  src/utils/homeCarousel.test.mjs
```

### Task 6: 实现企业预约意图与提交成功闭环

**Files:**
- Modify: `src/pages/booking/booking.vue:1-387`
- Create: `src/pages/booking/booking.enterprise.test.mjs`
- Modify: `src/utils/bookingDraft.test.mjs`
- Modify: `package.json`

- [ ] **Step 1: 更新预约页测试**

断言：

- `onShow` 消费一次性意图。
- 企业内训/团队工作坊/管理者培训均映射 `kind='enterprise'`。
- `intentText` 只在 `form.intent` 为空时预填。
- 已恢复的联系信息不被清空。
- 提交成功进入 `submitted` 页面内状态。
- 成功态包含“查看预约记录”“继续浏览老师课堂”“再提交一个需求”。

- [ ] **Step 2: 重构企业服务页**

页面顺序：企业服务 Hero → 适用场景 → 服务方式 → 合作流程 → 预约表单。读取 `home.enterprise` 和 `home.courses.items`，缺失时使用无数字默认文案。

- [ ] **Step 3: 实现成功态与路由**

提交成功清理草稿并设置 `submitted=true`；查看记录 `navigateTo`；继续课堂 `switchTab`；重新提交重置表单。

- [ ] **Step 4: 测试、观察 watcher、加入总测试并提交**

Run:

```bash
node src/utils/bookingIntent.test.mjs
node src/pages/booking/booking.enterprise.test.mjs
node src/utils/bookingDraft.test.mjs
```

Expected: PASS；轮询 Task 0 watcher 后再次出现构建成功。将 `booking.enterprise.test.mjs` 加入 `test:config`。

```bash
git commit --only -m "feat: build enterprise service booking flow" -- \
  src/pages/booking/booking.vue \
  src/pages/booking/booking.enterprise.test.mjs \
  src/utils/bookingDraft.test.mjs \
  package.json
```

### Task 7: 升级测试小游戏与结果承接

**Files:**
- Modify: `src/pages/test/test.vue:1-470`
- Create: `src/pages/test/test.brand.test.mjs`
- Modify: `src/pages/result/result.vue:1-620`
- Create: `src/pages/result/result.recommendation.test.mjs`
- Modify: `src/api/index.test.mjs`
- Modify: `package.json`

- [ ] **Step 1: 编写失败测试**

测试页断言：

- 文案为“九型测试小游戏/18 道生活情境题/约 3 分钟”。
- 使用统一品牌 Token，不再按性别使用高饱和主题色。
- 答题进度、返回和锁定逻辑仍存在。

结果页断言：

- 请求独立课件并按接口顺序最多展示 2 条。
- 请求失败时隐藏推荐列表但保留课堂入口。
- 企业 CTA 调用 `setBookingIntent({ kind: 'enterprise', intentText: '企业九型工作坊' })`。
- 保留分享、海报、深度报告、关系合盘和重新测试。

- [ ] **Step 2: 运行确认失败**

Run: `node src/pages/test/test.brand.test.mjs && node src/pages/result/result.recommendation.test.mjs`  
Expected: FAIL。

- [ ] **Step 3: Restyle 测试页**

只修改布局和样式，不修改 `start/choose/back/finish/calcType`。统一首屏、选项、进度和按压态。

- [ ] **Step 4: 实现结果页课件推荐和企业 CTA**

复用 `normalizeClassroomContent`、`classroomContentRoute`；推荐区是附加内容，不能阻塞结果页主流程。

- [ ] **Step 5: 测试、观察 watcher、加入总测试并提交**

Run:

```bash
node src/pages/test/test.brand.test.mjs
node src/pages/result/result.recommendation.test.mjs
```

Expected: PASS；轮询 Task 0 watcher 后再次出现构建成功。将两个新增测试加入 `test:config`。

```bash
git commit --only -m "feat: connect enneagram game to expert services" -- \
  src/pages/test/test.vue \
  src/pages/test/test.brand.test.mjs \
  src/pages/result/result.vue \
  src/pages/result/result.recommendation.test.mjs \
  src/api/index.test.mjs \
  package.json
```

---

## Chunk 3: 课堂内容体验

### Task 8: 统一老师课堂入口页和课堂列表

**Files:**
- Modify: `src/pages/learn/learn.vue:1-1033`
- Modify: `src/pages/learn/learn.content-state.test.mjs`
- Modify: `src/pages/learn.quote-card.test.mjs`
- Modify: `src/pages/classroom/classroom.vue:1-1065`
- Modify: `src/pages/classroom/classroom.test.mjs`

- [ ] **Step 1: 更新失败测试**

断言：

- Learn 页对外标题为“老师课堂”，优先展示视频/音频课件和老师专业信息。
- `NxAsyncState` 用于课堂首次失败/空状态/缓存刷新失败。
- classroom 默认 `standalone`，系列可切换。
- 两页均不包含旧绿色或紫色主视觉 Token。
- 空状态文案不出现“后台发布”“你发布的视频”。
- 卡片按钮触控高度不低于 88rpx，去除整卡点击中嵌套按钮冲突。

- [ ] **Step 2: 运行确认失败**

Run:

```bash
node src/pages/learn/learn.content-state.test.mjs
node src/pages/learn.quote-card.test.mjs
node src/pages/classroom/classroom.test.mjs
```

Expected: 至少视觉/文案断言失败。

- [ ] **Step 3: 重构 Learn 页层级**

顺序：老师课堂 Hero → 课堂精选 → 老师简介 → 课程方向/九型内容 → 引用内容。保留缓存刷新与局部失败逻辑。

- [ ] **Step 4: 统一课堂列表**

使用统一媒体卡表面、石墨蓝标签、香槟金 CTA。系列/独立课件均保持现有加载票据、缓存、购买 single-flight 和图片失败逻辑。

- [ ] **Step 5: 修复现有系列运行时测试前置**

所有系列相关用例在 `openSeries()` 前显式：

```js
page.activeTab.value = 'series';
```

保证默认独立课件后旧系列测试仍表达正确语义。

- [ ] **Step 6: 测试、观察 watcher 并提交**

Run:

```bash
node src/pages/learn/learn.content-state.test.mjs
node src/pages/learn.quote-card.test.mjs
node src/pages/classroom/classroom.test.mjs
```

Expected: PASS；轮询 Task 0 watcher 后再次出现构建成功。

```bash
git commit --only -m "feat: unify teacher classroom experience" -- \
  src/pages/learn/learn.vue \
  src/pages/learn/learn.content-state.test.mjs \
  src/pages/learn.quote-card.test.mjs \
  src/pages/classroom/classroom.vue \
  src/pages/classroom/classroom.test.mjs
```

### Task 9: 重构课堂详情并清除旧绿色

**Files:**
- Modify: `src/pages/classroom-detail/classroom-detail.vue:1-1076`
- Modify: `src/pages/classroom-detail/classroom-detail.test.mjs`

- [ ] **Step 1: 添加失败回归断言**

禁止：`#0f766e`、`#0f6b4f`、`#ecfdf5`、对应绿色 rgba。要求出现品牌 Token 或 CSS var；要求视频/音频共用媒体头；加载、错误、购买、播放按钮保留。

- [ ] **Step 2: 运行确认失败**

Run: `node src/pages/classroom-detail/classroom-detail.test.mjs`  
Expected: FAIL，旧绿色仍存在。

- [ ] **Step 3: 仅重构模板层级与样式**

保留所有 API、试看、购买、授权、播放进度、卸载隔离逻辑。统一 Header、媒体容器、内容信息、访问提示和底部操作区。

- [ ] **Step 4: 测试、观察 watcher 并提交**

Run: `node src/pages/classroom-detail/classroom-detail.test.mjs`  
Expected: PASS；轮询 Task 0 watcher 后再次出现构建成功。

```bash
git commit --only -m "feat: restyle classroom detail for expert brand" -- \
  src/pages/classroom-detail/classroom-detail.vue \
  src/pages/classroom-detail/classroom-detail.test.mjs
```

---

## Chunk 4: 其他页面统一与后台文案

### Task 10: 统一关系、档案与预约记录页面

**Files:**
- Modify: `src/pages/relation/relation.vue:1-477`
- Modify: `src/pages/profile/profile.vue:1-383`
- Modify: `src/pages/profile-edit/profile-edit.vue:1-337`
- Modify: `src/pages/booking-records/booking-records.vue:1-411`
- Modify: `src/pages/booking-detail/booking-detail.vue:1-519`
- Modify: `src/pages/profile/profile.session.test.mjs`
- Modify: `src/pages/booking-records/booking-records.session.test.mjs`
- Modify: `src/pages/booking-detail/booking-detail.session.test.mjs`
- Create: `src/pages/relation/relation.brand.test.mjs`
- Modify: `package.json`

- [ ] **Step 1: 编写/更新失败测试**

断言所有页面：

- 使用统一页面壳和品牌 Token。
- 没有大面积紫粉、绿色或橙蓝 Hero。
- 登录、记录加载、错误重试、空状态逻辑仍存在。
- booking detail 与 profile edit 具有统一横向留白。

- [ ] **Step 2: 分页重构样式**

不修改会话隔离、鉴权、历史记录和预约详情数据逻辑。关系页的人格差异使用局部标签/图形，不切换整页主题。

- [ ] **Step 3: 运行页面测试**

Run:

```bash
node src/pages/relation/relation.brand.test.mjs
node src/pages/profile/profile.session.test.mjs
node src/pages/booking-records/booking-records.session.test.mjs
node src/pages/booking-detail/booking-detail.session.test.mjs
```

Expected: PASS。

- [ ] **Step 4: 观察 watcher、加入总测试并提交**

轮询 Task 0 watcher，确认再次构建成功。将 `relation.brand.test.mjs` 加入 `test:config`。

```bash
git commit --only -m "feat: unify supporting miniapp pages" -- \
  src/pages/relation/relation.vue \
  src/pages/profile/profile.vue \
  src/pages/profile-edit/profile-edit.vue \
  src/pages/booking-records/booking-records.vue \
  src/pages/booking-detail/booking-detail.vue \
  src/pages/profile/profile.session.test.mjs \
  src/pages/booking-records/booking-records.session.test.mjs \
  src/pages/booking-detail/booking-detail.session.test.mjs \
  src/pages/relation/relation.brand.test.mjs \
  package.json
```

### Task 11: 更新后台“小程序首页”配置说明

**Files:**
- Modify: `../nx-backend/apps/web-antd/src/views/miniapp/home.vue`
- Modify: `../nx-backend/apps/web-antd/src/views/miniapp/home.test.ts`

- [ ] **Step 1: 更新失败测试**

要求后台显示：

- “九型测试小游戏独立模块”说明。
- `test` 开关控制首页独立小游戏区。
- 其他功能入口仍支持排序。
- 不声称 `test` 仍在通用宫格内。

- [ ] **Step 2: 运行后台定向测试确认失败**

Run with cwd `../nx-backend`:

```bash
pnpm exec vitest run --dom apps/web-antd/src/views/miniapp/home.test.ts
```

Expected: FAIL，旧文案仍为普通入口。

- [ ] **Step 3: 只修改配置说明与标签**

不新增 site config 字段，不触碰其他后台在制改动。

- [ ] **Step 4: 运行测试并限定提交**

Run with cwd `../nx-backend`:

```bash
pnpm exec vitest run --dom apps/web-antd/src/views/miniapp/home.test.ts
git commit --only -m "docs: clarify miniapp test game configuration" -- \
  apps/web-antd/src/views/miniapp/home.vue \
  apps/web-antd/src/views/miniapp/home.test.ts
```

Expected: 定向测试 PASS；提交只包含上述两个文件，其他后台暂存/未提交内容保持不变。

---

## Chunk 5: 全量回归、视觉验收与交付

### Task 12: 重写旧精确颜色测试为设计系统契约

**Files:**
- Modify: `scripts/ui-compat.test.mjs:1-4524`
- Modify: `package.json`

- [ ] **Step 1: 盘点本轮产生的旧断言失败**

Run: `npm run test:config`  
Expected: 输出所有仍锁定旧颜色、旧文案、旧页面顺序的失败。

- [ ] **Step 2: 逐条替换，不删除行为保护**

将精确十六进制和旧渐变断言改成：

- 使用语义 Token。
- 关键页面结构顺序。
- 触控尺寸。
- 错误/空/加载/旧缓存状态。
- 导航和路由。
- 图片失败兜底。
- reduced-motion。

保留登录隔离、支付 single-flight、缓存、防迟到回调等行为测试。

- [ ] **Step 3: 运行全量测试**

先确认 `package.json` 的 `test:config` 显式包含以下新增测试：

```text
src/styles/personal-expert-theme.test.mjs
src/components/NxAsyncState.test.mjs
src/utils/personalExpertHome.test.mjs
src/utils/bookingIntent.test.mjs
src/pages/booking/booking.enterprise.test.mjs
src/pages/test/test.brand.test.mjs
src/pages/result/result.recommendation.test.mjs
src/pages/relation/relation.brand.test.mjs
```

Run: `npm run test:config`  
Expected: PASS，退出码 0。

- [ ] **Step 4: 限定提交兼容测试迁移**

```bash
git commit --only -m "test: align ui contracts with expert brand" -- \
  scripts/ui-compat.test.mjs \
  package.json
```

### Task 13: 最终开发构建与视觉检查

**Files:**
- Verify: `dist/dev/mp-weixin/**`
- Verify: `dist/dev/mp-weixin/config.js`
- Verify: all modified source files

- [ ] **Step 1: 检查差异与格式**

Run:

```bash
git diff --check -- .
git status --short
```

Expected: miniapp 无空白错误；后台无关变更保持原样。

- [ ] **Step 2: 运行全量测试**

Run: `npm run test:config`  
Expected: PASS。

- [ ] **Step 3: 验证唯一测试环境 watcher**

轮询 Task 0 的同一 PTY session。  
Expected: 最近一次修改后出现 `DONE Build complete. Watching for changes...`；不存在第二个重复 watcher。

- [ ] **Step 4: 验证测试 API**

Run:

```bash
rg 'http://127.0.0.1:5320/api' dist/dev/mp-weixin/config.js
```

Expected: 命中本地测试 API。

- [ ] **Step 5: 视觉检查页面**

在微信开发者工具中依次检查：

1. 首页：老师形象、资历、企业服务、小游戏、课堂、轮播。
2. 老师课堂：独立课件默认可见、系列切换、空/错误状态。
3. 课件详情：视频、音频、购买/试看。
4. 企业服务：企业意图预选、草稿、提交成功态。
5. 测试与结果：答题、分享、课堂推荐、企业 CTA。
6. 关系、我的、编辑、预约记录和详情。

- [ ] **Step 6: 最终限定提交**

只提交本计划涉及的 miniapp 文件及后台首页配置说明文件。提交后记录最终 commit 和恢复标签。

---

## Completion Criteria

- [ ] 视觉统一为石墨蓝、暖灰、白和香槟金。
- [ ] 首页首先表达个人专家与企业服务，不再像测试工具首页。
- [ ] 九型测试小游戏以独立互动区呈现且无重复入口。
- [ ] 老师课堂默认显示后台已发布独立课件。
- [ ] 课堂详情无旧绿色。
- [ ] 企业 CTA 能预选 enterprise 并进入完整提交成功闭环。
- [ ] 12 个页面统一留白、按钮、卡片和异步状态。
- [ ] `npm run test:config` 通过。
- [ ] `npm run dev:mp-weixin` watch 构建成功。
- [ ] `dist/dev/mp-weixin/config.js` 仍使用 `http://127.0.0.1:5320/api`。
- [ ] 恢复标签 `miniapp-ui-baseline-before-training-refresh-20260728` 可用。
