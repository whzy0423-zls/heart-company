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

const { createLoginEnsurer } = await import(`file://${modulePath}`)

let loginCalls = 0
const ensureLogin = createLoginEnsurer({
  getToken: () => '',
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

console.log('auth tests passed')
await rm(dir, { force: true, recursive: true })
