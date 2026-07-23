# 微信小程序正式登录切换 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将九型芯之力小程序和正式 Go 服务切换到指定微信小程序 AppID 的真实 `code2session` 登录，并安全部署验证。

**Architecture:** 保留现有 `uni.login -> /api/wx/login -> jscode2session -> miniapp JWT` 架构，只更新公开 AppID 配置并补充构建回归检查。AppSecret 仅写入服务器 `.env`，正式环境关闭模拟登录，只重建 Go `server` 服务。

**Tech Stack:** uni-app、Vue 3、Node.js 测试脚本、Go、Docker Compose、微信小程序 `jscode2session`

---

## Chunk 1: 源码、配置与生产部署

### Task 1: 建立 AppID 源码与构建产物回归检查

**Files:**
- Create: `miniapp/scripts/verify-built-wechat-appid.mjs`
- Modify: `miniapp/scripts/project-config.test.mjs`
- Modify: `miniapp/package.json`

- [ ] **Step 1: 新增构建产物验证脚本**

创建 `miniapp/scripts/verify-built-wechat-appid.mjs`，读取 `dist/build/mp-weixin/project.config.json` 并断言 `appid` 等于正式 AppID。脚本只包含公开 AppID，不包含 AppSecret。

```js
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const EXPECTED_WECHAT_APPID = 'wx7d12bddbec8e17f7'
const EXPECTED_API_BASE = 'https://xn--9iq9az5uo8fz16d.com/api'
const BUILD_ROOT = resolve('dist/build/mp-weixin')
const builtConfig = JSON.parse(readFileSync(resolve(BUILD_ROOT, 'project.config.json'), 'utf8'))

assert.equal(
  builtConfig.appid,
  EXPECTED_WECHAT_APPID,
  'built WeChat project config must use the production AppID',
)

function collectJavaScript(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(directory, entry.name)
    if (entry.isDirectory()) return collectJavaScript(path)
    return entry.name.endsWith('.js') ? [readFileSync(path, 'utf8')] : []
  })
}

const builtJavaScript = collectJavaScript(BUILD_ROOT).join('\n')
assert.match(builtJavaScript, new RegExp(EXPECTED_API_BASE.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')))
assert.doesNotMatch(builtJavaScript, /api\.example\.com|yourdomain\.com|\.local\/api/)

console.log('built WeChat AppID and API base verified')
```

- [ ] **Step 2: 将产物验证接入构建生命周期**

在 `miniapp/package.json` 的 scripts 中增加：

```json
"postbuild:mp-weixin": "node scripts/verify-built-wechat-appid.mjs"
```

- [ ] **Step 3: 在源码配置测试中加入 AppID 一致性断言**

在 `miniapp/scripts/project-config.test.mjs` 中定义正式 AppID，并断言：

```js
const EXPECTED_WECHAT_APPID = 'wx7d12bddbec8e17f7'

assert.equal(projectConfig.appid, EXPECTED_WECHAT_APPID)
assert.equal(manifest?.['mp-weixin']?.appid, EXPECTED_WECHAT_APPID)
assert.equal(projectConfig.appid, manifest?.['mp-weixin']?.appid)
```

- [ ] **Step 4: 运行测试和构建，确认 RED**

Run:

```bash
cd miniapp
npm run test:config
npm run build:mp-weixin
```

Expected: 两个命令至少一个因源码或构建产物仍使用旧 AppID 而失败，失败信息指向 AppID 不匹配。

### Task 2: 更新小程序正式 AppID

**Files:**
- Modify: `miniapp/src/manifest.json`
- Modify: `miniapp/project.config.json`
- Test: `miniapp/scripts/project-config.test.mjs`
- Test: `miniapp/scripts/verify-built-wechat-appid.mjs`

- [ ] **Step 1: 更新两个源码配置**

将以下字段都设置为正式 AppID `wx7d12bddbec8e17f7`：

- `miniapp/src/manifest.json` 的 `mp-weixin.appid`
- `miniapp/project.config.json` 的 `appid`

不要把 AppSecret 写入任何小程序文件。

- [ ] **Step 2: 运行配置测试，确认 GREEN**

Run:

```bash
cd miniapp
npm run test:config
```

Expected: `project config tests passed`，其余配置测试全部通过。

- [ ] **Step 3: 构建微信小程序并验证产物**

Run:

```bash
cd miniapp
npm run build:mp-weixin
```

Expected: 构建成功，postbuild 输出 `built WeChat AppID and API base verified`，且生成的 JavaScript 包含正式 API 地址、不包含占位或 `.local` 地址。

- [ ] **Step 4: 确认构建产物不进入 Git**

Run:

```bash
git status --short
git check-ignore miniapp/dist/build/mp-weixin/project.config.json
```

Expected: `dist` 构建产物未出现在待提交文件中，`git check-ignore` 显示该产物被忽略。

### Task 3: 提交并推送公开源码改动

**Files:**
- Modify: `miniapp/src/manifest.json`
- Modify: `miniapp/project.config.json`
- Modify: `miniapp/package.json`
- Modify: `miniapp/scripts/project-config.test.mjs`
- Create: `miniapp/scripts/verify-built-wechat-appid.mjs`
- Create: `docs/superpowers/plans/2026-07-23-wechat-miniapp-login.md`

- [ ] **Step 1: 暂存预期文件**

```bash
git add miniapp/src/manifest.json miniapp/project.config.json miniapp/package.json miniapp/scripts/project-config.test.mjs miniapp/scripts/verify-built-wechat-appid.mjs docs/superpowers/plans/2026-07-23-wechat-miniapp-login.md
```

- [ ] **Step 2: 审查已暂存差异和敏感信息**

Run:

```bash
git diff --cached --check
git diff --cached --stat
git diff --cached -- miniapp docs/superpowers/plans/2026-07-23-wechat-miniapp-login.md
if git diff --cached -U0 -- miniapp | grep -Eq '^\+.*(WECHAT_SECRET|AppSecret)'; then
  echo 'SENSITIVE_KEY_IN_STAGED_MINIAPP'
  exit 1
fi
```

Expected: diff 只包含公开 AppID、测试和文档；对 `miniapp` 的敏感键扫描无输出。若命中，立即停止并移除。计划和命令中不得出现任何真实 Secret 或其片段。

- [ ] **Step 3: 提交源码改动**

```bash
git commit -m "feat: enable production WeChat miniapp login"
```

- [ ] **Step 4: 推送到 main**

```bash
git push origin main
```

Expected: `main` 包含新的小程序 AppID 配置和回归检查，且不包含 AppSecret 或构建产物。

### Task 4: 配置正式服务器微信凭证

**Files:**
- Modify on server: `/opt/heart-company/.env`
- Backup on server: `/opt/heart-company/.env.<timestamp>.bak`

- [ ] **Step 1: 确认服务器工作区和服务状态**

Run:

```bash
cd /opt/heart-company
git status --short
docker compose ps
docker compose ps -q db admin website reading > /tmp/wechat-login-other-container-ids.before
docker compose ps -q server > /tmp/wechat-login-server-container-id.before
```

Expected: 仅保留已知的服务器部署文件；所有现有服务正常运行。

- [ ] **Step 2: 备份 `.env`**

```bash
timestamp=$(date +%Y%m%d-%H%M%S)
cp -a .env ".env.${timestamp}.bak"
chmod 600 ".env.${timestamp}.bak"
```

- [ ] **Step 3: 通过交互输入安全更新微信环境变量**

在 SSH 交互终端中读取 Secret，不把 Secret 放入命令参数、Shell 历史、日志或输出。随后机械更新 `.env` 中的三个键；缺失时追加，存在时替换：

```bash
read -rsp 'WECHAT_SECRET: ' WECHAT_SECRET
printf '\n'
export WECHAT_SECRET
export TARGET_WECHAT_APPID='wx7d12bddbec8e17f7'
tmp=$(mktemp)
awk '
BEGIN { app=0; secret=0; dev=0 }
/^WECHAT_APPID=/ { print "WECHAT_APPID=" ENVIRON["TARGET_WECHAT_APPID"]; app=1; next }
/^WECHAT_SECRET=/ { print "WECHAT_SECRET=" ENVIRON["WECHAT_SECRET"]; secret=1; next }
/^WECHAT_LOGIN_DEV=/ { print "WECHAT_LOGIN_DEV=false"; dev=1; next }
{ print }
END {
  if (!app) print "WECHAT_APPID=" ENVIRON["TARGET_WECHAT_APPID"]
  if (!secret) print "WECHAT_SECRET=" ENVIRON["WECHAT_SECRET"]
  if (!dev) print "WECHAT_LOGIN_DEV=false"
}' .env > "$tmp"
install -m 600 "$tmp" .env
rm -f "$tmp"
unset WECHAT_SECRET TARGET_WECHAT_APPID
```

不得输出 `.env` 内容或 AppSecret。

- [ ] **Step 4: 使用布尔结果验证宿主机配置**

```bash
HOST_APPID=$(awk -F= '$1=="WECHAT_APPID" {value=substr($0,index($0,"=")+1)} END {print value}' .env)
HOST_LOGIN_DEV=$(awk -F= '$1=="WECHAT_LOGIN_DEV" {value=substr($0,index($0,"=")+1)} END {print value}' .env)
test "$HOST_APPID" = 'wx7d12bddbec8e17f7' && echo 'HOST_APPID=MATCH'
grep -Eq '^WECHAT_SECRET=.+$' .env && echo 'HOST_SECRET=SET'
test "$HOST_LOGIN_DEV" = 'false' && echo 'HOST_LOGIN_DEV=false'
unset HOST_APPID HOST_LOGIN_DEV
```

Expected: 只输出 `MATCH`、`SET` 和 `false` 状态，不输出实际 Secret。

- [ ] **Step 5: 拉取提交并仅重建 server**

Run:

```bash
git pull --ff-only origin main
docker compose build server
docker compose up -d --no-deps server
```

Expected: `server` 使用最新提交重新创建，数据库、admin、website、reading 不重建。

- [ ] **Step 6: 验证只有 server 容器被替换**

```bash
docker compose ps -q db admin website reading > /tmp/wechat-login-other-container-ids.after
docker compose ps -q server > /tmp/wechat-login-server-container-id.after
cmp /tmp/wechat-login-other-container-ids.before /tmp/wechat-login-other-container-ids.after
! cmp -s /tmp/wechat-login-server-container-id.before /tmp/wechat-login-server-container-id.after
```

Expected: 其他四个服务容器 ID 完全一致，server 容器 ID 已改变。

### Task 5: 验证正式微信登录链路

**Files:**
- No source changes

- [ ] **Step 1: 验证服务和环境变量状态**

Run:

```bash
docker compose ps server
curl -sS -o /dev/null -w '%{http_code}\n' https://xn--9iq9az5uo8fz16d.com/api/status
```

Expected: server 运行，状态接口返回 200。再执行：

```bash
docker compose exec -T server sh -c '
  test "$WECHAT_APPID" = "wx7d12bddbec8e17f7" &&
  test -n "$WECHAT_SECRET" &&
  test "$WECHAT_LOGIN_DEV" = "false"
' && echo 'CONTAINER_WECHAT_CONFIG=OK'
```

只输出布尔结果，不输出 Secret。

- [ ] **Step 2: 记录登录测试前微信用户数量**

```bash
BEFORE_WX_USERS=$(docker compose exec -T db sh -c '
  PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At \
    -c "SELECT count(*) FROM wx_users"
')
printf 'WX_USERS_BEFORE=%s\n' "$BEFORE_WX_USERS"
```

- [ ] **Step 3: 验证已关闭模拟登录且正式凭证有效**

向 `/api/wx/login` 提交一个无效的一次性 code：

```bash
LOGIN_HTTP=$(curl -sS -o /tmp/wechat-invalid-code-response.json -w '%{http_code}' \
  -H 'Content-Type: application/json' \
  --data '{"code":"invalid-production-verification","channel":"miniapp","scene":"verification"}' \
  https://xn--9iq9az5uo8fz16d.com/api/wx/login)
printf 'LOGIN_HTTP=%s\n' "$LOGIN_HTTP"
test "$LOGIN_HTTP" = '400'
python3 - <<'PY'
import json
d=json.load(open('/tmp/wechat-invalid-code-response.json'))
text=json.dumps(d, ensure_ascii=False).lower()
assert 'accesstoken' not in text
assert '"devmode": true' not in text
assert '40029' in text or 'invalid code' in text
print('REAL_CODE2SESSION_PATH=OK')
PY
```

Expected: HTTP 400；响应包含微信的无效 code 错误（通常为 40029），不包含 `accessToken` 或 `devMode: true`。若返回 AppID/AppSecret 错误，配置验证失败，停止部署结论。

- [ ] **Step 4: 验证无效 code 没有新增用户**

```bash
AFTER_WX_USERS=$(docker compose exec -T db sh -c '
  PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At \
    -c "SELECT count(*) FROM wx_users"
')
printf 'WX_USERS_AFTER=%s\n' "$AFTER_WX_USERS"
test "$BEFORE_WX_USERS" = "$AFTER_WX_USERS"
```

Expected: 前后数量一致。

- [ ] **Step 5: 检查运行日志**

Run:

```bash
docker compose logs --since=10m server
```

Expected: 无 panic、启动失败或数据库错误。

- [ ] **Step 6: 端到端验收说明**

记录：服务器配置和真实微信调用链已验证；最终成功登录仍需要在微信开发者工具或真实小程序中取得有效 `wx.login` code。提醒用户在微信公众平台配置 request 合法域名：

```text
https://九型芯之力.com
```

并将 `miniapp/dist/build/mp-weixin` 导入微信开发者工具完成一次真实登录验收。

### Task 6: 最终验证

**Files:**
- No changes

- [ ] **Step 1: 重新运行完整验证**

Run:

```bash
cd miniapp
npm run test:config
npm run build:mp-weixin
cd ../nx-backend/apps/server
go test ./... -count=1
```

Expected: 全部通过。

- [ ] **Step 2: 验证 Git 和部署提交一致**

Run:

```bash
git status --short
git rev-parse --short HEAD
git rev-parse --short origin/main
```

在正式服务器额外执行：

```bash
cd /opt/heart-company
git fetch origin main
git rev-parse --short HEAD
git rev-parse --short origin/main
git status --short
```

Expected: 本地 HEAD、服务器 HEAD 和各自 `origin/main` 都是同一提交；本地源码无未提交改动，服务器仅保留原有部署专用未跟踪文件。重新执行容器 ID 比较，确认只有 server 被替换。
