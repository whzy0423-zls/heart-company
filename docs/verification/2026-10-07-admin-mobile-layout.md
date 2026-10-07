# 后台手机布局适配验证

## 根因与修复范围

仅后台 `nx-backend/apps/web-antd` 展示层。本次无后端接口、权限码、数据库、Flutter App、用户 H5 或原生工程修改。

- 订单详情固定宽 620px，在 360px 视口超宽；后台手机样式统一限制侧边抽屉不超过屏幕，调整内边距及安全区。
- 手机表格左右固定列遮挡中间字段；手机取消 sticky，保留所有列在表格内部横向滚动。14 处原缺滚动设置的宽表补 `scroll.x`。
- 卡片标题/新增按钮、筛选栏和分页支持换行；模型语音三项参数在手机单列。客户、画像、小程序客户按实际内容宽度换行：修复前客户查询按钮右边界 1063.8px，844px 屏幕甚至 1024px 窗口均越界；修复后均在卡片内。
- 日期范围双月弹层实测 560px，改为手机双月纵排、限高滚动，验证可选择第二个月日期。
- 原侧栏仅通过 mouseleave 收起，手机触摸切页后仍遮挡页面。后台布局增加手机成功导航/重复点击当前菜单时收起，并在卸载时移除钩子。桌面导航逻辑保持；框架原有跨断点持久化折叠偏好不作改变。

## 验证方式与结果

真实 Vue 应用、认证、路由、Ant Design 组件；所有 API 使用拦截样本，不登录真实管理账号，不创建代理、订单、老师或修改会员。

- 后台完整单测 104 文件、525 项通过。最后导航和客户样式调整后分别追加布局 13 项、客户 47 项通过。
- Chrome 源码测试 19 项通过，追加客户筛选 8 项通过：320/360/390/430px、844 横屏、768/1024/1440px；真实查询、表格滚轮、详情开关、弹窗取消、导航、日期选择、大字号、Pixel 7 触摸及重复菜单点击。
- WebKit iPhone 13 环境 3 项通过：订单、审计、老师；WebKit 使用 `scrollBy` 验证滚动容器，Chrome 使用实际鼠标滚轮。模拟环境不等同于各品牌实机全覆盖。
- `vue-tsc`、后台生产构建、新增 CSS Stylelint、`git diff --check` 通过。
- 原代理管理权限/布局回归 9 项通过，覆盖管理员、二级/三级代理及窄屏/桌面。三级代理新增权限仍禁用。
- 正式构建候选使用真实登录表单和滑块测试；第一轮准确检出手机侧栏残留，修复后最终 7 项通过（46.6s，退出码 0），包含横屏客户筛选。

源码可重复测试：`playwright.mobile.config.ts` 和 `e2e/admin-mobile-layout.spec.ts`。设置 `ADMIN_LAYOUT_BROWSER=webkit` 可使用 WebKit 项目。需安装对应 Playwright 浏览器；Chrome 使用 `PLAYWRIGHT_CHANNEL=chrome`。测试 API 代理指向本机未监听端口，遗漏 mock 即失败，不回源真实后台。

证据目录 `/tmp/nx-admin-mobile-20261007/`；源码测试日志 `/tmp/admin-mobile-final-pass.log`、`/tmp/admin-mobile-webkit-pass.log`、`/tmp/admin-mobile-customer-after.log`。失败基线日志分别保留订单抽屉、日期弹层、菜单和客户过滤栏。第三方既有构建告警不影响退出码；已知旧代理编辑 Input 组件告警未在本次修正。

## 发布边界

冻结后台完整构建，以当前后台镜像为基础 COPY 静态资源，保留 Nginx 和旧哈希文件，兼容已打开页面的懒加载。仅替换 Compose 的 admin image；比较其余有效配置、运行容器 ID 和公网 App/H5 发布入口哈希；保留旧镜像和覆盖配置用于回滚。

## 线上发布结果

2026-10-07 已合并推送 main（实现提交 `164f145`），并更新后台镜像为 `heart-company-admin:mobile-layout-20261007`。

- 367 个新产物文件在运行容器中 SHA256 全部一致；Nginx 配置哈希保持原样，`nginx -t` 通过；有效 Compose 除 admin image 外相同，其他运行服务容器 ID/镜像不变。
- 公网 `/admin/` index SHA256：`0bdd95c89b8a5eb8233f7595661a508b6fbcb7aeb4948eb7a970ac69dfd75500`；响应 no-cache/no-store。10 个入口及新增样式资源的公网哈希与冻结产物一致，MIME 正确。
- 公网真实页面 + 模拟 API 的最终 7 项操作测试全部通过，55.1s，退出码 0；覆盖 360/390 手机、1440 桌面及 844×390 横屏。确认登录及切页始终留在 `/admin/`。未知接口、业务写请求、页面异常均为零；未登录真实后台账号。
- `/app`、`/h5/`、`/h5/version.json`、Android 最新发布接口均与更新前内容哈希一致。
- 初次部署预检使用了错误 Nginx 路径，未切换服务；下一次校验因 Alpine BusyBox 不支持 `sha256sum --quiet` 自动回滚。修正为实际 `/etc/nginx/nginx.conf` 和 `sha256sum -cs` 后完整发布与公网复测通过；旧版本镜像和各轮日志保留。

回滚目录：`/opt/heart-company/.deploy/admin-mobile-20261007-verified/`。恢复 `default.override.before.yml` 为 `/opt/heart-company/docker-compose.override.yml` 后运行 `docker compose -f docker-compose.yml -f docker-compose.override.yml up -d --no-deps --no-build admin`，回到 `heart-company-admin:agent-scroll-20261007`。备份有效配置仅保留服务器，不进入仓库。

最终发布证据：`deployment-verified.log`、`public-verified.json`、`public-browser.log`，均位于 `/tmp/nx-admin-mobile-20261007/`；本轮候选 HTTP 服务已停止。手机浏览器刷新原后台地址即可获取新页面，不需要更新 App。
