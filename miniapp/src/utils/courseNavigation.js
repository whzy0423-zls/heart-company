function navigateCoursePage(route, { bookingId = '', replace = false, fail } = {}) {
  const url = `/${route}${bookingId ? `?bookingId=${encodeURIComponent(bookingId)}` : ''}`
  let pages = []
  try {
    const current = typeof getCurrentPages === 'function' ? getCurrentPages() : []
    if (Array.isArray(current)) pages = current
  } catch { /* Direct-entry navigation remains available without a page stack. */ }

  // Reused pages revalidate the current session and backend status in onShow.
  for (let index = pages.length - 1; index >= 0; index--) {
    const page = pages[index]
    const pageRoute = String(page?.route || page?.$page?.route || '').replace(/^\/+/, '')
    const pageBooking = String(page?.options?.bookingId ?? page?.$page?.options?.bookingId ?? '')
    if (pageRoute !== route || (bookingId && pageBooking !== bookingId)) continue
    const delta = pages.length - 1 - index
    if (delta > 0) {
      uni.navigateBack({ delta, fail: () => uni.redirectTo({ url, fail }) })
    }
    return true
  }

  if (replace) uni.redirectTo({ url, fail })
  else uni.navigateTo({ url, fail })
  return true
}

export function navigateToOrders() {
  return navigateCoursePage('pages/orders/orders', { replace: true })
}

export function navigateToMyCourse(bookingId) {
  const id = String(bookingId || '')
  if (!/^[1-9]\d*$/.test(id)) return false
  return navigateCoursePage('pages/my-course/my-course', { bookingId: id })
}

export function navigateToPaymentResult(bookingId, { replace = false, fail } = {}) {
  const id = String(bookingId || '')
  if (!/^[1-9]\d*$/.test(id)) return false
  return navigateCoursePage('pages/payment-result/payment-result', { bookingId: id, replace, fail })
}
