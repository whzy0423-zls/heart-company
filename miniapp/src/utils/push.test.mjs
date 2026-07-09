import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-push-'))
const modulePath = join(dir, 'push.mjs')
const source = await readFile(new URL('./push.js', import.meta.url), 'utf8')
await writeFile(
  modulePath,
  source
    .replace(
      "import { registerAppPushApi, unregisterAppPushApi } from '../api'",
      "import { registerAppPushApi, unregisterAppPushApi } from './api-stub.mjs'",
    )
    .replace(
      "import { getToken } from '../api/request'",
      "import { getToken } from './request-stub.mjs'",
    ),
)
await writeFile(
  join(dir, 'api-stub.mjs'),
  [
    'export const registerCalls = []',
    'export const unregisterCalls = []',
    'export async function registerAppPushApi(data) { registerCalls.push(data); return { ok: true } }',
    'export async function unregisterAppPushApi(registrationId) { unregisterCalls.push(registrationId); return { ok: true } }',
  ].join('\n'),
)
await writeFile(
  join(dir, 'request-stub.mjs'),
  [
    'export let token = "access-token"',
    'export function getToken() { return token }',
    'export function setToken(value) { token = value || "" }',
  ].join('\n'),
)

const { createAppPushBridge, normalizePushNotification } = await import(`file://${modulePath}`)

let storage = {}
const modalCalls = []
const createPushMessageCalls = []
const navigateCalls = []
let pushHandler
const uni = {
  createPushMessage(options) {
    createPushMessageCalls.push(options)
    options.success?.({})
  },
  getStorageSync(key) {
    return storage[key] || ''
  },
  getSystemInfoSync() {
    return { brand: 'Apple', model: 'iPhone', platform: 'ios', system: 'iOS 17' }
  },
  navigateTo(options) {
    navigateCalls.push(options)
  },
  offPushMessage(handler) {
    if (pushHandler === handler) pushHandler = undefined
  },
  onPushMessage(handler) {
    pushHandler = handler
  },
  removeStorageSync(key) {
    delete storage[key]
  },
  requireNativePlugin(name) {
    assert.equal(name, 'JG-JPush')
    return {
      getRegistrationID(callback) {
        callback({ registerID: 'jpush-rid-1' })
      },
      initJPushService() {},
    }
  },
  setStorageSync(key, value) {
    storage[key] = value
  },
  showModal(options) {
    modalCalls.push(options)
    options.success?.({ confirm: true })
  },
}

const apiStub = await import(`file://${join(dir, 'api-stub.mjs')}`)
const bridge = createAppPushBridge({ uni })

const registerResult = await bridge.registerCurrentDevice()
assert.equal(registerResult.registered, true)
assert.equal(registerResult.registrationId, 'jpush-rid-1')
assert.equal(apiStub.registerCalls.length, 1)
assert.equal(apiStub.registerCalls[0].registrationId, 'jpush-rid-1')
assert.equal(apiStub.registerCalls[0].platform, 'ios')
assert.match(apiStub.registerCalls[0].deviceInfo, /iPhone/)

bridge.initMessageListener()
assert.equal(typeof pushHandler, 'function')
pushHandler({
  data: {
    content: '今天 5 道题等你完成',
    payload: { deep_link: '/daily-quiz' },
    title: '画像校准题',
  },
  type: 'receive',
})
assert.equal(modalCalls[0].title, '画像校准题')
assert.equal(createPushMessageCalls[0].payload.deep_link, '/daily-quiz')
assert.equal(navigateCalls[0].url, '/pages/test/test?from=push&deepLink=%2Fdaily-quiz')

assert.deepEqual(
  normalizePushNotification({
    data: {
      content: '正文',
      payload: JSON.stringify({ deep_link: '/reports' }),
      title: '标题',
    },
  }),
  { content: '正文', deepLink: '/reports', payload: { deep_link: '/reports' }, title: '标题' },
)

await rm(dir, { force: true, recursive: true })
console.log('push tests passed')
