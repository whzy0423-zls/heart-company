import { getSiteConfigApi } from '../api'

const SITE_CONFIG_CACHE_KEY = 'nx_site_config_cache'
const DEFAULT_TTL_MS = 5 * 60 * 1000
const TEACHER_SECTION_PATHS = [
  ['teacher'],
  ['teachers'],
  ['home', 'teacher'],
  ['home', 'teachers'],
  ['home', 'teacherTeaser'],
]
const COURSE_SECTION_PATHS = [
  ['courseware'],
  ['materials'],
  ['lessons'],
  ['courses'],
  ['home', 'courseware'],
  ['home', 'materials'],
  ['home', 'lessons'],
  ['home', 'courses'],
]
const QUOTE_SECTION_PATHS = [
  ['quotes'],
  ['home', 'quotes'],
]
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

function cloneConfigValue(value) {
  if (Array.isArray(value)) return value.map(cloneConfigValue)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => [key, cloneConfigValue(item)]),
  )
}

function hasOwnPath(source, path) {
  let current = source
  for (let index = 0; index < path.length; index += 1) {
    if (!current || typeof current !== 'object') return false
    const key = path[index]
    if (!Object.prototype.hasOwnProperty.call(current, key)) return false
    if (index === path.length - 1) return true
    current = current[key]
  }
  return false
}

function valueAtPath(source, path) {
  return path.reduce((current, key) => current?.[key], source)
}

function setPath(target, path, value) {
  let current = target
  for (let index = 0; index < path.length - 1; index += 1) {
    const key = path[index]
    if (!current[key] || typeof current[key] !== 'object' || Array.isArray(current[key])) {
      current[key] = {}
    }
    current = current[key]
  }
  current[path[path.length - 1]] = cloneConfigValue(value)
}

function preserveMissingDomain(merged, cachedData, incomingData, paths) {
  if (paths.some((path) => hasOwnPath(incomingData, path))) return
  for (const path of paths) {
    if (hasOwnPath(cachedData, path)) setPath(merged, path, valueAtPath(cachedData, path))
  }
}

function mergeCachedSections(cachedData, incomingData) {
  const merged = incomingData && typeof incomingData === 'object'
    ? cloneConfigValue(incomingData)
    : {}
  if (!cachedData || typeof cachedData !== 'object') return merged
  preserveMissingDomain(merged, cachedData, incomingData, TEACHER_SECTION_PATHS)
  preserveMissingDomain(merged, cachedData, incomingData, COURSE_SECTION_PATHS)
  preserveMissingDomain(merged, cachedData, incomingData, QUOTE_SECTION_PATHS)
  return merged
}

function writeCachePreservingLearningContent(data, now) {
  const cached = readStoredCache()
  writeCache(mergeCachedSections(cached?.data, data), now)
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

function hasItems(value) {
  if (Array.isArray(value)) return value.length > 0
  if (Array.isArray(value?.items)) return value.items.length > 0
  if (Array.isArray(value?.list)) return value.list.length > 0
  return !!(value && typeof value === 'object' && Object.keys(value).length > 0)
}

function hasSection(value) {
  return Array.isArray(value) || Array.isArray(value?.items) || Array.isArray(value?.list) || !!(value && typeof value === 'object')
}

function learningSources(config) {
  return [
    config?.teacher,
    config?.teachers,
    config?.courseware,
    config?.materials,
    config?.lessons,
    config?.courses,
    config?.home?.teacher,
    config?.home?.teachers,
    config?.home?.teacherTeaser,
    config?.home?.courseware,
    config?.home?.materials,
    config?.home?.lessons,
    config?.home?.courses,
    config?.home?.quotes,
  ]
}

export function hasSiteConfigLearningContent(config) {
  return learningSources(config).some(hasItems)
}

export function hasSiteConfigLearningSection(config) {
  return learningSources(config).some(hasSection)
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
