# 九型专项对话发布修复

## 根因

Flutter 场景表单打开 `enneagram-personality-library` 独立技能会话，但线上技能目录没有该 key。已有 `enneagram-core` 和九个型号的正式知识库，缺少技能发布绑定。另外 skill runtime 原有 300 字限制会拒绝完整场景表单。

## 实现

- `scripts/publish-enneagram-scenes.sql` 从十个已审核、启用的 active release 克隆卡片和 186 条知识片段，建立独立不可变知识快照和技能版本。整笔事务发布，可重复执行；不修改原知识库。
- 专项问题上限 6000 字，其他技能保持 300 字。
- 首次表单消息保存在当前独立会话中，每次追问读取该会话首条问题；即使旧消息已摘要，关系、事件、目标和问题仍在上下文。
- 根据表单头部明确选择的型号检索快照中的核心与对应型号，各层分配检索名额；未知型号不推断，不从事件正文提取型号。
- 线上存在本地尚未合入的 LangChain 检索支持。部署仅在此分支前增加专项检索路径，保留其他技能原有 remote/shadow/fallback 行为。对应补丁见 `enneagram-scenes-production-runtime.patch`。

## 部署与回滚

生产部署目录 `/opt/heart-company`，仅重建和重启 `server` 服务。备份位于 `backups/enneagram-scenes-20260918`，旧镜像标签 `heart-company-server:before-enneagram-scenes-20260918`。

如需回滚，先停用 key 为 `enneagram-personality-library` 的技能，再恢复旧镜像和此次修改的源码文件；保留专项会话与已发布快照，不清空用户数据。正常回滚无需整库还原。

## 验证

本地 skillchat、theorystore、server 测试通过。线上合并版本 skillchat、theorystore 检索测试与 server 全包测试通过；理论包测试 `TestValidateAndPlanRoundPackage` 依赖未挂载的 `/data/theory/xinzhili/round-001`，在该容器检查中跳过。

发布脚本在生产先执行 ROLLBACK 预演，确认完整复制 186 条内容及所有数据库约束成立，再正式发布。使用临时 QA 用户测试真实目录、两条独立会话、长表单 SSE 回答、后续背景延续、知识检索轨迹和消息隔离；QA 用户和会话在验证后移除。

正式验证结果：skill=77、version=77、release=87。406 字表单通过真实 SSE 返回 6 个知识来源；追问准确回答初始目标。持久化记录只引用 core、type-06、type-09；另一独立会话无消息。公网 HTTPS 健康检查通过。已重启连接手机上的应用并实际打开“九型应用 → 职场协作”表单；未在用户账户下提交模拟咨询。
