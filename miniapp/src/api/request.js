import { API_BASE } from '../config'

const TOKEN_KEY = 'nx_token'

export function getToken() {
  try {
    return uni.getStorageSync(TOKEN_KEY) || ''
  } catch {
    return ''
  }
}

export function setToken(token) {
  try {
    uni.setStorageSync(TOKEN_KEY, token || '')
  } catch {
    // 存储异常不阻塞主流程，后续鉴权请求会重新登录。
  }
}

export function clearToken() {
  try {
    uni.removeStorageSync(TOKEN_KEY)
  } catch {
    // ignore
  }
}

function createRequestError(message, extra = {}) {
  const error = new Error(message || '请求失败，请稍后重试')
  Object.assign(error, extra)
  return error
}

function appendQuery(url, query) {
  if (!query || typeof query !== 'object') return url
  const pairs = Object.entries(query)
    .filter(([, value]) => value !== undefined && value !== null)
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
  if (!pairs.length) return url
  return `${url}${url.includes('?') ? '&' : '?'}${pairs.join('&')}`
}

function joinUrl(base, path) {
  const cleanBase = String(base || '').replace(/\/+$/, '')
  const cleanPath = String(path || '').replace(/^\/+/, '')
  return `${cleanBase}/${cleanPath}`
}

function normalizeFailError(err) {
  const raw = err && err.errMsg ? String(err.errMsg) : ''
  const timeout = raw.toLowerCase().includes('timeout')
  return createRequestError(timeout ? '请求超时，请稍后重试' : '网络连接异常，请稍后重试', {
    cause: err,
    retryable: true,
    statusCode: 0,
    timeout,
  })
}

/**
 * 统一请求：自动带 token，解包后端 { code, data } 结构。
 * options: { url, method, data, query, auth, timeout }
 */
export function request(options) {
  const { url, method = 'GET', data, query, auth = false, timeout = 15000 } = options
  return new Promise((resolve, reject) => {
    const header = { 'Content-Type': 'application/json' }
    let requestToken = ''
    if (auth) {
      requestToken = getToken()
      if (!requestToken) {
        reject(createRequestError('请先登录后再继续', { statusCode: 401, authRequired: true }))
        return
      }
      header.Authorization = `Bearer ${requestToken}`
    }
    const requestUrl = joinUrl(API_BASE, appendQuery(url, query))
    uni.request({
      url: requestUrl,
      method,
      data,
      header,
      timeout,
      success: (res) => {
        const body = res.data || {}
        if (res.statusCode >= 200 && res.statusCode < 300 && body.code === 0) {
          resolve(body.data)
        } else {
          const authExpired = res.statusCode === 401 || res.statusCode === 403
          const clearRequestToken = authExpired && getToken() === requestToken
          const error = createRequestError(body.error || body.message || `请求失败(${res.statusCode})`, {
            code: body.code,
            statusCode: res.statusCode,
            authExpired,
            retryable: res.statusCode >= 500 || res.statusCode === 429,
          })
          reject(error)
          if (clearRequestToken) {
            // 先让页面级 catch 校验请求所属会话，再清理仍未被替换的过期 token。
            setTimeout(() => {
              if (getToken() === requestToken) clearToken()
            }, 0)
          }
        }
      },
      fail: (err) => reject(normalizeFailError(err)),
    })
  })
}
