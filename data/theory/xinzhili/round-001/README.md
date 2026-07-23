# 芯之力理论库首轮数据包

本目录是 `xinzhili-round-001` 的可审核、可校验、可幂等同步数据包。Task 3 的构包规格门禁和代码质量门禁已经通过，但当前包仍为 `draft`：仓库中的三份审核模板均为 `pending`，尚未由生产数据库中的合格审核用户录入三审，因此它不是已发布内容，也没有上线或接入 App 会话。

## 当前内容

- 来源目录：263 个物理文件中识别出 262 个内容文件；状态合计为 selected 24、backlog 226、duplicate 7、excluded 4、error 1。
- 首轮预算：24 个来源、1,435 页等价、257 个 OCR 页，均低于 24 / 2,000 / 300 的上限。
- 提炼内容：40 张理论卡、12 张实践卡、10 个领域、19 条显式语义关系。
- 版权边界：逐字引用 0 条；仓库数据包不含原始 PDF、EPUB、DOC、DOCX、PPT、PPTX 或分页全文。
- 草稿边界：52 个 chunk preview 均为自有提炼；数据包内正式 chunk 为 0，只有通过数据库三审并执行 `promote` 后才生成正式 chunk。
- 固定摘要：
  - `contentDigest`: `22a3be1be744f6ab39e8817fee55c6fbdcc701098d066f7153269325598043b8`
  - `packageDigest`: `466ad5b421770972fbed4e5c984fc283f4cccfb4f21b6ec96bfd6cb2a224e0c1`

全量来源快照在 `../source-catalog.json`。首轮包在仓库中的位置是 `data/theory/xinzhili/round-001/`；不进入 Git 的抽取/OCR 全文工作底稿位于 `var/theory-work/xinzhili/round-001/`。

## 校验与同步

在仓库根目录执行。同步工具只从环境变量读取数据库地址：优先使用 `THEORY_DATABASE_URL`，未设置时回退 `DATABASE_URL`。

```sh
# 纯文件校验，不连接数据库
nx-backend/scripts/theory/sync-theory-package.sh validate

# 查看计划，不写数据库
THEORY_DATABASE_URL="$THEORY_DATABASE_URL" \
  nx-backend/scripts/theory/sync-theory-package.sh plan --dry-run

# 导入草稿；ACTOR_ID 必须是数据库中存在的操作用户
THEORY_DATABASE_URL="$THEORY_DATABASE_URL" \
  nx-backend/scripts/theory/sync-theory-package.sh stage --apply --actor "$ACTOR_ID"
```

重复执行同一 `packageId + contentDigest` 的 `stage` 应返回 `noOp=true`。`stage` 只写入草稿和导入回执，不发布卡片、不生成正式 chunk。

## 人工三审与 ready release

三审必须由数据库中真实存在、状态有效且拥有相应角色的三个审核用户分别执行，并绑定当前 `contentDigest`：

| 审核类型 | 必需角色 |
| --- | --- |
| `source-verification` | `theory_source_reviewer` |
| `theory-review` | `theory_content_reviewer` |
| `safety-review` | `theory_safety_reviewer` |

```sh
THEORY_DATABASE_URL="$THEORY_DATABASE_URL" \
  nx-backend/scripts/theory/sync-theory-package.sh review \
  --package-id xinzhili-round-001 \
  --type source-verification \
  --reviewer "$SOURCE_REVIEWER_ID" \
  --notes '来源与定位已人工核验'

THEORY_DATABASE_URL="$THEORY_DATABASE_URL" \
  nx-backend/scripts/theory/sync-theory-package.sh review \
  --package-id xinzhili-round-001 \
  --type theory-review \
  --reviewer "$THEORY_REVIEWER_ID" \
  --notes '理论边界与归属已人工核验'

THEORY_DATABASE_URL="$THEORY_DATABASE_URL" \
  nx-backend/scripts/theory/sync-theory-package.sh review \
  --package-id xinzhili-round-001 \
  --type safety-review \
  --reviewer "$SAFETY_REVIEWER_ID" \
  --notes '安全边界与停止条件已人工核验'

# 发布操作人必须与三名审核人分离
THEORY_DATABASE_URL="$THEORY_DATABASE_URL" \
  nx-backend/scripts/theory/sync-theory-package.sh promote \
  --package-id xinzhili-round-001 \
  --actor "$PROMOTE_ACTOR_ID"
```

`promote` 在单个 SERIALIZABLE 事务内复验数据包快照和三审，发布 52 张卡片宿主（40 张理论卡 + 12 张实践卡）、保存 12 张实践定义、生成 52 个正式 chunk，并创建 `ready` release。任一步失败都会整体回滚；重复 promote 返回同一 release 且 `noOp=true`。

仓库中的 `review/*.json` 只是待录入材料，不能替代数据库审核。验证报告中的三审和 promote 是隔离测试库证据，不是生产审核记录。

## 为什么现在不能 activate

当前固定安全评测状态是 `not_runnable_for_activation`，因为里程碑 B 的知识库/理论库检索接入和里程碑 C 的会话安全链路尚未完成。即使已经得到 `ready` release，以下命令也必须失败关闭：

```sh
THEORY_DATABASE_URL="$THEORY_DATABASE_URL" \
  nx-backend/scripts/theory/sync-theory-package.sh activate \
  --package-id xinzhili-round-001 \
  --actor "$ACTOR_ID"
```

只有完成 B/C、以绑定当前 `contentDigest + safetyCaseSetDigest + runtime/version` 的正式安全评测取得通过结果后，才可以重新评估激活；不得绕过此门禁。

详细 fresh 验证命令和结果见 `reports/final-validation.md`。
