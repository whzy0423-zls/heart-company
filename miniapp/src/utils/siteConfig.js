import { getSiteConfigApi } from '../api'

const SITE_CONFIG_CACHE_KEY = 'nx_site_config_cache'
const DEFAULT_TTL_MS = 5 * 60 * 1000
let inflight = null

function readCache(now, ttlMs) {
  try {
    const raw = uni.getStorageSync(SITE_CONFIG_CACHE_KEY)
    if (!raw) return null
    const cached = typeof raw === 'string' ? JSON.parse(raw) : raw
    if (!cached || !cached.data || typeof cached.ts !== 'number') return null
    return now - cached.ts <= ttlMs ? cached.data : null
  } catch {
    return null
  }
}

function writeCache(data, now) {
  try {
    uni.setStorageSync(SITE_CONFIG_CACHE_KEY, { ts: now, data })
  } catch {
    // 缓存失败不影响页面展示。
  }
}

export function clearSiteConfigCache() {
  inflight = null
  try {
    uni.removeStorageSync(SITE_CONFIG_CACHE_KEY)
  } catch {
    // ignore
  }
}

export async function getCachedSiteConfig(options = {}) {
  const nowFn = options.now || (() => Date.now())
  const ttlMs = typeof options.ttlMs === 'number' ? options.ttlMs : DEFAULT_TTL_MS
  const api = options.api || getSiteConfigApi
  const now = nowFn()
  const cached = readCache(now, ttlMs)
  if (cached) return cached
  if (inflight) return inflight

  inflight = api()
    .then((data) => {
      writeCache(data, nowFn())
      return data
    })
    .finally(() => {
      inflight = null
    })
  return inflight
}
