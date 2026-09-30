export const BOOKING_DRAFT_KEY = import.meta.env?.DEV === true && import.meta.env?.VITE_UI_PREVIEW === 'true' ? 'nx_ui_preview_booking_draft' : 'nx_booking_draft'

const DEFAULT_KIND = 'consult'
const FIELDS = ['contactName', 'phone', 'intent', 'preferredTime', 'message']
const COURSE_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$/

function normalizeDraft(input) {
  if (!input || typeof input !== 'object') return null
  const kind = typeof input.kind === 'string' ? input.kind.trim() : ''
  const data = {
    kind: kind || DEFAULT_KIND,
  }
  for (const field of FIELDS) {
    data[field] = typeof input[field] === 'string' ? input[field] : ''
  }
  if (typeof input.courseId === 'string' && COURSE_ID_PATTERN.test(input.courseId.trim())) {
    data.courseId = input.courseId.trim()
  }
  return data
}

function hasMeaningfulDraft(data) {
  if (!data) return false
  const hasMeaningfulField = FIELDS.some((field) => String(data[field] || '').trim())
  if (data.kind === 'enterprise') return hasMeaningfulField
  if (data.kind && data.kind !== DEFAULT_KIND) return true
  return hasMeaningfulField
}

export function loadBookingDraft() {
  try {
    const raw = uni.getStorageSync(BOOKING_DRAFT_KEY)
    if (!raw) return null
    const cached = typeof raw === 'string' ? JSON.parse(raw) : raw
    const data = normalizeDraft(cached && cached.data ? cached.data : cached)
    return hasMeaningfulDraft(data) ? data : null
  } catch {
    return null
  }
}

export function saveBookingDraft(input, options = {}) {
  const data = normalizeDraft(input)
  if (!hasMeaningfulDraft(data)) {
    clearBookingDraft()
    return
  }
  const now = typeof options.now === 'function' ? options.now() : Date.now()
  try {
    uni.setStorageSync(BOOKING_DRAFT_KEY, { ts: now, data })
  } catch {
    // 本地草稿缓存失败不影响预约填写。
  }
}

export function clearBookingDraft() {
  try {
    uni.removeStorageSync(BOOKING_DRAFT_KEY)
  } catch {
    // ignore
  }
}
