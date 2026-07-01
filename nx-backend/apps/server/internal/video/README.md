# video

接入 New API / OpenAI 兼容网关（即梦 2.0）的视频生成模块。提供文生视频、图生视频能力，并以异步任务的方式管理生成生命周期。

## 概述

视频生成与同步的 `voice` 模块不同，是**异步**的：创建任务只返回 `task_id` 与初始状态，最终结果需轮询拉取。本模块据此拆为两步：

- **Generate**：调用网关创建任务，落库一行 `status='queued'` 并记录 `task_id`，立即返回，不阻塞等待结果。
- **Refresh**：按记录 id 轮询网关状态。完成后下载视频字节，经 `uploadasset` 落库为资产，并回填视频 URL 与元数据（时长、帧率、分辨率）。

## 特性

- **文生视频 / 图生视频**：`prompt` 与 `imageUrl` 至少提供其一；`prompt` 上限 2000 字。
- **异步任务管理**：创建即落库，状态机 `queued → in_progress → completed/failed`，前端按需轮询刷新。
- **状态归一化**：`normalizeStatus` 将网关返回的多种状态别名（pending/processing/success 等）收敛为统一集合。
- **容错轮询**：轮询网关失败时不改写本地状态，留待下次重试；下载失败标记为 `failed` 并记录原因。
- **响应解析鲁棒**：`parseTask` 以点分路径在嵌套 JSON 中查找字段，兼容网关的多种返回结构。
- **资源边界**：下载限制 200MB、网关响应体限制 4MB，防止内存失控。

## 使用方法

```go
store := video.NewStore(db, uploads, cfg) // cfg 为 config.VideoConfig

// 创建任务（不阻塞）
gen, err := store.Generate(ctx, video.GenerateInput{
    Prompt: "一只猫在草地上奔跑",
    Model:  "video-ds-2.0-fast", // 留空则用默认模型
})

// 轮询刷新单条任务，完成后回填视频资产
gen, err = store.Refresh(ctx, gen.ID)

// 分页查询历史
page, err := store.ListGenerations(ctx, query) // query 支持 status / keyword / page / pageSize
```

## API 概览

### 类型

| 类型 | 描述 |
| --- | --- |
| `Store` | 模块入口，封装 DB、网关客户端与资产存储，提供 Generate/Refresh/查询 |
| `Generation` | 一条视频生成记录（含状态、视频 URL、元数据） |
| `GenerateInput` | 创建任务的入参（Prompt / ImageURL / Model） |
| `Client` | New API 兼容视频网关的最小 HTTP 客户端 |
| `TaskResult` | 归一化网关创建/查询任务返回的字段 |

### 主要方法

| 方法 | 描述 |
| --- | --- |
| `NewStore()` | 构造 Store，缺省模型回落 `video-ds-2.0-fast` |
| `Store.Generate()` | 创建任务并落库 `queued` 行 |
| `Store.Refresh()` | 轮询单条任务，完成后下载视频并回填 |
| `Store.ListGenerations()` | 分页 + 状态/关键词过滤查询 |
| `Store.Generation()` | 按 id 查询单条记录 |
| `Client.CreateTask()` | `POST /v1/videos` |
| `Client.QueryTask()` | `GET /v1/videos/{task_id}` |
| `Client.DownloadTaskContent()` | `GET /v1/videos/{task_id}/content` |
| `Client.Download()` | 拉取最终视频字节（限 200MB） |

## 配置

通过 `config.VideoConfig` 注入（环境变量见 `.env.example`）：

| 变量                    | 说明                                |
| ----------------------- | ----------------------------------- |
| `VIDEO_API_BASE`        | 网关基址，如 `https://zz1cc.cc.cd`  |
| `VIDEO_API_KEY`         | 网关密钥（`Authorization: Bearer`） |
| `VIDEO_MODEL`           | 默认模型，缺省 `video-ds-2.0-fast`  |
| `VIDEO_TIMEOUT_SECONDS` | HTTP 超时，缺省 120s                |

## 目录结构

```text
video/
├── video.go     # Store + Client + 解析/分页/状态归一化
├── README.md
└── DESIGN.md
```

## 相关文档

- [设计文档](DESIGN.md)
