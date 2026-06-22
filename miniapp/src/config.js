const DEFAULT_DEV_API_BASE = 'http://localhost:8080/api'
// 占位：上线前应由 .env.production 的 VITE_API_BASE 覆盖为真实 https 域名。
const DEFAULT_PROD_API_BASE = 'https://api.example.com/api'

function cleanBaseUrl(value) {
  return String(value || '').trim().replace(/\/+$/, '')
}

// 后端 API 基址。
// 开发：默认 http://localhost:8080/api，可由 VITE_API_BASE 覆盖。
// 生产：读取 .env.production / CI 注入的 VITE_API_BASE，避免每次发布手改本文件。
export function resolveApiBase(options = {}) {
  const env = options.env || import.meta.env || {}
  const configured = cleanBaseUrl(env.VITE_API_BASE)
  if (configured) return configured
  return env.DEV ? DEFAULT_DEV_API_BASE : DEFAULT_PROD_API_BASE
}

export const API_BASE = resolveApiBase()

// 渠道标识（可用于统计来源）
export const APP_CHANNEL = 'miniapp'
