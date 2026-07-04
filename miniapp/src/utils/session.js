// 跨页传递最近一次测试结果；同时落本地缓存，避免页面刷新/小程序重启后结果页丢失。
const LAST_RESULT_KEY = 'nx_last_test_result'
const VALID_TYPE_MIN = 1
const VALID_TYPE_MAX = 9
let lastResult = null
let lastGender = null
let hydrated = false

export function normalizeTypeId(value) {
  const type = Number(value)
  return Number.isInteger(type) && type >= VALID_TYPE_MIN && type <= VALID_TYPE_MAX ? type : 0
}

export function isValidTypeId(value) {
  return normalizeTypeId(value) !== 0
}

function normalizeGender(gender) {
  return gender === 'male' || gender === 'female' ? gender : null
}

function normalizeCenterRows(centers) {
  if (!Array.isArray(centers)) return []
  return centers
    .filter((item) => item && typeof item === 'object')
    .map((item) => {
      const normalized = {
        key: typeof item.key === 'string' ? item.key.trim() : '',
        pct: Number(item.pct),
      }
      if (typeof item.name === 'string' && item.name.trim()) {
        normalized.name = item.name.trim()
      }
      return normalized
    })
    .filter((item) => item.key && Number.isFinite(item.pct))
}

export function normalizeLastResult(result) {
  if (!result || typeof result !== 'object') return null
  const type = normalizeTypeId(result.type)
  if (!type) return null
  const second = normalizeTypeId(result.second)
  return {
    ...result,
    type,
    second: second || null,
    score: result.score && typeof result.score === 'object' && !Array.isArray(result.score) ? result.score : {},
    centers: normalizeCenterRows(result.centers),
  }
}

function hydrateLastResult() {
  if (hydrated) return
  hydrated = true
  try {
    const raw = uni.getStorageSync(LAST_RESULT_KEY)
    if (!raw) return
    const cached = typeof raw === 'string' ? JSON.parse(raw) : raw
    const normalized = normalizeLastResult(cached && cached.result)
    if (!normalized) {
      lastResult = null
      lastGender = null
      uni.removeStorageSync(LAST_RESULT_KEY)
      return
    }
    lastResult = normalized
    lastGender = normalizeGender(cached.gender)
  } catch {
    lastResult = null
    lastGender = null
  }
}

export function setLastResult(result, gender) {
  hydrated = true
  lastResult = normalizeLastResult(result)
  lastGender = lastResult ? normalizeGender(gender) : null
  try {
    if (lastResult) {
      uni.setStorageSync(LAST_RESULT_KEY, { result: lastResult, gender: lastGender })
    } else {
      uni.removeStorageSync(LAST_RESULT_KEY)
    }
  } catch {
    // 本地缓存失败时仍保留模块内存，避免阻塞测试流程。
  }
}

export function getLastResult() {
  hydrateLastResult()
  return { result: lastResult, gender: lastGender }
}

export function clearLastResult() {
  hydrated = true
  lastResult = null
  lastGender = null
  try {
    uni.removeStorageSync(LAST_RESULT_KEY)
  } catch {
    // ignore
  }
}
