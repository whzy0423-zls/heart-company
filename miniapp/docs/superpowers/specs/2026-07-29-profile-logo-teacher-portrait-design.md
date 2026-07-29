# 个人档案 Logo 与首页导师人像设计

## 目标

- 个人档案未登录状态使用九型 Logo，替代单独的“九”字标识。
- 登录后用户没有头像或头像加载失败时，同样使用九型 Logo。
- 首页导师区域使用清晰、无小字的导师人像，避免把完整介绍海报缩进窄画框后无法阅读。
- 用户点击导师人像，可全屏查看完整导师介绍海报。

## 视觉方案

### 个人档案

- Logo 复用小程序现有 `/static/wheel.png`。
- 未登录 Hero 中保留现有圆角、深蓝背景和香槟金边框，Logo 使用 `aspectFit` 并留出内边距。
- 登录态有头像时继续显示用户头像；没有头像或加载失败时显示相同 Logo，不再显示文字占位。

### 首页导师区域

- 画框内使用公共站点 `/assets/teacher.jpg` 清晰人像。
- 保留现有深蓝 Hero、圆拱画框和金色边框，人像使用裁切模式覆盖画框。
- 画框底部增加半透明提示条“查看完整导师介绍”。
- 点击整个导师画框调用小程序图片预览，显示 `/assets/teacher-poster.jpg` 完整介绍海报。
- 左侧导师姓名、简介、企业课程和老师课堂入口不变。

## 数据兼容

- 导师模型新增 `portraitImage` 与 `detailImage`：
  - `portraitImage` 优先读取 `portraitImage/avatar/photo`，否则使用 `/assets/teacher.jpg`。
  - `detailImage` 优先读取 `detailImage/poster/image`，否则使用 `/assets/teacher-poster.jpg`。
- 所有公共媒体路径继续使用 `resolveContentAsset()`，兼容生产 HTTPS 与开发环境。
- 保留原 `image` 字段作为完整海报兼容来源，避免现有后台配置失效。

## 交互与异常处理

- `detailImage` 有效时导师画框可点击，并提供清晰的无障碍标签。
- `detailImage` 缺失时不触发预览。
- 人像加载失败时继续显示现有“九”字导师占位，不影响首页其他内容。
- 用户头像加载失败只影响头像区域，显示九型 Logo。

## 验证

- 模型测试覆盖导师人像与完整海报路径、旧 `image` 兼容和安全回退。
- 首页契约测试覆盖可点击导师画框、预览调用、提示条与人像绑定。
- 个人中心测试覆盖未登录 Logo、无头像 Logo 和头像失败回退。
- 执行 `npm run test:config`、`npm run build:mp-weixin`、`npm run dev:mp-weixin`，并在微信开发者工具验收。
