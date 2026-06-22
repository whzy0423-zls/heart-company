# 芯之力 · 读书 H5

九型芯之力的读书 H5 页面。后台「阅读管理 → 文章管理」配置文章（Markdown 正文），这里以列表 + 阅读页的形式展示给用户。

## 技术栈

- React 18 + React Router 6
- Vite 5
- `marked` 渲染 Markdown 正文

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

可通过 `VITE_API_BASE_URL` 注入线上 API 地址，默认走同源 `/api`。

## 安全说明

阅读页正文经 `marked` 渲染后用 `dangerouslySetInnerHTML` 注入。文章内容由**后台登录用户**撰写，属可信来源。若未来开放给不可信用户投稿，请引入 `DOMPurify` 等对渲染结果做净化，防止 XSS。
