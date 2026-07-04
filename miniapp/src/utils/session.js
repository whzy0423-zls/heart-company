// 跨页传递最近一次测试结果；同时落本地缓存，避免页面刷新/小程序重启后结果页丢失。
const LAST_RESULT_KEY = 'nx_last_test_result'
let lastResult = null
let lastGender = null
let hydrated = false

function hydrateLastResult() {
  if (hydrated) return
  hydrated = true
  try {
    const raw = uni.getStorageSync(LAST_RESULT_KEY)
    if (!raw) return
    const cached = typeof raw === 'string' ? JSON.parse(raw) : raw
    lastResult = cached && cached.result ? cached.result : null
    lastGender = cached && cached.gender ? cached.gender : null
  } catch {
    lastResult = null
    lastGender = null
  }
}

export function setLastResult(result, gender) {
  hydrated = true
  lastResult = result || null
  lastGender = gender || null
  try {
    uni.setStorageSync(LAST_RESULT_KEY, { result: lastResult, gender: lastGender })
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
