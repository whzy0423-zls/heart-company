import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-auth-'))
const modulePath = join(dir, 'auth.mjs')
const source = await readFile(new URL('./auth.js', import.meta.url), 'utf8')
await writeFile(
  modulePath,
  source
    .replace("import { wxLoginApi } from '../api'", "import { wxLoginApi } from './api-stub.mjs'")
    .replace(
      "import { getToken, setToken, clearToken } from '../api/request'",
      "import { getToken, setToken, clearToken } from './request-stub.mjs'",
    )
    .replace(
      "import { registerCurrentPushDevice } from './push'",
      "import { registerCurrentPushDevice } from './push-stub.mjs'",
    ),
)
await writeFile(
  join(dir, 'api-stub.mjs'),
  [
    'export const calls = []',
    'export async function wxLoginApi(code) { calls.push(code); return { accessToken: "token-" + code } }',
  ].join('\n'),
)
await writeFile(
  join(dir, 'request-stub.mjs'),
  [
    'let token = ""',
    'export function getToken() { return token }',
    'export function setToken(value) { token = value || "" }',
    'export function clearToken() { token = "" }',
  ].join('\n'),
)
await writeFile(
  join(dir, 'push-stub.mjs'),
  'export async function registerCurrentPushDevice() { return { registered: true } }',
)

const { createLoginEnsurer } = await import(`file://${modulePath}`)

let loginCalls = 0
let afterLoginToken = ''
const ensureLogin = createLoginEnsurer({
  getToken: () => '',
  afterLogin: async (token) => {
    afterLoginToken = token
  },
  login: ({ success }) => {
    loginCalls++
    success({ code: 'abc' })
  },
  setToken: (token) => {
    assert.equal(token, 'token-abc')
  },
  wxLoginApi: async (code) => ({ accessToken: `token-${code}` }),
})

assert.equal(await ensureLogin(), 'token-abc')
assert.equal(loginCalls, 1)
assert.equal(afterLoginToken, 'token-abc')

const emptyCodeLogin = createLoginEnsurer({
  getToken: () => '',
  login: ({ success }) => success({ code: '' }),
  setToken: () => {
    throw new Error('setToken should not be called')
  },
  wxLoginApi: async () => {
    throw new Error('wxLoginApi should not be called')
  },
})

await assert.rejects(emptyCodeLogin(), /微信登录未返回 code/)

let unsupportedLoginCalled = false
const unsupportedWechatLogin = createLoginEnsurer({
  getToken: () => '',
  getProvider: ({ service, success }) => {
    assert.equal(service, 'oauth')
    success({ provider: [] })
  },
  login: () => {
    unsupportedLoginCalled = true
    throw new Error('uni.login should not be called when weixin provider is unavailable')
  },
  setToken: () => {
    throw new Error('setToken should not be called')
  },
  wxLoginApi: async () => {
    throw new Error('wxLoginApi should not be called')
  },
})

await assert.rejects(unsupportedWechatLogin(), /当前环境不支持微信登录/)
assert.equal(unsupportedLoginCalled, false)

let providerFailLoginCalled = false
const h5ProviderFailLogin = createLoginEnsurer({
  getToken: () => '',
  getProvider: ({ fail }) => fail(new Error('getProvider unsupported')),
  login: () => {
    providerFailLoginCalled = true
    throw new Error('uni.login should not be called after getProvider reports unsupported')
  },
  setToken: () => {
    throw new Error('setToken should not be called')
  },
  wxLoginApi: async () => {
    throw new Error('wxLoginApi should not be called')
  },
})

await assert.rejects(h5ProviderFailLogin(), /当前环境不支持微信登录/)
assert.equal(providerFailLoginCalled, false)

console.log('auth tests passed')
await rm(dir, { force: true, recursive: true })
