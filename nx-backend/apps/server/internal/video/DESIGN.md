# video 设计文档

## 设计概述

### 目标

通过 New API 网关（`https://zz1cc.cc.cd`，OpenAI 兼容）接入视频生成模型（即梦/Dreamina 2.0），为后端提供文生视频 / 图生视频能力，并将生成结果持久化为站内资源。

### 非目标

- 不在本模块内做模型推理或转码，全部委托给上游网关。
- 不实现后台轮询 worker / webhook 回调（见「未来扩展点」），当前由前端按需触发刷新。
- 不做视频在线剪辑、转格式等二次加工。

## 架构设计

### 整体架构

```text
前端(web-antd /video/generate)
        │  generateVideoApi / refreshVideoGenerationApi
        ▼
server 路由 (s.requireAuth 保护)
        │
        ▼
video.Store ──Generate──► Client.CreateTask ──► New API /v1/videos
   │                                                   │ task_id
   │  落库 queued 行(含 task_id)  ◄────────────────────┘
   │
   └──Refresh──► Client.QueryTask ──► New API /v1/videos/{task_id}
                      │ completed
                      ▼
                Client.DownloadTaskContent ──► uploadasset.Store.Create ──► 回填 video_url/时长/分辨率
```

### 核心组件

- **Store**: 模块入口，编排 Generate（建任务+落库）与 Refresh（轮询+下载+回填），并提供 ListGenerations / Generation 查询。
- **Generation**: 数据传输/持久化模型，承载任务状态、提示词、视频资源 ID、时长、帧率、分辨率等。
- **GenerateInput**: 生成请求入参（Prompt / ImageURL / Model），至少需提供 Prompt 或 ImageURL 之一。
- **Client**: 对 New API 网关的 HTTP 封装，含 CreateTask / QueryTask / Download，以及鲁棒的响应解析（doJSON / parseTask / 点路径取值）。
- **TaskResult**: 网关任务查询结果的归一化中间结构。

## 设计决策

### 决策记录

| 日期 | 决策 | 理由 | 影响 |
| --- | --- | --- | --- |
| 2026-06-28 | Generate / Refresh 异步两步拆分，不阻塞等待 | 视频生成耗时长（数十秒~分钟级），同步阻塞会拖垮请求与连接 | 需要前端/调用方按 task_id 轮询；落库 queued 行作为游标 |
| 2026-06-28 | normalizeStatus 归一化上游状态别名 | 上游 pending/processing/success 等命名不统一 | 内部状态收敛为 queued/processing/completed/failed，便于前端判断 |
| 2026-06-28 | parseTask 采用点路径(dotted-path)容错解析 | 网关响应结构存在嵌套且字段位置不稳定 | 解析对字段缺失/层级变化更鲁棒，降低对端改动导致的破坏 |
| 2026-06-28 | 完成后下载视频字节并经 uploadasset 落地，而非直存网关 URL | 网关 URL 可能短时过期/受鉴权限制 | 站内资源可控、可长期访问；代价是占用存储与一次下载开销 |
| 2026-06-28 | 资源边界：下载 200MB、JSON 4MB、提示词 ≤2000 runes | 防止异常大响应/超长输入耗尽内存 | 超限会被截断/拒绝，保护服务稳定性 |

### 技术选型

- **语言**: Go
- **理由**: 与 server 主服务同栈，复用 `database/sql`、`net/http`、`uploadasset` 等现有基础设施，无需引入额外依赖；标准库即可满足 HTTP 调用与流式下载需求。

## 权衡取舍

### 已知限制

- **轮询驱动**: 无后台 worker，任务最终态依赖调用方触发 Refresh；若无人刷新，queued 行不会自动转终态。
- **下载体积上限 200MB**: 超长/超大视频可能被 LimitReader 截断，需后续按需调整或改为流式直存。
- **单网关耦合**: 仅适配 New API 的视频接口契约，更换供应商需调整 Client 解析逻辑。

### 技术债务

- **缺少后台轮询/回调**: 临时由前端轮询触发 | 原因：交付优先、先打通主链路 | 计划偿还：引入 background poll worker 或 webhook 后回填。

## 安全考量

### 威胁模型

- **未授权访问**: 视频生成会消耗上游配额/费用，接口若裸露会被滥用。
- **密钥泄露**: VIDEO_API_KEY 若硬编码或被日志回显，可能被窃取盗刷。
- **SQL 注入**: 列表查询含状态/关键字过滤，拼接不当会引入注入。
- **超大响应/输入**: 恶意或异常的大响应、超长提示词可能耗尽内存。

### 安全措施

- **访问控制**: 所有视频路由均挂载在 `s.requireAuth` 之后，需登录鉴权。
- **密钥管理**: VIDEO_API_KEY 经 config 注入，不硬编码、不回显到响应或日志。
- **参数化查询**: ListGenerations 的状态/关键字过滤使用参数化占位符，避免拼接注入。
- **资源边界**: 下载 200MB、JSON 4MB、提示词 2000 runes 上限，防止资源耗尽。

## 未来扩展点

- 后台轮询 worker：定时扫描非终态任务并自动 Refresh，去除对前端轮询的依赖。
- Webhook 回调：上游支持时改为事件驱动回填，降低轮询开销与延迟。
- 更多生成参数：分辨率、时长、帧率、风格等透传到 CreateTask。

## 变更历史

### 2026-06-28 - 初始版本

**变更内容**: 创建 video 模块，接入 New API 视频生成（文生/图生视频），实现异步两步任务管理与结果落地。

**变更理由**: 实现项目的视频生成功能需求。
