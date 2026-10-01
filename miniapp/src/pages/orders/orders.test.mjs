import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'

const source = await readFile(new URL('./orders.vue', import.meta.url), 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import[^\n]+\n/gm, '')
const routes = JSON.parse(await readFile(new URL('../../pages.json', import.meta.url), 'utf8'))
const profile = await readFile(new URL('../profile/profile.vue', import.meta.url), 'utf8')
assert.ok(routes.pages.find((item) => item.path === 'pages/orders/orders')?.style.enablePullDownRefresh)
assert.match(profile, /@click="openOrders"/)
assert.match(profile, /url: '\/pages\/orders\/orders'/)

const order = (id, extra = {}) => ({ id: String(id), outTradeNo: `order-${id}`, product: 'course_booking', title: `课程 ${id}`, amount: 1999, status: 'pending', bookingId: String(id + 100), courseId: `course-${id}`, createTime: '2026-10-01 12:00', ...extra })
function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
async function flush() { for (let i = 0; i < 8; i++) await Promise.resolve() }
function harness() {
  const state = {
    token: 'session-a', calls: [], toasts: [], navigation: [], stops: 0, controllerCount: 0, creates: [], checks: [], devPays: [], stoppedRefresh: 0,
    list: async () => ({ items: [order(1)], total: 1, page: 1, pageSize: 20 }),
    status: async () => ({ status: 'pending' }),
    purchase: async () => ({ state: 'cancelled', message: 'cancelled' }),
  }
  const context = vm.createContext({
    ref: (value) => ({ value }), computed: (fn) => ({ get value() { return fn() } }),
    onShow: (fn) => { state.show = fn }, onUnload: (fn) => { state.unload = fn },
    onPullDownRefresh: (fn) => { state.pull = fn }, onReachBottom: (fn) => { state.bottom = fn },
    getToken: () => state.token, clearToken: () => { state.token = '' },
    listMiniappOrdersApi: (query) => { state.calls.push(query); return state.list(query) },
    getCourseBookingOrderStatusApi: (id) => { state.checks.push(id); return state.status(id) },
    createCourseBookingOrderApi: async (id) => { state.creates.push(id); await state.createGate; return { bookingId: id, outTradeNo: 'new-order', payParams: {} } },
    payWechatOrder: async () => { state.pays = (state.pays || 0) + 1 },
    devPayCourseBookingOrderApi: async (outTradeNo) => { state.devPays.push(outTradeNo) },
    coursePaymentResultUrl: (order) => order?.bookingId ? `/pages/payment-result/payment-result?bookingId=${order.bookingId}` : '',
    createCoursePaymentController: (options) => {
      state.controllerCount++
      state.controllerOptions = options
      return { purchase: async () => { const created = await options.create(); await options.pay(created); return { ...(await state.purchase(options)), order: created } }, resume: () => { state.resumes = (state.resumes || 0) + 1 }, stop: () => { state.stops++ } }
    },
    userErrorMessage: (error, fallback) => error?.message || fallback,
    uni: {
      showToast: (value) => state.toasts.push(value), navigateTo: (value) => state.navigation.push(value),
      switchTab: (value) => state.navigation.push(value), stopPullDownRefresh: () => { state.stoppedRefresh++ },
    },
  })
  vm.runInContext(`${script}\nglobalThis.actions = { orders, loading, loadingMore, loadError, moreError, page, total, hasMore, payingId, paymentMessage, refresh, loadMore, selectFilter, continuePayment, openCourse, amountYuan, statusLabel, canContinue }`, context)
  return { state, page: context.actions }
}

{
  const { state, page } = harness()
  state.list = async ({ page: number }) => ({ items: number === 1 ? [order(1), order(2)] : [order(2), order(3)], total: 3, page: number })
  await state.show()
  assert.equal(page.orders.value.length, 2)
  assert.equal(page.amountYuan(page.orders.value[0].amount), '19.99', 'display historical cents from order snapshot')
  assert.equal(page.hasMore.value, true)
  await state.bottom()
  assert.deepEqual(Array.from(page.orders.value, (item) => item.id), ['1', '2', '3'], 'pagination appends and deduplicates rows')
  assert.equal(state.calls[1].page, 2)
  assert.equal(page.hasMore.value, false)
  await page.loadMore()
  assert.equal(state.calls.length, 2, 'loaded final page must not fetch again')
  await page.selectFilter('paid')
  assert.equal(state.calls.at(-1).status, 'paid')
  assert.equal(state.calls.at(-1).page, 1, 'filter change resets pagination')
  await state.pull()
  assert.equal(state.stoppedRefresh, 1)
}
{
  const { state, page } = harness()
  state.list = async ({ page: number }) => { if (number === 2) throw new Error('网络中断'); return { items: [order(1)], total: 2, page: 1 } }
  await state.show()
  await page.loadMore()
  assert.equal(page.moreError.value, '网络中断')
  assert.equal(page.orders.value.length, 1, 'load-more error retains already loaded orders')
  assert.equal(page.page.value, 1, 'failed page does not advance cursor')
  state.list = async () => ({ items: [order(2)], total: 2, page: 2 })
  await page.loadMore()
  assert.equal(state.calls.at(-1).page, 2, 'retry uses failed page')
  assert.equal(page.moreError.value, '')
  assert.equal(page.orders.value.length, 2)
}
{
  const { state, page } = harness()
  const old = deferred()
  state.list = () => old.promise
  const first = state.show()
  state.token = 'session-b'
  state.list = async () => ({ items: [order(8)], total: 1, page: 1 })
  await state.show()
  old.resolve({ items: [order(1)], total: 1, page: 1 })
  await first
  assert.equal(page.orders.value[0].id, '8', 'old account response must not restore private orders')
  assert.equal(state.token, 'session-b')
}
{
  const { state, page } = harness()
  const old = deferred()
  state.list = () => old.promise
  const first = state.show()
  state.token = 'session-b'
  old.reject(Object.assign(new Error('Expired'), { statusCode: 401, authExpired: true, requestToken: 'session-a' }))
  await first
  assert.equal(page.orders.value.length, 0)
  assert.equal(state.token, 'session-b', 'stale auth errors must not clear the newer login')
  assert.equal(state.navigation.length, 0)
}
{
  const { state, page } = harness()
  state.list = async () => { state.token = ''; throw Object.assign(new Error('Expired'), { statusCode: 401, authExpired: true, requestToken: 'session-a' }) }
  await state.show()
  assert.equal(page.orders.value.length, 0)
  assert.equal(state.navigation.at(-1).url, '/pages/profile/profile')
}
{
  const { state, page } = harness()
  state.token = ''
  await state.show()
  assert.equal(state.calls.length, 0, 'unauthenticated page never requests private orders')
  assert.equal(state.navigation.at(-1).url, '/pages/profile/profile')
  assert.equal(page.orders.value.length, 0)
}
{
  const { state, page } = harness()
  await state.show()
  state.status = async () => ({ status: 'paid' })
  state.list = async () => ({ items: [order(1, { status: 'paid' })], total: 1, page: 1 })
  await page.continuePayment(page.orders.value[0])
  assert.deepEqual(state.checks, ['101'], 'continue payment checks service status first')
  assert.equal(state.controllerCount, 0, 'already-paid order never opens payment controller')
  assert.equal(state.creates.length, 0, 'already-paid order never creates another order')
  assert.equal(state.navigation.at(-1).url, '/pages/payment-result/payment-result?bookingId=101')
  assert.equal(page.orders.value[0].status, 'paid')
  assert.equal(page.canContinue(page.orders.value[0]), false)
  page.openCourse(page.orders.value[0])
  assert.equal(state.navigation.at(-1).url, '/pages/my-course/my-course?bookingId=101', 'paid orders open their enrollment, never the purchase page')
}
{
  const { state, page } = harness()
  await state.show()
  await page.continuePayment(page.orders.value[0])
  assert.equal(state.creates[0], '101')
  assert.equal(page.orders.value[0].status, 'pending', 'cancellation retains pending order for a later retry')
  assert.equal(page.payingId.value, '')
  assert.equal(state.navigation.at(-1).url, '/pages/payment-result/payment-result?bookingId=101', 'uncertain native payment opens result page')
}
{
  const { state, page } = harness()
  await state.show()
  const payment = deferred()
  state.purchase = () => payment.promise
  const pending = page.continuePayment(page.orders.value[0])
  await flush()
  assert.equal(page.payingId.value, '1')
  const listCalls = state.calls.length
  await state.show()
  assert.equal(state.calls.length, listCalls, 'wallet return onShow does not cancel an in-flight payment')
  assert.equal(state.resumes, 1, 'wallet return starts independent status recovery')
  state.list = async () => ({ items: [order(1, { status: 'paid' })], total: 1, page: 1 })
  payment.resolve({ state: 'success', status: { status: 'paid' } })
  await pending
  assert.equal(page.orders.value[0].status, 'paid')
  assert.equal(page.payingId.value, '')
}
{
  const { state, page } = harness()
  await state.show()
  const payment = deferred()
  state.purchase = () => payment.promise
  const pending = page.continuePayment(page.orders.value[0])
  await flush()
  state.token = 'session-b'
  state.list = async () => ({ items: [order(8)], total: 1, page: 1 })
  await state.show()
  assert.equal(state.stops, 1, 'account switch stops the former payment controller')
  payment.resolve({ state: 'success', status: { status: 'paid' } })
  await pending
  assert.equal(page.orders.value[0].id, '8')
  assert.equal(state.toasts.length, 0, 'old payment success never reports success in a new account')
}
{
  const { state, page } = harness()
  await state.show()
  const payment = deferred()
  state.purchase = () => payment.promise
  const pending = page.continuePayment(page.orders.value[0])
  await flush()
  state.unload()
  assert.equal(state.stops, 1, 'unloading stops payment polling')
  payment.resolve({ state: 'success', status: { status: 'paid' } })
  await pending
  assert.equal(page.orders.value.length, 0)
  assert.equal(state.toasts.length, 0)
  await page.refresh()
  assert.equal(state.calls.length, 1, 'unloaded page cannot start more requests')
}
{
  const { state, page } = harness()
  const fetch = deferred()
  state.list = () => fetch.promise
  const pending = state.show()
  state.unload()
  fetch.resolve({ items: [order(1)], total: 1, page: 1 })
  await pending
  assert.equal(page.orders.value.length, 0, 'unloaded page ignores request completion')
}
{
  const { state, page } = harness()
  await state.show()
  const created = deferred()
  state.createGate = created.promise
  const pending = page.continuePayment(page.orders.value[0])
  await flush()
  state.token = 'session-b'
  created.resolve()
  await pending
  assert.equal(state.pays || 0, 0, 'account switch during order creation never opens the old account cashier')
  assert.equal(page.orders.value.length, 0)
}
{
  const { state, page } = harness()
  await state.show()
  state.status = async () => ({ status: 'pending', syncStatus: 'retrying' })
  await page.continuePayment(page.orders.value[0])
  assert.equal(state.creates.length, 0, 'uncertain upstream status must not trigger another cashier')
  assert.equal(state.navigation.at(-1).url, '/pages/payment-result/payment-result?bookingId=101')
}
console.log('Miniapp orders session, pagination and payment tests passed')

{
  const { state, page } = harness()
  await state.show()
  page.openCourse(order(1, {status:'paid', courseId:''}))
  assert.equal(state.navigation.at(-1).url, '/pages/my-course/my-course?bookingId=101', 'historical paid enrollment stays reachable without a catalog course')
  page.openCourse(order(2))
  assert.equal(state.navigation.at(-1).url, '/pages/course-detail/course-detail?id=course-2', 'unpaid discovery still opens the public course')
  state.token='session-b'
  const before=state.navigation.length
  page.openCourse(order(1, {status:'paid'}))
  assert.equal(state.navigation.length,before,'old account navigation cannot open its private enrollment')
}
