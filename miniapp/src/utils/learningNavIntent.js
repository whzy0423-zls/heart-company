export const LEARNING_NAV_INTENT_KEY = 'nx_learning_nav_intent'
export const LEARNING_NAV_INTENT_TTL_MS = 10_000

const VALID_INTENTS = new Set(['course', 'material', 'quote'])
let suppressStoredIntent = false

function nowFrom(options) {
  return typeof options?.now === 'function' ? options.now() : Date.now()
}

export function clearLearningNavIntent() {
  suppressStoredIntent = true
  try {
    uni.removeStorageSync(LEARNING_NAV_INTENT_KEY)
  } catch {
    // 导航意图是临时状态，清理失败不阻塞页面跳转。
  }
}

export function setLearningNavIntent(value, options = {}) {
  if (!VALID_INTENTS.has(value)) {
    clearLearningNavIntent()
    return false
  }
  try {
    uni.setStorageSync(LEARNING_NAV_INTENT_KEY, {
      value,
      expiresAt: nowFrom(options) + LEARNING_NAV_INTENT_TTL_MS,
    })
    suppressStoredIntent = false
    return true
  } catch {
    clearLearningNavIntent()
    return false
  }
}

export function readLearningNavIntent(options = {}) {
  let cached
  try {
    const raw = uni.getStorageSync(LEARNING_NAV_INTENT_KEY)
    if (raw === '') return null
    cached = typeof raw === 'string' ? JSON.parse(raw) : raw
  } catch {
    clearLearningNavIntent()
    return null
  }

  if (suppressStoredIntent) {
    clearLearningNavIntent()
    return null
  }

  clearLearningNavIntent()
  if (
    !cached
    || typeof cached !== 'object'
    || !VALID_INTENTS.has(cached.value)
    || !Number.isFinite(cached.expiresAt)
    || cached.expiresAt <= nowFrom(options)
  ) return null

  return cached.value
}
