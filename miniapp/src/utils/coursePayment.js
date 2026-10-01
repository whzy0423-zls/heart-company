// Course checkout keeps uncertain native outcomes separate from server payment truth.
// The report/classroom controller has a different access-grant contract and stays unchanged.
export function coursePaymentResultUrl(order) {
  const id = String(order?.bookingId || '')
  return /^[1-9]\d*$/.test(id) ? `/pages/payment-result/payment-result?bookingId=${encodeURIComponent(id)}` : ''
}

export function createCoursePaymentController(options = {}) {
  const wait = options.wait || ((ms) => new Promise(resolve => setTimeout(resolve, ms)))
  const timeoutMs = Math.max(1, Number(options.paymentTimeoutMs) || 30000)
  const maxAttempts = Math.max(1, Math.min(6, Number(options.maxAttempts) || 6))
  let generation = 0
  let operation = null
  let wakeWallet = null
  let walletTimer = null
  let current = { state: 'idle', message: '', order: null, status: null }
  const active = (run) => run === generation && (options.isCurrent?.() ?? true)
  const stopped = () => ({ state: 'stopped', message: '', order: null, status: null })
  function publish(state, message, extra = {}) {
    current = { ...current, ...extra, state, message }
    options.onChange?.({ ...current })
    return { ...current }
  }
  function releaseWallet() {
    if (walletTimer !== null) clearTimeout(walletTimer)
    walletTimer = null
    const wake = wakeWallet
    wakeWallet = null
    wake?.()
  }
  async function confirm(order, run) {
    publish('pending', '正在确认支付结果…')
    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      if (attempt) await wait(1200)
      if (!active(run)) return stopped()
      let status
      try { status = await options.status(order) }
      catch (error) {
        if (!active(run)) return stopped()
        return publish('pending', '支付结果仍在确认，请在结果页刷新查看', { error })
      }
      if (!active(run)) return stopped()
      if (status?.status === 'paid') return publish('success', '支付成功，报名已确认', { status })
      if (status?.syncStatus === 'retrying') {
        return publish('pending', '支付结果仍在确认，请稍后刷新查看', { status })
      }
      if (['closed', 'cancelled', 'canceled', 'failed', 'refunded'].includes(status?.status)) {
        return publish('closed', '订单状态已更新，请查看支付结果', { status })
      }
      publish('pending', status?.message || '支付结果仍在确认，请稍后刷新查看', { status })
    }
    return { ...current }
  }
  function purchase() {
    if (operation) return operation
    const run = ++generation
    const task = (async () => {
      publish('creating', '正在确认课程订单…', { order: null, status: null })
      const order = await options.create()
      if (!active(run)) return stopped()
      if (!coursePaymentResultUrl(order)) throw new Error('订单信息不完整，请到我的订单查看')
      publish('pending', '正在确认支付结果…', { order })
      // A paid/reconciling create response deliberately omits payParams.
      if (order.status === 'paid') return publish('success', '支付成功，报名已确认', { status: order })
      if (order.payParams && order.syncStatus !== 'retrying') {
        publish('pending', '正在打开微信支付…')
        const wake = new Promise(resolve => {
          wakeWallet = resolve
          walletTimer = setTimeout(releaseWallet, timeoutMs)
        })
        // Always observe rejection, including callbacks arriving after a resume/timeout.
        const native = Promise.resolve().then(() => {
          if (!active(run)) return
          return options.pay(order)
        }).catch(() => undefined)
        await Promise.race([native, wake])
        releaseWallet()
      }
      if (!active(run)) return stopped()
      return confirm(order, run)
    })()
    let tracked
    tracked = task.finally(() => {
      releaseWallet()
      if (operation === tracked) operation = null
    })
    operation = tracked
    return tracked
  }
  return {
    purchase,
    resume() { releaseWallet() },
    stop() { generation++; releaseWallet() },
    snapshot() { return { ...current } },
  }
}
