# 每日题 AI 生成与题库管理设计

## 背景
用户首次进入 App 完成引导答题后生成画像。第二天起系统每天中午 12:00 推送每日 5 道画像校准题。用户累计完成 100 题后，系统结合每日题、日常聊天提炼、语音语气/行为证据重新评估画像。

## 目标
- 后台新增「画像校准」业务菜单，包含「每日题库管理」和「每日题推送记录」。
- 每天 12:00 准时推送每日题；12 点前系统提前生成当天 5 道题。
- 管理员可在 12 点前查看今日生成题目，并对单题进行版本更换。
- 题目生成基于公共知识库 + 后台配置的大模型，主题不能偏离九型人格/画像校准。
- 每日题集和单题版本需要留存生成/更换记录，保护已答题数据。

## 设计
### 数据模型
新增 `app_daily_quiz_sets` 表记录每天一套题集，字段包括日期、状态、来源、模型 provider/model、prompt、raw_response、question_ids、error_message、generated_at、published_at、pushed_at。新增 `app_daily_quiz_question_versions` 表记录每日题集中每个 slot 的版本历史，字段包括 set_id、question_id、slot_no、version_no、is_active、题干、选项、维度、权重、source、模型信息、prompt、raw_response、operator、replace_reason。

### 生成流程
服务端提供 `EnsureDailyQuizSet(date)`：若当天题集不存在，优先调用后台管理配置的大模型生成 5 题并写入题库；若配置不可用或模型失败，回退默认题库选题并将状态标记为 `fallback`。题目必须有 4 个选项、每个选项携带 1-9 类型权重，服务端做 JSON 校验。

### 定时流程
服务端定时循环按 Asia/Shanghai 时间运行：11:30/11:40/11:50 尝试提前生成当天题集；12:00 推送前兜底确保题集存在，然后发送每日题推送。其他时间只处理复评报告推送，不重复发送每日题。

### App 读取
`TodayBatchForDate` 优先读取当天题集的 `question_ids`；无题集则兜底生成/选择默认题。App 仍只请求服务端，不直连任何大模型。

### 管理端
新增接口：
- `GET /api/daily-quiz/admin/sets/today`
- `GET /api/daily-quiz/admin/sets?date=YYYY-MM-DD`
- `POST /api/daily-quiz/admin/sets/generate`
- `POST /api/daily-quiz/admin/sets/{setId}/questions/{slotNo}/replace`

页面展示题集状态、5 道题、选项、类型权重、版本、是否已有答题、生成日志；单题更换只替换该 slot，生成新 question_id 和新版本记录。

### 换题保护
- 12:00 前且未推送/无人答题：允许更换。
- 已推送后原则上禁止；如果该题无人答过可允许（服务端以答题数据为准）。
- 已有人回答该 question_id：禁止更换，避免历史 batch 与答案错乱。

### 管理端大模型配置
在现有「模型配置」下保留「模型配对」，新增/扩展管理端大模型配置字段供后台生成每日题使用。后台可看到接口地址、密钥是否已配置、模型名、超时时间；密钥不回显。
