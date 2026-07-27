# 小程序企业培训推广与案例展示设计

**日期：** 2026-07-27  
**状态：** 已确认，待实施  
**功能分支：** `feature/enterprise-training-promotion`

## 1. 背景与素材结论

现有小程序以个人九型测试、学习、预约和成长档案为主。用户现阶段希望把产品整体调整为韩老师企业培训业务的对外推广入口，核心目标不是提供付费在线学习，而是用真实企业培训案例建立信任，并促成企业培训咨询。

本次抽样检查了以下素材：

- `/Volumes/KODAK/26-3-30 韩老师视频`：约 61GB，包含大量 1080P MOV 原始素材与现场照片；
- `/Volumes/KODAK/26-3-31`：约 33GB，包含第二天原始素材；
- `/Volumes/KODAK/遵义3月九型视频`：3 个合成长视频，约 14.8 小时。

代表画面显示培训包含：

- 韩老师现场讲授与白板梳理；
- 企业员工围圈交流；
- 小组讨论与任务练习；
- 视频案例播放与集体复盘；
- 情景演练、身体体验和学员分享；
- 关系、沟通、控制模式、团队协作与生命成长等主题。

素材体现的是韩老师进入企业开展的真实培训服务。产品定位应为“企业培训案例与解决方案展示”，而不是普通录播课堂、课表或个人付费课程。

## 2. 产品目标

### 2.1 核心目标

建立一条清晰的企业客户转化路径：

```text
认识韩老师
→ 查看真实培训案例
→ 理解可定制的企业培训方案
→ 提交企业合作需求
```

### 2.2 成功标准

- 企业客户在进入首页后能在 10 秒内理解韩老师提供企业培训服务；
- 用户可以从案例快速看到真实现场、培训形式和适用问题；
- 每个案例和课程方案都有明确的“咨询同类培训”入口；
- 咨询表单采集企业培训所需的关键信息；
- 后台可以独立维护案例、视频、课程方案和咨询线索；
- 可统计案例浏览、视频播放、咨询点击和表单提交转化。

### 2.3 主要受众

- 企业负责人、创始人和高层管理者；
- 人力资源负责人、培训负责人；
- 团队管理者；
- 企业服务渠道伙伴和转介绍人。

## 3. 一级菜单

底部菜单调整为：

```text
首页｜培训案例｜培训方案｜咨询合作
```

### 3.1 首页

负责建立品牌认知并引导下一步操作。

### 3.2 培训案例

集中展示韩老师真实企业培训现场和成果，是核心信任页面。

### 3.3 培训方案

展示企业可以定制的培训解决方案、适用问题和交付形式，不使用在线课节、购买或学习进度语义。

### 3.4 咨询合作

提供企业需求表单、联系方式和合作流程说明。

原“测一测、学一学、约课程、我的”不再作为一级菜单。九型测试、关系合盘等个人工具可在首页次级入口中保留，但不能抢占企业推广主路径。

## 4. 首页设计

首页内容顺序：

1. 品牌 Hero；
2. 真实培训宣传短片；
3. 核心能力与服务数据；
4. 精选企业案例；
5. 企业培训方案；
6. 韩老师介绍；
7. 经授权的基础文本客户反馈或现场反馈（无内容时隐藏）；
8. 咨询合作 CTA。

### 4.1 Hero

推荐主标题：

> 用九型读懂团队，让沟通与协作真正发生

推荐副标题：

> 韩老师企业培训真实案例、团队培训方案与定制化解决方案。

主要按钮：

- 查看培训案例；
- 咨询企业培训。

### 4.2 真实现场宣传片

首页只使用 30～90 秒横版或竖版宣传片，不加载长时间完整录像。封面需要体现韩老师、企业员工和现场互动，避免只使用 PPT 截图。

宣传片内容建议覆盖：

- 企业员工入场或现场全景；
- 韩老师讲授；
- 围圈交流；
- 小组练习；
- 学员分享；
- 品牌和咨询结尾页。

### 4.3 信任信息

可配置展示：

- 从业或授课年限；
- 企业培训场次；
- 覆盖城市；
- 累计参训人数；
- 服务行业；
- 代表客户 Logo（取得公开授权时使用）。

没有可靠数据时不展示虚构数字。

## 5. 培训案例设计

### 5.1 案例列表

列表卡片字段：

- 封面图；
- 案例标题；
- 城市；
- 企业行业；
- 培训主题；
- 培训日期；
- 参与规模；
- 视频时长；
- 内容标签；
- 是否精选。

企业名称没有公开许可时，使用“贵州某企业”“某制造企业”等脱敏名称。

首期不提供动态筛选器，只提供以下固定主题快捷入口：

- 团队沟通；
- 领导力；
- 团队凝聚力；
- 组织文化；
- 员工成长。

### 5.2 案例详情

页面结构：

```text
案例封面与标题
→ 案例摘要
→ 培训背景/企业问题
→ 培训目标
→ 培训模块
→ 现场视频精华
→ 培训形式
→ 现场照片
→ 经授权的客户/学员基础文本反馈
→ 相关培训方案
→ 咨询同类培训
```

建议字段：

- `title`
- `slug`
- `summary`
- `company_display_name`
- `industry`
- `city`
- `participant_range`
- `training_date`
- `duration_label`
- `training_topics`
- `business_challenges`
- `training_goals`
- `training_modules`
- `training_methods`
- `outcomes`
- `teacher_name`
- `cover_asset_id`
- `promo_video_asset_id`
- `highlight_video_asset_ids`
- `gallery_asset_ids`
- `testimonial_items`
- `related_solution_ids`
- `status`
- `featured`
- `sort_order`
- timestamps and audit fields

### 5.3 视频分层

原始素材不直接公开。每个企业案例分为：

1. **宣传短片：** 30～90 秒，用于首页、朋友圈和转发；
2. **案例精华：** 3～8 分钟，解释培训内容与现场效果；
3. **主题片段：** 30 秒～3 分钟，例如讲师观点、互动练习、学员分享；
4. **原始录像：** 仅作内部素材归档，不在首期小程序公开。

遵义两天培训建议整理为一个案例，按主题拆分片段，而不是按摄像机文件或超长时间轴展示。

### 5.4 素材质量与异常

抽样发现其中一条合成 MP4 存在 H.264 数据损坏，部分片段读取时出现 NAL unit 错误；其他合成文件也出现局部 AAC 解码错误。正式发布前必须完成：

- 可读性检查；
- H.264 + AAC 标准化转码；
- 音画同步检查；
- 音量归一化和降噪；
- 封面抽取；
- 视频时长与分辨率校验；
- 完整播放抽检；
- 失败素材回退到对应 MOV 原始文件重新剪辑。

## 6. 企业培训方案设计

首期培训方向：

### 6.1 团队沟通与关系

- 认识不同性格动机；
- 跨性格沟通；
- 关系冲突识别与处理。

### 6.2 领导力与团队管理

- 不同性格员工的管理方式；
- 控制、授权与责任边界；
- 团队角色与内在动力识别。

### 6.3 团队凝聚力工作坊

- 围圈互动；
- 小组任务；
- 情景体验；
- 团队关系建设。

### 6.4 组织文化与员工成长

- 情绪与压力觉察；
- 组织关系与文化共识；
- 员工内在动力和成长。

培训方案详情字段：

- 标题、摘要、封面；
- 适用企业和适用对象；
- 解决的问题；
- 培训目标；
- 模块安排；
- 培训形式；
- 建议人数；
- 建议时长；
- 可定制项；
- 相关案例；
- 咨询 CTA。

首期不公开固定价格，使用“提交需求后定制方案”。

## 7. 咨询合作设计

### 7.1 表单字段

- 企业名称；
- 所属行业；
- 所在城市；
- 预计参与人数；
- 希望解决的问题；
- 感兴趣的培训方案；
- 预计培训时间；
- 联系人；
- 手机号；
- 微信号（选填）；
- 备注。

### 7.2 来源归因

提交时记录：

- 来源页面；
- 来源案例；
- 来源培训方案；
- 分享人或渠道参数；
- 首次访问来源；
- 提交时间和客户端基本信息。

后台线索需显示“用户看过哪个案例后提交咨询”，便于销售跟进。

### 7.3 状态

表单覆盖：

- 默认；
- 字段校验；
- 提交中；
- 提交成功；
- 重复提交；
- 网络失败与重试。

成功文案：

> 已收到您的企业培训需求，培训顾问将尽快联系您。

## 8. 韩老师品牌信息

首页展示精简版老师介绍，案例和培训方案页统一引用老师资料。内容包括：

- 姓名和真实头像；
- 专业身份；
- 企业培训经验；
- 擅长方向；
- 培训方法；
- 代表案例或服务行业；
- 完整介绍入口。

避免把老师介绍写成过长履历，重点说明“能帮助企业解决什么问题”。

## 9. 现有代码复用与调整

现有 `feature/miniapp-teacher-classroom` 已实现大量课堂基础设施，但定位偏付费学习。企业推广版本只复用适合的能力。

### 9.1 可复用

- 后台视频/音频分片上传；
- 媒体资产和上传任务；
- 封面、时长和媒体状态；
- 草稿、发布、下线和排序；
- 短时媒体播放地址；
- 后台权限和操作审计；
- 系列/内容管理 UI 的通用编辑组件。

### 9.2 首期不进入推广主流程

- 会员观看；
- 单课购买和系列购买；
- 学习进度；
- 继续学习；
- 付费音频课程；
- 课堂权益和订单转化。

这些能力不要求删除，但必须与企业案例领域解耦，不能让案例播放依赖会员或购买状态。

### 9.3 领域边界

建议新增独立的 `training_cases` 和 `enterprise_solutions` 领域；媒体资产可以引用课堂媒体基础设施，但案例元数据、企业咨询和营销统计不塞入课堂系列模型。

## 10. 后台管理

新增或调整为：

```text
企业培训
├── 推广首页
├── 培训案例
├── 培训方案
├── 韩老师资料
├── 视频素材
├── 发布授权
└── 咨询线索
```

### 培训案例

支持列表、筛选、编辑、预览、发布、下线、精选、排序和脱敏名称。

### 培训方案

支持方案方向、模块、适用对象、相关案例、排序和上下线。

### 视频素材

复用大文件分片上传，增加转码/校验状态和案例引用情况。

### 咨询线索

支持来源归因、跟进状态、负责人、备注和导出。

## 11. 接口建议

公开接口：

```text
GET  /api/public/training-cases
GET  /api/public/training-cases/:slug
GET  /api/public/enterprise-solutions
GET  /api/public/enterprise-solutions/:slug
GET  /api/public/enterprise-promotion/home
POST /api/public/enterprise-consultations
```

后台接口：

```text
GET/POST/PUT /api/admin/training-cases
GET/POST/PUT /api/admin/enterprise-solutions
GET/PUT      /api/admin/enterprise-consultations
```

视频仍使用受控短时地址；公开案例允许匿名播放，但需限流、防盗链并支持 Range 请求。

## 12. 分享与推广

每个页面设置独立分享标题、封面和路径参数。

案例分享示例：

> 韩老师企业九型培训实录｜团队沟通与关系工作坊

分享落地页必须直接进入对应案例，不能先要求登录或跳到首页。

需要记录：

- 分享进入；
- 案例卡片点击；
- 视频开始播放；
- 25%/50%/90% 播放进度；
- 咨询按钮点击；
- 咨询提交成功。

## 13. 隐私与内容合规

发布企业现场视频前必须确认：

- 企业名称、Logo 和现场环境是否允许公开；
- 员工正脸和声音是否具备使用许可；
- PPT、白板、名单和屏幕内容是否包含内部信息；
- 客户反馈是否获得公开授权；
- 未取得授权的企业名称、人员和敏感画面需脱敏、裁切或打码。

后台为案例保存授权状态和备注，未确认授权的案例不能发布。

## 14. 错误处理

- 视频损坏：后台阻止发布并显示检测原因；
- 视频加载失败：保留封面、摘要、刷新和咨询入口；
- 空案例列表：展示品牌说明和咨询按钮；
- 表单失败：保留用户输入并允许重试；
- 配置缺失：使用安全默认文案，不展示虚构数据；
- 页面下线：分享链接进入友好下线页，并推荐其他案例。

## 15. 测试策略

### 小程序

- 四个一级菜单路由和文案；
- 首页区块显示/隐藏；
- 固定主题入口、案例列表和详情；
- 视频播放失败、重试和地址刷新；
- 企业咨询校验、幂等和来源归因；
- 分享路径和参数；
- 图片失败回退与可访问性。

### 后端

- 案例/课程发布状态；
- 未授权案例发布拦截；
- 媒体引用与删除保护；
- 匿名播放限流和短时地址；
- 表单防重复与来源字段；
- 后台权限和审计。

### 后台

- 案例编辑与预览；
- 企业名称脱敏；
- 视频状态与引用；
- 咨询线索跟进；
- 发布、下线、排序和失败状态。

## 16. 分阶段范围

### 第一阶段：推广 MVP

- 新一级菜单；
- 企业推广首页；
- 案例列表和详情；
- 企业培训方案列表和详情；
- 企业咨询表单；
- 后台案例、培训方案和线索管理；
- 宣传短片和案例精华播放；
- 基础分享与转化统计；
- 第 24 节列出的首页配置、媒体处理、授权门禁和基础归因均属于规范性 MVP 范围。

### 第二阶段：内容运营

- 动态行业/主题筛选；
- 更多主题片段；
- 客户反馈的聚合展示、视频反馈和样式升级（一期仅提供经授权的基础文本反馈）；
- 分享海报；
- 渠道二维码和销售归因；
- 案例推荐。

### 第三阶段：营销自动化

- 企业线索评分；
- 跟进提醒；
- 案例转化看板；
- 不同行业落地页；
- 与 CRM 或企业微信客户系统对接。


## 17. 页面路由与旧功能去向

### 17.1 新路由

```text
/pages/index/index                              首页 Tab
/pages/training-cases/index                     培训案例 Tab
/pages/enterprise-solutions/index               培训方案 Tab
/pages/enterprise-consultation/index            咨询合作 Tab（合作说明和通用入口）
/pages-enterprise/consultation/form                非 Tab 咨询表单（承接案例/方案参数）
/pages-enterprise/training-case/detail          案例详情（slug 参数）
/pages-enterprise/enterprise-solution/detail    培训方案详情（slug 参数）
/pages-enterprise/teacher/detail                韩老师介绍
/pages-enterprise/privacy/consultation          咨询隐私说明
```

案例详情、方案详情和老师介绍放入企业推广分包。分享链接直接进入详情；点击底部 Tab 时回到对应列表。详情页返回优先恢复来源列表位置，无来源历史时返回“培训案例”。

### 17.2 CTA 参数传递

案例和方案 CTA 使用非 Tab 页面 `/pages-enterprise/consultation/form`，通过 `navigateTo` 携带：

```text
source_page
case_slug
solution_slug
share_token
channel
```

咨询合作 Tab `/pages/enterprise-consultation/index` 只提供合作说明、联系方式和“提交通用需求”入口，不承接 query。通用入口再 `navigateTo` 非 Tab 表单。

前端参数只用于展示，服务端重新解析合法案例/方案并生成可信归因记录，不能直接信任客户端传入的企业名或标题。表单页为每次打开创建 `consultation_context_id`：同一页面返回时恢复未提交草稿；提交成功、用户主动清空或超过 24 小时后清除。冷启动分享进入先建立 promotion session，再生成上下文；重复打开以最近一次明确 CTA 为 last-touch，但不覆盖 first-touch。

### 17.3 旧功能去向

| 旧功能 | 首期处理 |
|---|---|
| 九型测试 | 首页“九型工具”次级入口，保留原深链 |
| 关系合盘 | 首页“九型工具”次级入口，保留原深链 |
| 学习中心 | 原 URL 保留兼容页，引导到培训案例；不再作为 Tab |
| 个人预约 | 原 URL 保留，但入口并入咨询页底部“个人咨询”弱入口 |
| 预约记录 | 登录后的个人服务入口保留 |
| 我的/账号 | 咨询页底部“账号与隐私”进入，不删除注销、订单和隐私能力 |
| 已有分享链接 | 继续可访问；不存在的内容显示兼容提示，不直接 404 白屏 |

旧 Tab 移除前必须完成微信小程序升级兼容测试，确保历史页面路径仍注册或提供明确跳转。

## 18. 实施级数据模型

### 18.1 `training_cases`

- `id`, `slug`（唯一且发布后稳定）
- `title`, `summary`, `cover_asset_id`
- `company_display_name`, `company_internal_name_encrypted`
- `industry`, `city`, `participant_range`
- `training_date`, `duration_label`
- `business_challenges`, `training_goals`, `training_modules`, `training_methods`
- `trainer_id`（FK → `enterprise_trainers.id`）, `trainer_name_snapshot`
- `status`: `draft | review | published | offline`
- `authorization_status`: `pending | approved | expired | revoked`
- `featured`, `sort_order`
- `version`（乐观并发）
- `published_at`, `created_by`, `updated_by`, timestamps

约束：`slug` 唯一；只有 `status=published`、`authorization_status=approved` 且必须具备 ready 的封面以及至少一个 `role=promo` 或 `role=highlight` 的 ready 视频才能进入公开接口。首期不发布纯图文案例。删除默认 RESTRICT，使用下线而不是物理删除。

### 18.2 `enterprise_solutions`

- `id`, `slug`（唯一）
- `title`, `summary`, `cover_asset_id`
- `audiences`, `problems`, `goals`, `modules`, `delivery_methods`
- `recommended_participants`, `recommended_duration`, `customizable_items`
- `trainer_id`（FK → `enterprise_trainers.id`）, `trainer_name_snapshot`
- `status`: `draft | review | published | offline`
- `featured`, `sort_order`, `version`
- `published_at`, `created_by`, `updated_by`, timestamps

公开接口只返回 published 方案。页面统一使用“培训方案/解决方案”，不出现购买、课节、会员或进度语义。

### 18.3 有序关联

- `training_case_media(id, case_id, media_asset_id, role, position, caption, status)`
  - `role`: `promo | highlight | topic_clip | gallery`
  - `(case_id, role, position)` 唯一；资产删除受 RESTRICT 保护。
- `training_case_solutions(case_id, solution_id, position)`
  - 关联案例与方案，组合唯一并稳定排序。
- `training_case_testimonials(id, case_id, quote, speaker_display, speaker_role, provenance, consent_id, status, position)`
- `training_case_claims(id, case_id, claim_type, statement, source_reference, reviewed_by, reviewed_at, status)`
  - `claim_type`: `fact | client_quote | editorial_summary`
  - 禁止将编辑摘要伪装成客户原话或保证性效果。
- `training_topics(id, key, title, sort_order, enabled)`
  - 首期固定 `team-communication | leadership | cohesion | culture | employee-growth` 五个 key；后台只可调整标题、顺序和启用状态，不能创建任意未审核 key。
- `training_case_topics(case_id, topic_id, position)`
  - 组合唯一；每个 published 案例至少一个主题；公开 API 的 `topic` 参数只接受启用 key。

### 18.4 企业讲师资料

`enterprise_trainers` 是企业推广域的老师单一数据源：

- `id`, `key`（唯一且稳定）
- `name`, `title`, `avatar_asset_id`
- `short_bio`, `full_bio`, `specialties`, `credentials`
- `service_industries`, `experience_summary`
- `status`, `sort_order`, `version`
- `created_by`, `updated_by`, timestamps

案例和培训方案统一保存 `trainer_id + trainer_name_snapshot`；稳定 `key` 只属于 `enterprise_trainers`，不作为业务表的第二套外键。页面优先读取当前 published 讲师资料，名称快照用于历史审计和资料下线降级。首页老师卡片、讲师详情、案例和方案统一引用此实体，不再从个人学习 `site-config` 或课堂 teacher 字段拼接。

### 18.5 首页配置

`enterprise_promotion_settings` 保存 Hero、可信数据、精选案例、精选方案、老师卡片与 CTA。单行版本化更新，支持草稿预览和发布；公开首页 API 只返回已发布快照。首页不是硬编码，也不复用通用站点 JSON 的个人学习字段。

### 18.6 咨询与跟进

`enterprise_consultations`：

- 企业、行业、城市、人数、需求、培训时间、联系人、手机号、微信、备注；
- `source_page`, `case_id`, `solution_id`；
- `first_touch_session_id`, `last_touch_session_id`, `share_token`, `channel`；
- `status`: `new | contacted | qualified | proposal | won | lost | spam`；
- `assignee_id`, `version`, timestamps；
- `consultation_reference_hash`（唯一，只存哈希；明文仅创建成功时返回）；
- `privacy_notice_version`, `consented_at`, `consent_source`；
- `consent_ip_hash`、`consent_user_agent_hash`（仅保存摘要，不保存多余原始设备数据）；
- PII 字段加密保存，列表默认脱敏。

`enterprise_consultation_notes` 独立保存跟进记录、操作者和时间，不能覆盖历史备注。导出、查看完整手机号和负责人变更均写审计日志。

## 19. 授权与隐私模型

### 19.1 发布授权

新增 `publication_consents`：

- 主体类型：`company | person | media_asset | testimonial | document_screen`；
- 主体标识和显示别名；
- 授权渠道：小程序、官网、朋友圈、宣传片等；
- 使用范围：Logo、正脸、声音、PPT、白板、客户原话；
- 凭证资产或合同引用；
- 生效时间、到期时间；
- `status`: `pending | approved | expired | revoked`；
- 审核人、审核时间、撤回原因和审计记录。

`training_case_consent_links` 将案例、素材、人员、反馈与授权记录关联。派生短片必须记录 `source_asset_id` 并继承源素材授权上限，不能因剪辑生成新文件而绕过授权。

发布门禁逐项检查案例引用的企业、Logo、人员、声音、屏幕内容、媒体和反馈。任一必需授权过期或撤回时：

1. 案例自动进入不可公开状态；
2. 停止签发新的播放地址；
3. 清理公开缓存；
4. 记录审核事件并通知负责人。

### 19.2 咨询隐私

表单提交前展示隐私告知并要求明确勾选“同意为企业培训咨询目的联系我”。记录隐私文本版本和同意时间。

- PII 加密存储，后台按权限解密；
- 默认保存期限 24 个月，可由配置调整；
- 支持查询、更正和删除请求；
- 导出文件设置有效期并记录下载审计；
- 垃圾线索也按保留策略清理；
- 统计事件不存完整手机号、微信或自由文本需求。

匿名咨询的查询、更正和删除采用最小隐私请求流程：提交咨询成功时返回不可枚举的 `consultation_reference`。申请人使用 reference + 原手机号发起短信验证码，验证通过后创建 `consultation_privacy_requests` 工单。

```text
POST /api/public/privacy/consultation-requests/initiate
POST /api/public/privacy/consultation-requests/verify
GET  /api/admin/privacy/consultation-requests
GET  /api/admin/privacy/consultation-requests/:id
POST /api/admin/privacy/consultation-requests/:id/approve
POST /api/admin/privacy/consultation-requests/:id/reject
POST /api/admin/privacy/consultation-requests/:id/complete
```

请求类型为 `access | correction | deletion`。短信验证按手机号/IP/reference 限流，reference 不在列表和埋点中暴露。`consultation_reference` 使用至少 128 bit 随机值，明文只在创建成功响应中返回；服务端只存带 server-side pepper 的哈希。reference 在咨询完成删除/匿名化并过审计保留期后失效，疑似泄露时可主动轮换并使旧值失效。

`consultation_privacy_requests` 至少包含：`id`、`consultation_id`、`request_type`、`verified_phone_hash`、`correction_payload_encrypted`、`status(pending|verified|approved|rejected|completed)`、`verification_expires_at`、`reviewed_by/reviewed_at`、`completed_by/completed_at`、处理依据和 timestamps。

删除请求完成时对业务 PII 做删除或不可逆匿名化；法律、风控和审计必须保留的最小记录按保留策略隔离保存。每一步记录处理人、依据和时间。

## 20. 媒体处理流水线

### 20.1 状态机

```text
reserved → uploading → uploaded → probing
                              ├─ 检测通过 → transcoding → validating → ready
                              ├─ 检测异常 → quarantined
                              └─ 系统错误 → failed

quarantined ─人工确认重新处理→ 新 processing_attempt → probing
quarantined ─确认不可用→ rejected
```

`quarantined`、`rejected` 和 `failed` 都不能直接进入 ready；重新处理必须创建新的 attempt 并保留上一轮检测记录。

### 20.2 检测与转码

每个源文件保存 manifest：原始相对路径或素材 ID、文件大小、SHA-256、拍摄日期、容器、编码、时长和备份位置。团队使用素材 ID，不把 `/Volumes/KODAK` 绝对路径作为业务依赖。

发布派生视频统一转码为 MP4/H.264/AAC，并执行：

1. `ffprobe` 容器和流检查；
2. 全量 decode 校验，而不是只抽帧；
3. 时间戳、音画同步和时长一致性检查；
4. 响度归一化和音轨存在检查；
5. Range/206 支持检查；
6. 派生文件 SHA-256 和媒体元数据入库；
7. 人工首尾、随机点位和完整播放抽检记录。

已发现 NAL/AAC 错误的合成文件进入 quarantine。系统保存错误码、检测日志、重试次数和操作者；失败时从对应 MOV 原片重新剪辑，禁止以“播放器偶尔能打开”作为 ready 标准。

### 20.3 播放地址刷新

匿名播放地址短期有效。播放器遇到 401/403 或签名过期时：

- 保存当前播放秒数；
- 最多自动刷新一次播放票据；
- 使用新 URL 从原秒数续播；
- 刷新失败显示重试和咨询入口；
- 统计失败不影响视频播放。

## 21. 来源归因与基础统计

### 21.1 MVP 范围

第一阶段包含基础来源归因，但不包含批量渠道二维码管理和复杂销售归因模型。

新增：

- `promotion_sessions`：匿名 session、first-touch、last-touch、share token、channel、首次和最近访问时间；
- `promotion_events`：session、事件类型、案例/方案、页面、事件时间、幂等键；
- `promotion_share_tokens`：创建人/渠道、目标页面、有效期和状态。

### 21.2 规则

- first-touch 首次写入后不覆盖；
- last-touch 每次有效推广来源更新；
- 咨询保存 first-touch、last-touch 和提交前最近浏览案例；
- “看过的案例”通过事件集合查询，不把完整集合重复塞入线索行；
- 客户端生成 event id，服务端唯一约束去重；
- 服务端根据请求路径和合法 token 重建可信字段；
- 匿名 session 保存 90 天，聚合统计按隐私策略长期保存；
- 播放进度事件按 25%/50%/90% 单视频单 session 去重。

MVP 事件接口：

```text
POST /api/public/promotion/sessions
POST /api/public/promotion/events
```

事件上报失败不阻塞页面、播放或咨询提交。

## 22. API 契约与状态操作

### 22.1 公开列表

```text
GET /api/public/training-cases?page=1&page_size=12&topic=team-communication
```

响应只包含已发布、已授权案例及安全显示字段，不返回真实企业内部名、授权凭证、源素材地址或后台备注。分页上限 50，稳定排序为 `featured DESC, sort_order ASC, id DESC`。

### 22.2 后台资源

```text
GET    /api/admin/training-cases
POST   /api/admin/training-cases
GET    /api/admin/training-cases/:id
PUT    /api/admin/training-cases/:id
POST   /api/admin/training-cases/:id/submit-review
POST   /api/admin/training-cases/:id/publish
POST   /api/admin/training-cases/:id/offline
GET    /api/admin/training-cases/:id/preview

GET/POST/PUT /api/admin/enterprise-solutions[/:id]
POST         /api/admin/enterprise-solutions/:id/publish
POST         /api/admin/enterprise-solutions/:id/offline

GET  /api/admin/enterprise-consultations
GET  /api/admin/enterprise-consultations/:id
PUT  /api/admin/enterprise-consultations/:id
POST /api/admin/enterprise-consultations/:id/notes
POST /api/admin/enterprise-consultations/export
```

更新请求必须携带 `version`；版本冲突返回 409。发布失败返回结构化门禁错误，例如 `CONSENT_MISSING`、`MEDIA_NOT_READY`、`CLAIM_UNREVIEWED`。草稿预览使用短时后台预览 token，不进入公开缓存。

### 22.3 一期资源端点矩阵

首页配置：

```text
GET  /api/public/enterprise-promotion/home
GET  /api/admin/enterprise-promotion/settings
PUT  /api/admin/enterprise-promotion/settings
GET  /api/admin/enterprise-promotion/settings/preview
POST /api/admin/enterprise-promotion/settings/publish
```

讲师资料：

```text
GET  /api/public/enterprise-trainers
GET  /api/public/enterprise-trainers/:key
GET/POST/PUT /api/admin/enterprise-trainers[/:id]
POST /api/admin/enterprise-trainers/:id/publish
POST /api/admin/enterprise-trainers/:id/offline
```

媒体上传与处理：

```text
POST /api/admin/promotion-media/uploads/initiate
POST /api/admin/promotion-media/uploads/:id/parts/:part/sign
POST /api/admin/promotion-media/uploads/:id/complete
POST /api/admin/promotion-media/uploads/:id/abort
GET  /api/admin/promotion-media/assets
GET  /api/admin/promotion-media/assets/:id
POST /api/admin/promotion-media/assets/:id/reprocess
GET  /api/admin/promotion-media/assets/:id/attempts
POST /api/public/training-cases/:slug/media/:case_media_id/play-ticket
POST /api/public/training-cases/:slug/media/:case_media_id/play-ticket/refresh
```

授权、反馈和宣传主张：

```text
GET/POST/PUT /api/admin/publication-consents[/:id]
POST /api/admin/publication-consents/:id/approve
POST /api/admin/publication-consents/:id/revoke
GET  /api/admin/training-cases/:id/consent-links
POST /api/admin/training-cases/:id/consent-links
DELETE /api/admin/training-cases/:id/consent-links/:link_id
GET  /api/admin/promotion-media/assets/:id/consent-links
POST /api/admin/promotion-media/assets/:id/consent-links
DELETE /api/admin/promotion-media/assets/:id/consent-links/:link_id
GET/POST/PUT /api/admin/training-cases/:id/testimonials[/:testimonial_id]
POST /api/admin/training-cases/:id/testimonials/:testimonial_id/review
GET/POST/PUT /api/admin/training-cases/:id/claims[/:claim_id]
POST /api/admin/training-cases/:id/claims/:claim_id/review
```

培训方案完整契约：

```text
GET    /api/admin/enterprise-solutions
POST   /api/admin/enterprise-solutions
GET    /api/admin/enterprise-solutions/:id
PUT    /api/admin/enterprise-solutions/:id
POST   /api/admin/enterprise-solutions/:id/submit-review
GET    /api/admin/enterprise-solutions/:id/preview
POST   /api/admin/enterprise-solutions/:id/publish
POST   /api/admin/enterprise-solutions/:id/offline
```

分享与统计：

```text
POST /api/admin/promotion/share-tokens
GET  /api/admin/promotion/share-tokens
POST /api/admin/promotion/share-tokens/:id/disable
POST /api/public/promotion/sessions
POST /api/public/promotion/events
GET  /api/admin/promotion/analytics/funnel
GET  /api/admin/promotion/analytics/cases
```

授权 link 创建请求包含 `consent_id/subject_type/subject_id/use_scope`；服务端验证主体确实属于案例或媒体。反馈和人员的授权 link 既可通过上述案例聚合端点维护，也必须在 GET 响应中返回门禁缺口。所有后台 PUT 请求带 `version`；所有列表支持分页和允许字段白名单筛选。播放 ticket 响应包含 `case_media_id/url/expires_at/media_version/resume_supported`，刷新请求包含当前 `case_media_id`、`media_version` 和播放秒数。服务端必须校验 `case_media_id` 属于该 slug、角色为 promo/highlight/topic_clip、状态公开、底层资产 ready 且所有授权有效；不得接受客户端直接指定底层 `media_asset_id`。版本变化时服务端返回可解释的重新开始指令。

### 22.4 咨询幂等与防滥用

`POST /api/public/enterprise-consultations` 要求 `Idempotency-Key`，请求体必须包含 `privacy_notice_version` 和明确的 `contact_consent=true`。服务端只接受当前有效或仍在兼容期的隐私版本，写入服务端生成的 `consented_at`、consent source 和请求摘要；版本已失效返回 `PRIVACY_NOTICE_OUTDATED`，客户端刷新文案后重新确认。相同 key 在 24 小时内返回同一结果；客户端超时后使用相同 key 重试。服务端同时按规范化手机号、企业和时间窗口标记疑似重复，但不静默丢弃真实线索。

防滥用包括：IP/设备限流、隐藏蜜罐字段、提交耗时检查、异常频率拦截；风险升高时启用验证码。手机号按中国大陆规则校验，联系同意为必填。

## 23. 课堂分支复用边界

截至本设计日期，`feature/miniapp-teacher-classroom` 尚未合入 `main`，且其工作树仍有未提交修改。推广功能不能把该分支视为已存在依赖。

实施顺序必须选择以下一种并在计划中固定：

1. 从课堂分支提取经过测试的中立媒体上传/资产能力，先形成不含会员、支付、权益的共享媒体基础；或
2. 在推广分支实现最小独立案例媒体服务，后续再统一基础设施。

推荐方案 1，但只能复用：multipart 上传、媒体资产、校验、短时播放、后台上传组件。通过 adapter 暴露 `MediaAssetRef`，推广域不直接调用课堂会员鉴权、订单、entitlement 或 progress。

跨域资产引用统一进入媒体引用表；存在案例引用时禁止删除资产。课堂权限码与企业推广权限码独立，推广菜单使用 `EnterprisePromotion:*` 权限。

## 24. MVP 范围消歧

第一阶段明确包含：

- 后台可配置首页；
- 案例和培训方案管理；
- 咨询线索管理；
- 视频素材上传、全量校验、标准化转码或接收已转码成品；
- 固定主题快捷入口，不做动态多维筛选；
- 基础 session/channel/share token 归因与漏斗事件；
- 分享直达；
- 公开匿名播放与续播刷新。

第二阶段才包含：动态行业/主题组合筛选、渠道二维码批量生成、销售渠道看板、推荐算法和分享海报。

## 25. 补充错误与降级规则

- 404：slug 从未存在，返回标准不存在页；
- offline/unauthorized：不暴露具体授权原因，显示“该案例暂未开放”；
- media processing：后台可见进度，公开页不返回未 ready 媒体；
- 首页部分接口失败：使用最近一次已发布快照，单区块失败不拖垮整页；
- 咨询超时：相同 Idempotency-Key 重试并恢复已提交结果；
- 上传中断：根据 upload task 恢复分片，不重新创建孤儿对象；
- 统计失败：本地有限重试，永不阻塞业务主流程；
- 缓存：发布、下线、授权撤回、排序后按资源 tag 精确失效。

## 26. 发布验收与跨层测试

除第 15 节测试外，发布前必须覆盖：

- 数据库迁移与回滚；
- API schema/contract 测试；
- 公开接口不泄漏草稿、真实企业内部名、PII、授权凭证和源资产地址；
- 每个引用资产的授权发布门禁；
- 授权撤回自动下线并停止签发 URL；
- 损坏媒体从检测、隔离到发布拦截的全链路；
- 签名 URL 过期后的刷新续播；
- 咨询并发幂等、超时重试和反滥用；
- 归因 first/last touch 与事件去重；
- PII RBAC、解密查看和导出审计；
- 首页 → 案例 → 培训方案 → 咨询完整 E2E；
- 分享直达详情 E2E；
- 旧 Tab、旧深链、账号注销和个人工具回归；
- 企业名称脱敏、客户原话与编辑摘要的展示区分。

发布验收口径：

- 5 名目标企业用户测试中，至少 4 名能在 10 秒内说出“这是韩老师企业培训案例与合作咨询小程序”；
- 所有公开案例具备完整授权门禁记录；
- 所有公开视频通过全量 decode 和人工抽检；
- 核心漏斗事件可在测试环境完整串联；
- 咨询提交重复请求不会生成重复线索；
- 页面首屏、视频失败和表单失败均有可操作降级状态。

商业转化率首期建立基线，不在没有历史数据时虚构目标值。

## 27. 非目标

首期不包含：

- 完整 4～8 小时原始录像公开播放；
- 面向个人用户的付费网课商城；
- 会员权益和学习进度；
- 复杂推荐算法；
- 在线排课和教师日历；
- 企业内部员工学习平台；
- `test` 分支相关改动。
