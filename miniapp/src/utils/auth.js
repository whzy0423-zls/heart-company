import { wxLoginApi } from '../api'
import { getToken, setToken, clearToken } from '../api/request'

export { getToken, clearToken }

let loginPromise = null

export function createLoginEnsurer(deps) {
  let currentLoginPromise = null
  return function ensureLoginWithDeps() {
    const token = deps.getToken()
    if (token) return Promise.resolve(token)
    if (currentLoginPromise) return currentLoginPromise

    currentLoginPromise = new Promise((resolve, reject) => {
      deps.login({
        provider: 'weixin',
        success: async ({ code }) => {
          if (!code) {
            currentLoginPromise = null
            reject(new Error('微信登录未返回 code，请稍后重试'))
            return
          }
          try {
            const res = await deps.wxLoginApi(code)
            deps.setToken(res.accessToken)
            resolve(res.accessToken)
          } catch (e) {
            reject(e)
          } finally {
            currentLoginPromise = null
          }
        },
        fail: (err) => {
          currentLoginPromise = null
          reject(err)
        },
      })
    })
    return currentLoginPromise
  }
}

/**
 * 确保已登录：有 token 直接返回；否则走 wx.login → 后端换 token。
 * 并发调用共享同一个登录流程。
 */
const defaultEnsureLogin = createLoginEnsurer({
  getToken,
  login: (options) => uni.login(options),
  setToken,
  wxLoginApi,
})

export function ensureLogin() {
  loginPromise = defaultEnsureLogin()
  return loginPromise
}
