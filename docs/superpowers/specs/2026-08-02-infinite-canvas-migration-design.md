# 无限画布精简移植设计

## 目标

后台仅保留“视频生成”一级菜单，并在其下提供“无限画布”入口。移植 `basketikun/infinite-canvas` 的核心画布能力，移除旧视频后台页面和上游外围功能。

## 架构

- 在 `nx-backend/apps/infinite-canvas` 建立独立 React/Vite 子应用，避免 React 依赖污染现有 Vue/Vben 后台。
- 后台 `/video/infinite-canvas` 使用同源 iframe 承载画布；开发环境支持配置独立画布地址，生产环境使用 `/infinite-canvas/`。
- 保留画布项目、节点、拖拽缩放、连线、撤销重做、本地持久化和导入导出。
- 移除上游首页、独立图片/视频页、Agent、Codex/Claude、插件市场、WebDAV、账号、版本、广告和赞助入口。
- 旧视频后端 API 暂时保留，只删除后台旧路由和页面，避免影响其他客户端或后续画布 API 接入。

## 菜单与页面

```text
视频生成
└── 无限画布  /video/infinite-canvas
```

Vue 包装页提供加载状态、加载失败提示、重新加载和独立打开入口。画布在后台内容区域内全尺寸显示。

## 构建与部署

- 后台默认同源读取 `/infinite-canvas/index.html#/canvas`，本地单独开发画布时可用 `VITE_INFINITE_CANVAS_URL` 指向端口 `3100`。
- 子应用 Vite `base` 为 `/infinite-canvas/`，构建产物输出到后台静态目录，随后台一起部署。
- 后台可通过 `VITE_INFINITE_CANVAS_URL` 覆盖 iframe 地址。

## 许可证

保留上游 AGPL-3.0 `LICENSE`，增加 `UPSTREAM.md`，记录来源、作者、同步提交和精简修改说明。

## 验收

1. 视频生成菜单下只有“无限画布”。
2. 旧视频页面不再参与前端构建。
3. 无限画布可打开、创建项目、创建及拖动节点、缩放、保存并重新载入。
4. Vue 后台和 React 子应用均通过类型检查与生产构建。
5. 许可证和来源说明完整保留。
