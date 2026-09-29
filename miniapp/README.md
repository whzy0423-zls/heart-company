# 九型芯之力 · 微信小程序（uni-app + Vue3）

基于现有官网与管理后台延伸的小程序前端。**复用同一个 Go 后端**（nx-backend），只新增了微信登录与小程序业务接口。

## 老师个人品牌 UI 预览

```bash
cd miniapp
npm run preview:ui
```

打开终端显示的本地地址，再访问 `/studio-preview.html`，可以并排操作「首页、日常、报名、我的」四个真实页面。端口占用时 Vite 会选择下一个可用端口。单页入口为 `/#/pages/index/index`。

老师肖像、简介和 19 段视频取自项目已有官网与公开课堂。课程排期、价格和学员记录仅用于演示；只有开发环境显式设置 `VITE_UI_PREVIEW=true` 才会启用。演示报名写入独立本地存储，未知请求也会在本地结束，不会提交到正式后端。视频与封面从现有 `website-react/public/assets/videos` 按需读取，不进入小程序发布包。

正式构建仍使用真实接口、微信登录与原有播放权限。设计说明和验证记录见 [UI 交付记录](docs/ui-preview/README.md)。

## 目录结构
```
miniapp/
├── package.json          # uni-app + vite 依赖
├── vite.config.js
├── index.html
└── src/
    ├── main.js / App.vue
    ├── manifest.json     # 小程序 appid 在这里填
    ├── pages.json        # 路由 + 4 个 tabBar
    ├── config.js         # 后端 API 基址（联调/生产切换）
    ├── data/enneagramGame.js   # 九型题目与解析（与官网同源）
    ├── utils/            # auth(微信登录) / enneagram(算分) / session
    ├── api/              # request 封装 + 接口
    └── pages/
        ├── index/        测一测（首页，含测试/学习/合盘入口）
        ├── test/         答题
        ├── result/       结果（解析 + 存档 + 分享好友 + 海报 + 合盘 + 预约）
        ├── relation/     关系合盘（两型相处底色/摩擦/建议）
        ├── learn/        学一学（课程/图鉴）
        ├── booking/      约课程（预约表单）
        └── profile/      我的（登录/档案/历史）
```

## 分享与海报
- **转发好友 / 朋友圈**：结果页用 `onShareAppMessage` / `onShareTimeline` 实现，「分享好友」按钮为 `open-type="share"`。
- **生成海报**：结果页「生成海报」用 canvas 2d 绘制（头像 + 型号 + summary + 引导语），可保存到相册或长按转发。
  - 当前海报底部为「微信搜索小程序」引导文案。上线拿到 AppID 后，可改为后端生成小程序码（`wxacode.getUnlimited`）贴到海报，实现扫码直达。

## 本地运行（微信开发者工具）
1. 安装依赖：
   ```bash
   cd miniapp
   npm install
   ```
   > 若 uni-app 版本号安装报错，可用官方预设重建壳工程再覆盖本 `src/`：
   > `npx degit dcloudio/uni-preset-vue#vite shell && cp -r src shell/ && cp package.json shell/`
2. 编译到微信小程序：
   ```bash
   npm run dev:mp-weixin
   ```
   产物在 `dist/dev/mp-weixin`。
3. 打开「微信开发者工具」→ 导入 `dist/dev/mp-weixin` 目录。
4. 在工具「详情 → 本地设置」勾选 **不校验合法域名**（联调用）。

## 视觉素材验证

首次验证前安装固定版本的 Python 开发依赖：

```bash
python3 -m pip install -r requirements-dev.txt
```

验证当前首批 7 张编辑风格素材：

```bash
python3 scripts/verify-editorial-assets.py --group initial
```

待 `result-1.webp` 至 `result-9.webp` 全部加入后，验证完整 16 张素材：

```bash
python3 scripts/verify-editorial-assets.py --group all
```

## 后端联调
- API 地址不再手改 `src/config.js`：
  - 开发默认读取 `.env.development`：`VITE_API_BASE=https://xn--9iq9az5uo8fz16d.com/api`
  - 生产必须读取 `.env.production` 或 CI 注入的真实 HTTPS：`VITE_API_BASE=https://xn--9iq9az5uo8fz16d.com/api`
  - 临时覆盖可用：`VITE_API_BASE=https://xn--9iq9az5uo8fz16d.com/api npm run build:mp-weixin`
- 后端**未配置微信 AppID/Secret 时自动启用 dev 登录回退**：`wx.login` 的 code 会被后端换成稳定的 `dev_xxx` openid，无需真实微信凭证即可跑通登录、存档、预约全流程。

## 微信支付接入

报告、课堂及后续付费功能的公共前端方法、后端接口契约和接入示例见 [小程序公共微信支付调用说明](docs/miniapp-payment-api.md)。金额由服务端配置并生成订单，支付完成后以服务端订单/权益状态为准。

## 上线前
1. `src/manifest.json` 与 `mp-weixin.appid` 填入你的小程序 AppID。
2. `.env.production` 的 `VITE_API_BASE` 改为你的 **HTTPS** 域名，并在小程序后台「开发管理 → 服务器域名」配置 request 合法域名。
3. 后端环境变量配置真实微信凭证（docker-compose / .env）：
   ```
   WECHAT_APPID=wx你的appid
   WECHAT_SECRET=你的secret
   ```
   配置后自动切换为真实 `code2session` 登录。
4. 重新执行 `npm run build:mp-weixin`，用微信开发者工具导入 `dist/build/mp-weixin`。

## 已对接的后端接口
| 接口 | 用途 |
|------|------|
| `POST /api/wx/login` | 微信登录换 token |
| `GET/PUT /api/wx/userinfo` | 用户资料 |
| `POST/GET /api/miniapp/test-records` | 测试存档 / 历史 |
| `POST/GET /api/miniapp/bookings` | 预约（同步落后台客户线索） |
| `GET /api/public/site-config` | 内容（课程等） |
| `POST /api/public/game-results` | 测试匿名统计上报 |
