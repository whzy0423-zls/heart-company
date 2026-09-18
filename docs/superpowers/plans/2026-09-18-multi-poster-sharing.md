# 多海报模板推广 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 发布多个可选代理海报模板，并在 Flutter 代理端完成邀请码自动填充、预览、保存相册与系统分享。

**Architecture:** 继续使用 `site_configs` JSON，规范化为 `templates` 数组并兼容旧单模板字段。后端 overview 只下发启用模板；管理端维护模板数组；Flutter 端负责选择、Canvas 生成和原生动作。

**Tech Stack:** Go `net/http` + PostgreSQL JSONB, Vue 3 + Ant Design Vue, Flutter Canvas/share_plus/photo_manager(or image_gallery_saver existing dependency).

---

## Chunk 1: 后端模板模型与接口

### Task 1: 扩展海报配置模型

**Files:**
- Modify: `nx-backend/apps/server/internal/server/distribution_poster.go`
- Test: `nx-backend/apps/server/internal/server/distribution_poster_test.go`

- [ ] 写旧配置迁移、重复 ID、无启用模板和多模板边界测试。
- [ ] 运行 `go test ./apps/server/internal/server -run 'Poster'`，确认新测试失败。
- [ ] 增加 `posterTemplate`、`posterConfig.Templates`，实现旧字段迁移、ID 生成、启用模板过滤和全量校验。
- [ ] 保持 GET/PUT 返回规范化数组并兼容现有管理端字段。
- [ ] 运行同一测试并提交 `feat: support multiple poster templates`。

### Task 2: 将模板下发到 App overview

**Files:**
- Modify: `nx-backend/apps/server/internal/server/distribution.go`
- Modify: `nx-backend/apps/server/internal/server/distribution_postgres_test.go`
- Modify: `nx-backend/apps/server/internal/server/distribution_routes_test.go`

- [ ] 增加 overview 响应包含启用 `posterTemplates` 的测试。
- [ ] 实现站点配置读取，过滤管理字段和停用模板，代理无权限或无记录时不下发。
- [ ] 运行 Go 分发测试并提交 `feat: expose active poster templates to app`。

## Chunk 2: 管理端多模板编辑

### Task 3: API 类型和组件状态

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/distribution-poster.ts`
- Modify: `nx-backend/apps/web-antd/src/views/app/distribution-poster-composer.vue`
- Test: `nx-backend/apps/web-antd/src/views/app/distribution-poster-composer.test.ts`

- [ ] 先补模板数组解析、复制、删除、排序、启停和保存请求测试。
- [ ] 将单模板 refs 收敛为当前模板 + 模板列表，增加新增/复制/删除/排序/启停操作。
- [ ] 保留拖拽编辑器，切换模板时加载对应布局并实时预览。
- [ ] 至少保留一个模板和一个启用模板；保存时提交完整数组。
- [ ] 运行 `pnpm vitest run apps/web-antd/src/views/app/distribution-poster-composer.test.ts apps/web-antd/src/views/app/distribution-management.test.ts`，提交 `feat: manage multiple poster templates`。

## Chunk 3: Flutter 代理端海报体验

### Task 4: 解析模板并持久化选择

**Files:**
- Modify: `lib/features/distribution/screens/distribution_screen.dart`
- Modify: `lib/features/distribution/distribution_repository.dart`
- Test: `test/features/distribution/distribution_screen_test.dart`

- [ ] 补 overview 模板解析、停用模板过滤和选择回退测试。
- [ ] 增加 `PosterTemplate` 数据模型，读取 `posterTemplates`，使用 SharedPreferences 保存上次选择 ID。
- [ ] 增加模板选择器和移动端海报预览区域，邀请码固定来自 overview。

### Task 5: 生成、保存和分享海报

**Files:**
- Modify: `lib/features/distribution/screens/distribution_screen.dart`
- Modify: `pubspec.yaml`（仅在现有依赖不足时）
- Test: `test/features/distribution/distribution_screen_test.dart`

- [ ] 先补无邀请码/无模板时动作禁用和按钮可见性测试。
- [ ] 使用 Flutter Canvas 绘制背景、官网二维码和邀请码；生成临时 PNG。
- [ ] 接入系统相册保存和 `share_plus` 分享，处理权限拒绝、取消和失败重试。
- [ ] 运行 `flutter analyze` 与定向 `flutter test`，提交 `feat: add mobile poster selection and sharing`。

## Chunk 4: 集成验证与部署

### Task 6: 全量测试和构建

- [ ] 运行 Go、Vue、Flutter 定向及全量相关测试。
- [ ] 构建 server/admin/website 和 Flutter 检查产物。
- [ ] 检查 git diff，保留用户未提交改动，不覆盖无关文件。

### Task 7: 部署与冒烟验证

- [ ] 推送 `main`，确认远端提交一致。
- [ ] 在 `/tmp/nine-xing-<commit>` 使用服务器现有 `.env`、证书和 Docker volumes 构建并强制重建 server/admin。
- [ ] 检查容器状态、server 日志、官网 `/app` 与后台 `/` 返回 `200`。
- [ ] 提交部署结果和可操作的代理端使用说明。
