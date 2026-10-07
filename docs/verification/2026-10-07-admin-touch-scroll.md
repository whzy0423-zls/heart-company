# 代理管理手机触摸横向滚动修复

## 用户反馈与复现

线上 `agent-names-20261007` 手机横向滑动看不到后续列。既有测试主要检查窄屏和鼠标滚轮，未覆盖从表头发起的手指滑动，不足以断言整体手机兼容完成。

对公网静态资源拦截业务 API，使用 Chrome Pixel 7 设备环境及 CDP `Input.dispatchTouchEvent` 发出真实浏览器触摸输入：数据区左移得到 scrollLeft=209，表头发起同一手势始终为0。源代码回归也先失败，明确复现根因。

Ant Design 固定表头 FixedHolder 设置 overflow:hidden，只转发 wheel.deltaX，不处理触摸拖动；代理管理全部5表配置scroll.y，因此表头与表体被分开。全局CSS只允许表体滚动，鼠标滚轮通过不代表表头触摸可用。

## 修改与边界

仅 `nx-backend/apps/web-antd`，不改用户H5、Flutter App、后端、数据库、代理权限或分成。

- 代理排行、代理列表、下级客户消费、下级订单和下级代理弹窗在 `(max-width: 767px), (pointer: coarse)` 环境保留横向scroll.x，取消固定高度scroll.y，让表头和数据行使用同一原生滚动容器。
- 粗指针设备包括手机横屏和触摸平板；同一媒体条件取消遮挡内容的固定左右列。普通电脑保留原固定表头和左右固定列。
- 手机列表随页面自然展开，原有数据、排序和操作全部保留。行数和请求数不增加。

## 范围审计

静态检查41个页面共55张Ant表格，本次固定表头触摸问题集中于代理管理5张表。没有发现主内容全局禁用touchmove。其他页面不应由这些自动化结果推断为全部机型实测通过。`dashboard/game-results.vue`另有未显式设置scroll.x的6列表格，作为单独兼容性审计项记录，本次不扩大代码范围。

## 验证

- 代理相关单测26项、vue-tsc、mobile.css Stylelint、生产构建通过。
- 9项桌面/窄屏及权限回归通过。
- 首轮新增触摸回归5项通过（管理员、二级/三级代理，390竖屏、844横屏），从表头和数据行触摸拖动并到达最右侧操作列。
- 正式构建包追加每张可见表、下级弹窗、右侧操作和纵向触摸验证：5/5通过（1.2分钟）。测试会等待弹窗稳定后定位手势，并在所有列已可见时直接验证末列可达，避免把不需要滚动的宽屏弹窗判为失败。线上复测结果见下方。

证据：`/tmp/nx-admin-touch-20261007/`。所有浏览器业务接口均mock，不创建真实用户/订单/代理，不修改生产业务数据。

## 生产发布

2026-10-07 北京时间22:28已更新生产后台，代码合并主分支 `56a671c`，镜像 `heart-company-admin:touch-scroll-20261007`。

- 容器中367个产物文件哈希一致，Nginx配置哈希保持、检查通过；Compose除admin镜像外相同，其他运行服务容器ID和镜像不变。
- 公网后台入口SHA256：`109f49c54f8c2122f7ee64ff62fbe3bffe5521e98de241d9843edd1b993d85d8`；11个入口/样式/代理管理分包与冻结产物一致。
- 官网、H5入口、H5版本、Android发布接口4项内容哈希与此次后台更新前一致。
- 公网真实静态资源 + 模拟业务API的完整触摸复测5/5通过（1.2分钟、退出码0）：管理员/二级/三级代理，390竖屏、844横屏，所有可见表头、数据行、最右侧操作、纵向触摸和下级弹窗；无真实业务写请求、未知接口。

回滚：恢复 `/opt/heart-company/.deploy/admin-touch-scroll-20261007/default.override.before.yml` 到 `/opt/heart-company/docker-compose.override.yml`，在此目录执行 `docker compose -f docker-compose.yml -f docker-compose.override.yml up -d --no-deps --no-build admin`，回到 `heart-company-admin:agent-names-20261007`。

部署与复测证据：`deployment.log`、`public-verified.json`、`public-touch.log`、`candidate-touch-final.log`、`desktop-regression.log`，均位于上述证据目录。Chrome设备触摸模拟不等同于用户手机实机全浏览器验证。
