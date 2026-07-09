import { wxLoginApi } from '../api'
import { getToken, setToken, clearToken } from '../api/request'
import { registerCurrentPushDevice } from './push'

export { getToken, clearToken }

let loginPromise = null
const WECHAT_LOGIN_UNSUPPORTED_MESSAGE = '当前环境不支持微信登录，请在微信小程序内使用'

function providerListIncludesWechat(provider) {
  return Array.isArray(provider) && provider.includes('weixin')
}

function checkWechatProvider(deps) {
  if (typeof deps.getProvider !== 'function') return Promise.resolve(true)
  return new Promise((resolve) => {
    try {
      deps.getProvider({
        service: 'oauth',
        success: (res = {}) => {
          const providers = res.provider || res.providers
          resolve(Array.isArray(providers) ? providerListIncludesWechat(providers) : true)
        },
        fail: () => {
          resolve(false)
        },
      })
    } catch {
      resolve(false)
    }
  })
}

function requestWechatLoginCode(deps) {
  return new Promise((resolve, reject) => {
    try {
      deps.login({
        provider: 'weixin',
        success: ({ code } = {}) => {
          if (!code) {
            reject(new Error('微信登录未返回 code，请稍后重试'))
            return
          }
          resolve(code)
        },
        fail: (err) => {
          reject(err)
        },
      })
    } catch (e) {
      reject(e)
    }
  })
}

export function createLoginEnsurer(deps) {
  let currentLoginPromise = null
  return function ensureLoginWithDeps() {
    const token = deps.getToken()
    if (token) return Promise.resolve(token)
    if (currentLoginPromise) return currentLoginPromise

    currentLoginPromise = (async () => {
      const hasWechatProvider = await checkWechatProvider(deps)
      if (!hasWechatProvider) {
        throw new Error(WECHAT_LOGIN_UNSUPPORTED_MESSAGE)
      }
      const code = await requestWechatLoginCode(deps)
      const res = await deps.wxLoginApi(code)
      deps.setToken(res.accessToken)
      try {
        await deps.afterLogin?.(res.accessToken)
      } catch {
        // 推送注册失败不阻断登录，后台推送页会用“0 设备”提示继续暴露问题。
      }
      return res.accessToken
    })().finally(() => {
      currentLoginPromise = null
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
  getProvider: (options) => uni.getProvider(options),
  login: (options) => uni.login(options),
  setToken,
  wxLoginApi,
  afterLogin: () => registerCurrentPushDevice(),
})

export function ensureLogin() {
  loginPromise = defaultEnsureLogin()
  return loginPromise
}
