# 芯之力 · 九型人格官网（React 版）

由原静态站点（`../website/`）迁移而来的 **Vite + React 18 + React Router** 项目，样式与内容与原站一致，并针对首屏加载做了优化。

## 运行

```bash
npm install      # 安装依赖
npm run dev      # 本地开发（默认 http://localhost:5173）
npm run build    # 生产构建，产物在 dist/
npm run preview  # 预览生产构建
```

## 首屏加速措施

- **路由懒加载**：首页直载，其余页面（老师/三阶段/各阶段/观看）按需加载，首页 JS gzip 仅 ~61KB。
- **字体异步加载**：`index.html` 用 `preconnect` + `preload onload` 加载 Google 字体，不阻塞首屏渲染。
- **图片压缩**：海报 2.5MB→~280KB（JPEG），师承合影 4.3MB→~120KB；大图带 `loading="lazy"`。

## 结构

```
public/assets/   图片与 SVG（含师承合影 teacher-mentor.jpg）
src/
  index.css      = 原 style.css（去掉 @import，字体改 index.html 加载）
  App.jsx        路由表
  data/types.js  九型数据
  hooks/         滚动揭示 / 进度条+视差 / 卡片光斑 / 数字滚动 / 治愈系音乐
  components/    Layout · Nav · Drawer · Tabbar · FxBackground · ScrollProgress · Music · Footer · Reveal
  pages/         Home · Teacher · Stages · Stage1/2/3 · Watch
```

## 路由

`/` 首页 · `/teacher` 老师简介 · `/stages` 三阶段总览 · `/stage1` `/stage2` `/stage3` · `/watch` 观看入口。
首页内的区块锚点（课程/小游戏/企业/语录/九种芯片/报名）通过 `/#区块id` 跳转并平滑滚动。
