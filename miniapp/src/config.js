const DEFAULT_API_BASE = 'https://xn--9iq9az5uo8fz16d.com/api'

function cleanBaseUrl(value) {
  return String(value || '').trim().replace(/\/+$/, '')
}

function isRealProductionApiBase(value) {
  if (!value.startsWith('https://')) return false

  const hostname = value.slice('https://'.length).split(/[/:?#]/, 1)[0].toLowerCase()
  if (!hostname) return false

  const labels = hostname.split('.')
  return !labels.some((label) => ['example', 'yourdomain', 'local'].includes(label))
}

// 后端 API 基址。
// App/小程序端 API 基址：BaseURL 固定到 /api，接口路径只写 /app/xxx。
// 例如：BaseURL=https://xn--9iq9az5uo8fz16d.com/api + /app/auth/sms
// 最终请求：https://xn--9iq9az5uo8fz16d.com/api/app/auth/sms
// 可由 VITE_API_BASE 覆盖，但生产环境必须是 HTTPS 且不能是占位域名。
export function resolveApiBase(options = {}) {
  const env = options.env || import.meta.env || { DEV: true }
  const configured = cleanBaseUrl(env.VITE_API_BASE)
  if (configured) {
    if (!env.DEV && !isRealProductionApiBase(configured)) {
      throw new Error('Production VITE_API_BASE must be a real HTTPS API URL')
    }
    return configured
  }
  return DEFAULT_API_BASE
}

export const API_BASE = resolveApiBase()

// 渠道标识（可用于统计来源）
export const APP_CHANNEL = 'miniapp'
