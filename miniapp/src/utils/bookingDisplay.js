const EMPTY_VALUE_LABEL = '未填写'

const BOOKING_KIND_LABELS = {
  consult: '1v1 咨询',
  course: '课程报名',
  enterprise: '企业课程',
}

const BOOKING_STATUS_LABELS = {
  pending: '待确认',
  confirmed: '已确认',
  completed: '已完成',
  cancelled: '已取消',
}

export function normalizeBookingId(value) {
  if (value === null || value === undefined) return ''

  try {
    const normalized = decodeURIComponent(String(value).trim()).trim()
    return /^\d+$/.test(normalized) ? normalized : ''
  } catch {
    return ''
  }
}

export function bookingValue(value) {
  if (value === null || value === undefined) return EMPTY_VALUE_LABEL
  const normalized = String(value).trim()
  return normalized || EMPTY_VALUE_LABEL
}

export function bookingKindLabel(value) {
  const normalized = bookingValue(value)
  return Object.prototype.hasOwnProperty.call(BOOKING_KIND_LABELS, normalized) ? BOOKING_KIND_LABELS[normalized] : normalized
}

export function bookingStatusLabel(value) {
  const normalized = bookingValue(value)
  return Object.prototype.hasOwnProperty.call(BOOKING_STATUS_LABELS, normalized) ? BOOKING_STATUS_LABELS[normalized] : normalized
}

export function maskBookingPhone(value) {
  const normalized = bookingValue(value)
  if (normalized === EMPTY_VALUE_LABEL || normalized.length < 7 || !/^\d+$/.test(normalized)) {
    return normalized
  }
  return `${normalized.slice(0, 3)}****${normalized.slice(-4)}`
}
