# 芯之力动效新版官网 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新建一个高级、有质感、业务完整且与旧官网完全隔离的动效官网。

**Architecture:** 从 `website-react` 创建独立的 `website-react-motion`，保留业务数据、路由和 API 契约；在新版内部新增独立的 motion primitives 与新版首页呈现。使用源码测试验证隔离与业务完整性，使用浏览器截图验证响应式和动态呈现。

**Tech Stack:** React 18、React Router、Vite、CSS、Web Animations / IntersectionObserver、Node test、Playwright 浏览器验收。

---

## Chunk 1: 隔离与契约

### Task 1: 创建独立新版项目

**Files:**
- Create: `website-react-motion/**`
- Preserve: `website-react/**`

- [ ] 在旧站目录之外记录 `website-react` 排除既有 `node_modules`、`dist` 和上传缓存后的完整相对路径与 SHA-256 清单。
- [ ] 复制源码、公共资源、脚本和项目配置到 `website-react-motion`，排除依赖与构建产物。
- [ ] 修改新版包名和 Vite 开发端口，不改旧版文件。
- [ ] 安装新版依赖并确认旧版哈希未变化。

### Task 2: 用失败测试固定新版要求

**Files:**
- Create: `website-react-motion/src/motion/motion-contract.test.mjs`
- Create: `website-react-motion/src/pages/motion-home-contract.test.mjs`
- Create: `website-react-motion/scripts/verify-old-site-untouched.mjs`

- [ ] 编写测试，要求新版包含独立动态背景、九型轨道、滚动揭示和降低动态支持。
- [ ] 编写测试，要求旧版首页的业务区块、路由和 API 契约在新版继续存在。
- [ ] 运行测试并确认因为新版动效实现尚不存在而失败。
- [ ] 添加旧版哈希校验脚本，并验证基线检查可执行。
- [ ] 校验脚本执行双向路径集合对比，任何新增、删除或内容变化均失败。
- [ ] 建立旧版 16 个路由、API、表单、下载、游戏和媒体资源的显式业务矩阵。

## Chunk 2: 设计系统与动效基础设施

### Task 3: 建立新版设计令牌

**Files:**
- Create: `website-react-motion/src/styles/tokens.css`
- Create: `website-react-motion/src/styles/motion.css`
- Modify: `website-react-motion/src/index.css`

- [ ] 定义颜色、字体、间距、阴影、层级、动效时长和缓动令牌。
- [ ] 建立可见焦点、44px 触控目标和降低动态规则。
- [ ] 替换旧版极光装饰为克制的纸张噪点、网格与光域系统。
- [ ] 运行契约测试并修正样式契约。

### Task 4: 建立 motion primitives

**Files:**
- Create: `website-react-motion/src/motion/MotionProvider.jsx`
- Create: `website-react-motion/src/motion/useMotionPreferences.js`
- Create: `website-react-motion/src/motion/usePointerField.js`
- Create: `website-react-motion/src/components/MotionBackdrop.jsx`
- Create: `website-react-motion/src/components/EnneagramOrbit.jsx`
- Modify: `website-react-motion/src/components/Layout.jsx`

- [ ] 实现系统动态偏好检测和统一 CSS 状态。
- [ ] 实现低频指针光域与滚动变量更新，避免逐组件监听。
- [ ] 实现动态九型轨道，使用稳定尺寸并提供静态降级。
- [ ] 让全局布局挂载新版背景和动效上下文。
- [ ] 运行测试与构建。

## Chunk 3: 首页与全局视觉

### Task 5: 重构新版首页首屏

**Files:**
- Modify: `website-react-motion/src/pages/Home.jsx`
- Create: `website-react-motion/src/pages/MotionHomeSections.jsx`
- Modify: `website-react-motion/src/components/Nav.jsx`

- [ ] 将品牌、九型人格业务和主行动放入第一视口。
- [ ] 接入动态九型轨道、滚动提示和业务统计。
- [ ] 保留原行动目标和路由。
- [ ] 用新版导航强化品牌，同时保持移动抽屉功能。

### Task 6: 重排老业务区块

**Files:**
- Modify: `website-react-motion/src/pages/Home.jsx`
- Modify: `website-react-motion/src/pages/MotionHomeSections.jsx`
- Modify: `website-react-motion/src/components/AppDownloadSection.jsx`

- [ ] 将九型、三阶段课程、导师、视频、金句、企业服务、App 下载和报名全部纳入新版首页。
- [ ] 复用原站点配置、业务数据和提交 API，不复制硬编码业务文案。
- [ ] 给每个章节配置唯一且克制的滚动动作。
- [ ] 运行首页业务契约测试。

### Task 7: 统一内页质感

**Files:**
- Modify: `website-react-motion/src/index.css`
- Modify: `website-react-motion/src/components/Footer.jsx`
- Modify: `website-react-motion/src/components/Tabbar.jsx`

- [ ] 让全部旧路由继承新版排版、表面、按钮、卡片和表单语言。
- [ ] 避免卡片嵌套、装饰性圆球和单色渐变堆砌。
- [ ] 分别审视导师/阶段、课程/课件、视频、游戏、九型列表/详情、金句/心语列表/详情和报名页面。
- [ ] 检查长标题、视频、课件、游戏、心语和九型详情在移动端不溢出。
- [ ] 对全部 16 个路由执行浏览器 smoke test，并验证运行时配置、访问统计、报名、游戏结果、视频音乐互斥和 App 发布/下载契约。
- [ ] 运行完整测试与构建。

## Chunk 4: 视觉与隔离验收

### Task 8: 浏览器验证

**Files:**
- Create: `website-react-motion/artifacts/visual-checks/*`

- [ ] 启动新版开发服务器并检查控制台错误。
- [ ] 在 1440x900、1024x768、390x844 视口截图首页及每一类内页。
- [ ] 对首页全部动效章节记录滚动前后状态与截图，检查品牌首屏、文字揭示、指针响应、章节转换、按钮、锚点、表单和视频入口。
- [ ] 自动断言 `scrollWidth <= clientWidth`、关键元素边界合法且控制台无错误。
- [ ] 模拟 `prefers-reduced-motion: reduce`，断言持续动画、平滑滚动和视差被关闭且内容立即可读。
- [ ] 修复发现的遮挡、溢出、空白或对比度问题。

### Task 9: 最终证明

**Files:**
- Verify: `website-react/**`
- Verify: `website-react-motion/**`

- [ ] 运行 `npm test`。
- [ ] 运行 `npm run build`。
- [ ] 重新生成旧版完整路径与哈希清单，与基线做双向比较。
- [ ] 确认 Git diff 中没有 `website-react/**` 变更。
- [ ] 提供新版本地访问地址和验证摘要。
