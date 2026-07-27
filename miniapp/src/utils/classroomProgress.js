export const CLASSROOM_PROGRESS_THROTTLE_MS = 12_000
const LOCAL_PREFIX = 'nx_classroom_progress:'

function contentID(value) {
  const normalized = String(value ?? '').trim()
  return /^\d+$/.test(normalized) && normalized !== '0' ? normalized : ''
}

function position(value, durationSeconds = 0) {
  const normalized = Math.max(0, Math.floor(Number(value) || 0))
  const duration = Math.max(0, Math.floor(Number(durationSeconds) || 0))
  return duration > 0 ? Math.min(normalized, duration) : normalized
}

export function classroomCompletion(positionSeconds, durationSeconds) {
  const duration = Math.max(0, Number(durationSeconds) || 0)
  if (duration <= 0) return { ratio: 0, completed: false }
  const ratio = Math.min(1, Math.max(0, Number(positionSeconds) || 0) / duration)
  return { ratio, completed: ratio >= 0.9 }
}

function localKey(id) {
  return `${LOCAL_PREFIX}${id}`
}

export function readAnonymousClassroomProgress(storage, value) {
  const id = contentID(value)
  if (!storage || !id) return null
  try {
    const raw = storage.getItem(localKey(id))
    const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw
    if (!parsed || typeof parsed !== 'object') return null
    return { positionSeconds: position(parsed.positionSeconds), completed: parsed.completed === true, updatedAt: Math.max(0, Number(parsed.updatedAt) || 0) }
  } catch {
    return null
  }
}

export function createClassroomProgressTracker(options = {}) {
  const id = contentID(options.contentId)
  if (!id) throw new Error('课件参数无效')
  const durationSeconds = Math.max(0, Number(options.durationSeconds) || 0)
  const loggedIn = options.loggedIn === true
  const storage = options.storage
  const send = options.send
  const now = typeof options.now === 'function' ? options.now : Date.now
  const throttleMs = Math.min(15_000, Math.max(10_000, Number(options.throttleMs) || CLASSROOM_PROGRESS_THROTTLE_MS))
  let queued = null
  let lastSentAt = null

  function snapshot(value) {
    const positionSeconds = position(value, durationSeconds)
    return { positionSeconds, completed: classroomCompletion(positionSeconds, durationSeconds).completed }
  }

  async function transmit() {
    if (!queued) return null
    if (typeof send !== 'function') throw new Error('学习进度同步方法未配置')
    const current = queued
    await send(id, current.positionSeconds)
    if (queued === current) queued = null
    lastSentAt = now()
    return current
  }

  return {
    async record(value, { force = false } = {}) {
      const current = snapshot(value)
      if (!loggedIn) {
        const local = { ...current, updatedAt: now() }
        try { storage?.setItem(localKey(id), JSON.stringify(local)) } catch { /* 本地存储异常不影响播放 */ }
        return { ...current, local: true }
      }
      queued = current
      if (force || lastSentAt === null || now() - lastSentAt >= throttleMs) await transmit()
      return current
    },
    flush() { return transmit() },
    retry() { return transmit() },
    pending() { return queued ? { ...queued } : null },
  }
}
