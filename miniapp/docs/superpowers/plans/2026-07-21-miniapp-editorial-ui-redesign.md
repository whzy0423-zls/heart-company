# 九型芯之力小程序 Editorial UI 改版实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变测试算法、登录、预约、支付接口和后台逻辑的前提下，把七个 uni-app 页面改造成已确认的编辑拼贴式视觉系统，并保证 H5 与微信小程序可构建。

**Architecture:** 在全局样式中建立语义 token 与基础组件，新增无业务依赖的九型徽章组件和报告展示状态工具，再按核心流程页、内容页、表单与个人页渐进改造。视觉资产放入独立静态目录并带尺寸和兜底；原生 tabBar 保持不变，只更新图标资源。

**Tech Stack:** uni-app 3、Vue 3 `<script setup>`、CSS/rpx、Node `.mjs` 测试、微信原生 tabBar、H5/Vite。

**Spec:** `docs/superpowers/specs/2026-07-21-miniapp-editorial-ui-redesign-design.md`

---

## 文件结构

新增：

- `src/components/type-badge.vue`：1～9 型统一徽章。
- `src/components/type-badge.test.mjs`：徽章 props、状态和回退契约。
- `src/utils/typeTheme.js` / `typeTheme.test.mjs`：型号颜色与安全回退。
- `src/utils/questionVisuals.js` / `questionVisuals.test.mjs`：12 题到三中心插画的纯展示映射。
- `src/utils/reportPresentation.js` / `reportPresentation.test.mjs`：报告价格、解锁和 H5 展示状态。
- `src/static/editorial/`：首页、三中心、结果和课程兜底视觉资产。
- `scripts/verify-editorial-assets.py`：用 Pillow 验证格式、尺寸和字节预算。

修改：

- `src/styles/apple-mobile.css`、`src/App.vue`：全局设计系统。
- `src/pages/*/*.vue`：七个页面。
- `src/pages.json`、`src/static/tabbar/*.png`：原生 tabBar。
- `scripts/ui-compat.test.mjs`、`package.json`：新视觉与可访问性测试。

---

## Chunk 1：设计系统基础

### Task 1：迁移 UI 兼容测试

**Files:** `scripts/ui-compat.test.mjs`, `package.json`

- [ ] 写失败断言：要求 `--nx-bg`、`--nx-ink`、`--nx-blue`、`--nx-coral`、`--nx-error`、`--nx-focus`，按钮高度至少 88rpx，并允许首页首屏图不懒加载。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期因新 token 不存在而 FAIL。
- [ ] 删除“所有区块必须 `ios-card`”“所有图片必须 lazy-load”等旧视觉绑定断言。
- [ ] 保留安全区、触控尺寸、禁用态、错误反馈、H5 条件编译和海报例外断言。
- [ ] 暂不提交；新断言保持 RED，和 Task 2、3 的实现一起形成可通过的 Chunk 提交。

### Task 2：建立型号主题和徽章

**Files:** `src/utils/typeTheme.js`, `src/utils/typeTheme.test.mjs`, `src/components/type-badge.vue`, `src/components/type-badge.test.mjs`, `package.json`

- [ ] 写失败测试，断言 1/6/9 型色值和非法型号回退 `#68727C`；徽章源码契约要求 `typeId/size/selected/label/disabled`、禁用语义和安全回退。
- [ ] 运行 `node src/utils/typeTheme.test.mjs && node src/components/type-badge.test.mjs`，预期模块/组件不存在而 FAIL。
- [ ] 实现 `TYPE_THEME` 和 `typeTheme(typeId)`，返回 `accent/soft/ink`。
- [ ] 实现徽章 props：`typeId`、`size`、`selected`、`label`、`disabled`。
- [ ] 将两个新测试显式追加到 `package.json` 的 `test:config` 链中。
- [ ] 运行 `node src/utils/typeTheme.test.mjs && node src/components/type-badge.test.mjs`，预期 PASS。
- [ ] 暂不提交；等待 Task 3 恢复全套 GREEN。

### Task 3：重构全局样式

**Files:** `src/styles/apple-mobile.css`, `src/App.vue`

- [ ] 添加规范中的色彩、圆角、阴影和 4/8 间距 token，旧 token 暂时别名到新 token。
- [ ] 实现 `.nx-page`、`.nx-editorial-hero`、`.nx-panel`、`.nx-media-row`、`.nx-quote`。
- [ ] 实现 primary、conversion、secondary、text 四级按钮和表单/空/错误状态。
- [ ] 加入 disabled、loading、pressed、focus-visible、safe-area 和 reduced-motion。
- [ ] 运行 `npm run test:config`，预期包含新测试且全部 PASS。
- [ ] 提交 Chunk：`git add scripts/ui-compat.test.mjs package.json src/App.vue src/styles/apple-mobile.css src/utils/typeTheme.js src/utils/typeTheme.test.mjs src/components/type-badge.vue src/components/type-badge.test.mjs && git commit -m "feat: establish miniapp editorial design system"`。

---

## Chunk 2：首页与答题流程

### Task 4：准备首批视觉资产

**Files:** `src/static/editorial/home-hero.webp`, `center-head.webp`, `center-heart.webp`, `center-gut.webp`, `course-intro.webp`, `course-growth.webp`, `course-relation.webp`, `scripts/verify-editorial-assets.py`

- [ ] 使用 `imagegen` 生成项目自有源图，不使用第三方版权素材。
- [ ] 使用 Pillow 转换 WebP：首页 1200×900 `<220KB`；三中心 720×480、单张 `<120KB`；三张课程封面 800×500、单张 `<140KB`。
- [ ] 先写 `scripts/verify-editorial-assets.py` 的尺寸、格式、数量和字节预算断言；支持 `--group initial` 与 `--group all`。资产生成前运行 `python3 scripts/verify-editorial-assets.py --group initial`，预期因文件缺失而 FAIL。
- [ ] 生成资产后再次运行 `python3 scripts/verify-editorial-assets.py --group initial`，预期 PASS。
- [ ] 提交：`git add scripts/verify-editorial-assets.py src/static/editorial && git commit -m "feat: add editorial miniapp visual assets"`。

### Task 5：首页

**Files:** `src/pages/index/index.vue`, `scripts/ui-compat.test.mjs`

- [ ] 写失败断言：唯一“开始测试”主按钮、主视觉、九型徽章、关系入口、学习入口、推荐内容入口。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期缺少新首页结构而 FAIL。
- [ ] 用编辑式 hero 替换重复卡片；保留 `startTest/goLearn/goRelation`。
- [ ] 推荐内容固定使用 `DEFAULT_COURSEWARE_ITEMS[0]` 作为本地兜底展示，点击只 `switchTab` 到学一学，不引入首页异步配置状态。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期 PASS。
- [ ] 提交：`git add src/pages/index/index.vue scripts/ui-compat.test.mjs && git commit -m "feat: redesign miniapp home experience"`。

### Task 6：答题页

**Files:** `src/pages/test/test.vue`, `src/utils/questionVisuals.js`, `src/utils/questionVisuals.test.mjs`, `scripts/ui-compat.test.mjs`, `package.json`

- [ ] 写失败断言：进度、题号、中心插画、选项状态、上一题弱操作、88rpx 触控区域。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期缺少新答题结构而 FAIL。
- [ ] 写 `QUESTION_VISUAL_CENTERS` 失败测试，要求 12 个题目索引均映射到 `head/heart/gut`，且不读取/修改选项分值。
- [ ] 运行 `node src/utils/questionVisuals.test.mjs`，预期模块不存在而 FAIL。
- [ ] 实现纯展示映射并将测试加入 `test:config`；页面按题目索引选择插画，不从答题结果推断中心。
- [ ] 保留 `choose`、自动前进、防连点、`back` 和 `finish` 语义。
- [ ] 运行 `npm run test:config`，预期 PASS。
- [ ] 提交：`git add src/pages/test/test.vue src/utils/questionVisuals.js src/utils/questionVisuals.test.mjs scripts/ui-compat.test.mjs package.json && git commit -m "feat: redesign enneagram quiz flow"`。

---

## Chunk 3：结果页与报告状态

### Task 7：报告展示状态

**Files:** `src/utils/reportPresentation.js`, `src/utils/reportPresentation.test.mjs`, `package.json`

- [ ] 写失败测试覆盖 unsaved/loading/locked/unlocked/error/h5-disabled，未存档不得返回价格。
- [ ] 运行 `node src/utils/reportPresentation.test.mjs`，预期模块不存在而 FAIL。
- [ ] 实现纯函数，输出 state、title、actionLabel、actionDisabled、showPrice、canRetry。
- [ ] 将测试显式追加到 `package.json` 的 `test:config`。
- [ ] 运行 `node src/utils/reportPresentation.test.mjs && npm run test:config`，预期 PASS。
- [ ] 提交：`git add src/utils/reportPresentation.js src/utils/reportPresentation.test.mjs package.json && git commit -m "feat: model report presentation states"`。

### Task 8：结果页和海报

**Files:** `src/pages/result/result.vue`, `src/static/editorial/result-1.webp` 至 `result-9.webp`, `scripts/ui-compat.test.mjs`

- [ ] 写失败断言：型号主视觉、三中心、报告状态、H5 禁用提示、微信专属动作条件编译，以及结果图片 `@error` 后切换到现有头像且不会重复绑定错误源。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期缺少结果页新结构/H5 状态/图片回退而 FAIL。
- [ ] 生成 9 张 640×640、单张 `<150KB` 的结果视觉；实现单次 `@error` 回退到现有头像。
- [ ] 运行 `python3 scripts/verify-editorial-assets.py --group all`，预期全部 16 张资产格式、尺寸和体积 PASS。
- [ ] 未存档显示“先存档，查看报告价格”；存档后查询真实价格；错误可重试。
- [ ] H5 禁用保存/支付，隐藏微信分享和保存海报。
- [ ] 重排为基础结果 → 三中心 → 成长/压力 → 报告 → 分享/存档 → 合盘/预约。
- [ ] 更新 Canvas 海报颜色和排版，不改变保存流程。
- [ ] 运行 `npm run test:config`，预期 PASS。
- [ ] 提交：`git add src/pages/result/result.vue src/static/editorial/result-*.webp scripts/ui-compat.test.mjs && git commit -m "feat: redesign personality result experience"`。

---

## Chunk 4：学习与关系合盘

### Task 9：学习页

**Files:** `src/pages/learn/learn.vue`, `scripts/ui-compat.test.mjs`

- [ ] 写失败断言：老师媒体区、课程媒体行、语录引用、紧凑徽章网格、loading/error/empty/retry，以及课程图片 `@error` 后只切换一次本地兜底封面。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期缺少学习页新结构/图片回退而 FAIL。
- [ ] 保留现有老师/课程 normalize 和缓存逻辑。
- [ ] 课程卡只展示，不直接打开 URL；图片失败使用兜底封面。
- [ ] 运行 `npm run test:config`，预期 PASS。
- [ ] 提交：`git add src/pages/learn/learn.vue scripts/ui-compat.test.mjs && git commit -m "feat: redesign miniapp learning hub"`。

### Task 10：关系合盘

**Files:** `src/pages/relation/relation.vue`, `scripts/ui-compat.test.mjs`

- [ ] 写失败断言：双方使用徽章选择，生成前一个主按钮，结果含“优势与相处底色”/摩擦/建议。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期缺少关系页新结构而 FAIL。
- [ ] 保留 `buildAnalysis`、非法型号回退和页面参数语义。
- [ ] 只调整选择器与结果展示；现有 `analysis.bond` 显示为“优势与相处底色”，不创造新的分析字段。
- [ ] 运行 `npm run test:config`，预期 PASS。
- [ ] 提交：`git add src/pages/relation/relation.vue scripts/ui-compat.test.mjs && git commit -m "feat: redesign relationship matching"`。

---

## Chunk 5：预约与我的

### Task 11：预约页和 H5 状态

**Files:** `src/pages/booking/booking.vue`, `scripts/ui-compat.test.mjs`

- [ ] 写失败断言：联系/意向/补充分组，H5 submit disabled，小程序提示，字段错误就近显示。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期缺少分组/H5 禁用结构而 FAIL。
- [ ] 保留草稿、校验、提交中保护和小程序提交逻辑。
- [ ] H5 允许填写但不调用 `ensureLogin`。
- [ ] 运行 `node src/utils/bookingDraft.test.mjs && node src/utils/auth.test.mjs && node scripts/ui-compat.test.mjs`，预期 PASS。
- [ ] 提交：`git add src/pages/booking/booking.vue scripts/ui-compat.test.mjs && git commit -m "feat: redesign booking form experience"`。

### Task 12：个人中心

**Files:** `src/pages/profile/profile.vue`, `scripts/ui-compat.test.mjs`

- [ ] 写失败断言：未登录价值说明、H5 限制、用户头部、资料编辑、测试/预约时间线、低优先级退出。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期缺少个人中心新结构而 FAIL。
- [ ] 保留 `loadAll`、并发加载、过期清理和微信资料同步语义。
- [ ] 只调整模板、状态展示和页面样式。
- [ ] 运行 `npm run test:config`，预期 PASS。
- [ ] 提交：`git add src/pages/profile/profile.vue scripts/ui-compat.test.mjs && git commit -m "feat: redesign miniapp profile and history"`。

---

## Chunk 6：导航、构建与视觉验收

### Task 13：原生 tabBar

**Files:** `src/pages.json`, `src/static/tabbar/*.png`, `scripts/ui-compat.test.mjs`

- [ ] 写失败断言：4 组默认/选中图标存在，`selectedColor` 为 `#315BEA`，仍使用原生 tabBar。
- [ ] 运行 `node scripts/project-config.test.mjs && node scripts/ui-compat.test.mjs`，预期颜色/图标断言 FAIL。
- [ ] 生成同一笔画和视觉尺寸的图标；浅蓝选中背景烘焙进 selected PNG。
- [ ] 运行 `node scripts/project-config.test.mjs && node scripts/ui-compat.test.mjs`，预期 PASS。
- [ ] 提交：`git add src/pages.json src/static/tabbar scripts/ui-compat.test.mjs && git commit -m "feat: refresh native miniapp tab bar"`。

### Task 14：全量验证

- [ ] 运行 `npm run test:config`，预期 0 failures。
- [ ] 运行 `npm run build:h5`，预期 exit 0。
- [ ] 运行 `npm run build:mp-weixin`，预期 exit 0。
- [ ] 启动 `npm run dev:h5 -- --host 127.0.0.1 --port 5175`。
- [ ] 检查七个页面在 375px、390px、768px 下无横向滚动和 tabBar 遮挡。
- [ ] 检查 H5 保存、支付、分享、预约不会触发微信流程。
- [ ] 运行 `python3 scripts/verify-editorial-assets.py --group all`，预期所有资产格式、尺寸和体积 PASS。
- [ ] 检查图片体积：`find src/static/editorial -maxdepth 1 -type f -print0 | xargs -0 du -h`。
- [ ] 运行 `git status --short`，确认无意外文件。
- [ ] 若验证产生必要修正，按 `git status --short` 列出的具体文件执行 `git add <files> && git commit -m "fix: finalize miniapp editorial ui verification"`；若工作区干净则不创建空提交。

---

## Chunk 7：首轮验收后的视觉层次深化

### Task 15：首页层次深化

**Files:** `src/pages/index/index.vue`, `scripts/ui-compat.test.mjs`

- [ ] 先写失败断言：全幅/重叠 hero 层、非交互悬浮徽章层、不同尺寸 Bento 区块、章节背景层、装饰层同时具备 `pointer-events: none` 与 `aria-hidden="true"`、平板双栏规则、reduced-motion；并禁止全页 `filter`/`backdrop-filter` 和持续纹理动画。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期缺少深化结构而 FAIL。
- [ ] 保留唯一主 CTA、三个原有导航函数和本地推荐课程数据；只调整模板层次与 CSS。
- [ ] 首页图片与文字形成叠层，但正文对比度保持 AA，按钮层级高于装饰层。
- [ ] 运行 `node scripts/ui-compat.test.mjs && npm run test:config && npm run build:h5 && npm run build:mp-weixin`，预期全部 PASS。
- [ ] 启动 `npm run dev:h5 -- --host 127.0.0.1 --port 5176`，分别以 375×812、390×844、768×1024 视口检查首页并保存截图；确认无横向滚动、hero 文本有高对比承载层、Bento 尺度明显不同、三个导航按钮可点击且 Tab 可聚焦、装饰层不截获事件。
- [ ] 提交：`git add src/pages/index/index.vue scripts/ui-compat.test.mjs && git commit -m "feat: enrich homepage editorial depth"`。

### Task 16：答题页层次深化

**Files:** `src/pages/test/test.vue`, `scripts/ui-compat.test.mjs`

- [ ] 先写失败断言：三中心氛围 class、选项编号/侧边强调/选中填充、题目切换动效、reduced-motion、768px 双栏。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期缺少深化样式而 FAIL。
- [ ] 根据现有 `questionVisualCenter(step)` 只派生展示 class，不改映射和评分。
- [ ] 保留焦点移动、live region、220ms 最终反馈、按钮语义和 88rpx 触控区。
- [ ] 运行 `node scripts/ui-compat.test.mjs && npm run test:config && npm run build:h5 && npm run build:mp-weixin`，预期全部 PASS。
- [ ] 复用 `http://127.0.0.1:5176/#/pages/test/test`，分别以 375×812、390×844、768×1024 视口检查性别页和至少一题答题状态并保存截图；确认插画仍为辅助层、前三个选项可见或可自然滚动、中心主题下文字对比度达到 AA、选项可点击/Tab 聚焦、无横向滚动。
- [ ] 提交：`git add src/pages/test/test.vue scripts/ui-compat.test.mjs && git commit -m "feat: enrich quiz visual rhythm"`。

---

## Chunk 8：老师品牌与课件资料最终方向

### Task 17：打通后台老师资料

**Files:** `src/utils/teacherCourseware.js`, `src/utils/teacherCourseware.test.mjs`

- [ ] 写失败测试：`normalizeTeachers({ home: { teacherTeaser: ... } })` 能返回后台老师姓名、身份、图片和简介；空配置仍使用本地兜底。
- [ ] 运行 `node src/utils/teacherCourseware.test.mjs`，预期 teacherTeaser 用例 FAIL。
- [ ] 实现 `home.teacherTeaser` 兼容，不破坏现有 `teacher/teachers/home.teacher` 来源优先级。
- [ ] 运行 `node src/utils/teacherCourseware.test.mjs && npm run test:config`，预期 PASS。
- [ ] 提交：`git add src/utils/teacherCourseware.js src/utils/teacherCourseware.test.mjs && git commit -m "feat: connect miniapp teacher content"`。

### Task 18：重做老师课程型首页

**Files:** `src/pages/index/index.vue`, `scripts/ui-compat.test.mjs`

- [ ] 写失败断言：老师照片主视觉、老师身份与简介、唯一“开始学习”主按钮、精选课程、课件资料、次要“九型自测”入口、后台缓存/刷新和兜底状态。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期新首页契约 FAIL。
- [ ] 使用 `getStoredSiteConfig/refreshSiteConfig`、`normalizeTeachers/normalizeCoursewareItems`；先展示缓存或本地兜底，再静默刷新。
- [ ] 使用米白/墨色/深绿/朱红出版物视觉；减少圆角卡片和装饰徽章；老师主图固定比例并带单次本地回退。
- [ ] 保留原有测试、学习和关系导航函数；“开始学习”进入学一学，“九型自测”进入测试。
- [ ] 运行 `node scripts/ui-compat.test.mjs && npm run test:config && npm run build:h5 && npm run build:mp-weixin`，预期 PASS。
- [ ] 启动 H5 并在 375×812、390×844、768×1024 检查老师视觉、课程书刊比例、对比度、焦点和无横向滚动。
- [ ] 提交：`git add src/pages/index/index.vue scripts/ui-compat.test.mjs && git commit -m "feat: redesign home around teacher and courses"`。

### Task 19：重做老师与课件资料页

**Files:** `src/pages/learn/learn.vue`, `scripts/ui-compat.test.mjs`

- [ ] 写失败断言：编辑式老师介绍、课程出版物列表、资料类型/时长、语录引用、后置紧凑九型索引，以及 loading/error/empty/retry 和图片单次回退。
- [ ] 运行 `node scripts/ui-compat.test.mjs`，预期新学习页契约 FAIL。
- [ ] 保留现有站点配置缓存、老师/课程 normalize 和重试逻辑；不新增课程路由。
- [ ] 页面采用老师访谈、书架和资料目录式布局；主要操作引导测试或预约，九型图鉴降级。
- [ ] 运行 `node scripts/ui-compat.test.mjs && npm run test:config && npm run build:h5 && npm run build:mp-weixin`，预期 PASS。
- [ ] 在三个视口检查老师图文、课程封面、长简介、空/错误状态和按钮焦点。
- [ ] 提交：`git add src/pages/learn/learn.vue scripts/ui-compat.test.mjs && git commit -m "feat: redesign teacher and courseware learning page"`。
