# 人工会员后台设计

本设计与 Flutter 仓库 `2026-07-20-manual-membership-and-member-posters-design.md` 配套。

后台将 App 会员从永久的通用 `vip` 改为带套餐和有效期的 `vip_month`、`vip_quarter`、`vip_year`。App 创建的订单进入 `pending_confirmation`，管理员选择生效时间确认收款后，服务端以事务和行锁计算会员到期时间：无有效会员从生效时间起算，有有效会员从原到期时间顺延。确认结果写入订单和审计日志，重复确认不重复加时。

App 订单页负责处理待确认订单；App 客户页负责展示当前套餐、开始时间、到期时间和剩余天数。商品接口只返回月、季、年三种套餐，客服二维码继续复用 `site.customerServiceQr` 和 `/api/public/customer-service-qr`。

接口、数据兼容、错误处理与测试规则以配套设计文档为准。
