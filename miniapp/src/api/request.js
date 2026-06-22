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
  uni.setStorageSync(TOKEN_KEY, token || '')
}

export function clearToken() {
  uni.removeStorageSync(TOKEN_KEY)
}

/**
 * 统一请求：自动带 token，解包后端 { code, data } 结构。
 * options: { url, method, data, auth, timeout }
 */
export function request(options) {
  const { url, method = 'GET', data, auth = false, timeout = 15000 } = options
  return new Promise((resolve, reject) => {
    const header = { 'Content-Type': 'application/json' }
    if (auth) {
      const token = getToken()
      if (token) header.Authorization = `Bearer ${token}`
    }
    uni.request({
      url: `${API_BASE}${url}`,
      method,
      data,
      header,
      timeout,
      success: (res) => {
        const body = res.data || {}
        if (res.statusCode >= 200 && res.statusCode < 300 && body.code === 0) {
          resolve(body.data)
        } else {
          if (res.statusCode === 401 || res.statusCode === 403) {
            clearToken()
          }
          const error = new Error(body.error || body.message || `请求失败(${res.statusCode})`)
          error.statusCode = res.statusCode
          reject(error)
        }
      },
      fail: (err) => reject(err),
    })
  })
}
