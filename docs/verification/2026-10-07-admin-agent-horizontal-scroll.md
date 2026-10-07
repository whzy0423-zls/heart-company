# 代理管理横向滚动修复

问题：后台代理管理中“新增代理”被挤到屏幕外，页面横向滚动也无法到达。生产 `.distribution-page` 使用隐式 Grid 单列，宽表格的 min-content 撑大了卡片；外层布局正常的 overflow-hidden 又裁掉超宽部分。

在真实 Vite 后台、路由、权限和 Ant Design 页面中复现：1024px 视口，Grid 宽 768px，但新增按钮右边界达 1555px。修复页面自身为 `grid-template-columns: minmax(0, 1fr)`，页面与直接子元素 `min-width: 0`。修复后按钮右边界 983px，表格容器 718px、内容 1362px，横向鼠标滚动有效。

范围仅代理管理页面 CSS，无共享布局、业务接口、代理权限、数据库、App/H5 或小程序功能修改。同页面管理员和二级代理的新增表单可达，三级代理的原权限禁用保持。经营日期控件在窄屏自动换行。

验证：

- Playwright 使用真实页面和模拟 API 数据，9 个角色/尺寸场景通过：管理员与二级代理各 390/768/1024/1440px，三级代理 390px。验证新增按钮在视口内、点击打开并关闭表单、横向滚轮改变 scrollLeft、日期控件未超出视口、无真实业务写请求。
- 类型检查通过，60 项代理/分成与既有课程配置回归通过，Node 22.23.1 下完整后台生产构建通过。
- 原页面存在未导入 Input 的编辑代理弹窗告警，本次未修改该无关功能。浏览器布局测试仅抑制图表 ResizeObserver 的无损交付通知，其他页面错误不做屏蔽。
- 复现截图、尺寸和测试日志：`/tmp/nx-admin-scroll-20261007/`。API 全部使用本地样本，不创建真实代理或更改真实分成。

生产发布采用保留现有静态资源的最小增量：仅添加带内容哈希的代理页面 scoped CSS，以及 index 的 stylesheet 引用。全新构建的 page scope 与生产 scope 不同，新增三条宽度约束按生产已核验的 scope 编译映射，其他所有现有 JS/CSS/字体与配置文件逐字节保持。正常源码构建已包含同一修复，后续完整部署无需依赖此次增量样式。

## 线上发布结果

2026-10-07 已部署并复测。代码提交 `8c918e3` 已合并推送 main。

- 镜像：`heart-company-admin:agent-scroll-20261007`，仅后台容器更新；有效 Compose 配置逐项比较确认只有 admin image 改变，其他运行容器 ID 与镜像保持原样。
- 样式：`/css/distribution-viewport-d9bb4726cf7183c2.css`，135 bytes，正确返回 `text/css`；内容哈希文件名可缓存，`/admin/` HTML 返回 no-cache/no-store，刷新即可取得新引用。
- 线上最终 576 个静态文件与冻结候选逐字节一致：575 个既有文件只修改 index，新加一个 CSS。运行中的旧容器无静态目录挂载或镜像外改动，新增镜像完整保留原有静态资源。
- 公网后台实际资源在 1024px 下完成模拟账号页面操作验证，新增弹窗打开/关闭和横向滚轮均通过，Playwright 1 项退出码 0。所有 API 使用拦截样本，未登录真实管理账号或创建真实代理。
- 生产静态快照的 9 组操作断言也均通过；批量运行的浏览器清理曾卡住，断言完成后终止，因此只将其记为操作证据，不记作成功退出的完整测试。源码浏览器全套 9 项与公网单项均成功退出。
- 公网 `/app`、`/h5/`、H5 版本和 Android 发布接口与部署前哈希一致。

备份与回滚：`/opt/heart-company/.deploy/admin-agent-scroll-20261007/` 保留旧覆盖配置、部署前后有效配置及容器记录。恢复 `default.override.before.yml` 为 `/opt/heart-company/docker-compose.override.yml`，再执行 `docker compose -f docker-compose.yml -f docker-compose.override.yml up -d --no-deps --no-build admin`，即可回到 `heart-company-admin:course-registration-20261002`。备份含私有运行配置，仅保留在服务器。
