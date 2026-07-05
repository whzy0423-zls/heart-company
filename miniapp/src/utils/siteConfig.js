import { getSiteConfigApi } from '../api'

const SITE_CONFIG_CACHE_KEY = 'nx_site_config_cache'
const DEFAULT_TTL_MS = 5 * 60 * 1000
let inflight = null

function readStoredCache() {
  try {
    const raw = uni.getStorageSync(SITE_CONFIG_CACHE_KEY)
    if (!raw) return null
    const cached = typeof raw === 'string' ? JSON.parse(raw) : raw
    if (!cached || !cached.data || typeof cached.ts !== 'number') return null
    return cached
  } catch {
    return null
  }
}

function readCache(now, ttlMs) {
  const cached = readStoredCache()
  if (!cached) return null
  return now - cached.ts <= ttlMs ? cached.data : null
}

function writeCache(data, now) {
  try {
    uni.setStorageSync(SITE_CONFIG_CACHE_KEY, { ts: now, data })
  } catch {
    // 缓存失败不影响页面展示。
  }
}

function writeCachePreservingLearningContent(data, now) {
  const cached = readStoredCache()
  if (
    cached &&
    hasSiteConfigLearningSection(cached.data) &&
    !hasSiteConfigLearningSection(data)
  ) {
    return
  }
  writeCache(data, now)
}

export function getStoredSiteConfig() {
  const cached = readStoredCache()
  return cached ? cached.data : null
}

export function clearSiteConfigCache() {
  inflight = null
  try {
    uni.removeStorageSync(SITE_CONFIG_CACHE_KEY)
  } catch {
    // ignore
  }
}

export function hasSiteConfigLearningContent(config) {
  const courses = config?.home?.courses?.items
  const quotes = config?.home?.quotes?.items
  return (Array.isArray(courses) && courses.length > 0) || (Array.isArray(quotes) && quotes.length > 0)
}

export function hasSiteConfigLearningSection(config) {
  const courses = config?.home?.courses?.items
  const quotes = config?.home?.quotes?.items
  return Array.isArray(courses) || Array.isArray(quotes)
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
      writeCachePreservingLearningContent(data, nowFn())
      return data
    })
    .finally(() => {
      inflight = null
    })
  return inflight
}

export async function refreshSiteConfig(options = {}) {
  const nowFn = options.now || (() => Date.now())
  const api = options.api || getSiteConfigApi
  if (inflight) return inflight

  inflight = api()
    .then((data) => {
      writeCachePreservingLearningContent(data, nowFn())
      return data
    })
    .finally(() => {
      inflight = null
    })
  return inflight
}
