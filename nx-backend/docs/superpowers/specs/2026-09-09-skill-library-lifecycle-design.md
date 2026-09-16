# 成长技能库生命周期操作设计

## 目标

在成长技能库管理页为每个技能提供发布、下架、启用和停用操作，使版本发布状态与 App 可用状态可独立管理。

## 语义与状态

- 发布：将技能最新草稿版本及其理论版本切换为已发布，并设置 `latest_published_version_id`，同时启用技能。
- 下架：保留已发布版本和历史记录，但清空 `latest_published_version_id` 并将技能置为停用，使其不再出现在 App 技能列表。
- 启用：仅允许已有已发布版本的技能，将技能置为启用并恢复其最新已发布版本指针。
- 停用：将技能置为停用并清空当前发布指针，不删除任何版本。

## 接口

新增受 `App:SkillLibrary:Edit` 保护的动作接口：

- `POST /api/skill-library-management/skills/{id}/publish`
- `POST /api/skill-library-management/skills/{id}/unpublish`
- `POST /api/skill-library-management/skills/{id}/enable`
- `POST /api/skill-library-management/skills/{id}/disable`

动作成功返回 `{id, status, published}`。不存在、无已发布版本或非法状态返回明确的 4xx 错误。

## 前端

操作列根据当前数据展示：

- 未发布：发布
- 已发布且已启用：下架、停用
- 已发布且已停用：启用、下架

发布、下架、停用需要确认弹窗；启用直接执行。所有操作成功后刷新目录并显示结果消息。

## 测试

- Go：路径解析、权限映射、状态动作 SQL/事务行为和无发布版本启用校验。
- 前端 API：四个动作的 HTTP 方法及路径。
- 前端页面：操作文案、状态条件和动作调用存在。
