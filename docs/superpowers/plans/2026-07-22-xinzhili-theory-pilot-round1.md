# 芯之力理论库第一轮数据包 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从“芯之力文件”中完成首轮 24 个以内、2,000 页以内、OCR 300 页以内的可审核数据包，产出至少 40 张理论卡和 12 张实践卡，并提供本地/线上幂等同步工具。

**Architecture:** 原始文件只保留在用户资料目录；抽取和 OCR 中间产物写入 gitignore 的 `var/theory-work/`；仓库只保存来源元数据、短引用、自有提炼、待审核模板和内容校验和。同步工具先验证数据包，再以单事务把内容导入为 draft；人工三审绑定稳定 `contentDigest` 后才能 promote，发布卡片后才从 preview 生成正式 chunk。里程碑 B/C 的检索与会话安全能力完成前，本轮不激活线上 Release，也不声称 App 已使用该理论库。

**Tech Stack:** Go 1.22、PostgreSQL、JSON、Python 3、Poppler、Tesseract、现有 `internal/theorystore`。

---

## Chunk 1: 资料盘点与抽取包

### Task 1: 固化全量目录和首轮 24 个来源

**Files:**
- Create: `nx-backend/scripts/theory/build-source-catalog.py`
- Create: `nx-backend/scripts/theory/round-001-selection.json`
- Create: `data/theory/xinzhili/source-catalog.json`
- Create: `data/theory/xinzhili/round-001/catalog/works.json`
- Create: `data/theory/xinzhili/round-001/catalog/source-files.json`
- Test: `nx-backend/scripts/theory/test_build_source_catalog.py`

- [ ] 先写失败测试，约束全量目录不得遗漏内容文件、首轮最多 24 个、处理页不超过 2,000、OCR 页不超过 300、SHA-256 不重复计权。
- [ ] 运行测试确认 RED。
- [ ] 实现目录扫描、格式识别、PDF 页数/文本量、SHA-256、重复组和动态批次状态生成。每个物理文件必须包含 `catalogStatus`、canonical work、duplicate group、extraction route、OCR 估算、priority、proposed batch 和 exclusion/error reason。
- [ ] 固化首轮最多 24 个来源：12 份能量课件、九型 EPUB、周易三份、英雄之旅、千面英雄、NLP 精义、亲子关系可读版、重塑心灵、NLP 简快心理疗法、情绪的语言、非暴力沟通。每项必须记录 `selectedRanges`、`processedUnitType`、`processedUnitCount`、`budgetPageEquivalent` 和 `ocrPageCount`，超预算时按低权威/重复/可后置顺序淘汰。
- [ ] 预算统一累计 `budgetPageEquivalent`：PDF/PPTX 使用实际 page/slide；EPUB/DOC/DOCX 按规范化 UTF-8 文本每 1,800 个非空字符向上取整折算。manifest 保存折算规则版本、转换器版本、输入/输出 hash；locator 始终使用真实 page/slide/spine/heading/paragraph，不把折算页当来源页码。
- [ ] 动态断言 `selected + backlog + duplicate + excluded/error = 实际扫描内容文件数`，不得硬编码 262、263 或剩余文件数；每个 backlog 项必须有未来处理路线。
- [ ] 运行测试确认 catalog 数量、预算和候选路径全部有效。
- [ ] 使用中文提交信息提交。

### Task 2: 生成可复现的分页抽取/OCR 工作包

**Files:**
- Create: `nx-backend/scripts/theory/extract-round.py`
- Create: `nx-backend/scripts/theory/test_extract_round.py`
- Create (ignored): `var/theory-work/xinzhili/round-001/**`
- Modify: `.gitignore`

- [ ] 写失败测试，约束 manifest、可回溯 locator、UTF-8、字符数、置信度、source SHA-256 和 OCR 页预算。
- [ ] 运行测试确认 RED。
- [ ] PDF 文本层使用 `pdftotext`；DOC/DOCX 使用 `textutil`；EPUB 解包提取 XHTML；选择性扫描页用 `pdftoppm` + `tesseract chi_sim+eng`。Tesseract/语言包缺失时 fail closed，并记录工具版本、参数和输出 hash。
- [ ] locator 合同：PDF=`page`，PPTX=`slide`，EPUB=`spineItem/chapter/paragraph`，DOC/DOCX=`heading/paragraph`；禁止为非分页格式伪造页码。EPUB 必须防 Zip Slip、压缩炸弹和畸形 XML/HTML。
- [ ] 能量超长页按可审查切片保存 locator；不得把全文复制到 `data/`。
- [ ] 输出每个来源的 `manifest.json`、分页文本、抽取质量和错误报告。
- [ ] 对每个 PDF 至少渲染封面和一个所选页面进行视觉抽查。
- [ ] 验证总处理页与 OCR 页预算，使用中文提交信息提交。

## Chunk 2: 理论卡、实践卡与审核包

### Task 3: 生成 40 张理论卡和 12 张实践卡

**Files:**
- Create: `nx-backend/scripts/theory/build-round-package.py`
- Create: `nx-backend/scripts/theory/test_build_round_package.py`
- Create: `data/theory/xinzhili/round-001/manifest.json`
- Create: `data/theory/xinzhili/round-001/cards/*.json`
- Create: `data/theory/xinzhili/round-001/practices/*.json`
- Create: `data/theory/xinzhili/round-001/relations.json`
- Create: `data/theory/xinzhili/round-001/chunk-previews/*.json`
- Create: `data/theory/xinzhili/round-001/review/*.json`
- Create: `data/theory/xinzhili/round-001/reports/coverage.md`
- Create: `data/theory/xinzhili/round-001/evaluation/safety-cases.json`
- Create: `data/theory/xinzhili/round-001/reports/safety-evaluation.md`

- [ ] 写失败测试，约束至少 40 张理论卡、至少 12 张实践卡、10 个领域覆盖、每卡主来源、格式化 locator、短引用上限、认识论/证据/安全字段和实践停止条件。
- [ ] 运行测试确认 RED。
- [ ] 从实际抽取包生成自有摘要和定义，不复制原书全文，不伪造页码或核验状态。
- [ ] 能量课程暂标 `course_adaptation/experiential/authority=3`；九型/NLP/易经分别保留标签、科学性和预测边界。
- [ ] 所有 AI 生成内容初始只能为 `draft`；OCR 引文默认 `quoteVerified=false`；课程归属审核设为发布硬门槛。
- [ ] 生成至少 12 张实践卡，包含步骤、停止条件、专业升级条件和 `xinzhili.practice.v1`。
- [ ] 只生成自有提炼的 chunk preview 与 SHA-256 content hash；不得在草稿阶段写入正式 `theory_chunks`。生成关系、覆盖报告和三份 `pending` 人工审核模板，构包脚本不得写 `approved`。
- [ ] 定义 digest：采用 UTF-8、LF、对象键排序、数组保持语义顺序、无多余空白的 canonical JSON。`contentDigest` 覆盖去除 digest 自身字段后的 manifest、预算/版权元数据、来源、卡片、实践、关系、chunk preview、coverage 对象清单和 safety case set；审核文件与 checksums 不参与，避免自引用。`packageDigest` 覆盖 `contentDigest`、三审记录和安全评测结果描述，但排除自身字段与 `checksums.sha256`；checksums 最后覆盖除自身外的所有文件。
- [ ] 来源核验、理论审核、安全审核分别绑定同一个 `contentDigest`；若离线 approval 没有可验证签名则只能作为待录入材料，不能直接授权 promote。正式审核记录必须由后台/CLI 写入数据库，reviewer 必须是存在且具有对应权限的后台用户；构包进程身份不得充当 reviewer，promote actor 与三名 reviewer 分开审计。
- [ ] 激活用安全评测结果必须绑定 `contentDigest + safetyCaseSetDigest + runtime/version`；任一内容、预算、版权范围、评测集或 runtime 变化均使结果失效。
- [ ] 版权门禁：单条引用、单卡累计、单作品累计均设上限；`metadata_only` 不得保存引用；OCR 未核验引文不得发布；chunk preview 只能包含自有提炼；`data/`、日志和同步请求不得包含分页全文。
- [ ] 增加固定安全评测，覆盖九型贴标签、NLP 科学性、易经预测、创伤、自伤、精神病性症状、家暴、医疗建议、课程价格和无资料问题；当前未接入会话链路时报告必须标记 `not_runnable_for_activation`，不得伪报通过。
- [ ] 运行测试和人工抽查，使用中文提交信息提交。

## Chunk 3: 数据包验证和线上同步

### Task 4: 实现数据包验证器

**Files:**
- Create: `nx-backend/apps/server/internal/theorypackage/models.go`
- Create: `nx-backend/apps/server/internal/theorypackage/validate.go`
- Create: `nx-backend/apps/server/internal/theorypackage/validate_test.go`
- Create: `data/theory/xinzhili/round-001/schema/theory-package-v1.schema.json`
- Create: `data/theory/xinzhili/round-001/checksums.sha256`

- [ ] 测试路径穿越、额外文件、摘要篡改、数量/预算超限、无主来源、单条/单卡/单作品引文超限、content hash 错误、实践安全字段缺失和审核摘要不匹配。
- [ ] 实现 JSON/语义/版权边界/digest/checksum 验证。
- [ ] 验证首轮包通过且任一篡改都会失败。
- [ ] 使用中文提交信息提交。

### Task 5: 实现本地与线上幂等同步 CLI

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Create: `nx-backend/apps/server/internal/theorystore/package_sync.go`
- Create: `nx-backend/apps/server/internal/theorystore/package_sync_test.go`
- Create: `nx-backend/apps/server/cmd/theorysync/main.go`
- Create: `nx-backend/scripts/theory/sync-theory-package.sh`
- Modify: `nx-backend/apps/server/Dockerfile`

- [ ] 先写失败测试，覆盖 validate/plan/stage、全包回滚、同包幂等、同 packageId 异 digest 拒绝、跨库拒绝和更高 active version 保护。
- [ ] 新增 `theory_package_imports` 审计表与幂等唯一约束。
- [ ] 实现 `validate`、`plan --dry-run`、`stage --apply`；数据库 URL 优先 `THEORY_DATABASE_URL`，回退 `DATABASE_URL`，只从环境变量读取，日志和错误不得输出密码。
- [ ] `stage` 使用 SERIALIZABLE 事务和 library advisory lock，只导入 draft/in_review，不自动发布。
- [ ] 提供显式 `review` 命令或后台写入入口，把三类审核记录写入数据库并核验 reviewer 角色；JSON 中的 pending 模板不能直接变成可信 approval。课程归属审核可按设计把暂定 authority=3 提升为“已审核韩老师课程提炼”的 4，但不得自动提升为正式理论 5。
- [ ] 提供显式 `promote` 门禁：三审与 digest 不完整时必须拒绝。新增 transaction-scoped Store API，确保一个 SERIALIZABLE 事务内完成 `复验 approvals/contentDigest → 发布全部 Card/Practice → 写关系 → 从 preview 生成正式 Chunk → BuildRelease → 完整校验并置为 ready → 写 promote receipt → commit`；任一步失败全部回滚。
- [ ] 相同 `packageId + contentDigest` 重复 promote 返回同一 ready release/no-op；同 packageId 异 digest、人工已修改内容、版本冲突均拒绝，不得留下半发布状态。
- [ ] `activate` 独立执行且必须验证固定安全评测通过；在 B/C 会话检索里程碑未完成时默认拒绝并报告前置条件。更高 active release 不得覆盖，release 版本在数据库锁内单调分配。
- [ ] Docker 同时构建 `/app/theorysync`。
- [ ] 使用临时 PostgreSQL 验证本地导入、重复 stage/promote/activate no-op、人工编辑不被覆盖、canonical key/version 冲突拒绝、approval 失效、部分发布失败全量回滚、并发同步/激活、更高 active 保护、crash 后可恢复和线上 dry-run 命令。
- [ ] 使用中文提交信息提交。

## Chunk 4: 最终验收与交付说明

### Task 6: 验证首轮数据包和同步流程

**Files:**
- Create: `data/theory/xinzhili/round-001/README.md`
- Create: `data/theory/xinzhili/round-001/reports/final-validation.md`

- [ ] 运行 Python 目录、抽取和构包测试。
- [ ] 运行 `go test ./...`、`go vet ./...`、`git diff --check`。
- [ ] 在隔离 PostgreSQL 执行 validate、plan、stage 两次，确认第二次 no-op。
- [ ] 查询并核对来源、卡片、实践、chunk 和 import receipt 数量。
- [ ] 确认没有原始 PDF/EPUB/DOC、全文或绝对用户路径进入提交数据包。
- [ ] 记录本地路径、线上同步命令、人工审核与 ready release 步骤、暂不 activate 的原因，以及由全量 catalog 动态计算的后续 backlog。
- [ ] 整体规格与代码质量终审后再交付。
