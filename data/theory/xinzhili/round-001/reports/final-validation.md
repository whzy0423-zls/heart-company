# 芯之力理论库首轮最终验证

## 结论

验证时间：2026-07-23 14:47—16:04 CST。

验证基线：`fb5c8a7f3f1d447eb0ba5f3687398855e63e6a77`（`修复：统一理论包计划校验与错误脱敏`）。

Task 3 的构包规格审查和代码质量审查均已通过。当前可提交包通过文件校验、预算/数量/来源追溯门禁以及 Task 4 验证器门禁；Task 5 的数据库同步、幂等、三审、promote 和激活拒绝路径均有 fresh 测试证据。

这不代表内容已经上线：包内状态仍为 `draft`，三份仓库审核模板仍为 `pending`，生产数据库尚未录入三审。固定安全评测为 `not_runnable_for_activation`，且里程碑 B/C 尚未完成，所以 `activate` 必须拒绝。

## 环境

- macOS 26.4.1（arm64）
- Python 3.12.7
- Go 1.26.4
- PostgreSQL 客户端 17.10
- PostgreSQL 隔离测试库：`nx_theory_round1_20260723_test`（连接凭据未写入仓库）

## 数据快照

来源目录由 `data/theory/xinzhili/source-catalog.json` 动态核对：

- 物理文件：263（其中 1 个非内容系统文件）；内容文件：262。
- 状态：selected 24、backlog 226、duplicate 7、excluded 4、error 1；状态合计 262。
- 重复组：7。
- 首轮处理：1,435 页等价，OCR 257 页；上限分别为 2,000 和 300。
- 卡片：40 张理论卡、12 张实践卡，覆盖 10 个领域。
- 显式语义关系：19。
- 逐字引用：0；草稿正式 chunk：0；chunk preview：52。
- `contentDigest`: `4bfbfc7f92d90d2e595343eae753d23f9ff5d941261378b5df97b68a1456af3d`
- `packageDigest`: `3ee7a374fa53db94989997dbebeca2c24fc9393cecef361657eeaceab3265d66`

后续批次不能只看 `backlog=226`：还需分别处理 7 个重复项、4 个排除项和 1 个损坏/错误项，并继续遵守每轮来源、页等价和 OCR 预算。

## Python 验证

执行：

```sh
python3 -m unittest \
  nx-backend/scripts/theory/test_build_source_catalog.py \
  nx-backend/scripts/theory/test_extract_round.py \
  nx-backend/scripts/theory/test_build_round_package.py

python3 -m py_compile \
  nx-backend/scripts/theory/build-source-catalog.py \
  nx-backend/scripts/theory/extract-round.py \
  nx-backend/scripts/theory/build-round-package.py
```

结果：74 个测试全部通过；`py_compile` 退出码 0。

## Go 验证

执行：

```sh
cd nx-backend/apps/server
go test ./...
go vet ./...
```

结果：两条命令退出码均为 0。

数据库增强的全包测试若让多个 Go 包并行复用同一个可变 PostgreSQL schema，会在各包同时重放 schema 时产生测试夹具级死锁。因此 package sync 的真实数据库用例按计划在重建后的唯一数据库中串行执行：

```sh
TEST_DATABASE_URL='<隔离测试库 DSN>' \
  go test -p 1 -count=1 -run '^TestPackageSyncPostgres$' -v ./internal/theorystore
```

结果：`TestPackageSyncPostgres` 通过。该用例实际覆盖 stage、重复 stage、三类审核角色、错误角色拒绝、操作人分离、promote、重复 promote、52 个正式 chunk 和激活阻断等合同；事务回滚、并发和篡改拒绝由同包其他定向测试覆盖。

## CLI 与 PostgreSQL 实测

在重建并应用 `internal/db/schema.sql` 的唯一隔离测试库中，使用数据库内独立的 actor 和三类 reviewer fixture 执行：

```sh
nx-backend/scripts/theory/sync-theory-package.sh validate
THEORY_DATABASE_URL='<隔离测试库 DSN>' nx-backend/scripts/theory/sync-theory-package.sh plan --dry-run
THEORY_DATABASE_URL='<隔离测试库 DSN>' nx-backend/scripts/theory/sync-theory-package.sh stage --apply --actor '<actor id>'
THEORY_DATABASE_URL='<隔离测试库 DSN>' nx-backend/scripts/theory/sync-theory-package.sh stage --apply --actor '<actor id>'
```

实测结果：

- validate：24 sources、40 cards、12 practices，`writeAllowed=false`。
- plan：operation=`stage`，`writeAllowed=false`。
- 第一次 stage：`writeAllowed=true`、`noOp=false`。
- 第二次 stage：`writeAllowed=false`、`noOp=true`。
- stage 后数据库：24 个 source work、52 个 card host、12 个 practice、0 个正式 chunk、1 条 staged import receipt。

测试库随后由三个不同、具有正确角色的用户分别录入 `source-verification`、`theory-review`、`safety-review`，再由独立 actor 执行 promote。结果：

- 第一次 promote：release 版本 1、状态 `ready`、52 cards、52 chunks、`noOp=false`。
- 第二次 promote：返回同一 release、`noOp=true`。
- 数据库核对：24 source works、52 cards、12 practices、52 chunks、3 条审核、1 条 promotion receipt、1 个 ready release；import 状态为 `promoted`。
- 测试 reviewer 和 receipt 已限定在隔离测试库，不构成生产人工审核或生产发布证明。

执行 activate：

```sh
THEORY_DATABASE_URL='<隔离测试库 DSN>' \
  nx-backend/scripts/theory/sync-theory-package.sh activate \
  --package-id xinzhili-round-001 \
  --actor '<actor id>'
```

结果：退出码 1，错误明确包含 `not_runnable_for_activation`、`milestone B` 和 `milestone C`。这是预期的失败关闭行为。

## 数据包边界与完整性

执行：

```sh
(cd data/theory/xinzhili/round-001 && shasum -a 256 -c checksums.sha256)
find data/theory/xinzhili/round-001 -type f | rg -i '\.(pdf|epub|docx?|pptx?)$'
rg -nF "$HOME" data/theory/xinzhili/round-001
find data/theory/xinzhili/round-001 -type f -size +200k -print
git diff --check
```

结果：

- `checksums.sha256` 中 118 个受保护文件全部为 `OK`。
- 没有 PDF、EPUB、DOC、DOCX、PPT、PPTX 文件。
- 没有绝对用户路径。
- 没有超过 200 KiB 的异常大文件；最大业务 JSON 为来源元数据目录，不含分页全文。
- 卡片和实践的证据只保存 source SHA、text SHA 和 locator；当前逐字引用总数为 0。
- `git diff --check` 在提交前最终复跑。

## 本地交付位置与线上流程

- 可提交数据包：`data/theory/xinzhili/round-001/`
- 全量来源目录：`data/theory/xinzhili/source-catalog.json`
- 不入 Git 的全文/OCR 底稿：`var/theory-work/xinzhili/round-001/`
- 同步脚本：`nx-backend/scripts/theory/sync-theory-package.sh`

线上顺序必须是 `validate → plan --dry-run → stage --apply → 三类数据库人工 review → promote`。当前不得执行成功的 activate；必须先完成 B/C 会话链路并获得绑定当前内容、评测集和 runtime/version 的正式安全评测通过结果。

## 中文提交链

本轮从来源目录、抽取、构包、验证器到同步工具均使用中文提交说明。关键功能提交包括：

- `596115f 功能：生成理论库首轮来源目录`
- `839476c 功能：生成理论库首轮抽取工作包`
- `0a69956 功能：生成理论库首轮审核数据包`
- `704f43e 功能：实现理论库数据包验证器`
- `ecb2538 功能：实现理论库数据包同步与三审发布`
- `88855f5 修复：锁定理论包数据库快照合同`
- `b551701 修复：支持理论包可选交付文档`
- `bbe5637 修复：绑定理论证据处理范围`
- `fb5c8a7 修复：统一理论包计划校验与错误脱敏`

本报告只记录验证事实，不表示当前分支已经合并到 `main` 或已经推送远端；合并和推送状态应在完成最终审查后由 Git 结果单独确认。
