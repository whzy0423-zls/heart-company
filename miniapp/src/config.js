const DEFAULT_API_BASE = 'https://xn--9iq9az5uo8fz16d.com/api'

function cleanBaseUrl(value) {
  return String(value || '').trim().replace(/\/+$/, '')
}

function extractHostname(value) {
  const authority = value.slice('https://'.length).split(/[/?#]/, 1)[0]
  const hostAndPort = authority.slice(authority.lastIndexOf('@') + 1)
  if (hostAndPort.startsWith('[')) {
    const closingBracket = hostAndPort.indexOf(']')
    return closingBracket > 0 ? hostAndPort.slice(1, closingBracket).toLowerCase() : ''
  }
  return hostAndPort.split(':', 1)[0].toLowerCase()
}

function isPrivateIPv4(value) {
  const parts = value.split('.').map((part) => Number(part))
  if (parts.length !== 4 || parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)) {
    return false
  }

  return (
    parts[0] === 10 ||
    parts[0] === 127 ||
    (parts[0] === 172 && parts[1] >= 16 && parts[1] <= 31) ||
    (parts[0] === 192 && parts[1] === 168) ||
    (parts[0] === 169 && parts[1] === 254) ||
    parts[0] === 0
  )
}

function hasBlockedDomainSuffix(hostname, secondLevelLabel) {
  const labels = hostname.split('.')
  return (
    labels.length >= 3 &&
    labels[labels.length - 2] === secondLevelLabel &&
    labels[labels.length - 1] === 'com'
  )
}

function isRealProductionApiBase(value) {
  if (!value.startsWith('https://')) return false

  const hostname = extractHostname(value)
  if (!hostname) return false

  return !(
    hostname === 'localhost' ||
    hostname.endsWith('.localhost') ||
    hostname.endsWith('.local') ||
    hostname === '::1' ||
    isPrivateIPv4(hostname) ||
    hasBlockedDomainSuffix(hostname, 'example') ||
    hasBlockedDomainSuffix(hostname, 'yourdomain')
  )
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
