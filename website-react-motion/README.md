# 芯之力 · 九型人格动效官网（独立新版）

这是从 `../website-react/` 独立复制出来的新版官网。旧官网保持不动；本目录复用原有业务、路由、接口、课程、视频和报名流程，并增加编辑感视觉系统、九型动态轨道、滚动揭示和指针光域。

## 运行

```bash
npm install      # 安装依赖
npm run dev      # 本地开发（http://localhost:5174）
npm run build    # 生产构建，产物在 dist/
npm run preview  # 预览生产构建
```

## 动效与可访问性

- 参考 Aceternity、GSAP、MotionSites、Godly、Lenis 的组合方法，但使用项目内可控的 React/CSS 动效实现。
- 首页首屏突出九型人格轨道，九个节点可进入原有 `/type/:id` 详情页。
- 动效使用统一令牌，并尊重 `prefers-reduced-motion`，降低动态时关闭持续旋转、视差和滚动过渡。

## 首屏加速措施

- **路由懒加载**：首页直载，其余页面（老师/三阶段/各阶段/观看）按需加载，首页 JS gzip 仅 ~61KB。
- **系统字体栈**：使用本机中文字体回退，避免首屏依赖外部字体服务。
- **图片压缩**：海报 2.5MB→~280KB（JPEG），师承合影 4.3MB→~120KB；大图带 `loading="lazy"`。

## 结构

```
public/assets/   图片与 SVG（含师承合影 teacher-mentor.jpg）
src/
  index.css      = 原 style.css（去掉 @import，使用系统字体栈）
  App.jsx        路由表
  data/types.js  九型数据
  hooks/         滚动揭示 / 进度条+视差 / 卡片光斑 / 数字滚动 / 治愈系音乐
  components/    Layout · Nav · Drawer · Tabbar · FxBackground · ScrollProgress · Music · Footer · Reveal
  pages/         Home · Teacher · Stages · Stage1/2/3 · Watch
```

## 路由

`/` 首页 · `/teacher` 老师简介 · `/stages` 三阶段总览 · `/stage1` `/stage2` `/stage3` · `/watch` 观看入口。
首页内的区块锚点（课程/小游戏/企业/语录/九种芯片/报名）通过 `/#区块id` 跳转并平滑滚动。
