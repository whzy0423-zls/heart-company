# 平台数据看板与消息提醒 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 统一官网与微信小程序业务消息，在后台提供可靠未读提醒、可用的业务跳转、小程序客户页面和双平台运营看板。

**Architecture:** PostgreSQL 仍是唯一数据源；新增 `businessmessage` 领域和 tx-aware store，由 signup/miniapp service 统一协调事务。管理端用 `messages` 的 15 秒游标轮询替换旧 signup SSE，所有业务跳转经过权限白名单；现有官网数据概览保留并叠加平台统计。

**Tech Stack:** Go 1.26、PostgreSQL、Vue 3、TypeScript、Pinia、Ant Design Vue、ECharts、Vitest、pnpm。

---

## 文件结构

- `nx-backend/apps/server/internal/db/schema.sql`：幂等数据库升级、历史回填、约束与索引。
- `nx-backend/apps/server/internal/dbtx/dbtx.go`：`*sql.DB`/`*sql.Tx` 共用的最小查询接口。
- `nx-backend/apps/server/internal/businessmessage/`：业务消息规范化与幂等写入。
- `nx-backend/apps/server/internal/privacy/`：手机号和文本脱敏。
- `nx-backend/apps/server/internal/signup/service.go`：官网报名事务边界。
- `nx-backend/apps/server/internal/miniapp/service.go`：小程序登录、测评、预约事务边界。
- `nx-backend/apps/server/internal/miniapp/admin.go`：后台小程序客户只读查询。
- `nx-backend/apps/server/internal/analytics/platform.go`：双平台只读聚合。
- `nx-backend/apps/server/internal/engagement/engagement.go`：消息列表、未读摘要、已读状态。
- `nx-backend/apps/web-antd/src/store/message-notifications.ts`：全局未读会话状态与追赶轮询。
- `nx-backend/apps/web-antd/src/router/business-target.ts`：业务目标白名单、权限和重复跳转。
- `nx-backend/apps/web-antd/src/views/customer/miniapp-users.vue`：小程序客户列表和详情。
- `nx-backend/apps/web-antd/src/views/dashboard/platform-analytics.ts`：平台看板纯数据映射与图表配置。
- `nx-backend/packages/effects/layouts/src/widgets/notification/`：共享顶部通知数量徽标。

## Chunk 1: 数据库、消息领域与原子事务

### Task 1: 数据库迁移与小程序客户 RBAC

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Create: `nx-backend/apps/server/internal/db/schema_platform_notification_test.go`

- [ ] **Step 1: 写失败的迁移规格测试**

测试必须用 `strings.Index` 断言迁移按以下顺序存在：先增加可空字段；回填 `messages` 和 `signups`；记录并清理 orphan/重复消息；增加 CHECK、FK、唯一索引；最后设为非空。再用 `TEST_DATABASE_URL` 建立旧版表和历史数据，连续执行 schema 两次，验证回填结果、重复清理、孤儿置空、FK/CHECK/唯一约束和幂等性。

```go
func TestPlatformNotificationMigrationContract(t *testing.T) {
    raw, err := os.ReadFile("schema.sql")
    if err != nil { t.Fatal(err) }
    schema := string(raw)
    ordered := []string{
        "ADD COLUMN IF NOT EXISTS source_platform",
        "UPDATE signups SET source_platform",
        "INSERT INTO migration_logs",
        "CREATE UNIQUE INDEX IF NOT EXISTS uq_messages_event_business",
        "ALTER COLUMN source_platform SET NOT NULL",
    }
    last := -1
    for _, want := range ordered {
        at := strings.Index(schema, want)
        if at <= last { t.Fatalf("%q missing or out of order", want) }
        last = at
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/db`

Expected: FAIL，提示缺少迁移字段、回填或约束顺序。

- [ ] **Step 3: 实现幂等迁移与授权**

在 `schema.sql` 中先创建幂等 `migration_logs(key,detail,create_time)`，再按明确矩阵处理：

- `business_type='signup'` 且关联 `signups.source_platform='website'`：`platform=website`、`event_key=signup.created`、目标 `/customer/signups?leadId={business_id}&open=detail`。
- `business_type='signup'` 且来源为 miniapp：`platform=miniapp`、`event_key=miniapp.booking.created`、相同报名目标。
- 能从 miniapp user/test 业务类型识别的历史消息回填对应 miniapp 事件与目标。
- 其余消息：`platform=system`、`event_key=system.legacy`、空业务身份用 `message/id`。
- 将孤儿 `bookings.signup_id` 数量和 ID 摘要写入 `migration_logs` 后置 NULL；唯一索引前把重复消息数量写日志并保留最小 ID。

核心迁移片段：

```sql
ALTER TABLE signups ADD COLUMN IF NOT EXISTS source_platform TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS platform TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS event_key TEXT;

UPDATE signups SET source_platform='website' WHERE source_platform IS NULL OR source_platform='';
UPDATE signups s SET source_platform='miniapp'
FROM bookings b WHERE b.signup_id=s.id;

UPDATE messages m SET
  platform=CASE WHEN s.source_platform='miniapp' THEN 'miniapp' ELSE 'website' END,
  event_key=CASE WHEN s.source_platform='miniapp' THEN 'miniapp.booking.created' ELSE 'signup.created' END,
  target_path='/customer/signups?leadId=' || m.business_id || '&open=detail'
FROM signups s
WHERE m.business_type='signup' AND m.business_id=s.id::text;
```

使用 `DO $$` 检查并增加 `source_platform IN ('website','miniapp')`、platform CHECK、`bookings.signup_id REFERENCES signups(id) ON DELETE SET NULL`，最后 `SET NOT NULL`。

- [ ] **Step 4: 运行数据库测试**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/db`

Expected: PASS。

Run: `cd nx-backend/apps/server && TEST_DATABASE_URL='postgres://nx:nx@127.0.0.1:5432/nx_admin_test?sslmode=disable' GOCACHE=/private/tmp/nine-xing-platform-go-cache go test -count=1 ./internal/db -run PlatformNotificationMigration`

Expected: PASS，测试不得 Skip。

- [ ] **Step 5: 中文提交**

```bash
git add nx-backend/apps/server/internal/db/schema.sql nx-backend/apps/server/internal/db/schema_platform_notification_test.go
git commit -m "数据库：增加平台来源与消息幂等迁移"
```

### Task 1B: 小程序客户菜单与升级角色补授权

**Files:**
- Modify: `nx-backend/apps/server/internal/db/db.go`
- Modify: `nx-backend/apps/server/internal/db/menu_test.go`
- Modify: `nx-backend/apps/server/internal/db/seed_test.go`

- [ ] **Step 1: 写失败的 RBAC 测试**

“已有客户查看角色”精确定义为升级前绑定 501、502、504、505、507、508 任一客户只读菜单的非管理员角色；这些角色补 511，无上述绑定的角色不得获得 511。测试管理员、符合条件角色、不符合条件角色和重复 seed 幂等。

- [ ] **Step 2: 运行测试确认红灯原因**

Run: `cd nx-backend/apps/server && TEST_DATABASE_URL='postgres://nx:nx@127.0.0.1:5432/nx_admin_test?sslmode=disable' GOCACHE=/private/tmp/nine-xing-platform-go-cache go test -count=1 ./internal/db -run 'MiniappMenu|CustomerMenuBinding'`

Expected: FAIL，511 菜单或角色绑定尚不存在。

- [ ] **Step 3: 实现菜单和限定补授权**

在 `defaultMenus` 增加固定 ID 511；新增 `seedCustomerMiniappMenuBindings`，用 `INSERT ... SELECT DISTINCT ... ON CONFLICT DO NOTHING` 从上述旧客户菜单绑定派生 511，放在菜单 seed 后、角色 seed 完成前后均保证幂等。

- [ ] **Step 4: 运行测试确认通过**

```bash
cd nx-backend/apps/server
TEST_DATABASE_URL='postgres://nx:nx@127.0.0.1:5432/nx_admin_test?sslmode=disable' GOCACHE=/private/tmp/nine-xing-platform-go-cache go test -count=1 ./internal/db -run 'MiniappMenu|CustomerMenuBinding'
```

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add internal/db
git commit -m "权限：增加小程序客户菜单授权"
```

### Task 2: DBTX、脱敏与业务消息领域

**Files:**
- Create: `nx-backend/apps/server/internal/dbtx/dbtx.go`
- Create: `nx-backend/apps/server/internal/dbtx/dbtx_test.go`
- Create: `nx-backend/apps/server/internal/privacy/mask.go`
- Create: `nx-backend/apps/server/internal/privacy/mask_test.go`
- Create: `nx-backend/apps/server/internal/businessmessage/store.go`
- Create: `nx-backend/apps/server/internal/businessmessage/store_test.go`

- [ ] **Step 1: 写失败测试**

覆盖手机号 `13812345678 -> 138****5678`、文本内手机号脱敏、空 `event_key/business_type/business_id` 拒绝、相同事件写两次只保留一条。

```go
func TestValidateEventRequiresStableIdentity(t *testing.T) {
    err := Validate(Event{EventKey: "", BusinessType: "signup", BusinessID: "1"})
    if err == nil { t.Fatal("expected validation error") }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/dbtx ./internal/privacy ./internal/businessmessage`

Expected: FAIL，包或函数不存在。

- [ ] **Step 3: 实现最小领域接口**

`dbtx.DBTX` 只包含 `ExecContext`、`QueryContext`、`QueryRowContext`；同时定义可替换事务边界：

```go
type Tx interface {
    DBTX
    Commit() error
    Rollback() error
}
type Beginner interface {
    BeginTx(context.Context, *sql.TxOptions) (Tx, error)
}
type SQLBeginner struct { DB *sql.DB }
```

`SQLBeginner.BeginTx` 返回包装后的 `*sql.Tx`，测试使用 fake Tx 观察 commit/rollback。`businessmessage.Event` 包含 `Type/Title/Content/Platform/EventKey/BusinessID/BusinessType/TargetPath`；消息 store 暴露窄接口 `Create(context.Context,dbtx.DBTX,Event)(bool,error)`。构造器只接受本包 DTO 或基础字段，禁止导入 signup/miniapp 包，避免循环依赖：

```go
func WebsiteSignupCreated(id, name, contactLabel, maskedContact string) Event
func MiniappUserCreated(id, displayName string) Event
func MiniappQuizSubmitted(recordID, userID, displayName string, resultType int) Event
func MiniappBookingCreated(bookingID, signupID, displayName, maskedPhone string) Event
```

`Create` 先校验稳定身份，再脱敏内容并执行：

```sql
INSERT INTO messages
  (type,title,content,platform,event_key,business_id,business_type,target_path)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (event_key,business_type,business_id) DO NOTHING
```

提供 `WebsiteSignupCreated`、`MiniappUserCreated`、`MiniappQuizSubmitted`、`MiniappBookingCreated` 四个构造器，目标路径严格匹配规格。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/dbtx ./internal/privacy ./internal/businessmessage`

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add nx-backend/apps/server/internal/dbtx nx-backend/apps/server/internal/privacy nx-backend/apps/server/internal/businessmessage
git commit -m "功能：增加幂等业务消息领域"
```

### Task 3: 官网报名事务与来源字段

**Files:**
- Modify: `nx-backend/apps/server/internal/signup/signup.go`
- Create: `nx-backend/apps/server/internal/signup/service.go`
- Create: `nx-backend/apps/server/internal/signup/service_test.go`
- Modify: `nx-backend/apps/server/internal/signup/signup_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: 写失败的事务测试**

定义并用 fake 覆盖以下窄接口：signup writer、message writer、`dbtx.Beginner`/`dbtx.Tx`。断言 signup 成功+消息失败会 rollback 且不 commit；commit 失败返回错误；全部成功只 commit 一次；官网消息目标含 `leadId`；客户端不能伪造 `sourcePlatform`。

```go
type leadWriter interface {
    CreateWithDBTX(context.Context, dbtx.DBTX, LeadInput, *http.Request, string) (Lead, error)
}
type messageWriter interface {
    Create(context.Context, dbtx.DBTX, businessmessage.Event) (bool, error)
}
func NewService(beginner dbtx.Beginner, leads leadWriter, messages messageWriter) *Service
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/signup ./internal/server -run 'WebsiteSignup|PublicSignup'`

Expected: FAIL，提示 `NewService`、`CreateWithDBTX` 或 `SourcePlatform` 不存在。

- [ ] **Step 3: 拆分 tx-aware store 和 service**

`Store.CreateWithDBTX(ctx,q,input,request,sourcePlatform)` 只做校验和 INSERT，不开事务、不写消息。`Service.CreateWebsiteSignup`：

```go
tx, err := s.db.BeginTx(ctx, nil)
if err != nil { return Lead{}, err }
defer tx.Rollback()
lead, err := s.store.CreateWithDBTX(ctx, tx, input, r, "website")
if err != nil { return Lead{}, err }
event := businessmessage.WebsiteSignupCreated(lead.ID, lead.Name, contactTypeLabel(lead.ContactType), privacy.MaskPhone(lead.Contact))
if _, err = s.messages.Create(ctx, tx, event); err != nil {
    return Lead{}, err
}
if err = tx.Commit(); err != nil { return Lead{}, err }
return lead, nil
```

`Lead`、List、Detail 增加 `SourcePlatform`。`publicSignup` 调 service，成功后才 `broadcastSignup`。

- [ ] **Step 4: 运行相关测试**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/signup ./internal/server -run 'WebsiteSignup|PublicSignup'`

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add nx-backend/apps/server/internal/signup nx-backend/apps/server/internal/server/server.go
git commit -m "修复：保证官网报名与消息原子写入"
```

### Task 4A: 小程序首次登录事务

**Files:**
- Modify: `nx-backend/apps/server/internal/miniapp/miniapp.go`
- Create: `nx-backend/apps/server/internal/miniapp/service.go`
- Create: `nx-backend/apps/server/internal/miniapp/service_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/miniapp_handlers.go`
- Modify: `nx-backend/apps/server/internal/server/miniapp_handlers_test.go`

- [ ] **Step 1: 写失败的首次登录测试**

为 miniapp service 定义 tx-aware `userWriter` 和 `messageWriter`。测试首次 INSERT 返回 `created=true` 并写消息；重复登录 `created=false` 不写消息；消息失败 rollback；并发集成测试最终只有一个用户和一条 `miniapp.user.created`。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/miniapp ./internal/server -run 'MiniappUser'`

Expected: FAIL，提示 `UpsertUser` 或 created 返回值不存在。

- [ ] **Step 3: 实现登录事务**

miniapp service 的边界一次定义完整：

```go
type userWriter interface {
    UpsertByOpenIDWithDBTX(context.Context, dbtx.DBTX, string, string, string, string) (id int64, created bool, err error)
    GetUserWithDBTX(context.Context, dbtx.DBTX, int64) (User, error)
}
type testRecordWriter interface {
    InsertTestRecord(context.Context, dbtx.DBTX, int64, TestRecordInput) (TestRecord, error)
    UpdateMainType(context.Context, dbtx.DBTX, int64, int) error
}
type bookingWriter interface {
    InsertBooking(context.Context, dbtx.DBTX, int64, BookingInput, int64) (Booking, error)
}
type signupWriter interface {
    CreateWithDBTX(context.Context, dbtx.DBTX, signup.LeadInput, *http.Request, string) (signup.Lead, error)
}
type messageWriter interface {
    Create(context.Context, dbtx.DBTX, businessmessage.Event) (bool, error)
}
func NewService(beginner dbtx.Beginner, users userWriter, tests testRecordWriter, bookings bookingWriter, signups signupWriter, messages messageWriter) *Service
func (s *Service) UpsertUser(context.Context, string, string, string, string) (int64, error)
func (s *Service) SaveTestRecord(context.Context, int64, TestRecordInput) (TestRecord, error)
func (s *Service) CreateBooking(context.Context, int64, BookingInput, *http.Request) (BookingResult, error)
```

Server 同时保留 `miniapp *miniapp.Store` 供只读方法，新增 `miniappService *miniapp.Service` 供三条写入 handler。首次登录用 `INSERT ... ON CONFLICT DO NOTHING RETURNING id` 判断 `created`，冲突时再 `UPDATE ... RETURNING id`，不使用 `xmax`；仅首次插入写 `MiniappUserCreated(id,displayName)`。

- [ ] **Step 4: 运行相关测试**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/miniapp ./internal/server -run 'MiniappUser'`

Expected: PASS。

- [ ] **Step 5: 运行首次登录并发数据库门禁**

Run: `cd nx-backend/apps/server && TEST_DATABASE_URL='postgres://nx:nx@127.0.0.1:5432/nx_admin_test?sslmode=disable' GOCACHE=/private/tmp/nine-xing-platform-go-cache go test -count=1 ./internal/miniapp -run 'MiniappUserConcurrent'`

Expected: PASS 且不得 Skip；最终一条 wx_user 和一条 user-created 消息。

- [ ] **Step 6: 中文提交**

```bash
git add nx-backend/apps/server/internal/miniapp nx-backend/apps/server/internal/server
git commit -m "功能：增加小程序新用户消息事务"
```

### Task 4B: 小程序测评事务

**Files:**
- Modify: `nx-backend/apps/server/internal/miniapp/miniapp.go`
- Modify: `nx-backend/apps/server/internal/miniapp/service.go`
- Modify: `nx-backend/apps/server/internal/miniapp/service_test.go`
- Modify: `nx-backend/apps/server/internal/server/miniapp_handlers.go`
- Modify: `nx-backend/apps/server/internal/server/miniapp_handlers_test.go`

- [ ] **Step 1: 写失败的测评原子性测试**

定义 `testRecordWriter` 的“写记录”和“更新 main_type”两个 tx-aware 方法；分别注入第二步失败、消息失败、commit 失败，断言全部 rollback。成功时消息身份为 `recordID/userID`，正文不含完整手机号/openid。

- [ ] **Step 2: 运行测试确认红灯原因**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/miniapp ./internal/server -run 'MiniappTestRecord'`

Expected: FAIL，现有 `SaveTestRecord` 自行开事务且不写消息。

- [ ] **Step 3: 把事务提升到 service**

store 方法只接受 `dbtx.DBTX`；service 按“写 test_records → 更新 wx_users.main_type → 写 MiniappQuizSubmitted → commit”执行，handler 保持原有 TestRecord JSON 响应。

- [ ] **Step 4: 运行测试确认通过**

```bash
cd nx-backend/apps/server
GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/miniapp ./internal/server -run 'MiniappTestRecord'
```

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add internal/miniapp internal/server
git commit -m "修复：保证小程序测评与消息原子写入"
```

### Task 4C: 小程序预约事务

**Files:**
- Modify: `nx-backend/apps/server/internal/miniapp/miniapp.go`
- Modify: `nx-backend/apps/server/internal/miniapp/service.go`
- Modify: `nx-backend/apps/server/internal/miniapp/service_test.go`
- Modify: `nx-backend/apps/server/internal/server/miniapp_handlers.go`
- Modify: `nx-backend/apps/server/internal/server/miniapp_handlers_test.go`

- [ ] **Step 1: 写失败的三表事务测试**

分别注入 signup、booking、message、commit 失败，断言 rollback；成功只写 `miniapp.booking.created`，signup 为 miniapp 来源并绑定 booking。handler 测试必须断言 HTTP 响应仍是现有 `Booking` JSON，不暴露内部 `BookingResult{Booking,Lead}`。

- [ ] **Step 2: 运行测试确认红灯原因**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/miniapp ./internal/server -run 'MiniappBooking'`

Expected: FAIL，现 handler 吞 signup 错误并在事务外创建 booking。

- [ ] **Step 3: 实现预约 service 与 commit 后广播**

service 依次创建 `source_platform=miniapp` signup、booking、`MiniappBookingCreated(bookingID,signupID,displayName,maskedPhone)`，成功 commit 后返回内部结果；handler 只 `httpx.OK(w,result.Booking)`，随后才 `broadcastSignup(result.Lead)`。

- [ ] **Step 4: 运行测试确认通过**

```bash
cd nx-backend/apps/server
GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/miniapp ./internal/server -run 'MiniappBooking'
```

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add internal/miniapp internal/server
git commit -m "修复：保证小程序预约与消息原子写入"
```

## Chunk 2: 后台查询 API 与权限

### Task 5: 消息列表、游标未读摘要和已读生命周期

**Files:**
- Modify: `nx-backend/apps/server/internal/engagement/engagement.go`
- Modify: `nx-backend/apps/server/internal/engagement/engagement_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Create: `nx-backend/apps/server/internal/server/messages_test.go`

- [ ] **Step 1: 写失败的契约测试**

覆盖 platform 筛选、`afterId` 空值视为 0、非法/负数/溢出返回 400、超前游标返回空 items、默认单页 50、`limit` 只允许 1..100、`limit+1` 判定 `hasMore`、`nextAfterId`、全表最大 `latestId`、ID JSON 字符串、响应不含 content/openid/unionid。全部已读唯一合法请求为 `{ids:[],read:true}`；`{ids:[],read:false}` 必须 400，禁止把全表改回未读。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/engagement ./internal/server -run 'Message|Unread'`

Expected: FAIL，路由未注册或 `UnreadSummary`/platform 字段不存在；不得因测试筛选未命中而假绿。

- [ ] **Step 3: 实现查询和路由**

新增完整 DTO：

```go
type UnreadSummary struct {
    Count int `json:"count"`
    LatestID string `json:"latestId"`
    Items []UnreadSummaryItem `json:"items"`
    HasMore bool `json:"hasMore"`
    NextAfterID string `json:"nextAfterId"`
}
type UnreadSummaryItem struct {
    ID string `json:"id"`
    Title string `json:"title"`
    Summary string `json:"summary"`
    Platform string `json:"platform"`
    EventKey string `json:"eventKey"`
    BusinessType string `json:"businessType"`
    TargetPath string `json:"targetPath"`
    CreateTime string `json:"createTime"`
}
```

`Summary` 由消息 content 脱敏后截取，不返回原始 content。handler 使用 `parseNonNegativeDecimalID` 和 `parseUnreadLimit`，空 afterId=0，溢出/非法统一返回 400；超前游标自然返回空 items，nextAfterId 保持传入值。查询 `count(*) FILTER (WHERE is_read=false)` 与全表 `max(id)`；items 使用 `WHERE is_read=false AND id>$after ORDER BY id ASC LIMIT limit+1`。注册：

```go
s.mux.HandleFunc("/api/messages/unread-summary",
    s.method(http.MethodGet,
        s.requirePermission("Message:Manage:List", s.messagesUnreadSummary)))
```

Messages 构造 WHERE 时加入 `platform=$n`（只接受 website/miniapp/system）；SELECT 增加 platform/event_key，并在 DTO 输出前调用脱敏函数。mark handler 严格校验 IDs 都是正十进制字符串；空 IDs 仅允许 `read=true`。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/engagement ./internal/server -run 'Message|Unread'`

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add nx-backend/apps/server/internal/engagement nx-backend/apps/server/internal/server
git commit -m "功能：增加消息未读游标接口"
```

### Task 6: 小程序客户后台 API

**Files:**
- Create: `nx-backend/apps/server/internal/miniapp/admin.go`
- Create: `nx-backend/apps/server/internal/miniapp/admin_test.go`
- Create: `nx-backend/apps/server/internal/server/miniapp_admin.go`
- Create: `nx-backend/apps/server/internal/server/miniapp_admin_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: 写失败的列表、详情和权限测试**

列表参数为 `page/pageSize/keyword/channel`，默认 1/20、最大 100；keyword 匹配 nickname、脱敏前 phone、channel、scene。详情参数固定为 `testPage/testPageSize/bookingPage/bookingPageSize`，各默认 1/20、最大 100；响应为 `{user,testRecords:{items,total},bookings:{items,total}}`。断言无 openid/unionid/完整手机号，预约返回 `signupId`，无权限为 403。非法/溢出 ID 或分页为 400，不存在为 404，数据库错误为 500。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/miniapp ./internal/server -run 'AdminMiniapp|MiniappUsers'`

Expected: FAIL，两个管理端路由尚未注册或 admin 查询方法不存在。

- [ ] **Step 3: 实现只读查询与路由**

`admin.go` 只 SELECT 展示所需列，手机号在 DTO 层脱敏。Go ServeMux 同时注册集合和详情前缀：

```text
GET /api/miniapp/users
GET /api/miniapp/users/{id}
Customer:Miniapp:List
```

```go
s.mux.HandleFunc("/api/miniapp/users", s.method(http.MethodGet,
    s.requirePermission("Customer:Miniapp:List", s.miniappUsers)))
s.mux.HandleFunc("/api/miniapp/users/", s.method(http.MethodGet,
    s.requirePermission("Customer:Miniapp:List", s.miniappUserByID)))
```

不存在返回 404；非法分页返回 400；两个子列表独立分页。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/miniapp ./internal/server -run 'AdminMiniapp|MiniappUsers'`

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add nx-backend/apps/server/internal/miniapp nx-backend/apps/server/internal/server
git commit -m "功能：增加小程序客户后台查询"
```

### Task 7: 双平台运营统计 API

**Files:**
- Modify: `nx-backend/apps/server/internal/analytics/analytics.go`
- Create: `nx-backend/apps/server/internal/analytics/platform.go`
- Create: `nx-backend/apps/server/internal/analytics/platform_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Create: `nx-backend/apps/server/internal/server/platform_analytics_test.go`

- [ ] **Step 1: 写失败统计测试**

固定 `now` 覆盖 Asia/Shanghai 零点；断言官网总访客/新增访客/活跃访客/PV/今日 PV、仅 website signup、转化率；小程序用户/测评/预约；7/30 天补零；最近动态最多 10 条且预约不重复成官网报名；手机号脱敏。server 测试覆盖路由、401/403、缺省 days=7、days=7/30、非法 days=400。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/analytics ./internal/server -run 'PlatformOverview|AnalyticsOverview'`

Expected: FAIL，`PlatformOverview` 或路由不存在；非法 days 当前不能得到预期 400。

- [ ] **Step 3: 实现平台查询并修正旧官网口径**

新增 `PlatformOverview(ctx,days,now)`；days 只接受 7/30，领域返回可识别的 `ErrInvalidDays`，handler 映射 400。响应契约固定为：

```go
type PlatformOverview struct {
    Website struct {
        TotalUsers int `json:"totalUsers"`
        NewUsersToday int `json:"newUsersToday"`
        ActiveUsersToday int `json:"activeUsersToday"`
        TotalPV int `json:"totalPV"`
        TodayPV int `json:"todayPV"`
        TotalSubmissions int `json:"totalSubmissions"`
        SubmissionsToday int `json:"submissionsToday"`
        ConversionRate float64 `json:"conversionRate"`
    } `json:"website"`
    Miniapp struct {
        TotalUsers int `json:"totalUsers"`
        NewUsersToday int `json:"newUsersToday"`
        TotalTests int `json:"totalTests"`
        TestsToday int `json:"testsToday"`
        TotalBookings int `json:"totalBookings"`
        BookingsToday int `json:"bookingsToday"`
    } `json:"miniapp"`
    Series []struct {
        Date string `json:"date"`
        WebsiteActiveUsers int `json:"websiteActiveUsers"`
        MiniappNewUsers int `json:"miniappNewUsers"`
        WebsiteSubmissions int `json:"websiteSubmissions"`
        MiniappTests int `json:"miniappTests"`
        MiniappBookings int `json:"miniappBookings"`
    } `json:"series"`
    RecentActivities []RecentActivity `json:"recentActivities"`
}
```

转化率零分母返回 0，非零保留一位小数。`RecentActivity` 固定返回：

```go
type RecentActivity struct {
    ID string `json:"id"`
    EventKey string `json:"eventKey"`
    Title string `json:"title"`
    Summary string `json:"summary"`
    TargetPath string `json:"targetPath"`
    CreateTime string `json:"createTime"`
    Platform string `json:"platform"`
}
```

给现有 `Overview/rangeTotals/series` 的官网报名统计增加 `source_platform='website'`。`followupStats/followupItems` 继续包含 website 和 miniapp 两种来源，避免小程序预约从运营工作台消失；`FollowupItem` 增加 `sourcePlatform`，前端显示平台标签。注册 `GET /api/analytics/platform-overview?days=7|30`，权限 `Analytics:Overview`。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./internal/analytics ./internal/server -run 'PlatformOverview|AnalyticsOverview'`

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add nx-backend/apps/server/internal/analytics nx-backend/apps/server/internal/server/server.go nx-backend/apps/server/internal/server/platform_analytics_test.go
git commit -m "功能：增加官网与小程序平台统计"
```

## Chunk 3: 管理端消息体验、客户页与看板

### Task 8: 前端消息 API、十进制 ID 与通知 store

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/message.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/message.test.ts`
- Create: `nx-backend/apps/web-antd/src/layouts/message-notice.ts`
- Create: `nx-backend/apps/web-antd/src/layouts/message-notice.test.ts`
- Create: `nx-backend/apps/web-antd/src/store/message-notifications.ts`
- Create: `nx-backend/apps/web-antd/src/store/message-notifications.test.ts`

- [ ] **Step 1: 写失败测试**

覆盖 `9007199254740993` 排序、首次只同步 count/latestId 不弹历史、后续分页追赶、网络错误保留数量、单条/全部已读失败回滚、旧 localStorage key 只清理一次。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend && pnpm test:unit apps/web-antd/src/api/core/message.test.ts apps/web-antd/src/layouts/message-notice.test.ts apps/web-antd/src/store/message-notifications.test.ts`

Expected: FAIL，提示 unread API、十进制 ID helper 或 message store 不存在。

- [ ] **Step 3: 实现 API 和 store**

十进制比较用“去前导零后长度，再字典序”，禁止 Number/parseInt/Math.max。`message-notice.ts` 明确负责：

```ts
compareDecimalIds(a: string, b: string): number
toNotificationItem(item: UnreadMessageItem): NotificationItem
dedupeUnreadItems(items: UnreadMessageItem[]): UnreadMessageItem[]
removeLegacySignupNoticeStorage(storage: Storage, origin: string): void
```

映射保留 `messageId/platform/eventKey/targetPath` 自定义字段，title 使用服务端 title，message 使用服务端安全 summary，link 不直接信任而由业务跳转器处理。store 状态为 `unreadCount/notifications/afterId/bootstrapped`；`pollUnread` 后续循环请求直到 `hasMore=false`。`markOneRead/markAllRead/clearNotifications` 仅在 API 成功后提交本地状态。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2。

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add nx-backend/apps/web-antd/src/api/core/message* nx-backend/apps/web-antd/src/layouts/message-notice* nx-backend/apps/web-antd/src/store/message-notifications*
git commit -m "功能：增加后台消息轮询状态管理"
```

### Task 9: 顶部数量徽标并替换旧 signup SSE

**Files:**
- Create: `nx-backend/packages/effects/layouts/src/widgets/notification/notification-count.ts`
- Create: `nx-backend/packages/effects/layouts/src/widgets/notification/notification-count.test.ts`
- Modify: `nx-backend/packages/effects/layouts/src/widgets/notification/notification.vue`
- Create: `nx-backend/apps/web-antd/src/layouts/message-polling.ts`
- Create: `nx-backend/apps/web-antd/src/layouts/message-polling.test.ts`
- Modify: `nx-backend/apps/web-antd/src/layouts/basic.vue`
- Delete: `nx-backend/apps/web-antd/src/layouts/signup-events.ts`
- Delete: `nx-backend/apps/web-antd/src/layouts/signup-events.test.ts`
- Delete: `nx-backend/apps/web-antd/src/layouts/signup-notice.ts`
- Delete: `nx-backend/apps/web-antd/src/layouts/signup-notice.test.ts`

- [ ] **Step 1: 写失败徽标与布局测试**

断言 0 不显示、1..99 原样、100 显示 `99+`。抽取 `createMessagePollingController({poll,canPoll,isHidden,setInterval,clearInterval})`，用 fake timer 测试立即请求、15 秒间隔、隐藏不请求、恢复立即请求、stop 清理。静态断言 `basic.vue` 使用 `Message:Manage:List` 且不再出现 `/signups/events`、Math.max、旧 key 作为游标。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend && pnpm test:unit packages/effects/layouts/src/widgets/notification/notification-count.test.ts apps/web-antd/src/layouts/message-polling.test.ts apps/web-antd/src/store/message-notifications.test.ts`

Expected: FAIL，`formatNotificationCount`/polling controller 不存在或 basic 仍包含旧 signup 连接。

- [ ] **Step 3: 实现共享 count prop 和布局轮询**

Notification 保留 `dot` 兼容，新增 `count?: number`；basic 传 `:count="unreadCount"`。布局只负责把权限/token/visibility 接入 polling controller；token/用户变化重置会话；弹窗只展示本轮新增 items。read/make-all/clear 调 store，失败提示但不清空。

- [ ] **Step 4: 验证旧链路移除**

Run: `cd nx-backend && rg -n "/signups/events|buildSignupEventsURL|connectSignupEvents" apps/web-antd/src`

Expected: 无匹配。

- [ ] **Step 5: 运行测试并中文提交**

```bash
pnpm test:unit packages/effects/layouts/src/widgets/notification/notification-count.test.ts apps/web-antd/src/layouts/message-polling.test.ts apps/web-antd/src/store/message-notifications.test.ts
git add nx-backend/packages/effects/layouts/src/widgets/notification nx-backend/apps/web-antd/src/layouts/basic.vue nx-backend/apps/web-antd/src/layouts/message-polling* nx-backend/apps/web-antd/src/layouts/signup-*
git commit -m "修复：统一后台消息提醒并显示未读数量"
```

### Task 10: 安全业务跳转、消息管理与报名深链

**Files:**
- Create: `nx-backend/apps/web-antd/src/router/business-target.ts`
- Create: `nx-backend/apps/web-antd/src/router/business-target.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/message/management.vue`
- Create: `nx-backend/apps/web-antd/src/views/message/management.test.ts`
- Modify: `nx-backend/apps/web-antd/src/layouts/basic.vue`
- Create: `nx-backend/apps/web-antd/src/layouts/basic-business-target.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/site-config/signup-leads.vue`
- Create: `nx-backend/apps/web-antd/src/views/site-config/signup-leads.test.ts`

- [ ] **Step 1: 写失败测试**

严格白名单：signups/miniapp-users/user-insights；拒绝跨域和前缀伪造；校验权限；每次增加 `_businessOpen`。消息页先标已读后跳转，失败不跳；`forbidden` 显示“无权查看该业务”，`invalid` 显示“无法打开该业务”，目标 404 显示“业务记录不存在或已删除”。顶部通知调用同一 helper 并显示相同提示。报名页当前页/跨页自动打开，同 leadId 新 token 再打开，404 提示。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend && pnpm test:unit apps/web-antd/src/router/business-target.test.ts apps/web-antd/src/views/message/management.test.ts apps/web-antd/src/layouts/basic-business-target.test.ts apps/web-antd/src/views/site-config/signup-leads.test.ts`

Expected: FAIL，业务目标 helper 不存在，且消息页/报名页不能满足错误提示和重复打开断言。

- [ ] **Step 3: 实现统一跳转与深链**

`parseBusinessTarget` 用 `new URL(target, origin)` 严格比较 origin/pathname；权限不足返回 `forbidden`，非法返回 `invalid`。提供 `businessTargetErrorMessage(result)` 供消息页、顶部通知、最近动态共用。basic 的顶部通知点击不再直接 router.push，而是调用统一 helper 并显示错误信息；对应测试静态/组件断言调用存在。消息页增加平台筛选和标签。报名页监听 `leadId/open/_businessOpen`，不在当前页直接 `getSignupDetailApi`。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2。

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add nx-backend/apps/web-antd/src/router/business-target* nx-backend/apps/web-antd/src/views/message/management* nx-backend/apps/web-antd/src/layouts/basic.vue nx-backend/apps/web-antd/src/layouts/basic-business-target.test.ts nx-backend/apps/web-antd/src/views/site-config/signup-leads*
git commit -m "修复：让消息业务跳转可用且安全"
```

### Task 11: 小程序客户管理页面

**Files:**
- Create: `nx-backend/apps/web-antd/src/api/core/miniapp-customer.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/miniapp-customer.test.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/index.ts`
- Create: `nx-backend/apps/web-antd/src/views/customer/miniapp-user.ts`
- Create: `nx-backend/apps/web-antd/src/views/customer/miniapp-user.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/customer/miniapp-users.vue`
- Create: `nx-backend/apps/web-antd/src/views/customer/miniapp-users.test.ts`

- [ ] **Step 1: 写失败的 API 与页面测试**

覆盖关键词/渠道/分页、详情两个独立分页、脱敏字段、`userId/open=detail`、`testRecordId/open=test`、`_businessOpen` 重开、404、预约跳报名详情。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend && pnpm test:unit apps/web-antd/src/api/core/miniapp-customer.test.ts apps/web-antd/src/views/customer/miniapp-user.test.ts apps/web-antd/src/views/customer/miniapp-users.test.ts`

Expected: FAIL，miniapp customer API、页面或深链 helper 不存在。

- [ ] **Step 3: 实现页面**

复用 `PageShell + Alert + Card + Table + Drawer`，用 requestId 防旧响应覆盖。详情响应固定为 `user/testRecords/bookings` 三段；测评与预约分页分别请求；预约的 signupId 使用统一业务跳转器。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2。

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add nx-backend/apps/web-antd/src/api/core nx-backend/apps/web-antd/src/views/customer/miniapp-user*
git commit -m "功能：增加小程序客户管理页面"
```

### Task 12A: 双平台数据概览

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/analytics.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/analytics.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/dashboard/platform-analytics.ts`
- Create: `nx-backend/apps/web-antd/src/views/dashboard/platform-analytics.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/dashboard/analytics.vue`
- Create: `nx-backend/apps/web-antd/src/views/dashboard/analytics.platform.test.ts`

- [ ] **Step 1: 写失败测试**

断言现有 PV/今日 PV/转化率和跟进工作台保留。官网卡精确显示：累计访客、今日新增访客、今日活跃访客、累计 PV、今日 PV、累计报名、今日报名、报名转化率；小程序卡精确显示：累计用户、今日新增、累计测评、今日测评、累计预约、今日预约，不虚构小程序活跃指标。覆盖 7/30 参数、空态、两张图五类序列、快速切换只采用最后响应。最近动态 forbidden/invalid 必须分别提示“无权查看该业务”/“无法打开该业务”。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd nx-backend && pnpm test:unit apps/web-antd/src/api/core/analytics.test.ts apps/web-antd/src/views/dashboard/platform-analytics.test.ts apps/web-antd/src/views/dashboard/analytics.platform.test.ts`

Expected: FAIL，平台统计 API 类型/卡片映射/页面区域尚不存在。

- [ ] **Step 3: 实现平台 UI**

保留 `getAnalyticsOverviewApi`，并行调用 `getPlatformAnalyticsOverviewApi({days})`。`platform-analytics.ts` 只负责卡片映射、空值和两个 chart option；页面负责 requestId、渲染、dispose 和业务跳转。跟进列表按 `sourcePlatform` 显示官网/小程序标签。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2。

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add nx-backend/apps/web-antd/src/api/core/analytics* nx-backend/apps/web-antd/src/views/dashboard
git commit -m "功能：升级官网与小程序数据看板"
```

### Task 12B: 显式环境标签与数据来源说明

**Files:**
- Create: `nx-backend/apps/web-antd/src/components/environment-label/environment-label.ts`
- Create: `nx-backend/apps/web-antd/src/components/environment-label/environment-label.vue`
- Create: `nx-backend/apps/web-antd/src/components/environment-label/environment-label.test.ts`
- Modify: `nx-backend/apps/web-antd/src/layouts/basic.vue`
- Modify: `nx-backend/apps/web-antd/.env.development`
- Modify: `nx-backend/apps/web-antd/.env.production`
- Modify: `nx-backend/apps/web-antd/.env.analyze`

- [ ] **Step 1: 写失败测试**

只读取 `VITE_APP_ENV_KIND/LABEL`；空标签不显示；development 显示“本地开发”和完整说明“本地后台只显示本地数据库数据，线上小程序提交不会进入本地库。”；不得读取 `import.meta.env.PROD` 决定显示。

- [ ] **Step 2: 运行测试确认红灯原因**

Run: `cd nx-backend && pnpm test:unit apps/web-antd/src/components/environment-label/environment-label.test.ts`

Expected: FAIL，环境组件尚不存在。

- [ ] **Step 3: 实现组件和环境值**

开发设置 `development/本地开发`，生产设置 `production/空标签`，analyze 设置 `analyze/分析构建`；basic 的 header 扩展槽接入组件，Tooltip/Popover 展示来源说明。

- [ ] **Step 4: 运行测试确认通过**

```bash
cd nx-backend
pnpm test:unit apps/web-antd/src/components/environment-label/environment-label.test.ts
```

Expected: PASS。

- [ ] **Step 5: 中文提交**

```bash
git add apps/web-antd/src/components/environment-label apps/web-antd/src/layouts/basic.vue apps/web-antd/.env.*
git commit -m "功能：增加后台环境与数据来源提示"
```

### Task 13: 全量验证与本地非 Docker 联调

**Files:**
- Modify if needed: `docs/superpowers/plans/2026-07-23-platform-notification-analytics.md`（只勾选步骤和记录实际验证）

本任务不修改用户主工作区的 `miniapp/.env.development`。小程序业务造数直接调用本地 API；如需启动小程序开发服务器，只在启动命令前临时传 `VITE_API_BASE=http://127.0.0.1:5320/api`，不写入文件、不提交配置。

- [ ] **Step 1: 后端全量测试**

Run: `cd nx-backend/apps/server && GOCACHE=/private/tmp/nine-xing-platform-go-cache go test ./...`

Expected: PASS。

- [ ] **Step 2: 前端全量测试、类型和构建**

```bash
cd nx-backend
pnpm test:unit
pnpm --filter @vben/web-antd typecheck
pnpm --filter @vben/web-antd build
```

Expected: 全部退出码 0。

- [ ] **Step 3: 启动本地服务，不使用 Docker**

终端 A：

```bash
cd nx-backend/apps/server
PORT=5320 DATABASE_URL='postgres://nx:nx@127.0.0.1:5432/nx_admin?sslmode=disable' \
ADMIN_USERNAME=admin ADMIN_PASSWORD=123456 JWT_SECRET='local-platform-notification-secret' \
WECHAT_LOGIN_DEV=true APP_ENV=dev \
go run ./cmd/server
```

终端 B：

```bash
cd nx-backend
pnpm dev:antd
```

健康检查：

```bash
curl -sS http://127.0.0.1:5320/api/status
curl -sS -I http://127.0.0.1:5666
```

Expected: 后端 JSON `code=0`，前端 HTTP 200。管理端地址 `http://127.0.0.1:5666`。

- [ ] **Step 4: 执行端到端验收**

先记录基线：

```bash
psql 'postgres://nx:nx@127.0.0.1:5432/nx_admin?sslmode=disable' -Atc \
"SELECT (SELECT count(*) FROM signups),(SELECT count(*) FROM wx_users),(SELECT count(*) FROM test_records),(SELECT count(*) FROM bookings),(SELECT count(*) FROM messages);"
```

依次造数：

```bash
curl -sS -X POST http://127.0.0.1:5320/api/public/signups \
  -H 'Content-Type: application/json' \
  -d '{"name":"官网联调用户","contact":"13800000001","contactType":"phone","interest":"平台提醒联调"}'

WX_CODE="platform-notification-$(date +%s)"
WX_LOGIN_JSON=$(curl -sS -X POST http://127.0.0.1:5320/api/wx/login \
  -H 'Content-Type: application/json' \
  -d "{\"code\":\"$WX_CODE\",\"channel\":\"codex\",\"scene\":\"local\"}")
WX_TOKEN=$(printf '%s' "$WX_LOGIN_JSON" | jq -r '.data.accessToken')
test -n "$WX_TOKEN" && test "$WX_TOKEN" != "null"
```

用相同 code 再登录一次，验证不重复创建用户消息，然后提交测评和预约：

```bash
curl -sS -X POST http://127.0.0.1:5320/api/wx/login \
  -H 'Content-Type: application/json' \
  -d "{\"code\":\"$WX_CODE\",\"channel\":\"codex\",\"scene\":\"local\"}"

curl -sS -X POST http://127.0.0.1:5320/api/miniapp/test-records \
  -H "Authorization: Bearer $WX_TOKEN" -H 'Content-Type: application/json' \
  -d '{"gender":"unknown","resultType":5,"secondType":6,"scores":{},"centers":[]}'

curl -sS -X POST http://127.0.0.1:5320/api/miniapp/bookings \
  -H "Authorization: Bearer $WX_TOKEN" -H 'Content-Type: application/json' \
  -d '{"kind":"consult","contactName":"小程序联调用户","phone":"13800000002","intent":"平台提醒联调","message":"本地验收"}'
```

Expected 基线增量：signups +2、wx_users +1、test_records +1、bookings +1、messages +4；第二次相同 WX_CODE 登录不增加用户或消息。数据库中两个 signup 来源分别为 website/miniapp，消息正文手机号已脱敏。浏览器验证 15 秒内顶部数量更新、每条只弹一次、消息页与最近动态可打开详情、无权限/非法/已删除目标有明确提示，7/30 天看板不串平台。

- [ ] **Step 5: 最终中文提交**

```bash
git add docs/superpowers/plans/2026-07-23-platform-notification-analytics.md
git commit -m "验证：完成平台提醒与数据看板验收"
```
