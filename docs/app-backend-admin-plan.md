# 九型问答 App 后端与后台实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在当前 `nine-xing` 项目中为独立 Flutter App 提供后端 API、RAG 能力、计费权益和后台管理。

**Architecture:** 当前仓库继续承载 Go 后端和 Vben 后台。Flutter App 作为独立项目，通过 `/api/app/**` 调用 App API，通过 `/api/admin/app/**` 支撑后台运营；公共知识库复用现有 RAG，私库和主副卡按 App 用户隔离。

**Tech Stack:** Go HTTP server, PostgreSQL/pgvector, Vben Admin, Ant Design Vue, existing RAG/LLM/embedding modules.

---

## Current Status

当前阶段：Phase 1 - App API 基础与健康检查
当前任务：未开始；等待实现 `GET /api/app/health` 和 App API 路由分组。
验收状态：未验收。
阻塞问题：无。
最后更新时间：2026-06-25

## Phase Progress

- [ ] Phase 1: App API 基础与健康检查
- [ ] Phase 2: 手机号登录与 App 用户体系
- [ ] Phase 3: 主卡生成与九型画像
- [ ] Phase 4: 会话与消息落库
- [ ] Phase 5: 公共知识库与基础 RAG
- [ ] Phase 6: 私库记忆
- [ ] Phase 7: 追问建议、回答反馈、收藏与搜索
- [ ] Phase 8: 成长画像、每日练习、任务、周报与趋势
- [ ] Phase 9: 海报分享与分享归因
- [ ] Phase 10: 计费、权益、支付与模型配置
- [ ] Phase 11: 隐私与数据安全
- [ ] Phase 12: 推送通知与触达
- [ ] Phase 13: 可观测性与运营监控
- [ ] Phase 14: 关系合盘

## Change Log

- 2026-06-25: 拆分后端/后台与 Flutter App 计划，明确当前 `nine-xing` 项目只承载后端 API、RAG、计费和后台运营能力。
- 2026-06-25: 增加固定控制区，用于跨会话追踪当前阶段、验收状态、阻塞问题和阶段进度。
- 2026-06-25: 审查补全缺口——Phase 2 增加 token 刷新与验证码防刷；Phase 3 明确九型题库/算分由后端提供；Phase 4 增加内容安全审核；Phase 7 增加追问建议生成；Phase 8 补全周报与趋势的表和接口；Phase 9 补 App 安装延迟深链归因；Phase 10 补支付下单与回调链路；新增 Phase 12 推送通知、Phase 13 可观测性；Release Gate 增加内容安全与合规验证。
- 2026-06-25: 补回关系合盘（Phase 14）：两张本人卡片生成两型关系解读，按权益控制深度合盘，禁止跨用户合盘。

## Scope

这份计划只覆盖当前 `nine-xing` 项目中的后端与后台工作。

不在本计划内：
- Flutter App 页面与构建，见 `/Users/wohenzaiyi/Desktop/nine-xing-app/docs/flutter-app-plan.md`
- 官网与现有小程序改版
- iOS 发布

## Phase 1: App API 基础与健康检查

**目标:** 让独立 Flutter App 能识别 API 环境，并完成最小联调。

**后端工作:**
- 新增 `GET /api/app/health`，返回服务状态、版本、当前环境、服务器时间。
- 新增 App API 路由分组，区别现有小程序 `/api/miniapp/**`。
- 保留现有后台 JWT，不与 App JWT 混用。

**后台工作:**
- 暂不新增页面。

**验收:**
- Flutter App 可请求 `GET /api/app/health`。
- 未登录用户只能访问公开 App API。
- 后端测试覆盖健康检查和路由前缀。

## Phase 2: 手机号登录与 App 用户体系

**目标:** App 用户可通过手机号验证码登录。

**后端工作:**
- 新增 `app_users` 表：手机号、昵称、头像、状态、会员等级、注册来源、最后登录时间。
- 新增验证码表或缓存：手机号、验证码哈希、过期时间、使用状态、发送 IP。
- 验证码防刷：同手机号/同 IP 发送频率限制（如 60s 一次、单日上限）、连续错误锁定、必要时图形验证码或行为校验前置。
- 新增接口：
  - `POST /api/app/auth/sms/send`
  - `POST /api/app/auth/sms/login`
  - `POST /api/app/auth/token/refresh`
  - `POST /api/app/auth/logout`
  - `GET /api/app/me`
- App JWT payload 使用 App 用户 ID 和 `app` role；短期 access token + 长期 refresh token，refresh token 可吊销并按设备记录。
- 接入云短信；开发环境允许日志验证码。

**后台工作:**
- 新增“九型 App 管理 / App 用户”页面。
- 支持手机号搜索、状态筛选、注册时间筛选、用户详情抽屉。

**验收:**
- 正确验证码登录成功。
- 错误、过期、已使用验证码登录失败。
- App JWT 可访问 `/api/app/me`。
- 后台可看到新注册 App 用户。

## Phase 3: 主卡生成与九型画像

**目标:** 后端统一提供九型人物画像，主卡来源沿用现有九型数据。

**后端工作:**
- 新增 `app_user_cards` 表：用户 ID、卡片类型 `primary|secondary`、姓名、关系、九型主型、副型、三中心、状态。
- 九型测试题库与算分由后端统一提供，App 不重写规则：
  - `GET /api/app/quiz/questions` 返回题目、选项与维度权重。
  - `POST /api/app/quiz/submit` 接收作答，后端算分得出主型/副型/三中心并据此创建主卡。
  - 题库与算分逻辑沿用现有九型体系（`enneagramGame` / `TYPES_INFO`），版本化以便后续题目调整。
- 主卡唯一；副卡数量限制先按权益读取，默认免费 1 个、会员 5 个。
- 提供接口：
  - `GET /api/app/cards`
  - `POST /api/app/cards`
  - `PUT /api/app/cards/{id}`
  - `DELETE /api/app/cards/{id}`
  - `GET /api/app/cards/{id}/persona`
- 后端返回的人物画像字段来自当前九型体系：类型名、英文名、关键词、中心、恐惧、欲望、成长方向、压力方向、画像摘要。

**后台工作:**
- 新增“主副卡管理”页面。
- 支持按用户、主型、副型、卡片类型筛选。
- 用户详情里展示主卡和副卡。

**验收:**
- 新用户完成测试后可创建主卡。
- 主卡画像与当前 `TYPES_INFO`、`RESULTS`、`CENTERS` 一致。
- 一个用户只能有一张主卡。
- 免费用户不能创建超过 1 张副卡，会员不能超过 5 张副卡。

## Phase 4: 会话与消息落库

**目标:** 每张卡拥有独立问答窗口和历史。

**后端工作:**
- 新增 `app_chat_sessions`、`app_chat_messages`。
- 接口：
  - `POST /api/app/chat`
  - `GET /api/app/chat/sessions`
  - `GET /api/app/chat/sessions/{id}/messages`
- `POST /api/app/chat` 必须携带 `cardId`，消息按卡片隔离。
- 消息记录用户问题、回答、来源摘要、错误、耗时。
- 内容安全审核：用户输入（提问、卡片姓名）与 AI 输出在落库/返回前过内容安全检测；命中违规则拦截或脱敏，并记录审核结果与命中类型，供后台追溯。

**后台工作:**
- 新增“问答日志”页面。
- 支持按用户、卡片、时间、状态、关键词筛选。
- 详情抽屉展示问题、回答、来源、耗时和错误。

**验收:**
- 主卡和副卡的消息不混淆。
- 重新进入会话可恢复历史。
- 后台可查看问答日志。
- 未登录或访问他人会话返回 401/403。
- 违规输入/输出被拦截或脱敏，后台可查看审核命中记录。

## Phase 5: 公共知识库批量导入与基础 RAG

**目标:** App 问答可使用公共知识库。

**后端工作:**
- 复用 `rag_documents`，新增批量导入接口：
  - `POST /api/admin/rag/import`
  - `POST /api/admin/rag/reindex`
- 支持粘贴文本或文件内容入库、切分、标签、状态、来源。
- 完善 embedding 配置和 reindex 状态。

**后台工作:**
- 扩展现有“知识库管理”页面。
- 增加批量导入、重建索引、索引状态、导入结果提示。

**验收:**
- 后台可导入知识文档。
- 导入后 App 问答能命中对应内容。
- 重建索引后检索正常。
- 未命中时有温和兜底回答。

## Phase 6: 私库记忆

**目标:** 每张卡有独立私库，问答优先命中私库。

**后端工作:**
- 新增 `app_user_memories`、`app_memory_extraction_jobs`，必要时新增私库 chunk/embedding 表。
- 聊天完成后异步提炼记忆，写入当前 `user_id + card_id`。
- 检索顺序：当前卡片私库 -> 公共知识库。
- 接口：
  - `GET /api/app/cards/{id}/memories`
  - `DELETE /api/app/memories/{id}`
  - `PUT /api/admin/app/memories/{id}`

**后台工作:**
- 新增“私库记忆”页面。
- 支持按用户、卡片、状态、来源问答筛选。
- 支持停用、删除、查看来源消息。

**验收:**
- A 卡记忆不会被 B 卡命中。
- 私库命中优先于公共知识库。
- 停用/删除后不再参与检索。
- 后台可追溯记忆来源。

## Phase 7: 反馈、收藏与历史搜索

**目标:** 支撑回答质量分析和用户回看。

**后端工作:**
- 新增收藏表、反馈表。
- 追问建议：`POST /api/app/chat` 的回答里附带 3 条结构化追问建议（基于当前问题、卡片画像与命中内容生成），或提供 `GET /api/app/chat/messages/{id}/suggestions`；建议文案与回答同源，避免暴露技术词。
- 接口：
  - `POST /api/app/chat/messages/{id}/favorite`
  - `POST /api/app/chat/messages/{id}/feedback`
  - `GET /api/app/favorites`
  - `GET /api/app/chat/search`
- 搜索必须按当前用户和卡片隔离。

**后台工作:**
- 新增“反馈分析”和“收藏分析”。
- 支持查看高频收藏内容、低分反馈和原始回答。

**验收:**
- 用户可收藏/取消收藏。
- 用户可反馈“有帮助 / 不准确 / 想继续问”。
- 每次回答返回可用的追问建议。
- 后台可筛选反馈类型。
- 历史搜索不能跨用户或跨卡片泄露。

## Phase 8: 成长状态画像、每日练习、任务、周报与趋势

**目标:** 提供留存型成长体验，并沉淀周期性价值（周报、趋势）。

**后端工作:**
- 新增 `app_card_state_profiles`、`app_daily_practices`、`app_growth_tasks`。
- 新增 `app_growth_weekly_reports`（按卡片+周维度）：高频问题、压力点、关系关键词、新增记忆、下周建议、生成时间、权益可见范围。
- 新增 `app_state_trend_points`（按卡片+日期维度）：压力、能量、关系困扰、自我觉察等指标打点，用于趋势聚合。
- 生成状态画像：压力点、关系模式、成长建议、自我觉察提示。
- 每日练习按主型/副型生成。
- 7 天任务记录用户完成状态。
- 周报定时聚合（如每周一生成上周报告）；趋势点在状态画像更新或每日打卡时写入。
- 接口：
  - `GET /api/app/cards/{id}/state`
  - `GET /api/app/daily-practice`
  - `POST /api/app/daily-practice/checkin`
  - `GET /api/app/growth-tasks`
  - `POST /api/app/growth-tasks/{id}/complete`
  - `GET /api/app/cards/{id}/weekly-reports`
  - `GET /api/app/cards/{id}/weekly-reports/{reportId}`
  - `GET /api/app/cards/{id}/trends?range=7d|30d`

**后台工作:**
- 状态画像可查看。
- 每日练习和 7 天任务完成率可统计。
- 周报生成情况与趋势数据可查看。

**验收:**
- 每张卡有独立状态画像。
- 每日练习按主型变化。
- 7 天任务状态可保存。
- 周报可按卡片+周生成，免费用户只返回摘要、会员返回完整内容。
- 趋势接口按卡片隔离，支持 7 天/30 天聚合。
- 后台可查看打卡、任务完成、周报与趋势数据。

## Phase 9: 海报分享与归因

**目标:** 支撑带九型图的人物画像海报和转化统计。

**后端工作:**
- 新增 `app_share_records`。
- 接口：
  - `POST /api/app/share/posters`
  - `GET /api/app/share/{code}`
  - `POST /api/app/attribution/resolve`
- 记录分享人、卡片、海报类型、邀请码、打开、注册、付费转化。
- App 安装归因（区别于小程序扫码即用）：扫码/点链接先到 H5 落地页，记录邀请码与设备指纹（IP+UA+时间窗），用户安装并首次打开 App 后调用 `attribution/resolve` 做延迟深链（deferred deep link）匹配，把新用户回填到对应邀请码；匹配窗口与去重规则需明确，避免误归因。

**后台工作:**
- 新增“分享数据”页面。
- 看板展示海报生成、打开、安装、注册、付费转化漏斗。

**验收:**
- 分享邀请码可生成并解析。
- 扫码→落地页→安装→首开的延迟深链归因正确。
- 打开和注册归因正确，去重规则生效。
- 后台分享数据与数据库一致。

## Phase 10: 计费、权益、支付与模型配置

**目标:** 支持免费、会员、次数包、报告解锁、分享奖励、支付下单与模型配置。

**后端工作:**
- 新增权益、套餐、订单、额度流水、模型配置、模型调用日志表。
- 支持免费额度、月卡/季卡/年卡、深度分析次数包、深度报告、分享奖励。
- 支付链路（核心，不可省略）：
  - 接入支付渠道（微信支付 App 支付 / 支付宝 App 支付，按上架主体选择）。
  - `POST /api/app/orders` 创建订单并返回客户端拉起支付所需参数（预支付串/签名）。
  - `POST /api/app/pay/notify`（或各渠道独立回调）做回调验签，幂等更新订单状态。
  - `GET /api/app/orders` 我的订单；订单状态机 `pending/paid/closed/refunded`。
  - 支付成功后发放权益/扣减额度，保证回调与权益变更的事务一致性与重试幂等。
- 支持配置模型供应商、模型名、fallback、是否收费、每次消耗额度。
- 记录 token、耗时、成本估算。

**后台工作:**
- 新增“计费配置”“订单管理”“模型配置”“模型成本看板”。
- 订单管理支持查看支付状态、退款、对账。
- 看板展示收入、成本、毛利、深度分析使用量。

**验收:**
- 免费用户每日 5 次基础问答。
- 月卡和次数包权益生效。
- 下单可拉起支付，支付回调验签通过并幂等发放权益。
- 无权益不能调用付费模型能力。
- 后台切换模型后问答链路生效。
- App 端不暴露模型供应商，只返回“芯之力专属模型”。

## Phase 11: 隐私与数据安全

**目标:** 心理成长类数据可控、可删除、可审计。

**后端工作:**
- 接口：
  - `POST /api/app/privacy/export`
  - `POST /api/app/privacy/clear-memory`
  - `POST /api/app/account/delete`
- 新增敏感操作审计。
- 删除策略覆盖私库、会话、收藏、分享归因和订单保留规则。

**后台工作:**
- 用户详情展示隐私操作记录。
- 支持查看账号删除状态。

**验收:**
- 用户可导出数据。
- 用户可清空私库。
- 删除账号后无法登录。
- 后台有操作记录。

## Phase 12: 推送通知与触达

**目标:** 支撑每日练习、7 天任务、周报等留存型功能的主动触达。

**后端工作:**
- 接入推送服务（如 FCM 或国内厂商通道/聚合 SDK，按上架渠道选择），新增设备 token 表与推送记录表。
- 接口：
  - `POST /api/app/devices`（注册/更新推送 token 与设备信息）
  - `DELETE /api/app/devices/{token}`（登出或卸载时注销）
- 支持按场景触发：每日练习提醒、7 天任务进度、周报就绪、未读问答跟进。
- 支持用户推送偏好（开关、免打扰时段），不向关闭推送的用户发送。

**后台工作:**
- 新增“推送管理”：模板配置、定向人群、发送记录与到达统计。

**验收:**
- 设备 token 可注册与注销。
- 周报就绪、每日练习等场景能按规则推送。
- 关闭推送或免打扰时段不发送。
- 后台可查看发送与到达记录。

## Phase 13: 可观测性与运营监控

**目标:** 保证线上问题可发现、可定位，运营数据可追踪。

**后端工作:**
- 结构化日志与请求链路追踪，关键 App 接口埋点（登录、问答、支付、推送）。
- 错误率、延迟、模型调用成本等指标可采集，异常可告警。
- 行为埋点上报接口，沉淀关键漏斗（进入→测试→主卡→问答→付费→分享）。

**后台工作:**
- 新增“运营看板”：核心漏斗、留存、付费转化、接口健康度。

**验收:**
- 关键接口有日志与指标。
- 异常可告警。
- 后台可查看核心漏斗与健康度。

## Phase 14: 关系合盘

**目标:** 基于两张卡（本人主卡与某张副卡/另一份结果）生成两型关系解读，作为差异化卖点。

**后端工作:**
- 新增 `app_relation_matches` 表：发起用户、卡片 A、卡片 B、两型组合、解读结果、生成时间、权益可见范围。
- 合盘解读基于现有九型体系生成：两型关系动力、易冲突点、相处建议、互补优势；可结合双方私库画像增强（仅本人可见，遵守数据隔离）。
- 接口：
  - `POST /api/app/relation-match`（传入两张卡 ID，返回合盘结果）
  - `GET /api/app/relation-matches`（我的合盘历史）
  - `GET /api/app/relation-matches/{id}`
- 合盘消耗按权益控制：免费可试用基础合盘，深度合盘按会员/次数包扣减。
- 卡片必须归属当前用户，禁止跨用户合盘他人卡片。

**后台工作:**
- 新增“关系合盘”页面，可查看合盘量、热门型号组合、付费转化。

**验收:**
- 两张本人卡片可生成合盘解读。
- 解读内容与九型体系一致。
- 跨用户卡片合盘返回 403。
- 深度合盘按权益扣减，无权益有付费引导。
- 后台可查看合盘数据。

## Release Gate

每个 Phase 完成后必须通过：
- Go 后端测试
- Vben 后台页面验证
- API 权限验证
- 用户数据隔离验证
- 内容安全与合规验证（输入/输出审核、隐私操作可追溯）
- 与 Flutter App 对接验证

当前 Phase 未通过，不进入下一个 Phase。
