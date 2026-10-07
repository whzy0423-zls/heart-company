# 代理管理用户名称展示

## 验收与边界

后台管理在电脑和手机端统一调整经营排行、代理列表、下级代理列表：代理号后显示用户名称，优先昵称，缺失时依次回退账号、手机号、未设置名称。仅后台 Vue 展示修改；用户 App、H5、原生工程、后台服务和数据库无需修改。代理关联、代理号编辑、权限、订单及分成计算仍使用原 ID。

经营排行可能包含最新 200 条代理列表之外的早期代理。对于这些记录，复用已有且受 `Customer:App:List` 权限保护的用户详情接口补取名称。普通代理角色不触发详情查询；失败显示暂无名称，过期查询通过 watcher 清理标记隔离，不覆盖刷新后结果。

## 验证

- 现有代理管理和分析加载单测：2 文件、26 项通过。
- `vue-tsc --noEmit --skipLibCheck` 通过。
- Chrome 浏览器回归：9 项通过，覆盖管理员/二级/三级代理，390/768/1024/1440px，代理名称及列表外早期代理名称、新增弹窗取消、横向滚动；三级代理新增仍禁用。
- 后台生产构建通过；沿用既有无限画布分包体积和第三方 PURE 注释告警。现有编辑 Input 解析告警非本次新增。
- 浏览器业务接口全部使用拦截样本，无真实创建代理或充值。
- 独立审查识别并修复 200 条截断问题，再次审查通过。

## 发布

2026-10-07 已合并并推送主分支，实现提交 `bb96951`。生产后台更新为 `heart-company-admin:agent-names-20261007`。

- 完整产物冻结于 `/tmp/nx-admin-agent-names-20261007/admin/`，367 文件在容器中 SHA256 全部一致。
- 仅替换 admin 镜像；Nginx 配置哈希、有效 Compose 其他字段、所有其他服务容器 ID/镜像均不变，`nginx -t` 通过（沿用原有 MIME 重复定义告警）。
- 公网 `/admin/` index SHA256：`2f40f04a49248b38869741f262f3e66a47f174b9f864bbb7ea3d735b22fb23d9`；11 个入口资源、代理管理分包及相关 CSS 的公网哈希和冻结产物一致。
- 候选生产包真实登录表单/滑块 + 模拟业务接口：1440px、390px 两项通过。首次脚本使用中文按钮文本定位，但按钮 accessible name 为 login，改正定位后通过；不涉及业务代码修改。
- 公网实际静态资源 + 模拟业务接口同样 2/2 通过（9.4s），验证代理号后的名称、老代理补充查询、新增弹窗取消、横向滚动、手机无页面溢出；无 JS 异常、未知接口或真实业务写入。
- `/app`、`/h5/`、`/h5/version.json`、Android 最新发布接口内容哈希均与本次后台发布前一致。H5 保持 1.1.29+213，Android 保持 1.1.29+195。

回滚：恢复 `/opt/heart-company/.deploy/admin-agent-names-20261007/default.override.before.yml` 到 `/opt/heart-company/docker-compose.override.yml`，然后在该目录执行 `docker compose -f docker-compose.yml -f docker-compose.override.yml up -d --no-deps --no-build admin`，回到 `heart-company-admin:mobile-layout-20261007`。保留旧哈希静态文件，支持已经打开页面的后续懒加载。

证据目录 `/tmp/nx-admin-agent-names-20261007/`：`tests.log`、`typecheck.log`、`browser-chrome.log`、`build.log`、`production-browser-fixed.log`、`deployment.log`、`public-verified.json`、`public-browser.log` 和 `browser/admin-390.png` / `admin-1440.png`。
