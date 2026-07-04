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

clearToken()
requestCalls = []
await assert.rejects(
  request({ url: '/secure', auth: true }),
  (err) => err.statusCode === 401 && /请先登录/.test(err.message),
)
assert.equal(requestCalls.length, 0, 'auth request without token must not hit network')

setToken('abc')
await assert.rejects(
  request({ url: '/unauthorized', auth: true }),
  (err) => err.statusCode === 401 && err.authExpired === true,
)
assert.equal(getToken(), '', '401/403 should clear stored token')

await assert.rejects(
  request({ url: '/network-fail' }),
  (err) => err.statusCode === 0 && err.retryable === true && /请求超时/.test(err.message),
)

console.log('request tests passed')
await rm(dir, { force: true, recursive: true })
