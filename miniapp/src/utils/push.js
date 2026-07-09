import { registerAppPushApi, unregisterAppPushApi } from '../api'
import { getToken } from '../api/request'

const PUSH_REGISTRATION_KEY = 'nx_push_registration_id'
const DEFAULT_JPUSH_PLUGIN_NAMES = ['JG-JPush']

function safeCall(fn, fallback) {
  try {
    return fn()
  } catch {
    return fallback
  }
}

function storageGet(api, key) {
  return safeCall(() => api.getStorageSync(key), '')
}

function storageSet(api, key, value) {
  safeCall(() => api.setStorageSync(key, value), undefined)
}

function storageRemove(api, key) {
  safeCall(() => api.removeStorageSync(key), undefined)
}

function normalizeRegistrationId(value) {
  if (!value) return ''
  if (typeof value === 'string') return value.trim()
  if (typeof value !== 'object') return ''
  return String(
    value.registrationId ||
      value.registrationID ||
      value.registerID ||
      value.registerId ||
      value.cid ||
      '',
  ).trim()
}

function parsePayload(payload) {
  if (!payload) return {}
  if (typeof payload === 'string') {
    return safeCall(() => JSON.parse(payload), { raw: payload })
  }
  if (typeof payload === 'object') return payload
  return {}
}

function payloadDeepLink(payload) {
  const data = parsePayload(payload)
  return String(data.deep_link || data.deepLink || data.url || '').trim()
}

export function normalizePushNotification(result = {}) {
  const data = result.data || result
  const payload = parsePayload(data.payload || data.extra || data.extras)
  return {
    content: String(data.content || data.alert || data.message || '').trim(),
    deepLink: payloadDeepLink(payload),
    payload,
    title: String(data.title || '').trim(),
  }
}

function getPlatform(api) {
  const info = safeCall(() => api.getSystemInfoSync(), {}) || {}
  return String(info.platform || '').trim() || 'app'
}

function getDeviceInfo(api) {
  const info = safeCall(() => api.getSystemInfoSync(), {}) || {}
  return safeCall(() => JSON.stringify(info), '')
}

function resolveJPushModule(api, pluginNames) {
  if (!api || typeof api.requireNativePlugin !== 'function') return null
  for (const name of pluginNames) {
    const module = safeCall(() => api.requireNativePlugin(name), null)
    if (module) return module
  }
  return null
}

function getJPushRegistrationId(api, pluginNames) {
  const module = resolveJPushModule(api, pluginNames)
  if (!module) return Promise.resolve('')

  safeCall(() => module.initJPushService && module.initJPushService(), undefined)
  safeCall(() => module.init && module.init(), undefined)

  return new Promise((resolve) => {
    let settled = false
    const done = (value) => {
      if (settled) return
      settled = true
      resolve(normalizeRegistrationId(value))
    }

    const callback = (res) => done(res)
    const getter =
      module.getRegistrationID ||
      module.getRegistrationId ||
      module.getRegisterID ||
      module.getRegisterId

    if (typeof getter !== 'function') {
      done('')
      return
    }

    const result = safeCall(() => getter.call(module, callback), undefined)
    const normalized = normalizeRegistrationId(result)
    if (normalized) done(normalized)

    setTimeout(() => done(''), 3000)
  })
}

function getUniPushClientId(api) {
  if (!api || typeof api.getPushClientId !== 'function') return Promise.resolve('')
  return new Promise((resolve) => {
    api.getPushClientId({
      fail: () => resolve(''),
      success: (res) => resolve(normalizeRegistrationId(res)),
    })
  })
}

function appendPushQuery(pagePath, deepLink) {
  const joiner = pagePath.includes('?') ? '&' : '?'
  return `${pagePath}${joiner}from=push&deepLink=${encodeURIComponent(deepLink)}`
}

function resolveDeepLinkUrl(deepLink) {
  if (!deepLink) return ''
  if (deepLink.startsWith('/pages/')) return deepLink
  const path = deepLink.split('?')[0]
  let pagePath = ''
  switch (path) {
    case '/daily':
    case '/daily-quiz':
    case '/tasks':
      pagePath = '/pages/test/test'
      break
    case '/compatibility':
      pagePath = '/pages/relation/relation'
      break
    case '/reports':
      pagePath = '/pages/result/result'
      break
    default:
      return ''
  }
  return appendPushQuery(pagePath, deepLink)
}

function openDeepLink(api, deepLink) {
  if (!deepLink || !api || typeof api.navigateTo !== 'function') return
  const url = resolveDeepLinkUrl(deepLink)
  if (!url) return
  safeCall(() => api.navigateTo({ url }), undefined)
}

function showForegroundPush(api, notification) {
  if (!notification.title && !notification.content) return
  if (api && typeof api.createPushMessage === 'function') {
    safeCall(
      () =>
        api.createPushMessage({
          content: notification.content || notification.title,
          payload: notification.payload,
          sound: 'system',
          title: notification.title || '消息提醒',
        }),
      undefined,
    )
  }
  if (api && typeof api.showModal === 'function') {
    safeCall(
      () =>
        api.showModal({
          cancelText: '关闭',
          confirmText: notification.deepLink ? '去查看' : '知道了',
          content: notification.content || '',
          showCancel: Boolean(notification.deepLink),
          success: (res) => {
            if (res && res.confirm) openDeepLink(api, notification.deepLink)
          },
          title: notification.title || '消息提醒',
        }),
      undefined,
    )
  }
}

export function createAppPushBridge(options = {}) {
  const api = options.uni || globalThis.uni || {}
  const getTokenFn = options.getToken || getToken
  const registerApi = options.registerAppPushApi || registerAppPushApi
  const unregisterApi = options.unregisterAppPushApi || unregisterAppPushApi
  const pluginNames = options.jpushPluginNames || DEFAULT_JPUSH_PLUGIN_NAMES
  const allowUniPushClientId = Boolean(options.allowUniPushClientId)
  let pushHandler = null
  let jpushModule = null

  async function getRegistrationId() {
    const jpushId = await getJPushRegistrationId(api, pluginNames)
    if (jpushId) return jpushId
    return allowUniPushClientId ? getUniPushClientId(api) : ''
  }

  async function registerCurrentDevice() {
    if (!getTokenFn()) {
      return { reason: 'missing-token', registered: false }
    }
    const registrationId = await getRegistrationId()
    if (!registrationId) {
      return { reason: 'missing-registration-id', registered: false }
    }
    await registerApi({
      deviceInfo: getDeviceInfo(api),
      platform: getPlatform(api),
      registrationId,
    })
    storageSet(api, PUSH_REGISTRATION_KEY, registrationId)
    return { registered: true, registrationId }
  }

  async function unregisterCurrentDevice() {
    const registrationId = storageGet(api, PUSH_REGISTRATION_KEY)
    if (!registrationId || !getTokenFn()) {
      return { reason: 'missing-registration-id', unregistered: false }
    }
    await unregisterApi(registrationId)
    storageRemove(api, PUSH_REGISTRATION_KEY)
    return { registrationId, unregistered: true }
  }

  function handlePushMessage(result) {
    const notification = normalizePushNotification(result)
    if (result && result.type === 'click') {
      openDeepLink(api, notification.deepLink)
      return
    }
    showForegroundPush(api, notification)
  }

  function initMessageListener() {
    if (pushHandler) return
    pushHandler = handlePushMessage
    jpushModule = resolveJPushModule(api, pluginNames)
    safeCall(
      () =>
        jpushModule &&
        typeof jpushModule.addNotificationListener === 'function' &&
        jpushModule.addNotificationListener(pushHandler),
      undefined,
    )
    safeCall(
      () =>
        jpushModule &&
        typeof jpushModule.addCustomMessageListener === 'function' &&
        jpushModule.addCustomMessageListener(pushHandler),
      undefined,
    )
    if (api && typeof api.onPushMessage === 'function') {
      api.onPushMessage(pushHandler)
    }
  }

  function teardownMessageListener() {
    if (!pushHandler) return
    safeCall(
      () =>
        jpushModule &&
        typeof jpushModule.removeNotificationListener === 'function' &&
        jpushModule.removeNotificationListener(pushHandler),
      undefined,
    )
    safeCall(
      () =>
        jpushModule &&
        typeof jpushModule.removeCustomMessageListener === 'function' &&
        jpushModule.removeCustomMessageListener(pushHandler),
      undefined,
    )
    if (api && typeof api.offPushMessage === 'function') {
      api.offPushMessage(pushHandler)
    }
    pushHandler = null
    jpushModule = null
  }

  return {
    getRegistrationId,
    initMessageListener,
    registerCurrentDevice,
    teardownMessageListener,
    unregisterCurrentDevice,
  }
}

const defaultBridge = createAppPushBridge()

export function initAppPush() {
  defaultBridge.initMessageListener()
  void defaultBridge.registerCurrentDevice()
}

export function registerCurrentPushDevice() {
  return defaultBridge.registerCurrentDevice()
}

export function unregisterCurrentPushDevice() {
  return defaultBridge.unregisterCurrentDevice()
}
