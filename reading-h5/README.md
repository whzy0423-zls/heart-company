# 芯之力 · 读书 H5

九型芯之力的读书 H5 页面。后台「阅读管理 → 文章管理」配置文章（Markdown 正文），这里以列表 + 阅读页的形式展示给用户。

## 技术栈

- React 18 + React Router 6
- Vite 7
- `marked` 渲染 Markdown 正文，`isomorphic-dompurify` 净化 HTML

## 开发

```bash
npm install
npm run dev   # 默认 http://localhost:5330，/api 代理到 http://localhost:5320
```

需要后端 Go server（`nx-backend/apps/server`）在 5320 端口运行，以提供：

- `GET /api/public/articles` 文章列表（已发布）
- `GET /api/public/articles/:id` 文章详情（含正文，自增阅读量）
- `GET /api/public/article-categories` 分类列表

## 构建

```bash
npm run build      # 产物输出到 dist/
npm run preview
```

可通过 `VITE_API_BASE_URL` 注入线上 API 路由前缀，默认走同源 `/api`。

## 生产部署

- 如果 H5 与 API 同源部署，必须确保网关或 Nginx 已把同源 `/api` 正确反代到后端服务。
- 如果 H5 走 CDN 或与 API 不同域，必须显式设置 `VITE_API_BASE_URL` 为包含后端路由前缀的真实 HTTPS 地址，通常是 `https://api.example.com/api`，不能只写 `https://api.example.com`。
- 文章 JSON 接口、封面图、音频等媒体资源都会基于同一个 `VITE_API_BASE_URL` 解析；跨域部署时同步配置后端 `CORS_ALLOWED_ORIGINS`。

## 安全说明

阅读页正文先经 `marked` 渲染，再用 DOMPurify 净化后注入页面。当前会过滤危险 HTML、脚本协议、协议相对资源和不安全媒体地址。
