import { isCourseRegistrationEnabled } from '../../utils/courseRegistration.js'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import vm from 'node:vm'

const pageUrl = new URL('./payment-result.vue', import.meta.url)
assert.ok(existsSync(pageUrl), 'payment result page must exist')
const routes = JSON.parse(readFileSync(new URL('../../pages.json', import.meta.url), 'utf8'))
const route = routes.pages.find(item => item.path === 'pages/payment-result/payment-result')
assert.ok(route, 'payment result must be registered in mini-program routes')
assert.equal(route.style.enableShareAppMessage, undefined, 'unsupported sharing fields must not enter WeChat page.json')
assert.equal(route.style.enableShareTimeline, undefined, 'unsupported sharing fields must not enter WeChat page.json')
const source = readFileSync(pageUrl, 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import[^\n]+\n/gm, '')
const navigationScript = readFileSync(new URL('../../utils/courseNavigation.js', import.meta.url), 'utf8').replace(/^export /gm, '')
assert.doesNotMatch(script, /createCourse\w*OrderApi|payWechatOrder|requestPayment/, 'result page only queries payment status')
assert.doesNotMatch(source, /再次支付|重新支付|立即支付/)

const response = (status = 'pending', extra = {}) => ({ bookingId: '52', status, title: '认识自己的成长课', amount: 19900, outTradeNo: 'course-order-52', courseId: 'growth', ...extra })
function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
function harness({ shareAvailable = true } = {}) {
  const state = { token: 'session-a', queries: [], navigation: [], pages: [], shareCalls: [], timers: new Map(), nextTimer: 0, status: async () => response() }
  const pageFromUrl = url => {
    const parsed = new URL(url, 'https://miniapp.example')
    return { route: parsed.pathname.slice(1), options: Object.fromEntries(parsed.searchParams) }
  }
  const context = vm.createContext({
    isCourseRegistrationEnabled,
    getStoredSiteConfig: () => state.config || {}, refreshSiteConfig: async () => state.config || {},
    ref: value => ({ value }), computed: getter => ({ get value() { return getter() } }),
    onLoad: fn => { state.load = fn }, onShow: fn => { state.show = fn },
    onHide: fn => { state.hide = fn }, onUnload: fn => { state.unload = fn },
    getToken: () => state.token,
    getCurrentPages: () => state.pages,
    getCourseBookingOrderStatusApi: id => { state.queries.push(id); return state.status(id) },
    setTimeout: (fn, delay) => { const id = ++state.nextTimer; state.timers.set(id, { fn, delay }); return id },
    clearTimeout: id => state.timers.delete(id),
    uni: {
      ...(shareAvailable ? { hideShareMenu: options => state.shareCalls.push(options) } : {}),
      switchTab: options => state.navigation.push(options),
      navigateTo: options => { state.navigation.push({ ...options, method: 'navigateTo' }); state.pages.push(pageFromUrl(options.url)) },
      redirectTo: options => { state.navigation.push({ ...options, method: 'redirectTo' }); if (state.pages.length) state.pages.pop(); state.pages.push(pageFromUrl(options.url)) },
      navigateBack: options => { state.navigation.push({ ...options, method: 'navigateBack' }); state.pages.splice(-options.delta) },
    },
  })
  vm.runInContext(`${navigationScript}\n${script}\nglobalThis.page = { bookingId, order, status, loading, queryError, loginNeeded, invalidBooking, isPaid, headline, refreshResult, goOrders, openCourse, browseCourses }`, context)
  state.visitPaymentResult = id => context.navigateToPaymentResult(id)
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
  assert.deepEqual(Array.from(state.shareCalls[0].menus), ['shareAppMessage', 'shareTimeline'], 'private results hide both share menus at runtime')
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
  assert.equal(state.navigation.at(-1).url, '/pages/my-course/my-course?bookingId=52', 'success opens the confirmed enrollment')
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

{
  const { state, page } = harness({ shareAvailable: false })
  state.status = async () => response('paid')
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.isPaid.value, true, 'environments without the WeChat sharing API still query normally')
}
console.log('payment result behavior passed: confirmation, bounded polling, forged params, session and lifecycle isolation')

{
  const { state, page } = harness()
  state.status = async () => response('paid', {courseId:''})
  state.load({bookingId:'52'})
  await state.show()
  page.openCourse()
  assert.equal(state.navigation.at(-1).url, '/pages/my-course/my-course?bookingId=52', 'historical enrollment opens even after course removal')
}
{
  const { state, page } = harness()
  state.status = async () => response('pending')
  state.load({bookingId:'52', paid:'true'})
  await state.show()
  page.openCourse()
  assert.equal(state.navigation.at(-1).url, '/pages/course-detail/course-detail?id=growth', 'unconfirmed payment does not open owned course')
}
{
  const { state, page } = harness()
  const ordersPage = { route: 'pages/orders/orders', options: {} }
  state.pages = [ordersPage, { route: 'pages/my-course/my-course', options: { bookingId: '52' } }, { route: 'pages/payment-result/payment-result', options: { bookingId: '52' } }]
  state.status = async () => response('paid')
  state.load({ bookingId: '52' })
  await state.show()
  page.goOrders()
  assert.equal(state.navigation.at(-1).method, 'navigateBack')
  assert.equal(state.navigation.at(-1).delta, 2, 'return to the existing orders even through intervening pages')
  assert.equal(state.pages.length, 1)
  assert.equal(state.pages[0], ordersPage)
}
{
  const { state, page } = harness()
  state.status = async () => response('paid')
  state.load({ bookingId: '52' })
  await state.show()
  page.goOrders()
  assert.equal(state.navigation.at(-1).method, 'redirectTo', 'missing stack entries retain a direct-entry fallback')
  assert.equal(state.navigation.at(-1).url, '/pages/orders/orders')
}
{
  const { state, page } = harness()
  const coursePage = { route: 'pages/my-course/my-course', options: { bookingId: '52' } }
  state.pages = [coursePage, { route: 'pages/payment-result/payment-result', options: { bookingId: '52' } }]
  state.status = async () => response('paid')
  state.load({ bookingId: '52' })
  await state.show()
  for (let visit = 0; visit < 20; visit++) {
    page.openCourse()
    assert.equal(state.pages.length, 1, 'course/receipt cycles must not grow the page stack')
    assert.equal(state.pages[0], coursePage)
    assert.equal(state.navigation.at(-1).method, 'navigateBack')
    state.visitPaymentResult('52')
    assert.equal(state.pages.length, 2)
  }
}
{
  const { state, page } = harness()
  state.pages = [{ route: 'pages/my-course/my-course', options: { bookingId: '99' } }, { route: 'pages/payment-result/payment-result', options: { bookingId: '52' } }]
  state.status = async () => response('paid')
  state.load({ bookingId: '52' })
  await state.show()
  page.openCourse()
  assert.equal(state.navigation.at(-1).method, 'navigateTo', 'a different booking never reuses the old owned-course page')
  assert.equal(state.navigation.at(-1).url, '/pages/my-course/my-course?bookingId=52')
  assert.equal(state.pages.length, 3)
  assert.equal(state.pages.at(-1).options.bookingId, '52')
}
{
  const { state, page } = harness()
  const matching = { route: '/pages/my-course/my-course', $page: { options: { bookingId: '52' } } }
  state.pages = [matching, { route: 'pages/my-course/my-course', options: { bookingId: '99' } }, { route: 'pages/payment-result/payment-result', options: { bookingId: '52' } }]
  state.status = async () => response('paid')
  state.load({ bookingId: '52' })
  await state.show()
  page.openCourse()
  assert.equal(state.navigation.at(-1).method, 'navigateBack')
  assert.equal(state.navigation.at(-1).delta, 2, 'match both route and booking rather than the closest course page')
  assert.equal(state.pages[0], matching)
  const callsBefore = state.navigation.length
  page.openCourse()
  assert.equal(state.navigation.length, callsBefore, 'a repeated tap already at its destination adds no page')
}
console.log('payment result navigation stack regression passed')
