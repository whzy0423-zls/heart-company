import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./index.js', import.meta.url), 'utf8')

assert.match(
  source,
  /url:\s*['"]\/app\/auth\/sms\/send['"]/,
  'sendAppSmsApi must call /app/auth/sms/send under the /api BaseURL',
)
assert.match(
  source,
  /url:\s*['"]\/app\/auth\/sms\/login['"]/,
  'loginByAppSmsApi must call /app/auth/sms/login under the /api BaseURL',
)
assert.match(
  source,
  /url:\s*['"]\/app\/push\/register['"]/,
  'registerAppPushApi must call /app/push/register under the /api BaseURL',
)
assert.match(
  source,
  /url:\s*['"]\/app\/push\/unregister['"]/,
  'unregisterAppPushApi must call /app/push/unregister under the /api BaseURL',
)
assert.doesNotMatch(
  source,
  /url:\s*['"]\/api\/app\/auth/,
  'API paths must not include /api because API_BASE already ends with /api',
)

console.log('api index tests passed')
