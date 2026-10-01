import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import vm from 'node:vm'

const pageUrl = new URL('./payment-result.vue', import.meta.url)
assert.ok(existsSync(pageUrl), 'payment result page must exist')
const routes = JSON.parse(readFileSync(new URL('../../pages.json', import.meta.url), 'utf8'))
const route = routes.pages.find(item => item.path === 'pages/payment-result/payment-result')
assert.ok(route, 'payment result must be registered in mini-program routes')
assert.equal(route.style.enableShareAppMessage, false, 'private payment results cannot be shared')
const source = readFileSync(pageUrl, 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import[^\n]+\n/gm, '')
assert.doesNotMatch(script, /createCourse\w*OrderApi|payWechatOrder|requestPayment/, 'result page only queries payment status')
assert.doesNotMatch(source, /再次支付|重新支付|立即支付/)

const response = (status = 'pending', extra = {}) => ({ bookingId: '52', status, title: '认识自己的成长课', amount: 19900, outTradeNo: 'course-order-52', courseId: 'growth', ...extra })
function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
function harness() {
  const state = { token: 'session-a', queries: [], navigation: [], timers: new Map(), nextTimer: 0, status: async () => response() }
  const context = vm.createContext({
    ref: value => ({ value }), computed: getter => ({ get value() { return getter() } }),
    onLoad: fn => { state.load = fn }, onShow: fn => { state.show = fn },
    onHide: fn => { state.hide = fn }, onUnload: fn => { state.unload = fn },
    getToken: () => state.token,
    getCourseBookingOrderStatusApi: id => { state.queries.push(id); return state.status(id) },
    setTimeout: (fn, delay) => { const id = ++state.nextTimer; state.timers.set(id, { fn, delay }); return id },
    clearTimeout: id => state.timers.delete(id),
    uni: {
      switchTab: options => state.navigation.push(options),
      navigateTo: options => state.navigation.push(options),
      redirectTo: options => state.navigation.push(options),
    },
  })
  vm.runInContext(`${script}\nglobalThis.page = { bookingId, order, status, loading, queryError, loginNeeded, invalidBooking, isPaid, headline, refreshResult, goOrders, openCourse, browseCourses }`, context)
  state.tick = async () => {
    const next = state.timers.entries().next().value
    assert.ok(next, 'expected a scheduled status query')
    const [id, timer] = next
    state.timers.delete(id)
    assert.equal(timer.delay, 1200)
    await timer.fn()
  }
  return { state, page: context.page }
}

{
  const { state, page } = harness()
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.isPaid.value, false)
  assert.equal(page.order.value.title, '认识自己的成长课')
  await state.tick()
  const delayed = deferred()
  state.status = () => delayed.promise
  const finalQuery = state.tick()
  assert.equal(page.isPaid.value, false, 'an outstanding query must not imply success')
  delayed.resolve(response('paid', { paidAt: '2026-10-02 12:30:00' }))
  await finalQuery
  assert.equal(page.isPaid.value, true)
  assert.equal(page.headline.value, '支付成功')
  assert.equal(page.order.value.amount, 19900, 'historical cents come from the backend')
  assert.deepEqual(state.queries, ['52', '52', '52'])
  assert.equal(state.timers.size, 0, 'confirmed payment stops polling')
  page.goOrders()
  assert.equal(state.navigation.at(-1).url, '/pages/orders/orders')
  page.openCourse()
  assert.equal(state.navigation.at(-1).url, '/pages/course-detail/course-detail?id=growth')
  page.browseCourses()
  assert.equal(state.navigation.at(-1).url, '/pages/booking/booking')
}
{
  const { state, page } = harness()
  state.status = async () => { throw new Error('network unavailable') }
  state.load({ bookingId: '52', status: 'paid', paid: 'true', title: 'forged course', amount: '1', courseId: 'forged' })
  await state.show()
  assert.equal(page.isPaid.value, false)
  assert.equal(page.order.value, null, 'query metadata never creates private order details')
  assert.equal(page.headline.value, '支付结果确认中')
  assert.ok(page.queryError.value)
  while (state.timers.size) await state.tick()
  assert.equal(state.queries.length, 6, 'query failures have a bounded retry budget')
  assert.equal(page.isPaid.value, false)
  state.status = async () => response('paid')
  await page.refreshResult()
  assert.equal(page.isPaid.value, true, 'manual refresh can confirm a delayed payment')
  assert.equal(state.queries.length, 7)
}
{
  const { state, page } = harness()
  state.status = async () => response('pending', { syncStatus: 'retrying', message: '正在同步微信支付状态' })
  state.load({ bookingId: '52', status: 'paid', paid: '1', title: 'forged course', amount: '0' })
  await state.show()
  while (state.timers.size) await state.tick()
  assert.equal(state.queries.length, 6)
  assert.equal(page.isPaid.value, false, 'pending/retrying from the server is never paid')
  assert.equal(page.order.value.title, '认识自己的成长课')
  assert.equal(page.order.value.amount, 19900)
  assert.equal(page.status.value, 'pending')
}
for (const terminal of ['closed', 'refunded', 'cancelled']) {
  const { state, page } = harness()
  state.status = async () => response(terminal)
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.status.value, terminal)
  assert.equal(page.isPaid.value, false)
  assert.equal(state.timers.size, 0, `${terminal} stops polling`)
}
for (const previousStatus of ['pending', 'closed', 'refunded', 'cancelled']) {
  const { state, page } = harness()
  state.status = async () => response(previousStatus, { syncStatus: 'retrying' })
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.status.value, 'pending', `${previousStatus}/retrying must keep confirming`)
  assert.equal(page.order.value.status, 'pending')
  assert.equal(page.isPaid.value, false)
  assert.equal(page.headline.value, '支付结果确认中')
  assert.equal(state.timers.size, 1, 'an incomplete WeChat query must not stop polling early')
  state.status = async () => response('paid', { syncStatus: 'confirmed' })
  await state.tick()
  assert.equal(page.isPaid.value, true)
  assert.equal(state.timers.size, 0)
}
{
  const { state, page } = harness()
  state.status = async () => response('paid', { syncStatus: 'retrying' })
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.isPaid.value, true, 'an already settled paid status remains authoritative')
  assert.equal(state.timers.size, 0)
}
{
  const { state, page } = harness()
  state.load({ bookingId: '52' })
  await state.show()
  state.token = 'session-b'
  await state.tick()
  assert.equal(state.queries.length, 1, 'an account switch does not query the old account booking')
  assert.equal(page.order.value, null)
  assert.equal(page.bookingId.value, '')
  assert.equal(page.loginNeeded.value, true)
  assert.equal(state.navigation.at(-1).url, '/pages/profile/profile')
  assert.equal(state.timers.size, 0)
}
{
  const { state, page } = harness()
  const late = deferred()
  state.status = () => late.promise
  state.load({ bookingId: '52' })
  const loading = state.show()
  state.token = 'session-b'
  late.resolve(response('paid'))
  await loading
  assert.equal(page.order.value, null, 'old account response cannot disclose an order')
  assert.equal(page.isPaid.value, false)
  assert.equal(page.loginNeeded.value, true)
  assert.equal(state.token, 'session-b', 'stale results never clear the new account token')
}
{
  const { state, page } = harness()
  state.status = async () => response('paid')
  state.load({ bookingId: '52' })
  await state.show()
  state.token = 'session-b'
  await state.show()
  assert.equal(page.order.value, null, 'onShow clears private details before any new account query')
  assert.equal(page.isPaid.value, false)
  assert.equal(state.queries.length, 1)
}
for (const lifecycle of ['hide', 'unload']) {
  const { state, page } = harness()
  const late = deferred()
  state.status = () => late.promise
  state.load({ bookingId: '52' })
  const loading = state.show()
  state[lifecycle]()
  late.resolve(response('paid'))
  await loading
  assert.equal(page.order.value, null, `${lifecycle} invalidates in-flight responses`)
  assert.equal(page.isPaid.value, false)
  assert.equal(page.loading.value, false)
  assert.equal(state.timers.size, 0)
  state.status = async () => response('paid')
  await state.show()
  assert.equal(state.queries.length, lifecycle === 'hide' ? 2 : 1)
  assert.equal(page.isPaid.value, lifecycle === 'hide', 'a hidden page can restart, an unloaded page cannot')
}
{
  const { state, page } = harness()
  state.load({ bookingId: '52' })
  await state.show()
  const staleTimer = state.timers.values().next().value.fn
  state.hide()
  assert.equal(state.timers.size, 0, 'onHide removes scheduled polling')
  await staleTimer()
  assert.equal(state.queries.length, 1)
  assert.equal(page.order.value, null)
  await state.show()
  assert.equal(state.queries.length, 2, 'onShow restarts a pending confirmation')
  state.unload()
  assert.equal(state.timers.size, 0, 'onUnload removes scheduled polling')
}
{
  const { state, page } = harness()
  state.token = ''
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(state.queries.length, 0)
  assert.equal(page.order.value, null)
  assert.equal(page.loginNeeded.value, true)
  assert.equal(state.navigation.at(-1).url, '/pages/profile/profile')
}
{
  const { state, page } = harness()
  state.status = async () => { throw Object.assign(new Error('expired'), { authExpired: true, statusCode: 401 }) }
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.loginNeeded.value, true)
  assert.equal(page.order.value, null)
  assert.equal(state.timers.size, 0)
}
for (const bookingId of [undefined, '', '0', '-1', '52x', '52&status=paid']) {
  const { state, page } = harness()
  state.load({ bookingId, status: 'paid' })
  await state.show()
  assert.equal(state.queries.length, 0, 'invalid identities never reach the API')
  assert.equal(page.invalidBooking.value, true)
  assert.equal(page.isPaid.value, false)
}
{
  const { state, page } = harness()
  state.status = async () => response('paid', { bookingId: '99' })
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.isPaid.value, false, 'a mismatched backend order never confirms this booking')
  assert.equal(page.order.value, null)
}

console.log('payment result behavior passed: confirmation, bounded polling, forged params, session and lifecycle isolation')
