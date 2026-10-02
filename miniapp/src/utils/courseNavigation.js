function navigateCoursePage(route, { bookingId = '', replace = false, fail, success, complete, isCurrent = () => true } = {}) {
  const url = `/${route}${bookingId ? `?bookingId=${encodeURIComponent(bookingId)}` : ''}`
  let finished = false
  const finish = (ok, result) => {
    if (finished) return
    finished = true
    if (isCurrent()) {
      if (ok) success?.(result)
      else if (fail) fail(result)
      else uni.showToast({ title: '页面打开失败，请稍后重试', icon: 'none' })
    }
    complete?.(result)
  }
  const done = result => finish(true, result)
  const failed = error => finish(false, error)
  const invoke = (method, options) => {
    if (finished) return
    if (!isCurrent()) { finish(false, { errMsg: 'navigation cancelled' }); return }
    try { uni[method](options) } catch (error) { options.fail(error) }
  }
  // Replacing the current page avoids the WeChat 10-page limit and gives a
  // failed push one recovery attempt. Surface the final failure to the caller.
  const redirect = () => invoke('redirectTo', { url, success: done, fail: failed })
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
      invoke('navigateBack', { delta, success: done, fail: redirect })
    } else done({ errMsg: 'navigateTo:ok already open' })
    return true
  }

  if (replace || pages.length >= 10) redirect()
  else invoke('navigateTo', { url, success: done, fail: redirect })
  return true
}

export function navigateToOrders() {
  return navigateCoursePage('pages/orders/orders', { replace: true })
}

export function navigateToMyCourse(bookingId, options = {}) {
  const id = String(bookingId || '')
  if (!/^[1-9]\d*$/.test(id)) return false
  return navigateCoursePage('pages/my-course/my-course', { ...options, bookingId: id })
}

export function navigateToPaymentResult(bookingId, options = {}) {
  const id = String(bookingId || '')
  if (!/^[1-9]\d*$/.test(id)) return false
  return navigateCoursePage('pages/payment-result/payment-result', { ...options, bookingId: id })
}
