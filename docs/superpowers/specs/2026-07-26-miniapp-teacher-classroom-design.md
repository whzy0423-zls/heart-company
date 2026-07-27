# 小程序「老师课堂」视频与音频课件设计

**日期：** 2026-07-26
**状态：** 已确认，待拆解实施计划

## 1. 背景与问题

当前小程序的主流程（测试、结果、关系分析、预约、档案）已经可用，但学习中心的课程区域仍是静态卡片：

- `coursewareItems` 已归一化 `title/description/cover/badge/duration/url`，但卡片没有点击、详情或播放能力；
- 后台“课程管理”维护的是课程方向与报名产品，不是可发布的视频/音频内容；
- 后台老师配置使用 `home.teacherTeaser`，小程序老师归一化逻辑读取的结构不一致；
- 通用上传链路约 20MiB、整文件读入内存并可能写入数据库，不适合历史开课长视频；
- 现有视频资产库服务视频制作，不应直接作为面向用户的课程内容库。

## 2. 用户确认的产品决策

### 2.1 术语

- 后台一级模块：**老师课堂**
- 后台子模块：课件管理、课程系列、上传任务
- 小程序栏目：**老师课堂**
- 内容类型：视频课件、音频课件
- 完整历史授课录像标签：课程回放
- 课程方向/报名产品继续保留为后台“课程管理”，不与老师课堂合并

### 2.2 内容组织

采用双入口：

```text
老师课堂
├── 课程系列 → 多个视频/音频课时
└── 独立内容 → 单条视频/音频
```

同一内容可以属于系列，也可以作为独立内容发布。系列和独立内容在小程序中分别展示，并提供最近播放/继续学习入口。

### 2.3 权限与付费

系列和单课均可配置：

- 公开
- 登录后观看
- 会员观看
- 单独付费

系列支持打包价；单课支持独立定价。已购买系列解锁系列内课程，购买单课只解锁对应内容。会员权限按有效期实时判断。系列或单课下线后停止新购买，已购用户的保留策略由业务配置确定。

## 3. 架构方案

采用独立“老师课堂”领域与 OSS 分片直传，不扩展站点 JSON 作为正式内容库，也不复用视频制作 `video_assets` 表。

### 3.1 数据模型

#### `classroom_series`

- `id`
- `title`
- `summary`
- `cover_url` / `cover_asset_id`
- `teacher_key`
- `teacher_name_snapshot`
- `sort_order`
- `status`: `draft | published | offline`
- `playback_blocked`
- `access_level`: `public | login | member | paid`
- `price_cents`（系列打包价）
- `published_at`
- `created_by`, `updated_by`, timestamps

#### `classroom_contents`

- `id`
- `series_id`（可空）
- `show_as_standalone`（属于系列时是否也进入独立内容入口）
- `title`, `description`
- `content_type`: `video | audio`
- `media_asset_id` / object key
- `cover_url`
- `duration_seconds`
- `teacher_key`
- `teacher_name_snapshot`
- `recorded_at`
- `badge`, `tags`
- `episode_no`, `sort_order`
- `status`: `draft | processing | ready | published | offline | failed`
- `playback_blocked`
- `access_level`: `inherit | public | login | member | paid`
- `price_cents`
- `published_at`
- `created_by`, `updated_by`, timestamps

#### `classroom_upload_tasks`

记录分片上传任务、媒体校验、封面抽取、时长读取、失败原因、重试次数和最终资产引用。大文件不写入数据库 BYTEA。

任务至少包含：`content_id`（唯一绑定草稿）、`creator_id`、`oss_upload_id`、服务端生成的 `object_key`、`expected_size`、`checksum`、`part_size`、`max_parts`、`expires_at`、`attempt_count`、`cleanup_status` 和 timestamps，并为 `content_id/status/expires_at` 建索引。

#### `classroom_media_assets`

独立保存媒体元数据，不复用 `upload_assets.data BYTEA`：

- `id`, `bucket`, `object_key`, `etag`, `checksum`
- `content_type`, `size_bytes`, `duration_seconds`
- `width`, `height`, `cover_object_key`
- `storage_status`: `pending | uploaded | processing | ready | failed | deleted`
- `created_by`, timestamps

视频/音频对象统一放在服务端生成的前缀下，客户端不能自行指定 bucket、目录或 object key。

#### `classroom_entitlements`

记录用户对系列或单课的购买、人工补发、撤销和有效期，用于播放鉴权。课堂会员状态直接读取现有用户会员等级和到期时间，不复制到课堂权益表，避免双写。

#### `classroom_progress`

记录用户最近播放、播放秒数、完成比例和最近访问时间。第一阶段提供继续学习能力，收藏与统计可在后续扩展。

#### 订单与权益

复用现有 `orders` 支付回调体系，不引入第二套订单表。现表继续使用 `orders.product` 字段，新增课堂 product 枚举值，不另造 `product_type`：

- `product`: `classroom_series | classroom_content`
- `ref_id` 指向系列或课件
- `ref_id` 由 `product` 决定目标类型，创建订单时由服务层验证目标存在且可售
- 保存商品标题/金额快照、支付状态、退款状态和幂等键；同一用户同一目标只允许一个有效 pending 订单
- 支付成功回调在同一事务内发放 `classroom_entitlements`
- 权益约束保证 series/content 恰好命中一个目标；记录订单来源、有效期、撤销时间
- 退款、人工补发、重复回调和已购保留策略必须显式处理

购买流程复用现有微信支付订单能力：创建订单时保存商品、目标、金额和标题快照；支付回调按订单幂等更新状态并事务性发放权益；取消、支付失败、退款和人工补发均保留审计记录。不新建并行支付回调体系。

### 3.2 上传流程

```text
后台选择视频/音频
→ 服务端创建上传任务与分片凭证
→ 浏览器直传私有 OSS
→ 完成回调
→ 服务端校验 MIME、扩展名、大小与媒体可读性
→ 读取时长/抽取封面
→ 状态 ready
→ 填写元数据并保存草稿/发布
```

支持断点续传、进度显示、失败重试、孤儿文件清理。第一阶段只接受可直接在微信小程序稳定播放的媒体：视频为 MP4（H.264 + AAC），音频为 MP3 或 M4A（AAC）。服务端验证容器和编码，`ready` 必须代表可播放。MOV、裸 AAC 和其他格式在转码能力上线后再开放；具体大小由环境配置控制。

上传使用独立的课堂媒体 Multipart API，不改造通用 `/api/upload`：

```text
POST /api/admin/classroom/uploads/initiate
POST /api/admin/classroom/uploads/:id/parts/:part/sign
POST /api/admin/classroom/uploads/:id/complete
POST /api/admin/classroom/uploads/:id/abort
```

服务端负责校验任务所属管理员、object key 前缀、分片数量、大小、ETag/Checksum 和 OSS HeadObject；complete 必须幂等。任务包含凭证过期时间、最大分片数、重试次数和清理状态，过期任务执行 Abort/孤儿对象清理。浏览器直传需要显式配置 OSS endpoint、bucket、region、CORS、part size 和凭证 TTL。

### 3.3 播放流程

小程序不保存永久 OSS 地址。详情页只展示公开元数据；播放前请求短时签名地址：

```text
POST /api/miniapp/classroom/content/:id/play
```

服务端依次检查发布状态、登录状态、会员有效期、系列购买或单课购买，再返回短时播放 URL。URL 过期时小程序自动刷新；播放失败保留错误状态和重试操作。

公开内容也统一经过播放票据/签名接口：未登录用户使用短时匿名票据并限流，登录用户使用小程序 JWT。列表和详情接口只返回元数据，不返回永久 `object_url`。播放对象必须支持 HTTP Range/206；不经过现有 BYTEA 公共资源响应。

匿名播放票据由服务端签名，claims 至少包含 `content_id`、媒体版本、`exp`（不超过 5 分钟）和随机 nonce；限流键使用 IP + 设备标识，刷新接口按内容和客户端限频。票据在 TTL 内允许播放器 Range 请求复用，但不能跨内容或媒体版本使用；OSS/CDN 同时配置短 TTL、防盗链和签名 URL。

第一阶段音频采用页面内播放器，支持播放、暂停、拖动、续播和地址刷新；离开详情页后暂停。锁屏控制、系统后台持续播放和播放队列放到第二阶段。

### 3.4 后台权限

独立拆分查看、编辑、上传媒体、发布/下线、设置价格与权限等权限。上传权限不能继续复用任意 RAG/阅读/视频/语音权限 OR 逻辑。发布、改价、下线记录操作者、时间和变更原因。

权限码与菜单种子至少包含：`Miniapp:Classroom:List`、`Write`、`Upload`、`Publish`、`Price`，并覆盖路由、按钮级权限、角色绑定、缓存刷新和审计测试。

## 3.5 状态、列表与归属约束

- 课件状态：`draft → processing → ready → published → offline`；失败状态只能重试或回到草稿，未 `ready` 不得发布；
- 系列状态：`draft → published → offline`；属于系列的课件发布前要求系列已发布；系列下线后默认隐藏系列及其课时，但已购用户仍可按权益规则播放；
- 系列和课件均有 `playback_blocked` 紧急停播开关；普通 `offline` 停止公开展示和新购买，但已购用户仍可播放；硬停播时所有播放都拒绝；
- `series_id` 可空；系列内容通过 `show_as_standalone=true` 才额外进入独立内容入口，两个入口分别排序且单个列表内不重复；删除系列默认 `RESTRICT`，先迁移或解除课时归属；
- 系列/课件排序使用稳定的 `sort_order + id`，列表接口分页、过滤权限后再返回；公开列表只查 `published` 且有效媒体；
- API 定义 ETag/缓存失效规则；发布、下线、改价、系列排序会清理相关缓存；
- 进度写入校验用户与课件归属，服务端限制频率并使用幂等 upsert，不接受任意客户端 completed 百分比；
- 老师信息第一阶段统一使用 `teacher_key + teacher_name_snapshot`，不引入 `teacher_id`；先修复站点配置兼容映射，发布时写入名称快照；
- 权限过滤在查询层或服务层统一完成：所有已发布且可售内容可返回安全元数据和价格，服务端计算 `canPlay=false`；只有媒体地址、受保护详情和播放票据受限，不能依赖前端隐藏付费卡片。

### 3.5.1 权限解析规则

1. 无系列内容必须设置具体权限，不允许 `inherit`；
2. 系列内容为 `inherit` 时使用系列权限；设置具体值时单课覆盖系列权限，允许升级或降级；
3. 播放鉴权先判断硬下线/媒体状态，再计算 `effective_access`；
4. 系列购买解锁仍属于该系列的当前和未来课时；课时移出系列后不再由系列权益覆盖；
5. 单课购买只解锁该课；先单买后买系列，第一阶段不做金额抵扣；
6. `paid` 发布时必须有大于 0 的人民币分值价格；非付费权限价格必须为空或 0；改价不影响已有订单金额快照；
7. 系列下线只影响系列入口；其中 `show_as_standalone=true` 且课件仍为 published 的内容继续出现在独立内容入口。普通下线停止新购买但保留已购播放；`playback_blocked=true` 时系列/课件全部拒绝播放；
8. 公开内容无需登录，登录内容要求 JWT，会员内容读取现有会员权威状态，付费内容检查系列或单课权益。

会员权威字段为 `wx_users.member_level` 与新增的 `member_started_at/member_expires_at`；现有会员支付回调同步写入有效期。历史永久会员可用 `member_expires_at IS NULL` 表示，新的周期会员必须写入明确到期时间。

### 3.5.2 公开与小程序 API

```text
GET  /api/public/classroom/series
GET  /api/public/classroom/standalone
GET  /api/public/classroom/series/:id
GET  /api/public/classroom/content/:id
POST /api/miniapp/classroom/content/:id/play
POST /api/miniapp/classroom/orders
GET  /api/miniapp/classroom/orders/:id
PUT  /api/miniapp/classroom/content/:id/progress
GET  /api/miniapp/classroom/continue-learning
```

列表和详情返回 `effectiveAccess`、`canPlay`、`purchaseState`、`priceCents` 等展示字段，但不返回媒体 object key/永久 URL。已发布且可售的付费内容仍返回安全元数据供发现和购买。公开匿名响应与登录用户响应分开缓存，登录响应使用 `Vary: Authorization` 或不进入共享 CDN 缓存。

### 3.5.3 进度与匿名行为

- 登录用户使用服务端进度，每 10–15 秒或暂停/离页时限频 upsert；
- 完成状态由服务端根据时长和播放位置计算，默认达到 90%；
- 匿名公开内容只保存在本机，不跨设备同步，登录后可选择合并最新进度；
- 第一阶段的“继续学习”属于 MVP；第二阶段扩展收藏、详细学习记录和统计分析。

### 3.5.4 内容草稿与上传绑定

先创建课件草稿，再为该草稿创建上传任务。上传任务只允许绑定一个课件草稿；完成上传后生成媒体资产并回写草稿。取消编辑的已完成资产进入短期孤儿保留期，由定时任务清理；重复 complete/回调保持幂等。

### 3.5.5 老师数据来源

第一阶段暂不新建复杂老师实体：先修复后台 `home.teacherTeaser` 与小程序 `teacher/teachers` 的兼容映射，并为课堂内容保存稳定 `teacher_key` 与发布时 `teacher_name_snapshot`。后续引入独立老师实体时再迁移引用。

## 4. 页面与交互

### 4.1 后台

- **课件管理**：列表、筛选、类型、系列、权限、状态、排序、批量上下架；
- **课程系列**：系列信息、课时排序、系列价格、默认权限；
- **上传任务**：分片进度、校验状态、失败原因、重试和孤儿任务；
- **新建课件抽屉/页面**：选择视频或音频、归属系列、封面、标题、简介、老师、标签、时长、权限、单课价格、预览、保存草稿/发布。

### 4.2 小程序

- 学习中心新增“老师课堂”区块；
- 双入口：课程系列、独立内容；
- 系列详情展示课时、学习进度和继续学习；
- 独立内容展示视频/音频类型、时长、权限标签；
- 详情页根据类型展示 video 或 audio 播放器；
- 覆盖加载中、空态、权限不足、支付中、播放失败、地址刷新和重试状态；
- 保留返回列表位置和最近播放。

## 5. 分阶段范围

### 第一阶段：内容发布与播放 MVP

1. 修复老师配置字段断链；
2. 新增课件/系列/媒体资产/上传任务/权益数据模型与迁移；
3. 独立 Multipart 上传服务、媒体校验和过期清理；
4. 后台课件管理、系列管理、草稿/发布/下线；
5. 公开列表、详情、匿名/JWT 播放鉴权 API；
6. 小程序老师课堂双入口、详情页、视频/音频播放器；
7. 扩展现有订单回调实现系列/单课购买和权益幂等发放；
8. 四级权限、基础最近播放；
9. 端到端上传、发布、鉴权、支付、播放与失败重试测试。

第一阶段按可独立验收的里程碑交付：

- A：数据模型、老师字段兼容、媒体分片上传；
- B：后台内容/系列管理与小程序双入口播放；
- C：公开、登录、会员权限；
- D：系列/单课支付、回调与权益；
- E：进度、继续学习与发布审计。

### 第二阶段：学习体验

- 收藏、搜索、筛选、标签和更完整的学习历史；
- 字幕、倍速、多清晰度、转码状态；
- 配套 PDF/PPT、章节与期次；
- 定时发布、批量管理和内容推荐。

### 第三阶段：运营与数据

- 播放次数、完播率、转化漏斗；
- 老师维度内容主页；
- 会员/付费数据看板；
- CDN、盗链策略、媒体生命周期和孤儿资源清理。

## 6. 验收标准

- 长视频和长音频支持分片、断点、重试，不经过 20MiB 的整文件内存上传；
- 未发布、已下线或无权限内容不会被公开播放；
- 公开、登录、会员、付费四种权限均有自动化覆盖；
- 系列购买和单课购买解锁范围正确；
- 小程序不暴露永久 OSS 地址，签名过期可自动刷新；
- 视频/音频播放器支持基本拖动、续播和失败重试；
- 后台可完成上传、预览、草稿、发布、下线、排序、改价；
- 老师配置修改能正确同步到小程序；
- 上传、媒体校验、权限、支付、播放、发布审计均可追踪。
- 分片 complete/abort 幂等，重复回调、过期任务、孤儿对象和 OSS 故障均有覆盖；
- 非法 MIME、损坏媒体、Range/206、签名过期、越权播放、退款与人工补发均有测试。

## 7. 非目标

第一阶段不改造本地九型测试题库和九型数据为后台动态配置，不将视频制作资产库直接暴露给小程序，也不在首版加入复杂推荐算法和社交互动。
