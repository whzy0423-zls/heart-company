# Nine-Xing 全局 LangChain RAG 改造实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `nine-xing` 后端、App 主会话、技能会话及芯之力文字/语音入口统一迁移到 LangChain RAG，同时保留 Go 作为鉴权、计费、会话、知识范围和实时协议网关。

**Architecture:** 新增独立 Python/FastAPI LangChain 知识服务，统一承担文档导入、Embedding、混合检索、重排、上下文组装和回答链。Go 后端负责确定用户允许访问的知识库与不可变 Release，并通过内部 HTTP/SSE 调用知识服务；Flutter App 不直接依赖或调用 LangChain。

**Tech Stack:** Python 3.12、FastAPI、LangChain、PostgreSQL 16、pgvector、Go 1.22、Flutter/Dart、Docker Compose、pytest、Go testing。

---

## 1. 当前系统基线

### 1.1 当前知识链路

```text
Flutter App
  -> Go HTTP/SSE/WebSocket
  -> appknowledge.Coordinator
  -> rag_documents + theory release + 九型类型库
  -> Go 关键词检索 / 部分 pgvector 检索
  -> Go LLM Generator
```

当前不是 LangChain，而是 Go 自研 RAG。已有能力包括：

- 公共知识、正式理论、当前九型类型三层知识；
- 卡片、用户、技能版本和 Release 隔离；
- 普通文字、流式文字、上传语音和实时 WebSocket；
- 对话摘要、偏好、记忆、来源和知识命中 trace；
- PostgreSQL/pgvector 可选向量列和索引。

### 1.2 当前基线阻塞

隔离 worktree：

```text
/Users/wohenzaiyi/Desktop/nine-xing/.worktrees/langchain-rag-migration
```

基线命令：

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/.worktrees/langchain-rag-migration/nx-backend/apps/server
go test ./internal/appknowledge ./internal/rag ./internal/ragstore ./internal/theorystore ./internal/skillchat ./internal/server
```

当前结果：

```text
internal/db/db.go:139:12: undefined: seedAppPlanManagementMenu
```

原因：当前提交引用了 `seedAppPlanManagementMenu`，实现仅存在主工作目录的未提交文件中。正式开发前需先把该既有修复以独立提交同步到 worktree，或让基线分支包含该函数。

---

## 2. 目标架构与责任边界

```text
Flutter App
  | HTTP / SSE / WebSocket
  v
Go Business Gateway
  |- 用户鉴权、会员、配额、计费
  |- 卡片、主型、用户记忆、偏好
  |- 会话与消息持久化
  |- 知识库与 Release 授权范围
  |- ASR、TTS、SSE、WebSocket
  v
LangChain Knowledge Service
  |- Loader / OCR / Splitter
  |- Embedding / pgvector
  |- PostgreSQL lexical retrieval
  |- Hybrid fusion / Reranker
  |- Context builder / LCEL chains
  |- Citations / retrieval trace
  v
PostgreSQL + pgvector
```

必须保持的边界：

1. Flutter 不保存 LangChain 服务地址或密钥。
2. LangChain 不接收 App 登录 Token，不决定用户权限。
3. Go 先解析用户、卡片、技能版本和允许访问的 Release ID。
4. LangChain 只能在 Go 传入的 Release 范围内检索。
5. ASR/TTS、SSE/WebSocket 协议和消息落库仍由 Go 管理。
6. LangChain 故障时必须可回退旧 Go RAG。

---

## 3. 文件结构

### 3.1 新增 Python 知识服务

```text
services/knowledge-service/
├── app/
│   ├── main.py
│   ├── config.py
│   ├── dependencies.py
│   ├── api/{health,retrieval,generation,ingestion,evaluation}.py
│   ├── domain/{documents,queries,citations,traces,safety}.py
│   ├── ingestion/{catalog,deduplication,loaders,splitters,metadata,pipeline}.py
│   ├── embeddings/{client,batching,indexing}.py
│   ├── retrieval/{lexical,vector,hybrid,reranker,filters,context_builder}.py
│   ├── chains/{app_chat,xinzhili,skill_chat,compatibility,daily_quiz}.py
│   ├── repositories/{documents,releases,traces,ingestion_jobs}.py
│   └── observability/{logging,metrics,tracing}.py
├── scripts/{catalog_books,ingest_books,reindex,evaluate}.py
├── tests/
├── pyproject.toml
├── Dockerfile
└── README.md
```

### 3.2 新增 Go 内部客户端

```text
nx-backend/apps/server/internal/knowledgeclient/
├── client.go
├── models.go
├── retrieve.go
├── generate.go
├── stream.go
├── health.go
├── errors.go
└── client_test.go
```

### 3.3 主要修改文件

```text
nx-backend/apps/server/internal/config/env.go
nx-backend/apps/server/internal/server/server.go
nx-backend/apps/server/internal/appknowledge/coordinator.go
nx-backend/apps/server/internal/server/app_chat.go
nx-backend/apps/server/internal/server/app_chat_voice.go
nx-backend/apps/server/internal/server/app_xinzhili_voice.go
nx-backend/apps/server/internal/server/app_xinzhili_realtime_deps.go
nx-backend/apps/server/internal/xinzhili/session.go
nx-backend/apps/server/internal/skillchat/runtime.go
nx-backend/apps/server/internal/server/app_skill_library.go
nx-backend/apps/server/internal/server/app_skill_voice.go
nx-backend/apps/server/internal/server/app_compatibility.go
nx-backend/apps/server/internal/server/daily_quiz_generator.go
nx-backend/apps/server/internal/db/schema.sql
docker-compose.yml
.env.example
```

Flutter 后续修改：

```text
/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/chat/models/chat_message.dart
/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/chat/chat_repository.dart
/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/skill_library/models/skill_api_models.dart
/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/skill_library/skill_chat_repository.dart
/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/xinzhili/xinzhili_repository.dart
/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/xinzhili/protocol/xinzhili_protocol.dart
/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/xinzhili/services/xinzhili_realtime_session.dart
```

---

## 4. 内部接口契约

### 4.1 检索接口

```http
POST /internal/v1/retrieve
Authorization: Bearer SERVICE_TOKEN
```

请求必须包含：

```json
{
  "requestId": "REQUEST_ID",
  "query": "用户问题",
  "scene": "app_chat",
  "scope": {
    "public": true,
    "theoryReleaseIds": [101],
    "enneagramReleaseIds": [103],
    "skillReleaseId": null
  },
  "profile": {"mainType": 3, "wingType": 2},
  "retrieval": {
    "topK": 8,
    "vectorK": 20,
    "lexicalK": 20,
    "rerankK": 8,
    "maxContextRunes": 8000
  }
}
```

响应必须返回文档、分数、来源定位和诊断 trace，且禁止返回请求范围外的 Release。

### 4.2 回答接口

```http
POST /internal/v1/answer
POST /internal/v1/answer/stream
```

流式服务间事件：

```text
retrieval_started
retrieval_done
token
citations
done
error
```

### 4.3 导入接口

```http
POST /internal/v1/ingestion/jobs
GET  /internal/v1/ingestion/jobs/{jobId}
POST /internal/v1/ingestion/jobs/{jobId}/cancel
POST /internal/v1/indexes/{releaseId}/rebuild
```

---

## Chunk 1: 基线、契约和服务骨架

### Task 1: 修复并锁定可复现基线

**Files:**
- Modify/Create: `nx-backend/apps/server/internal/db/app_plan_menu.go`
- Test: `nx-backend/apps/server/internal/db/menu_test.go`

- [ ] 写或同步 `seedAppPlanManagementMenu` 的回归测试。
- [ ] 运行测试，确认因缺少实现失败。
- [ ] 同步最小实现，不引入主目录其他未提交改动。
- [ ] 运行数据库及目标后端测试。
- [ ] 提交：`fix: restore app plan menu seed baseline`。

### Task 2: 建立 LangChain 服务契约测试

**Files:**
- Create: `services/knowledge-service/tests/test_health.py`
- Create: `services/knowledge-service/tests/test_retrieval_contract.py`
- Create: `services/knowledge-service/tests/test_scope_filtering.py`

- [ ] 编写 `/health/live` 和 `/health/ready` 失败测试。
- [ ] 编写检索请求/响应 schema 失败测试。
- [ ] 编写跨 Release 返回必须被拒绝的失败测试。
- [ ] 运行 `pytest` 并确认因服务未实现而失败。

### Task 3: 创建 FastAPI/LangChain 最小服务

**Files:**
- Create: `services/knowledge-service/pyproject.toml`
- Create: `services/knowledge-service/app/main.py`
- Create: `services/knowledge-service/app/config.py`
- Create: `services/knowledge-service/app/domain/queries.py`
- Create: `services/knowledge-service/app/domain/documents.py`
- Create: `services/knowledge-service/app/api/health.py`
- Create: `services/knowledge-service/app/api/retrieval.py`

- [ ] 实现健康检查。
- [ ] 实现 Pydantic 契约。
- [ ] 实现内部 Bearer Token 校验。
- [ ] 实现只返回请求 Release 范围的空/fixture Retriever。
- [ ] 运行 `pytest`，确认契约测试通过。
- [ ] 提交：`feat: scaffold langchain knowledge service`。

### Task 4: 增加 Go knowledgeclient

**Files:**
- Create: `nx-backend/apps/server/internal/knowledgeclient/*.go`
- Test: `nx-backend/apps/server/internal/knowledgeclient/client_test.go`

- [ ] 编写请求序列化、响应解析测试。
- [ ] 编写认证头、超时、非 2xx、超大响应测试。
- [ ] 编写 SSE token/done/error/中断测试。
- [ ] 逐个运行测试，确认先失败。
- [ ] 实现最小客户端。
- [ ] 运行 `go test ./internal/knowledgeclient -count=1`。
- [ ] 提交：`feat: add langchain knowledge service client`。

### Task 5: 加入配置和 Docker

**Files:**
- Modify: `.env.example`
- Modify: `docker-compose.yml`
- Modify: `nx-backend/apps/server/internal/config/env.go`
- Test: `nx-backend/apps/server/internal/config/env_test.go`
- Create: `services/knowledge-service/Dockerfile`

- [ ] 先写配置默认值和非法 URL 测试。
- [ ] 增加 `KNOWLEDGE_BACKEND=local|shadow|langchain|fallback`。
- [ ] 增加服务 URL、Token、连接/检索/生成超时。
- [ ] Docker 中仅开放内部网络，不映射公网端口。
- [ ] 运行配置测试和 `docker compose config`。
- [ ] 提交：`feat: deploy internal langchain knowledge service`。

---

## Chunk 2: 文档导入、Embedding 和检索

### Task 6: 建立书目清单与去重

**Input:**

```text
/Users/wohenzaiyi/Downloads/9.10心理书籍/
```

**Files:**
- Create: `services/knowledge-service/app/ingestion/catalog.py`
- Create: `services/knowledge-service/app/ingestion/deduplication.py`
- Create: `services/knowledge-service/scripts/catalog_books.py`
- Test: `services/knowledge-service/tests/ingestion/test_catalog.py`

- [ ] 测试 SHA-256 完全重复识别。
- [ ] 测试同书 EPUB/PDF 版本归并。
- [ ] 测试空文件、EXE、损坏文件和未完成下载排除。
- [ ] 输出稳定、可重复生成的 JSONL catalog。
- [ ] 不删除原始文件，只记录 canonical/duplicate/rejected。

### Task 7: 实现文档 Loader 和 OCR 路由

**Files:**
- Create: `services/knowledge-service/app/ingestion/loaders.py`
- Create: `services/knowledge-service/app/ingestion/pdf_loader.py`
- Create: `services/knowledge-service/app/ingestion/epub_loader.py`
- Create: `services/knowledge-service/app/ingestion/office_loader.py`
- Create: `services/knowledge-service/app/ingestion/ocr_loader.py`

- [ ] EPUB 按 OPF spine/XHTML 抽取。
- [ ] PDF 优先文本层，低文本密度转 OCR。
- [ ] DOC/DOCX 使用受控转换。
- [ ] MOBI/AZW3 仅在转换器存在时处理。
- [ ] 保存页码/章节 locator 和抽取工具版本。
- [ ] 任何书内文字只作为数据，不作为程序指令。

### Task 8: 实现章节和语义切分

**Files:**
- Create: `services/knowledge-service/app/ingestion/chapter_splitter.py`
- Create: `services/knowledge-service/app/ingestion/semantic_splitter.py`
- Test: `services/knowledge-service/tests/ingestion/test_splitters.py`

- [ ] 先按章、节、段落切分。
- [ ] 再生成约 700～1200 中文字的 chunk。
- [ ] 重叠约 100～150 字。
- [ ] 定义、案例、练习和警告独立成块。
- [ ] 保存原始页码和内容 hash。

### Task 9: 实现 Embedding 和 pgvector 索引

**Files:**
- Create: `services/knowledge-service/app/embeddings/client.py`
- Create: `services/knowledge-service/app/embeddings/batching.py`
- Create: `services/knowledge-service/app/embeddings/indexing.py`
- Test: `services/knowledge-service/tests/embeddings/`

- [ ] 校验模型实际维度与数据库 `vector(N)` 一致。
- [ ] 测试批处理、限流、重试、部分失败和幂等。
- [ ] 通过 `content_hash + model + index_version` 避免重复计算。
- [ ] 首批只处理 10～20 本核心书。

### Task 10: 实现混合检索与重排

**Files:**
- Create: `services/knowledge-service/app/retrieval/lexical.py`
- Create: `services/knowledge-service/app/retrieval/vector.py`
- Create: `services/knowledge-service/app/retrieval/hybrid.py`
- Create: `services/knowledge-service/app/retrieval/reranker.py`
- Create: `services/knowledge-service/app/retrieval/filters.py`
- Test: `services/knowledge-service/tests/retrieval/`

- [ ] 先按 Release、库、九型类型和安全等级过滤。
- [ ] 分别执行 lexical Top 20 和 vector Top 20。
- [ ] 用 RRF 合并并去重。
- [ ] 可选 reranker 取 Top 8。
- [ ] 限制上下文总长度。
- [ ] 测试跨 Release 和跨九型类型命中数量必须为零。

---

## Chunk 3: 全局在线入口迁移

### Task 11: 接入 shadow/fallback 路由

**Files:**
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/appknowledge/coordinator.go`
- Test: `nx-backend/apps/server/internal/appknowledge/coordinator_test.go`

- [ ] `local` 保持原逻辑。
- [ ] `shadow` 返回旧结果并异步记录 LangChain 差异。
- [ ] `langchain` 只用 LangChain。
- [ ] `fallback` 在超时/5xx 时回退旧 RAG。
- [ ] 4xx 权限/契约错误不得静默回退。

### Task 12: 迁移 App 主会话

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_voice.go`
- Test: `app_chat_layered_knowledge_test.go`
- Test: `app_chat_stream_test.go`
- Test: `app_chat_voice_test.go`

- [ ] 替换 `rag.NewService().Ask/AskStream`。
- [ ] 保留原有 App HTTP/SSE 契约。
- [ ] 传递摘要、历史、卡片、记忆、偏好、tier 和 Release scope。
- [ ] 保存 citations 和 retrieval trace。
- [ ] 测试取消、超时、中断和回退。

### Task 13: 迁移技能会话

**Files:**
- Modify: `nx-backend/apps/server/internal/skillchat/runtime.go`
- Modify: `nx-backend/apps/server/internal/server/app_skill_library.go`
- Modify: `nx-backend/apps/server/internal/server/app_skill_voice.go`

- [ ] 技能版本固定绑定单一 theory release。
- [ ] 不允许自动访问其他 Release。
- [ ] 保持技能指令、版本、消息和 trace 不变。

### Task 14: 迁移芯之力 SSE 与 WebSocket

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_voice.go`
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_realtime_deps.go`
- Modify: `nx-backend/apps/server/internal/xinzhili/session.go`

- [ ] Go 完成 ASR 后调用 LangChain。
- [ ] LangChain token 继续由 Go 分句并触发 TTS。
- [ ] 保留 turn key、generation、音频帧、播放确认和重连协议。
- [ ] 检索超时不得阻断无知识回答。
- [ ] WebSocket P95 检索目标小于 800ms。

### Task 15: 迁移其他知识消费者

- [ ] 合盘/关系分析明确固定知识范围。
- [ ] 每日题目生成改为受限检索。
- [ ] 小程序继续与 App 公共知识隔离。
- [ ] 与知识无关的支付和静态配置保持原逻辑。

---

## Chunk 4: Flutter 契约与展示

### Task 16: 增加引用模型

- [ ] 新增 `KnowledgeCitation`。
- [ ] `ChatAnswer` 和技能回答增加 citations、traceId、retrievalMethod。
- [ ] 对旧服务缺少新字段保持兼容。
- [ ] 聊天详情支持展开来源，但不展示内部分数和敏感诊断。

### Task 17: 扩展芯之力状态

- [ ] 支持 `retrieving_knowledge`。
- [ ] 支持 `reranking_knowledge`。
- [ ] 支持 `building_context`。
- [ ] 支持 `generating_answer`。
- [ ] 支持 `synthesizing_voice`。
- [ ] 未知状态必须安全降级，不中断会话。

---

## Chunk 5: 评测、灰度和旧链路下线

### Task 18: 建立 500 条离线评测集

```text
九型人格 100
焦虑情绪 100
亲密关系 100
家庭成长 100
习惯行动 100
```

指标：

```text
Recall@5 / Recall@10
MRR / nDCG
正确书籍和章节命中率
跨 Release / 跨型号污染率
引用正确率
回答忠实度
P50 / P95 延迟
```

### Task 19: 灰度发布

- [ ] `local` 建立旧系统基线。
- [ ] `shadow` 运行并比较至少一轮评测集。
- [ ] 主会话按 5% → 20% → 50% → 100% 灰度。
- [ ] 技能会话单独灰度。
- [ ] 芯之力 SSE 再迁移。
- [ ] 芯之力 WebSocket 最后迁移。
- [ ] 每个阶段均验证回滚。

### Task 20: 下线旧 RAG

只有满足以下条件才执行：

- LangChain 全量稳定运行；
- 跨 Release 和跨类型污染率为零；
- 回滚演练通过；
- 新链路质量不低于旧链路；
- 线上错误率和延迟达标。

随后才删除或归档：

```text
internal/rag/rag.go 中的本地搜索职责
internal/ragstore/vector.go 的在线查询职责
internal/server/rag_vector.go
internal/theorystore/search.go 的在线搜索职责
旧 reindex API 和旧缓存
```

---

## 5. 配置清单

```env
KNOWLEDGE_BACKEND=shadow
LANGCHAIN_SERVICE_URL=http://knowledge-service:8081
LANGCHAIN_SERVICE_TOKEN=TOKEN
LANGCHAIN_CONNECT_TIMEOUT_MS=500
LANGCHAIN_RETRIEVE_TIMEOUT_MS=1200
LANGCHAIN_GENERATE_TIMEOUT_SECONDS=90
LANGCHAIN_STREAM_IDLE_TIMEOUT_SECONDS=30

EMBEDDING_PROVIDER=openai-compatible
EMBEDDING_API_BASE=https://EMBEDDING_HOST
EMBEDDING_API_KEY=TOKEN
EMBEDDING_MODEL=EMBEDDING_MODEL
EMBEDDING_DIMENSION=1536

RERANK_PROVIDER=
RERANK_API_BASE=
RERANK_API_KEY=
RERANK_MODEL=

RAG_VECTOR_K=20
RAG_LEXICAL_K=20
RAG_RERANK_K=8
RAG_MAX_CONTEXT_RUNES=8000
RAG_MIN_SCORE=0.2
```

真实密钥只能进入本地 `.env`、部署环境或秘密管理器，不进入 Git。

---

## 6. 验收标准

- [ ] 所有知识型 AI 入口均可使用 LangChain。
- [ ] Flutter 不直连 LangChain。
- [ ] Go 仍掌握用户权限、知识范围和 Release ID。
- [ ] 主会话、技能、芯之力共享统一检索协议。
- [ ] 文字与语音使用同一知识结果。
- [ ] 引用可以定位书籍、章节和页码。
- [ ] 跨用户、跨技能、跨 Release、跨九型类型召回为零。
- [ ] LangChain 故障时按配置回退。
- [ ] 每次检索都有 trace，但不长期保存未脱敏敏感文本。
- [ ] Embedding 模型和索引版本可以回滚。
- [ ] 正确来源 Recall@5 ≥ 85%。
- [ ] 引用定位正确率 ≥ 95%。
- [ ] 普通文字检索 P95 ≤ 1.2 秒。
- [ ] 芯之力实时检索 P95 ≤ 800 毫秒。

---

## 7. 推荐第一批实施范围

第一批只完成：

1. 修复可编译基线；
2. FastAPI/LangChain 服务骨架；
3. Go `knowledgeclient`；
4. Docker 与配置；
5. 10～20 本核心心理学书籍的 catalog、抽取和索引；
6. shadow 模式；
7. 主会话灰度。

技能会话和芯之力实时 WebSocket 在主会话检索质量、故障回退和延迟达到门槛后再迁移。
