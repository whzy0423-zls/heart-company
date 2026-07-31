# Full Project Audit Fixes Design

## Goal

修复 2026-07-31 全量审计中已确认的生产缺陷，优先恢复数据库兼容与芯之力实时语音正确性，再收敛课堂、人声克隆、推送和后台 E2E 的一致性问题。

## Scope

本轮包含：

1. 旧数据库 `app_daily_quiz_questions.type_weights` 幂等迁移。
2. 芯之力实时会话的 partial/final、静音、策略 Tick、Action 执行与 FinishInput。
3. WebSocket 配置版本刷新、`session.config_changed` 和会话替换竞态。
4. 课堂写操作与审计一致性、上传完成错误传播、清理任务隔离。
5. 人声克隆原子 claim 与失败响应语义。
6. JPush APNs 生产标记、坏令牌隔离基础能力。
7. 后台版本抽屉关闭后的焦点恢复。
8. 部署配置中硅基流动/MiniMax 旧默认值清理与测试门禁补强。

依赖大版本升级、生产服务器部署、数据库真实执行和真机联调在代码修复验证后单独执行，避免把供应链升级与业务修复混在同一提交。

## Architecture

### Database migration

继续复用启动时执行的嵌入式 `schema.sql`。新表定义保留，同时追加 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`，为旧表回填默认 JSON、恢复 default 和 NOT NULL。使用 legacy schema 集成测试证明从旧结构升级成功。

### Xinzhili realtime orchestration

会话层成为策略引擎唯一执行者：ASR partial/final 转成策略事件，固定 ticker 驱动 `Tick`，策略 Action 统一经 executor 转换成现有 WebSocket 事件和 ASR `FinishInput`。所有状态变化保持 generation/session/turn 序列约束，不在 transport 层复制策略。

每轮读取配置时同步版本；版本变化时先发送 `session.config_changed` 和最新 mode snapshot，再使用新配置。会话安装与关闭通过同一锁完成，关闭后的连接不得重新安装上游 session。

### Classroom consistency

业务成功不得被事后审计失败反转为 HTTP 500。短期采用明确的 best-effort 审计并记录错误；需要强一致的字段修改继续由业务事务保证。上传完成路径必须传播 repo 错误。清理器逐项执行并聚合错误，单个 poison task 不阻塞其余任务。

### Voice cloning

所有供应商克隆入口先通过数据库 CAS claim 从可克隆状态进入 `cloning`，只有 claim 获得者调用供应商。同步请求若最终状态为 failed，返回可机读错误；已接受的异步处理使用明确状态响应，不再以 nil error 隐藏失败。

### Push

JPush payload 根据生产环境设置 `options.apns_production=true`。供应商返回无效 registration ID 时，从批次中隔离并清退，再对有效 ID 保持现有结果统计。非生产 Noop 行为在响应中标记 simulated，避免被当成真实送达。

### Admin focus

版本抽屉打开时保存实际触发元素引用。关闭后优先恢复该元素；若筛选切换导致元素卸载，则聚焦当前可见列表中对应或最近的“查看版本”按钮，保证键盘操作连续。

## Error handling

- 数据库迁移必须幂等，重复启动无副作用。
- 策略 Action 执行失败统一产生协议 `error`，并确保 turn 收敛到 done/idle。
- 配置版本变化必须显式通知 App，不静默切换。
- 审计失败记录日志和指标，但业务成功响应保持成功。
- 克隆并发请求只有一个外部供应商调用，其余返回当前状态。
- 清理任务返回聚合错误并继续处理后续任务。

## Testing

- 所有修复遵循回归测试先行。
- Go：目标包测试、`go test ./...`、`go vet ./...`、重点 `-race`。
- 数据库：使用隔离 `TEST_DATABASE_URL` 跑 legacy migration 与并发 claim 测试。
- 后台：Vitest 全量、类型检查、生产构建、Playwright 失败用例重复 3 次。
- App：协议单测和现有 GitHub Actions 保持通过。

## Rollout

先备份数据库，再部署后端并观察自动迁移；确认每日题生成、实时语音和克隆状态正常后部署后台。App 协议无破坏性版本提升，仍需完成真机完整链路验证。
