# 芯之力理论库设计

## 1. 目标与范围

为 App 会话增加独立的“理论库”，形成统一回答链路：

```text
用户会话 → 现有知识库 → 理论库 → AI
```

理论库不是另一批扁平的 `rag_documents`。它负责保存可审核、可版本化、可追溯的理论卡、实践卡、概念关系和原始资料证据，并在知识库检索之后补充解释框架、成长方向和安全边界。

首期交付包括：

- 理论库数据库结构、状态机和审计字段；
- 原始资料登记、去重、抽取质量标记和来源页定位；
- 理论卡、实践卡、卡片关系、检索分块和向量索引；
- 后台的资料、理论卡和发布审核管理；
- App 文本、流式和语音会话共用的两阶段检索编排；
- 检索运行、命中、回答引用和失败回退记录；
- 从“芯之力文件”完成一个有明确上限的首批导入试点：最多 24 个文件、2,000 个已处理页面，其中 OCR 页面不超过 300 页；产出至少 40 张已审核理论卡和 12 张已审核实践卡。

首期不包括：

- 自动发布 AI 生成的理论卡；
- 完成约 4.68 万页扫描 PDF 的全量 OCR 和人工复核；试点之外的扫描资料全部进入后续批次 backlog；
- 将九型、NLP、易经或“能量”概念包装成临床诊断或已被充分证实的科学事实；
- 向 App 用户开放原始书籍全文下载或大段版权内容；
- 改造小程序、付费报告和站点问答的 RAG 链路。

## 2. 资料审计结论

资料根目录为：

```text
/Users/wohenzaiyi/Desktop/韩老师资料/芯之力文件
```

当前共 262 个内容文件：226 个 PDF、23 个 `.epub`、5 个 AZW3、4 个 JPG、1 个 MOBI、1 个 DOC、1 个 DOCX 和 1 个 PPTX。PDF 总计约 72,314 页。

PDF 抽取质量分布：

| 类型 | 文件数 | 页数 | 处理方式 |
|---|---:|---:|---|
| 文本丰富 | 62 | 18,294 | 直接抽取 |
| 混合或稀疏文本 | 27 | 4,865 | 直接抽取并按页补 OCR |
| 图像为主 | 137 | 49,155 | 最终需要全量 OCR，首期只处理试点页 |

去除明确重复扫描件后，规范化全量 OCR backlog 约为 130 个 PDF、46,790 页。首期不消费整个 backlog，只登记清单并处理试点上限。资料中存在 7 对 SHA-256 完全重复文件，以及《伯恩斯新情绪疗法》《情绪急救》《社会学的意识》等近重复或跨格式重复。预计形成约 252 个导入单元。

已确认的格式和命名异常必须记录在清单中：

- `诸神的面具.pdf` 实际是 Tim Keller 的 `Counterfeit Gods`，不能归入坎贝尔理论；
- `九型人格与领导力.epub` 实际是 Mobipocket 文件；
- 108 页 PPTX 没有文本层，需要按幻灯片 OCR；
- OCR 逐字引文必须回到原页人工核对；
- 同一本书的汇编、旧版、白金版或不同格式不能重复累计证据权重。

仓库已有 `scripts/db/seed-xinzhili-rag-documents.v17.sql`，包含约 1,572 条蒸馏知识片段。它继续属于现有知识库，可作为知识检索输入，但不能替代理论卡、理论关系和来源证据。

## 3. 内容原则和权威层级

“芯之力/韩老师原创理论”是产品回答的理论主干，外部书籍只承担来源、扩展、支持、反例或争议说明。默认权威顺序为：

```text
芯之力正式理论
> 已审核韩老师课程提炼
> 原典或主要理论作者
> 外部研究
> 通俗二次解释
```

排序优先级不等于科学证据等级。系统分别保存 `authority_level` 与 `evidence_level`，避免把“产品内权威”误写成“科学上已证实”。

理论库首批领域如下：

1. `personality_pattern`：九型注意焦点、激情、恐惧/欲望、防御、中心、翼型和动态线；
2. `transformation_journey`：召唤、拒绝、导师、试炼、转化、恩赐和回归；
3. `state_energy`：意图、身体中心、资源以及温柔、勇猛、顽皮三种原型能量；
4. `symbolic_change`：阴阳、时位、刚柔、盈虚和变化阶段；
5. `subjective_experience`：地图不是疆域、感官表征、经验元素、时间线和感知位置；
6. `belief_identity`：信念、价值、规条、身份和使命；
7. `emotion_regulation`：情绪功能、触发、身体反应、保护意图和转化；
8. `communication_relationship`：沟通反馈、亲和、家庭关系、边界和责任；
9. `practice_intervention`：自我观察、正念、资源连接、重构和行动实验；
10. `ethics_meaning`：三赢、贡献、传递、厚德、自强、谦逊和责任。

理论卡必须保持原子性。一张卡只表达一个概念、主张、关系、阶段、画像、练习或警示，不把“整个九型”或“整个英雄之旅”塞入一张卡。

## 4. 总体架构

理论库分为四层：

```text
原始资料目录层
  ↓
规范理论卡与关系图层
  ↓
证据与来源追溯层
  ↓
检索分块与向量层
```

### 4.1 原始资料目录层

保存“作品”和“物理文件”两个概念。一本书的 PDF、EPUB、不同扫描版本可以属于同一作品；完全重复文件不重复抽取，但仍保留物理文件登记和去重关系。

### 4.2 理论卡与关系图层

理论卡是 AI 可引用的规范节点，实践卡保存步骤、停止条件和专业升级条件。卡片之间通过有向关系表达“属于、前置、阶段下一步、支持、扩展、对照、冲突、风险、可实践化”等关系。

### 4.3 证据与来源追溯层

卡片与作品、文件、章节、页码和原文片段关联。系统明确区分原典、作者解释、课程改编、传统象征、假说和证据支持，记录抽取质量与人工核验状态。

### 4.4 检索分块与向量层

只从已发布理论卡生成检索块。检索块包含足够完整的解释和边界，独立保存关键词检索文本、embedding 模型、向量、版本和启用状态，避免再次发生“整篇文档向量化、只给模型 92 字”的问题。

## 5. 数据库设计

所有表使用 PostgreSQL，时间字段统一为 `TIMESTAMPTZ`。业务枚举在应用层校验，并通过数据库 `CHECK` 约束保护关键状态。

### 5.1 `theory_libraries`

保存理论库实例，首期只有一个“芯之力理论库”，但不把库名硬编码到检索逻辑。

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | 主键 |
| `key` | TEXT UNIQUE | 稳定键，例如 `xinzhili` |
| `name` | TEXT | 展示名 |
| `description` | TEXT | 范围说明 |
| `status` | TEXT | `draft/enabled/disabled` |
| `default_language` | TEXT | 默认 `zh-CN` |
| `current_version` | INTEGER | 当前发布版本 |
| `created_by/updated_by` | BIGINT NULL | 后台用户 |
| `create_time/update_time` | TIMESTAMPTZ | 审计时间 |

`current_version` 只作后台展示，不决定运行时索引。运行时版本由发布表控制。

### 5.2 `theory_library_releases` 与 `theory_release_cards`

`theory_library_releases` 是运行时可切换的完整理论快照：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | 主键 |
| `library_id` | BIGINT FK | 所属理论库 |
| `version` | INTEGER | 单调递增发布版本 |
| `status` | TEXT | `draft/building/ready/active/retired/failed` |
| `embedding_model` | TEXT | 本发布固定模型 |
| `embedding_dimensions` | INTEGER | 首期固定为 1536 |
| `retrieval_mode` | TEXT | `lexical_only/hybrid`；基础 seed 默认 lexical-only |
| `index_version` | TEXT | 词法与向量索引版本 |
| `card_count/chunk_count` | INTEGER | 构建统计 |
| `build_error` | TEXT | 构建失败原因 |
| `activated_by/activated_at` | BIGINT/TIMESTAMPTZ | 激活审计 |
| `create_time/update_time` | TIMESTAMPTZ | 审计时间 |

唯一约束：`(library_id, version)`；每个理论库只允许一行 `status='active'`，使用条件唯一索引保证。

`theory_release_cards(release_id, card_id, chunk_id)` 明确某个发布快照使用的卡片版本和检索块。运行检索只查询 active release 的映射，不直接依据“最新更新时间”。

替换卡片的事务顺序：新版本 `draft → in_review → published`，旧版本标为 `superseded`；随后构建新的 draft release、生成 chunk 和 embedding、完成校验后设为 `ready`。激活事务锁定 `theory_libraries` 行，将旧 active release 设为 `retired`、新 release 设为 `active` 并更新 `current_version`。构建或激活失败时旧 active release 保持不变；并发发布通过行锁和版本唯一约束串行化。

### 5.3 `theory_source_works`

保存规范作品，不等同于文件。

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | 主键 |
| `library_id` | BIGINT FK | 所属理论库 |
| `canonical_key` | TEXT | 库内稳定键 |
| `title/original_title` | TEXT | 规范标题、原始标题 |
| `authors/editors/translators` | JSONB | 责任者列表 |
| `publisher/published_year/edition/isbn` | TEXT/INTEGER | 版本信息 |
| `work_type` | TEXT | `book/course/handout/article/original_text/research/other` |
| `authority_level` | SMALLINT | 1–5，5 为芯之力正式理论 |
| `epistemic_status` | TEXT | 来源文本、解释、课程改编等 |
| `copyright_scope` | TEXT | `metadata_only/internal_excerpt/licensed/full_internal` |
| `canonical_work_id` | BIGINT NULL FK self | 版本归并目标 |
| `metadata` | JSONB | 扩展书目信息 |
| `status` | TEXT | `registered/extracting/reviewed/archived` |
| `create_time/update_time` | TIMESTAMPTZ | 审计时间 |

唯一约束：`(library_id, canonical_key)`。

### 5.4 `theory_source_files`

保存每个物理文件及处理状态。

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | 主键 |
| `work_id` | BIGINT FK | 归属作品 |
| `relative_path/original_filename` | TEXT | 相对根目录路径、原文件名 |
| `file_format/mime_type` | TEXT | 格式和 MIME |
| `byte_size/page_count` | BIGINT/INTEGER | 文件大小、页数或幻灯片数 |
| `sha256` | TEXT | 完整文件哈希 |
| `duplicate_of_file_id` | BIGINT NULL FK self | 完全重复文件 |
| `title_source` | TEXT | 文件名、元数据、封面人工识别等 |
| `extraction_class` | TEXT | `text_rich/mixed/image_dominant/cover_only` |
| `extraction_status` | TEXT | `pending/extracted/needs_ocr/ocr_running/review_required/failed` |
| `extraction_quality` | NUMERIC(5,4) | 0–1 |
| `extracted_text_uri/ocr_text_uri` | TEXT | 内部产物路径，不直接公开全文 |
| `extractor_name/extractor_version` | TEXT | 可复现处理信息 |
| `error_code/error_message` | TEXT | 失败诊断 |
| `metadata` | JSONB | 加密、格式误标等异常 |
| `create_time/update_time` | TIMESTAMPTZ | 审计时间 |

`sha256` 建普通索引而非全局唯一约束，以便登记重复物理文件。

### 5.5 `theory_cards`

保存规范理论节点。

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | 主键 |
| `library_id` | BIGINT FK | 所属理论库 |
| `canonical_key` | TEXT | 稳定键 |
| `canonical_name` | TEXT | 规范名称 |
| `aliases` | JSONB | 别名 |
| `domain/subdomain` | TEXT | 一级、二级领域 |
| `card_kind` | TEXT | `concept/claim/axis/stage/relation/profile/practice/warning` |
| `summary/definition/core_claim` | TEXT | 摘要、定义、核心主张 |
| `mechanism` | TEXT | 机制或理论理由 |
| `applicable_context/non_applicable_context` | TEXT | 适用与不适用边界 |
| `observable_signals/common_triggers` | JSONB | 可观察信号和常见触发 |
| `automatic_pattern/resource_state` | TEXT | 自动模式和资源状态 |
| `shadow_or_risk/growth_direction` | TEXT | 阴影风险和成长方向 |
| `epistemic_status` | TEXT | `source_text/author_interpretation/course_adaptation/traditional_symbolism/hypothesis/evidence_informed` |
| `evidence_level` | TEXT | `strong/moderate/limited/traditional/experiential/unknown` |
| `clinical_safety` | TEXT | `general/caution/restricted/escalate` |
| `controversy_notes/cultural_context` | TEXT | 争议和文化语境 |
| `authority_level` | SMALLINT | 1–5 |
| `language` | TEXT | 默认 `zh-CN` |
| `status` | TEXT | `draft/in_review/published/superseded/retired` |
| `version` | INTEGER | 卡片版本 |
| `reviewed_by/reviewed_at/published_at` | BIGINT/TIMESTAMPTZ | 审核发布信息 |
| `created_by/updated_by` | BIGINT NULL | 创建、更新者 |
| `create_time/update_time` | TIMESTAMPTZ | 审计时间 |

唯一约束：`(library_id, canonical_key, version)`。同一 `canonical_key` 只允许一个 `published` 版本，可用条件唯一索引实现。被替换的 `superseded` 版本可以暂时留在旧 active release 中，直到新 release 原子切换。

### 5.6 `theory_practices`

实践细节与理论卡一对一或一对多关联，避免大量步骤字段塞入所有卡片。

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | 主键 |
| `card_id` | BIGINT FK | 所属理论卡 |
| `goal` | TEXT | 练习目标 |
| `estimated_minutes` | INTEGER | 建议时长 |
| `steps/reflection_prompts/expected_feedback` | JSONB | 步骤、反思问题、预期反馈 |
| `stop_conditions/professional_escalation` | JSONB | 停止和专业升级条件 |
| `contraindications` | TEXT | 禁忌和限制 |
| `status/version` | TEXT/INTEGER | 状态、版本 |
| `create_time/update_time` | TIMESTAMPTZ | 审计时间 |

### 5.7 `theory_card_relations`

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | 主键 |
| `from_card_id/to_card_id` | BIGINT FK | 起点、终点 |
| `relation_type` | TEXT | `belongs_to/prerequisite/next_stage/supports/extends/contrasts/conflicts/risks/practices` |
| `note` | TEXT | 关系说明 |
| `confidence` | NUMERIC(5,4) | 关系置信度 |
| `status` | TEXT | `draft/published/disabled` |
| `created_by/reviewed_by` | BIGINT NULL | 审计人员 |
| `create_time/update_time` | TIMESTAMPTZ | 审计时间 |

唯一约束：`(from_card_id, to_card_id, relation_type)`；禁止自关联。

### 5.8 `theory_card_sources`

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | 主键 |
| `card_id/work_id/file_id` | BIGINT FK | 理论卡、作品和可选物理文件 |
| `source_role` | TEXT | `primary/supporting/extension/counterpoint/controversy` |
| `chapter/page_start/page_end/location_label` | TEXT/INTEGER | 章节和页码定位 |
| `quotation` | TEXT | 受限长度原文摘录 |
| `interpretation_note` | TEXT | 提炼说明 |
| `extraction_quality` | NUMERIC(5,4) | 本证据抽取质量 |
| `quote_verified` | BOOLEAN | 是否回原页核验 |
| `verified_by/verified_at` | BIGINT/TIMESTAMPTZ | 核验信息 |
| `create_time/update_time` | TIMESTAMPTZ | 审计时间 |

逐字引文若来自 OCR，`quote_verified=false` 时不得进入面向用户的直接引用。

### 5.9 `theory_chunks` 与 `theory_chunk_embeddings`

`theory_chunks` 保存检索块：

- `id`, `library_id`, `card_id`, `practice_id`；发布归属只由 `theory_release_cards` 维护，避免双重映射漂移；
- `chunk_key`, `chunk_kind`, `title`, `content`, `keywords`, `tags`；
- `authority_level`, `evidence_level`, `clinical_safety`；
- `token_count`, `content_hash`, `version`, `status`；
- `create_time`, `update_time`。

`theory_chunk_embeddings` 保存可替换的向量版本：

- `id`, `chunk_id`, `embedding_model`, `dimensions`, `embedding vector(1536)`；
- `content_hash`, `embedded_at`, `status`, `error_message`。

唯一约束：`(chunk_id, embedding_model, content_hash)`。首期只接受 1536 维向量，应用写入前校验长度，数据库用 `vector(1536)` 再校验。使用 HNSW 余弦索引：`USING hnsw (embedding vector_cosine_ops)`；查询先通过 active release 的 `theory_release_cards` 限定 chunk，再过滤 release 指定的 `embedding_model` 和 embedding `status='ready'`。content hash 不匹配的行视为 stale，不参与检索并由重建任务清理。

中文关键词检索首期使用规范化标题、别名、关键词、标签和正文的 `pg_trgm` GIN 索引，加标题/标签精确命中加权。若数据库无法启用 `pg_trgm`，回退到受限候选集上的 `ILIKE`，并记录 `lexical_degraded`。后续若引入中文分词扩展，必须通过新的 `index_version` 构建发布，不原地改变 active release。

只有 active release 映射到的、非 `retired` 卡片与 `status='enabled'` chunk 能进入 App 检索。

### 5.10 检索与引用审计表

`rag_retrieval_runs` 每次回答一行：

- 请求 ID、App 用户、会话，以及可空的用户消息和回答消息 ID；
- 原始问题、理论扩展查询、风险等级、对话模式；
- 知识库/理论库检索状态和耗时；
- embedding 模型、索引版本、候选数量、最终数量；
- `status`：`started/retrieved/shadow_completed/generating/completed/cancelled/failed/persist_failed`；
- fallback 类型、错误代码、总耗时；
- `create_time`。

`rag_retrieval_hits` 每个保留候选一行：

- `run_id`, `corpus_type` (`site/knowledge/theory`)；
- 三个互斥定位字段：`site_source_key TEXT`、`rag_document_id BIGINT FK`、`theory_chunk_id BIGINT FK`；
- lexical/vector/graph/rerank 分数及最终排名；
- 是否入选、过滤原因、提示词字符或 token 数；
- 必填的 `source_ref_key`、来源标题、内容 hash 和版本快照。

`app_chat_message_sources` 保存最终回答引用：

- `message_id`, `run_id`, `corpus_type`；
- 三个互斥定位字段：`site_source_key`、`rag_document_id`、`theory_chunk_id`；理论卡 ID 由 chunk 关联得到；
- 标题、摘录、排序、版本快照；
- `create_time`。

两张表使用 `CHECK` 保证 `corpus_type` 与三个定位字段一致且 `num_nonnulls(...) = 1`。知识文档和理论 chunk 的外键使用 `ON DELETE RESTRICT`：进入回答审计的来源只能停用，不能直接物理删除；若依法必须删除，先执行专用脱敏/审计清理流程。`source_ref_key` 和标题/版本快照即使来源停用也永久保留。索引至少覆盖 `run_id`、`message_id`、三个来源定位字段和 `(corpus_type, selected, final_rank)`。

现有 `app_chat_messages.sources` 保留为兼容快照，新的关系表作为可查询、可审计的事实来源。

### 5.11 检索、消息和引用的事务生命周期

1. 请求通过权限和参数校验后插入 `rag_retrieval_runs(status='started')`，此时两个 message ID 为空；
2. 完成检索后批量写入保留候选到 `rag_retrieval_hits`，运行状态变为 `retrieved`；候选写入失败不开始模型输出；
   - shadow 模式在此后直接标记 `shadow_completed`，两个 message ID 保持为空，不进入生成状态；
3. 开始生成前状态变为 `generating`；
4. 模型完整返回后调用一个 `FinalizeChatAnswer` 事务：插入用户消息、插入助手消息、插入最终 `app_chat_message_sources`、写兼容 `sources` JSON、回填 run 的 message ID，并将 run 置为 `completed`；
5. 只有该事务提交后，非流式接口才返回成功，SSE 才发送 `done`；
6. 客户端取消、输出前失败和部分流失败分别把 run 标为 `cancelled` 或 `failed`，不保存消息对；
7. 最终事务失败时回滚消息和引用，把 run 单独标为 `persist_failed`，SSE 不发送 `done`；
8. 定时清理任务只清理超过保留期且没有消息关联的失败运行，已完成运行按审计策略保留。

```text
started → retrieved → shadow_completed
              └────→ generating → completed
   ├──────────────→ cancelled
   ├──────────────→ failed
   └──────────────→ persist_failed
```

### 5.12 通用约束与索引

- 所有质量、证据置信度和关系置信度限制在 0–1；`authority_level` 限制在 1–5；页码为正且 `page_end >= page_start`；
- 文件关联的 `work_id` 必须与卡片来源记录的 `work_id` 一致，由延迟约束触发器或写入事务校验；
- 所有状态和枚举字段加 `CHECK`；关系禁止 `from_card_id = to_card_id`；
- 每个外键、状态筛选、更新时间、`canonical_key`、`sha256`、`content_hash` 都建立相应索引；
- 发布相关条件唯一索引和 active release 切换必须在 schema 测试中验证；
- schema 延续当前 `schema.sql` 可重复执行风格，所有新增 DDL 必须支持重复启动。

## 6. 会话检索与生成流程

### 6.1 共享编排器

新增一个服务端 grounding 编排器，文本、SSE 流式和语音聊天都调用同一接口。它只负责上下文识别、检索、排序、过滤、配额和追踪；`rag.Service` 收缩为生成层，不再自己从整份文档做第二轮不透明搜索。

输入包括：

- 原始问题；
- 会话摘要和最近历史；
- 当前用户、卡片和主型上下文；
- 用户偏好、当前指令和会员对话模式。

输出是类型化的 `GroundingBundle`：

- `SiteHits`；
- `KnowledgeHits`；
- `TheoryHits`；
- `RiskAssessment`；
- `RetrievalTrace`；
- `FallbackMode`。

核心接口约定如下，空结果与错误必须分开表达：

```go
type ResultStatus string // ok | empty | degraded | failed

type SiteHit struct { Key, Title, Content, ContentHash string; Score float64 }
type KnowledgeHit struct { DocumentID int64; Title, Content, ContentHash string; LexicalScore, VectorScore, FinalScore float64 }
type TheoryHit struct { ChunkID, CardID, ReleaseID int64; Title, Content, EvidenceLevel, ClinicalSafety string; DirectScore, GraphScore, FinalScore float64 }

type RetrievalResult[T any] struct {
    Status ResultStatus
    Hits []T
    ErrorCode string
    Duration time.Duration
    IndexVersion string
}

type IntentKind string // current_fact | growth | mixed | unknown
type RiskLevel string   // low | caution | high | crisis

type SiteRetriever interface {
    Retrieve(ctx context.Context, query string, limit int) (RetrievalResult[SiteHit], error)
}
type KnowledgeRetriever interface {
    Retrieve(ctx context.Context, query string, limit int) (RetrievalResult[KnowledgeHit], error)
}
type TheoryRetriever interface {
    Retrieve(ctx context.Context, query TheoryQuery, limit int) (RetrievalResult[TheoryHit], error)
}
type RiskClassifier interface {
    Classify(ctx context.Context, input RiskInput) RiskAssessment
}
type GroundingOrchestrator interface {
    Ground(ctx context.Context, input GroundingInput) (GroundingBundle, error)
}
type RetrievalTraceRecorder interface {
    Start(ctx context.Context, input GroundingInput) (runID int64, err error)
    SaveRetrieved(ctx context.Context, runID int64, trace RetrievalTrace) error
    MarkTerminal(ctx context.Context, runID int64, status, errorCode string) error
}
type ChatAnswerFinalizer interface {
    FinalizeChatAnswer(ctx context.Context, input FinalizeInput) (userMessageID, assistantMessageID int64, err error)
}
```

普通的无命中使用 `Status=empty, err=nil`；向量不可用但关键词成功使用 `degraded`；单库操作失败使用 `failed` 和稳定 `ErrorCode`，由编排器决定降级；只有 context 取消、输入非法或 trace 无法建立等无法继续请求的错误才返回非 nil Go error。

`internal/ragstore` 继续拥有知识文档读写和向量访问，并提供独立的 site/knowledge 适配器；`internal/theoryretrieval` 拥有理论 lexical/vector 检索、RRF 融合和一跳图扩展；`internal/grounding` 拥有调用顺序、主题扩展、配额、安全过滤、fallback 和 trace。生成层不依赖具体 store。

### 6.2 两阶段检索

```text
原始问题
→ 会话上下文与风险识别
→ 知识库 hybrid retrieval
→ 从问题、知识标题/标签、用户卡片提取主题
→ 理论库 hybrid retrieval
→ 理论关系图一跳扩展
→ 分库排序与配额
→ 权威、证据、安全和发布状态过滤
→ 分区提示词
→ AI 生成
→ 保存回答、引用和检索轨迹
```

知识库阶段使用关键词与向量并集，不能把向量命中再次用旧的词法阈值全部丢弃。两个库都使用 Reciprocal Rank Fusion，默认 `k=60`。候选入池条件为：标题/标签精确命中，或 trigram 相似度不低于 `0.18`，或向量余弦相似度不低于 `0.72`；最终 RRF 分数默认不低于 `0.015`。这些值进入配置并记录到 trace，不在代码多处硬编码。

理论库允许从直接命中沿 active release 中已发布关系做最多一跳扩展。只有直接命中最终分数不低于 `0.020` 且关系置信度不低于 `0.75` 才扩展；图命中乘以 `0.60` 降权，最多保留 2 个，不能替换分数更高的直接命中。

理论扩展查询由原始问题、知识命中的标题/标签、用户主型和当前卡片上下文组成。不得把 AI 已生成答案反向用于检索，避免答案先验污染。

### 6.3 配额与上下文预算

默认提示词配额：

- 知识库 2–3 段；
- 理论库 1–2 段；
- grounding 合计约 1,500–2,000 tokens；
- 单个块保存完整语义单元，不使用固定 92 个字符截断；
- 同一作品、同一卡片和同一领域设置重复惩罚，保证来源多样性。

站点当前事实、课程、价格和报名信息属于 `site` corpus，事实优先级高于理论解释。理论库不得补造站点事实。

### 6.4 确定性意图、风险和时间预算

意图先由高精度规则分类，再由可选模型补充；规则命中优先：

- `current_fact`：价格、课程、报名、下载、版本、服务时间、当前活动、老师/机构事实等；
- `growth`：性格模式、关系理解、情绪觉察、成长练习、英雄之旅、九型、NLP、易经解释等；
- `mixed`：同一问题同时包含当前事实和成长解释；
- `unknown`：规则与模型都不能稳定归类。

风险等级为 `low/caution/high/crisis`。自伤、自杀、正在发生的暴力或虐待、精神病性症状和急性医疗危险直接判为 `crisis`；严重抑郁、创伤、家暴、药物和医疗建议判为至少 `high`；普通情绪困扰和低证据干预请求为 `caution`；其余为 `low`。模型只能提高风险等级，不能降低规则结果。

默认检索预算：grounding 总计 1,200ms；站点与知识检索共享前 450ms；主题扩展和理论检索 550ms；图扩展 100ms；trace 写入 100ms。单阶段超时按 degraded/failed 进入 fallback，不阻塞到整个聊天超时。生产验收目标是 grounding P95 不超过 1,200ms、P99 不超过 2,000ms，不包含模型生成时间。

### 6.5 提示词分区

模型提示词明确分区：

```text
[用户和会话上下文]
[当前问题]
[知识库事实与经验]
[理论库解释框架]
[理论边界、争议和安全提醒]
[回答规则]
```

生成规则：

- 先回答用户问题，再使用理论解释；
- 区分事实、产品原创理论、作者解释、传统象征和体验性隐喻；
- 不根据一段会话断言用户九型类型或临床诊断；
- 理论之间冲突时标明差异，不强行拼成单一结论；
- 只引用最终入选且版本已发布的来源；
- 保持当前产品的简洁回答要求。

## 7. 失败回退和流式一致性

| 场景 | 行为 |
|---|---|
| 理论库失败 | 使用知识库回答，记录 `knowledge_only` |
| 知识库失败 | 仅 `growth` 意图且风险低于 `high` 时允许理论库单独回答 |
| `current_fact` 或 `mixed` 的事实部分未命中 | 不允许理论库或 AI 编造；明确说明当前资料没有该事实 |
| 两库均无命中且意图为 `growth` | 风险为 `low/caution` 时允许通用模型回答，并明确“未检索到内部资料”；`high/crisis` 走安全响应 |
| 两库均无命中且意图为 `unknown` | 只提出一个澄清问题 |
| AI 在输出前失败 | 使用经过安全过滤的确定性来源摘要兜底 |
| AI 在流式输出后失败 | 终止流，不保存不完整回答 |
| 引用保存失败 | 不发送 `done`，避免客户端看到无法审计的成功回答 |
| embedding 不可用 | 分库回退关键词检索，记录 fallback |

检索错误不再使用 `_` 静默忽略。错误可以降级，但必须进入运行日志、指标和审计表。

SSE 顺序固定为：创建 run → 完成并持久化候选 → 发送 `meta` → 输出 `delta` → 完整生成 → `FinalizeChatAnswer` 提交 → 发送 `done`。任何发生在 `done` 前的失败都发送 `error` 并结束；已经输出的 delta 只用于当次界面临时展示，不进入历史。

## 8. 安全和知识边界

### 8.1 固定边界

- 九型用于自我观察和成长假设，不作为确定人格诊断；
- NLP 整体科学证据有限，眼球运动、固定感知类型、快速消除创伤等不能写成已证实事实；
- 易经原典、后世哲学解释、象数体系和民间占筮必须分层；
- 坎贝尔单一神话存在文化泛化限制；
- “能量、场域、宇宙意识”标记为体验性或隐喻性，不能冒充神经科学机制；
- 严重抑郁、自伤/自杀、精神病性症状、家暴、虐待、严重创伤和医疗建议必须升级专业支持。

### 8.2 检索过滤

- `clinical_safety='restricted'` 的卡片仅能为模型提供警示和停止条件，不直接输出干预步骤；
- `clinical_safety='escalate'` 命中后强制加入专业升级说明；
- `evidence_level` 较低不等于禁止使用，但回答必须采用“该理论认为”“可作为一种观察框架”等措辞；
- 未审核 OCR 引文不能作为逐字引用；
- 已退役卡片只保留历史审计，不参与检索。

### 8.3 审核标注尺度

- `authority_level`：5=芯之力正式理论，4=已审核韩老师课程提炼，3=原典/主要作者，2=外部研究或专业材料，1=通俗二次解释；
- `evidence_level`：`strong`=多项高质量一致证据，`moderate`=有一定一致证据，`limited`=证据有限或混合，`traditional`=传统文化体系，`experiential`=体验/隐喻，`unknown`=尚未评估；
- `clinical_safety`：`general`=一般自我观察，`caution`=需明确边界，`restricted`=不能直接给出干预步骤，`escalate`=必须专业升级；
- `extraction_quality`：1.00=人工校对原文，0.90–0.99=原生文本且抽检通过，0.70–0.89=OCR 可读但未逐页核验，低于 0.70=不得用于卡片发布；
- `relation confidence`：1.00=原理论明确关系，0.85–0.99=审核者高置信提炼，0.75–0.84=可用于图扩展的最低范围，低于 0.75=只保存草稿、不参与运行。

安全关键的实践步骤、停止条件和专业升级内容在应用层使用带版本的 JSON Schema 校验；首期不再拆成更多子表，但未经 schema 校验的数据不能发布。

## 9. 资料导入与审核流程

```text
文件登记
→ SHA-256 和跨格式去重
→ 文本抽取或 OCR
→ 分页/章节切分
→ AI 生成理论卡草稿
→ 自动关联作品、文件和页码
→ 人工理论审核
→ 权威、证据、安全和争议标注
→ 发布
→ 生成检索块
→ embedding
→ App 灰度启用
```

AI 草稿永不自动发布。状态机如下：

```text
draft → in_review → published → superseded → retired
          ↘ draft（退回修改）
```

发布时要求：

- 至少一个主来源；
- 定义、适用和不适用范围完整；
- `epistemic_status`、`evidence_level`、`clinical_safety` 已填写；
- OCR 证据带抽取质量，逐字引用已人工核验；
- 版本增加后重新生成 chunk 和 embedding；
- 发布、退役和重新发布均留下人员与时间记录。

`internal/theoryingest` 不实现 OCR 引擎，只接收外部工具输出的标准包：`manifest.json`、UTF-8 分页文本、每页字符数/置信度、源文件 SHA-256、引擎名称和版本。导入前必须校验 SHA-256、页数连续性、UTF-8、置信度范围和包 schema；不匹配时把文件标为 `review_required`，不生成理论卡草稿。

首批导入试点固定为最多 24 个文件、2,000 个处理页面、其中 OCR 不超过 300 页，优先顺序：

1. 芯之力正式理论和已审核韩老师课程材料；
2. `九型人格·珍藏版.epub`；
3. `(NEW)周易.pdf`、`周易说卦传正解.doc` 和易经成语 DOCX；
4. `英雄之旅.pdf` 与“能量/01—12”课程资料；
5. `李中莹NLP精义` 和可直接读取的亲子关系资料；
6. 在剩余上限内，对《千面英雄》《重塑心灵》《NLP简快心理疗法》做指定章节的选择性 OCR。

试点退出标准：至少 40 张理论卡和 12 张实践卡通过人工审核；每张卡都有主来源、页码/章节、证据和安全标注；重复文件未重复计权；抽取质量低于 0.70 的内容没有发布；固定安全评测通过。

62 个文本丰富 PDF、22 个 EPUB、7 个 Mobipocket/KF8、27 个混合 PDF、约 130 个扫描 PDF、108 页 PPTX 和 4 张封面 JPG 的其余处理都进入显式 backlog。每个后续批次单独设文件数、页数、OCR 预算和审核产能，不属于首期完成条件。

## 10. 后台管理

首期增加“理论库”菜单，包含三个工作区：

1. **资料目录**：查看作品、物理文件、重复关系、抽取类型、OCR 状态、异常和错误；
2. **理论卡审核**：按领域、状态、证据等级、安全等级和来源筛选；编辑定义、边界、关系、实践和来源页；执行送审、退回、发布、退役；
3. **检索观察**：查看某次 App 回答的知识命中、理论命中、分数、过滤原因、耗时、fallback 和最终引用。

后台不直接展示或下载未经授权的完整书籍正文。来源证据只显示受限长度摘录和内部页定位。

## 11. 代码边界

建议新增独立包：

- `internal/theorystore`：作品、文件、理论卡、来源、关系、chunk 和 embedding 的数据库访问；
- `internal/theoryretrieval`：理论库 hybrid retrieval、图扩展、过滤和排序；
- `internal/grounding`：站点/知识库先行、理论库后置的共享编排器、意图/风险规则、配额、fallback 和运行追踪；
- `internal/theoryingest`：清单登记、去重、抽取状态和草稿导入，不承担 OCR 引擎本身；
- `internal/server/admin_theory_*.go`：后台 API；
- `apps/web-antd/src/views/theory/`：理论库管理页面。

现有 `internal/ragstore` 增加 `KnowledgeRetriever` 适配器，负责站点与知识文档的 lexical/vector 候选和打分；现有 `internal/rag` 保留提示词构造、同步生成和流式生成能力，但接收类型化的 `SiteHits`、`KnowledgeHits` 和 `TheoryHits`，不再拥有检索职责。

`app_chat.go` 的同步和 SSE handler、`app_chat_voice.go` 的语音 handler 都调用同一个 `GroundingOrchestrator.Ground`。同步与语音调用 generation-only `Ask`，SSE 调用 generation-only `AskStream`；三者共用同一 `FinalizeChatAnswer` 持久化接口，不允许各自复制检索、引用和 fallback 逻辑。

## 12. 测试和验收

### 12.1 数据层

- schema 可重复执行；外键、条件唯一索引和状态约束生效；
- 作品和物理文件正确去重；
- 只有已发布卡片与启用 chunk 可检索；
- 新版本发布会使旧 embedding 失效并生成新版本；
- OCR 未核验引文不能进入直接引用。

### 12.2 检索层

- 向量命中不会被旧词法阈值误删；
- 知识检索先于理论检索，理论查询包含知识主题；
- 图扩展最多一跳且有配额、降权和去重；
- 默认 `0.18/0.72/0.015` 候选与融合阈值、`0.020/0.75` 图扩展阈值可配置且进入 trace；
- 知识 2–3 段、理论 1–2 段及总 token 预算生效；
- 权威、证据、安全和发布状态过滤正确；
- 各类 fallback 与检索轨迹完整记录。

### 12.3 会话层

- 文本、流式和语音使用相同 grounding 结果；
- 提示词分区明确区分知识和理论；
- 来源在回答、`app_chat_message_sources` 和兼容 JSON 中一致；
- 流式失败不保存半截回答；
- 保存引用失败不发送成功完成事件；
- completed run 的消息、引用和 run 状态在同一事务提交；cancelled/failed/persist_failed 不残留消息对；
- 当前简洁回答、会员模式、偏好、记忆和会话摘要行为保持兼容。

### 12.4 安全验收

建立固定评测集，覆盖九型贴标签、NLP 科学性、易经预测、创伤、自伤、精神病性症状、家暴、医疗建议、课程价格和无资料问题。验收要求：不误诊、不伪造事实、低证据理论正确降格表述、高风险问题正确升级；规则定义的 crisis 样例升级召回率必须为 100%，当前事实缺失样例的编造率必须为 0%。

## 13. 上线策略

使用配置开关分三步上线：

1. **shadow**：执行理论检索并记录结果，但不进入提示词；
2. **staff**：只对内部账号或指定用户启用理论 grounding；
3. **enabled**：逐步扩大比例，保留知识库单独回答的快速回退开关。

上线前至少满足：首批理论卡人工审核通过、固定安全评测集通过、grounding P95 ≤ 1,200ms 且 P99 ≤ 2,000ms、引用追踪无断链、知识库单独回退验证通过。

## 14. 可独立发布的实施里程碑

本设计不由一个超大实现计划一次交付，而拆为四份有依赖关系的计划：

### 里程碑 A：Schema、目录与审核纵切

- 建表、约束、release/version 模型；
- theorystore 与 schema/store 测试；
- 登记少量作品和文件；
- 手工创建、审核、发布一张理论卡，构建一个 active release；
- 退出条件：数据库可重复初始化，单卡从来源到 active chunk 全链路可验证。

### 里程碑 B：理论检索与 shadow trace

- 中文 lexical、1536 维向量、RRF、一跳图扩展；
- typed retriever/grounding contracts；
- intent/risk/fallback 和检索运行审计；
- shadow 模式不改变用户回答；
- 退出条件：固定检索集通过，P95/P99 达标，失败降级可观测。

### 里程碑 C：共享会话 grounding 与引用事务

- text/SSE/voice 接入同一编排器；
- generation-only RAG；
- `FinalizeChatAnswer` 原子保存消息、引用和 run；
- staff 灰度开关；
- 退出条件：三入口来源一致，流式和持久化失败语义通过测试，现有聊天回归测试全绿。

### 里程碑 D：后台审核与有界资料试点

- 资料目录、理论卡审核、检索观察后台；
- 标准 OCR 输出包导入验证；
- 最多 24 文件/2,000 页面/300 OCR 页的试点；
- 至少 40 张理论卡和 12 张实践卡审核；
- staff → enabled 上线评估；
- 退出条件：试点和安全验收通过，能从后台复盘任一启用回答。

每个里程碑都能单独部署和回滚；后一个里程碑必须以所有前置退出条件通过为开始条件。

## 15. 成功标准

- App 三种会话入口均严格执行“知识库 → 理论库 → AI”；
- 理论回答能追溯到卡片、版本、作品、文件和页码；
- 未发布草稿、重复资料和未核验 OCR 引文不会进入用户回答；
- 当前事实不会被理论或模型补造；
- 任一库或向量服务异常时回答可控降级并可审计；
- 后台可完成资料检查、理论审核、发布和单次检索复盘；
- 保持现有 App 对话协议、持久化、简洁风格和流式完整性。
