import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-request-'))
const modulePath = join(dir, 'request.mjs')
let source = await (await import('node:fs/promises')).readFile(new URL('./request.js', import.meta.url), 'utf8')
source = source.replace("import { API_BASE } from '../config'", "const API_BASE = 'https://api.test/api'")
await writeFile(modulePath, source)

let storage = {}
let requestCalls = []
let delayedUnauthorizedSuccess = null
globalThis.uni = {
  getStorageSync(key) { return storage[key] || '' },
  setStorageSync(key, value) { storage[key] = value },
  removeStorageSync(key) { delete storage[key] },
  request(options) {
    requestCalls.push(options)
    if (options.url.includes('/network-fail')) {
      options.fail({ errMsg: 'request:fail timeout' })
      return
    }
    if (options.url.includes('/unauthorized')) {
      if (options.url.includes('/unauthorized-delayed')) {
        delayedUnauthorizedSuccess = options.success
        return
      }
      options.success({ statusCode: 401, data: { code: -1, message: 'Unauthorized' } })
      return
    }
    options.success({ statusCode: 200, data: { code: 0, data: { ok: true, url: options.url, header: options.header } } })
  },
}

const { clearToken, getToken, request, setToken } = await import(`file://${modulePath}`)

setToken('abc')
const data = await request({
  url: '/miniapp/report/status',
  query: { testRecordId: 'id 1', empty: '', skip: null },
  auth: true,
})
assert.equal(data.ok, true)
assert.equal(requestCalls[0].url, 'https://api.test/api/miniapp/report/status?testRecordId=id%201&empty=')
assert.equal(requestCalls[0].header.Authorization, 'Bearer abc')

requestCalls = []
await request({
  url: '/app/auth/sms',
  method: 'POST',
  data: { phone: '13800000000' },
})
assert.equal(
  requestCalls[0].url,
  'https://api.test/api/app/auth/sms',
  'BaseURL already includes /api; /app/xxx must not be joined into /api/api/app/xxx',
)

clearToken()
requestCalls = []
await assert.rejects(
  request({ url: '/secure', auth: true }),
  (err) => err.statusCode === 401 && /请先登录/.test(err.message),
)
assert.equal(requestCalls.length, 0, 'auth request without token must not hit network')

setToken('abc')
let tokenObservedByAuthHandler = ''
let currentAuthError = null
await assert.rejects(
  request({ url: '/unauthorized', auth: true }).catch((err) => {
    tokenObservedByAuthHandler = getToken()
    currentAuthError = err
    throw err
  }),
  (err) => err.statusCode === 401 && err.authExpired === true,
)
assert.equal(tokenObservedByAuthHandler, '', '401/403 should preserve immediate token cleanup before rejection handlers run')
assert.equal(currentAuthError.requestToken, 'abc', 'auth errors should identify the token used by the failed request')
assert.equal(currentAuthError.authSessionCurrent, true, 'auth errors should identify when they cleared the still-current request token')
assert.equal(getToken(), '', '401/403 should clear stored token')

setToken('shared-token')
const secondConcurrentUnauthorized = request({ url: '/unauthorized-delayed', auth: true })
await assert.rejects(request({ url: '/unauthorized', auth: true }), (err) => err.authSessionCurrent === true)
delayedUnauthorizedSuccess({ statusCode: 401, data: { code: -1, message: 'Unauthorized' } })
await assert.rejects(
  secondConcurrentUnauthorized,
  (err) => err.statusCode === 401
    && err.requestToken === 'shared-token'
    && err.authSessionCurrent === false,
)
assert.equal(getToken(), '', 'concurrent 401 responses for the same expired token should leave the invalid session cleared')

setToken('token-a')
const staleUnauthorized = request({ url: '/unauthorized-delayed', auth: true })
setToken('token-b')
delayedUnauthorizedSuccess({ statusCode: 401, data: { code: -1, message: 'Unauthorized' } })
await assert.rejects(
  staleUnauthorized,
  (err) => err.statusCode === 401
    && err.authExpired === true
    && err.requestToken === 'token-a'
    && err.authSessionCurrent === false,
)
assert.equal(getToken(), 'token-b', 'a stale 401 response must not clear a newer session token')

await assert.rejects(
  request({ url: '/network-fail' }),
  (err) => err.statusCode === 0 && err.retryable === true && /请求超时/.test(err.message),
)

console.log('request tests passed')
await rm(dir, { force: true, recursive: true })
