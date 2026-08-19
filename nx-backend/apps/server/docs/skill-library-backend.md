# 技能库后端接口契约

所有接口位于 `/api/app`，均要求 `Authorization: Bearer <app access token>`。响应沿用统一结构：

```json
{"code":0,"data":{},"error":null,"message":"ok"}
```

目录接口只返回 `library/category/skill = enabled` 且 `latest version = published` 的数据。当前内置目录为 7 类、35 条，其中 3 条 `sourceNeeded` 保持 disabled/draft，公开接口返回 32 条。

## 目录

- `GET /api/app/skill-libraries`
  - 返回 `Library[]`：`id,key,name,description,iconKey,skillCount`
- `GET /api/app/skill-libraries/{libraryId}/categories`
  - 返回 `Category[]`：`id,libraryId,key,name,iconKey,colorToken,skillCount`
- `GET /api/app/skill-libraries/{libraryId}/skills?categoryId=&query=&cursor=&limit=`
  - `limit` 默认 20，最大 50；`cursor` 使用响应中的 `nextCursor`
  - 返回 `{items: SkillSummary[], hasMore, nextCursor?}`
  - `SkillSummary`：`id,categoryId,categoryKey,categoryName,key,name,summary,iconKey,colorToken,versionId,version`
- `GET /api/app/skills/{skillId}`
  - 返回技能详情和 `publishedVersion`
  - 客户端不会收到 `instructions`、`theoryReleaseId` 或 `contentHash`

## 多会话

- `GET /api/app/skills/{skillId}/sessions`
- `GET /api/app/skills/{skillId}/sessions/latest`
- `POST /api/app/skills/{skillId}/sessions`
  - 请求：`{"title":"可选标题"}`
  - 服务端只接受 `skillId`，并把该技能当前发布版本固定到 `skillVersionId`
- `GET /api/app/skill-sessions/{sessionId}/messages`
- `PATCH /api/app/skill-sessions/{sessionId}`
  - 重命名：`{"title":"新的标题"}`
  - 清空消息、转写、摘要及关联录音：`{"clear":true}`
  - 可一次提交标题和清空操作
- `DELETE /api/app/skill-sessions/{sessionId}`
  - 删除消息、隐藏转写、摘要和关联 `upload_assets` 音频资产

所有会话方法都在 SQL 中同时限定 `app_user_id + session_id + scene='skill_chat'`。同一用户同一技能可以创建多条 session；不同 session 不共享历史或摘要。

会话 DTO 的 `version`、`minAppVersion` 和 `sourceMetadata` 均来自该 session 固定的技能版本。`sourceMetadata` 只公开 `reviewPolicy`、`reviewDecisionRef`、`reviewDecision`、`riskNotices`、`sourceNeeded` 和 `compilerPolicy`；服务端文件路径、内容哈希及 manifest 哈希不会返回客户端。`minAppVersion` 只用于客户端在新建会话前检查当前 latest 版本，恢复旧 session 时不得重新套用 latest 版本门槛。

## 文字问答

- `POST /api/app/skill-sessions/{sessionId}/ask`
  - 请求：`{"question":"..."}`，最多 300 字
  - 返回：`{answer,sources,suggestions,messageId}`
- `POST /api/app/skill-sessions/{sessionId}/ask/stream`
  - 请求同上，响应 `text/event-stream`
  - 建连注释：`: connected`
  - 增量：`event: delta`，`data: {"content":"..."}`
  - 完成：`event: done`，数据为同步接口同结构
  - 失败：`event: error`，`data: {"message":"..."}`

每轮只加载当前 session 的摘要和消息；不读取人物卡、人物画像、`app_memories`、普通聊天偏好、其他普通会话或其他技能会话。RAG 使用 session 固定版本的 `theory_release_id`，SQL 直接限定 `theory_release_cards.release_id`，零命中时不回退其他理论库。

同步、SSE 和语音回答保存时会原子写入 `app_skill_chat_traces`，只记录 `session_id`、assistant message ID、session revision、`skill_version_id`、`theory_release_id` 和 chunk IDs。追踪表不保存问题、回答、转写或知识正文。

固定 release 内的全部知识块都会参与相关性评分，不按 chunk ID 截断候选；模型最多接收 6 个命中块和约 4,000 个字符的完整知识上下文，客户端来源摘要保持短文本。同步、SSE 和语音入口使用同一聊天限流器。

## 语音问答

- `POST /api/app/skill-sessions/{sessionId}/voice`
  - `multipart/form-data`
  - `audio`：现有 ASR 支持格式，最大 10 MiB
  - `durationMs`：800 至 60000
  - 复用现有 ASR，转写后进入同一个 `SkillChatRuntime`
  - 返回：`{userMessage,answer,messageId}`
- `GET /api/app/skill-messages/{messageId}/audio`
- `GET /api/app/skill-messages/{messageId}/transcript`

音频和转写读取同样限定当前账号与 `scene='skill_chat'`。技能语音不会写普通偏好、人物记忆或画像证据。

## 编译导入

先执行数据库 schema，再运行：

```bash
DATABASE_URL='postgresql://...' go run ./cmd/skillsync \
  -action publish -version 1.0.0 \
  -source-dir '/path/to/学习成长类书籍-Skills' \
  -review-manifest ./config/skill-review-manifest.json
```

生产镜像包含 `/app/skillsync`。服务完成 schema 迁移后，用一次性容器只读挂载审核过的技能源目录：

```bash
SKILL_SOURCE='/srv/private/学习成长类书籍-Skills'
REVIEW_MANIFEST='/srv/private/skill-review-manifest.json'
test "$(find "$SKILL_SOURCE" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')" = 35

docker compose run --rm --no-deps \
  -v "$SKILL_SOURCE:/skills:ro" \
  -v "$REVIEW_MANIFEST:/review/manifest.json:ro" \
  server /app/skillsync -action publish -version 1.0.0 \
    -source-dir /skills -review-manifest /review/manifest.json
```

manifest 必须精确覆盖 35 个技能，并使用机器可验证的 `product-baseline-v1` 策略及非空 `decisionRef`。当前基线为 32 个 `publish`、3 个 `hide + sourceNeeded`；3 个条件发布技能必须带 `riskNotices`。这里记录产品基线决策，不伪造法律审批人或版权审批结论。

导入应输出 7 个分类、35 个技能和 32 个公开技能；相同版本、内容和审核决策再次导入为 no-op。技能源目录不打入镜像，也不长期挂载到在线服务。

编译器确定性读取每个技能的 `SKILL.md`、`chapters/**/*.md`、`glossary.md`、`patterns.md`、`cheatsheet.md`，排除 `agents/` 和验证/诊断运行元数据；文件路径和正文共同参与 SHA-256。全部批准 Markdown 作为不可信检索知识进入该版本独占 release；只有 `SKILL.md` 中明确的使用、默认工作流、输出和范围边界章节会按 `skill-compiler-v2` 提取为受控行为规则。编译审计元数据记录知识文件、被提取章节、策略、决策引用和哈希。运行时不读取本地目录。编译入口拒绝符号链接、无效 UTF-8、超过 2 MiB 的单文件、可执行 HTML 和危险 URI。

关系与健康技能分别使用 `sensitive-relationships-v1` 和 `health-information-v1` 运行时安全策略。已发布版本内容或审核决策变化会失败，必须使用新的语义版本。

版本工作流：

```bash
# 构建草稿或进入 ready，不切换新会话使用的 latest 指针
/app/skillsync -action draft -version 1.1.0 -source-dir /skills -review-manifest /review/manifest-1.1.0.json
/app/skillsync -action ready -version 1.1.0 -source-dir /skills -review-manifest /review/manifest-1.1.0.json

# 发布后仅新会话使用 1.1.0；旧会话仍固定原版本
/app/skillsync -action publish -version 1.1.0 -source-dir /skills -review-manifest /review/manifest-1.1.0.json

# latest 指针回到仍为 published 的旧版本；或不可逆退役指定版本
/app/skillsync -action rollback -version 1.0.0
/app/skillsync -action retire -version 1.1.0
```
