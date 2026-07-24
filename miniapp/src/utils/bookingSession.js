import { normalizeBookingId } from './bookingDisplay.js'

let bookingSession = null

function isValidOwnerToken(ownerToken) {
  return typeof ownerToken === 'string' && Boolean(ownerToken.trim())
}

export function setBookingSession(ownerToken, record) {
  clearBookingSession()

  if (!isValidOwnerToken(ownerToken)) return false
  if (!record || typeof record !== 'object' || Array.isArray(record)) return false
  if (!normalizeBookingId(record.id)) return false

  bookingSession = {
    ownerToken,
    record: { ...record },
  }
  return true
}

export function readBookingSession(ownerToken, bookingId) {
  const normalizedId = normalizeBookingId(bookingId)
  const cachedId = normalizeBookingId(bookingSession?.record?.id)

  if (
    !bookingSession ||
    !isValidOwnerToken(ownerToken) ||
    bookingSession.ownerToken !== ownerToken ||
    !normalizedId ||
    cachedId !== normalizedId
  ) {
    clearBookingSession()
    return null
  }

  return { ...bookingSession.record }
}

export function clearBookingSession() {
  bookingSession = null
}
