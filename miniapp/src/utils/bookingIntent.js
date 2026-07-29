export const BOOKING_INTENT_KEY = 'nx_booking_intent'

const ALLOWED_KINDS = new Set(['consult', 'course', 'enterprise'])
const MAX_INTENT_TEXT_LENGTH = 120

function normalizeIntent(input) {
  if (!input || typeof input !== 'object') return null

  const kind = typeof input.kind === 'string' ? input.kind.trim() : ''
  if (!ALLOWED_KINDS.has(kind)) return null

  const intentText = typeof input.intentText === 'string'
    ? Array.from(input.intentText.trim()).slice(0, MAX_INTENT_TEXT_LENGTH).join('')
    : ''

  return { kind, intentText }
}

export function clearBookingIntent() {
  try {
    uni.removeStorageSync(BOOKING_INTENT_KEY)
  } catch {
    // Local storage failures must not interrupt page flow.
  }
}

export function setBookingIntent(input) {
  const intent = normalizeIntent(input)
  if (!intent) {
    clearBookingIntent()
    return false
  }

  try {
    uni.setStorageSync(BOOKING_INTENT_KEY, intent)
    return true
  } catch {
    clearBookingIntent()
    return false
  }
}

export function consumeBookingIntent() {
  let intent = null
  let readFailed = false

  try {
    const raw = uni.getStorageSync(BOOKING_INTENT_KEY)
    const value = typeof raw === 'string' ? JSON.parse(raw) : raw
    intent = normalizeIntent(value)
  } catch {
    readFailed = true
  }

  try {
    uni.removeStorageSync(BOOKING_INTENT_KEY)
  } catch {
    return null
  }

  return readFailed ? null : intent
}
