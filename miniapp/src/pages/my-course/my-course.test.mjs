import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import vm from 'node:vm'

const path = new URL('./my-course.vue', import.meta.url)
assert.ok(existsSync(path), 'enrolled courses need a dedicated page')
const source = readFileSync(path, 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import[^\n]+\n/gm, '')
const navigationScript = readFileSync(new URL('../../utils/courseNavigation.js', import.meta.url), 'utf8').replace(/^export /gm, '')
assert.doesNotMatch(source, /requestPayment|payWechatOrder|createCourse\w*OrderApi|<video|立即支付/)
const routes = JSON.parse(readFileSync(new URL('../../pages.json', import.meta.url), 'utf8'))
assert.ok(routes.pages.some(item => item.path === 'pages/my-course/my-course'))
function fixture(extra = {}) {
  return {
    owned: true, bookingId: '52', courseId: 'growth', syncStatus: 'confirmed', catalogAvailable: true,
    order: { id: '88', bookingId: '52', courseId: 'growth', status: 'paid', title: '购买时的课程标题', amount: 19900, outTradeNo: 'order-52', paidAt: '2026/10/03 09:30:00' },
    course: { id: 'growth', title: '后台更新后的标题', cover: 'https://cdn.example/course-original.jpg', schedule: '10月18日 09:00', location: '工作室', duration: '2天', outline: ['认识行为模式', '日常觉察练习'], notice: '请提前十分钟签到', enabled: true },
    customerServiceQr: 'https://cdn.example/service-qr.jpg', ...extra,
  }
}
function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function harness() {
  const state = { token: 'session-a', queries: [], navigation: [], pages: [], previews: [], hiddenMenus: [], devtools: false, get: async () => fixture() }
  const pageFromUrl = url => {
    const parsed = new URL(url, 'https://miniapp.example')
    return { route: parsed.pathname.slice(1), options: Object.fromEntries(parsed.searchParams) }
  }
  const context = vm.createContext({
    ref: value => ({ value }), computed: getter => ({ get value() { return getter() } }),
    onLoad: fn => { state.load = fn }, onShow: fn => { state.show = fn }, onHide: fn => { state.hide = fn }, onUnload: fn => { state.unload = fn }, onPullDownRefresh: fn => { state.pull = fn },
    getToken: () => state.token,
    getCurrentPages: () => state.pages,
    getCourseEnrollmentApi: query => { state.queries.push(query); return state.get(query) },
    previewImage: url => state.previews.push(url), isWechatDevtools: () => state.devtools,
    uni: {
      navigateTo: options => { state.navigation.push({ ...options, method: 'navigateTo' }); state.pages.push(pageFromUrl(options.url)) },
      redirectTo: options => { state.navigation.push({ ...options, method: 'redirectTo' }); if (state.pages.length) state.pages.pop(); state.pages.push(pageFromUrl(options.url)) },
      navigateBack: options => { state.navigation.push({ ...options, method: 'navigateBack' }); state.pages.splice(-options.delta) },
      switchTab: options => state.navigation.push(options),
      hideShareMenu: options => state.hiddenMenus.push(options), stopPullDownRefresh() {},
    },
  })
  vm.runInContext(`${navigationScript}\n${script}\nglobalThis.page = { enrollment, state: viewState, isOwned, title, amountText, schedule, location, duration, outline, notice, previewVisible, previewSrc, loadEnrollment, previewCover, previewContact, goOrders, goPaymentResult, openTeacher }`, context)
  return { state, page: context.page }
}
{
  const { state, page } = harness()
  const pending = deferred()
  state.get = () => pending.promise
  state.load({ bookingId: '52', owned: 'true', status: 'paid', amount: 1 })
  const loading = state.show()
  assert.equal(page.isOwned.value, false, 'URL flags cannot grant ownership')
  assert.equal(page.enrollment.value, null)
  pending.resolve(fixture())
  await loading
  assert.equal(page.isOwned.value, true)
  assert.equal(page.title.value, '购买时的课程标题')
  assert.equal(page.amountText.value, '199.00', 'paid amount is the historical server order amount')
  assert.equal(page.schedule.value, '10月18日 09:00')
  assert.equal(page.location.value, '工作室')
  assert.equal(page.duration.value, '2天')
  assert.deepEqual(Array.from(page.outline.value), ['认识行为模式', '日常觉察练习'])
  assert.equal(page.notice.value, '请提前十分钟签到')
  page.previewCover()
  assert.equal(state.previews[0], 'https://cdn.example/course-original.jpg')
  state.devtools = true
  page.previewContact()
  assert.equal(page.previewVisible.value, true)
  assert.equal(page.previewSrc.value, 'https://cdn.example/service-qr.jpg')
  page.goPaymentResult()
  assert.equal(state.navigation.at(-1).url, '/pages/payment-result/payment-result?bookingId=52')
  page.goOrders()
  assert.equal(state.navigation.at(-1).url, '/pages/orders/orders')
  page.openTeacher()
  assert.equal(state.navigation.at(-1).url, '/pages/teacher/teacher')
  assert.deepEqual(Array.from(state.hiddenMenus[0].menus), ['shareAppMessage', 'shareTimeline'])
}
for (const status of ['pending', 'closed', 'refunded']) {
  const { state, page } = harness()
  state.get = async () => fixture({ owned: false, order: { ...fixture().order, status } })
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.isOwned.value, false)
  assert.equal(state.navigation.at(-1).url, '/pages/payment-result/payment-result?bookingId=52')
  page.previewCover()
  assert.equal(state.previews.length, 0, 'unconfirmed purchases do not expose enrolled-course actions')
}
{
  const { state, page } = harness()
  state.get = async () => fixture({ owned: true, order: { ...fixture().order, status: 'pending' } })
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.isOwned.value, false, 'owned flag alone does not override pending payment')
}
{
  const { state, page } = harness()
  state.get = async () => fixture({ catalogAvailable: false, course: { id: 'growth', title: '购买时的课程标题', enabled: false }, customerServiceQr: '' })
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.isOwned.value, true, 'a deleted catalog entry keeps the paid enrollment receipt')
  assert.equal(page.schedule.value, '待通知')
  assert.equal(page.location.value, '待通知')
  assert.equal(page.duration.value, '待通知')
  assert.equal(page.outline.value.length, 0)
  page.previewContact()
  assert.equal(state.previews.length, 0, 'missing QR never fabricates a contact')
}
{
  const { state, page } = harness()
  state.get = async () => fixture({ course: { ...fixture().course, enabled: false } })
  state.load({ courseId: 'growth' })
  await state.show()
  assert.equal(page.isOwned.value, true, 'unlisted but purchased courses remain accessible')
  assert.equal(state.queries[0].courseId, 'growth')
}
for (const code of [403, 404]) {
  const { state, page } = harness()
  state.get = async () => { throw Object.assign(new Error('private backend message'), { statusCode: code }) }
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.enrollment.value, null)
  assert.equal(page.isOwned.value, false)
  assert.equal(page.state.value, 'unavailable')
  assert.equal(state.token, 'session-a', 'ownership errors never clear a valid session')
}
{
  const { state, page } = harness()
  state.load({ bookingId: '52' })
  await state.show()
  state.token = 'session-b'
  await state.show()
  assert.equal(page.enrollment.value, null)
  assert.equal(page.isOwned.value, false)
  assert.equal(state.queries.length, 1)
  assert.equal(state.navigation.at(-1).url, '/pages/profile/profile')
}
for (const finish of ['account', 'hide', 'unload']) {
  const { state, page } = harness()
  const late = deferred()
  state.get = () => late.promise
  state.load({ bookingId: '52' })
  const loading = state.show()
  if (finish === 'account') state.token = 'session-b'
  else state[finish]()
  late.resolve(fixture())
  await loading
  assert.equal(page.enrollment.value, null, `${finish} invalidates stale private content`)
  assert.equal(page.isOwned.value, false)
}
{
  const { state, page } = harness()
  state.load({ bookingId: '52' })
  await state.show()
  state.hide()
  assert.equal(page.enrollment.value, null)
  await state.show()
  assert.equal(page.isOwned.value, true, 'returning to the page reconfirms backend ownership')
  assert.equal(state.queries.length, 2)
}
for (const query of [{}, { bookingId: '0' }, { bookingId: 'bad' }, { bookingId: '52', courseId: 'growth' }]) {
  const { state, page } = harness()
  state.load(query)
  await state.show()
  assert.equal(page.isOwned.value, false)
  assert.equal(state.queries.length, 0)
}
{
  const { state, page } = harness()
  state.get = async () => fixture({ bookingId: '99' })
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(page.isOwned.value, false, 'a different booking response never grants this enrollment')
  assert.equal(page.enrollment.value, null)
}
{
  const { state, page } = harness()
  state.get = async () => fixture({ bookingId: '', order: { ...fixture().order, bookingId: '' } })
  state.load({ courseId: 'growth' })
  await state.show()
  assert.equal(page.isOwned.value, false, 'a paid flag without a valid booking identity is not an enrollment receipt')
}
{
  const { state, page } = harness()
  state.get = async () => fixture({ owned: false, bookingId: '', order: null })
  state.load({ courseId: 'growth' })
  await state.show()
  assert.equal(page.isOwned.value, false)
  assert.equal(page.state.value, 'unavailable')
  assert.equal(state.navigation.length, 0, 'a course without an order never opens a fabricated payment result')
}
{
  const { state, page } = harness()
  state.load({ bookingId: '52' })
  await state.show()
  state.get = async () => { throw new Error('offline') }
  await page.loadEnrollment()
  assert.equal(page.enrollment.value, null, 'failed revalidation does not leave a stale paid receipt visible')
  assert.equal(page.state.value, 'error')
}
{
  const { state, page } = harness()
  state.token = ''
  state.load({ bookingId: '52' })
  await state.show()
  assert.equal(state.queries.length, 0)
  assert.equal(page.isOwned.value, false)
}
{
  const { state, page } = harness()
  const ordersPage = { route: 'pages/orders/orders', options: {} }
  const coursePage = { route: 'pages/my-course/my-course', options: { bookingId: '52' } }
  state.pages = [ordersPage, coursePage]
  state.load({ bookingId: '52' })
  await state.show()
  for (let visit = 0; visit < 20; visit++) {
    page.goOrders()
    assert.equal(state.pages.length, 1, 'returning to orders must not accumulate duplicate orders pages')
    assert.equal(state.pages[0], ordersPage, 'the existing orders instance is reused')
    assert.equal(state.navigation.at(-1).method, 'navigateBack')
    assert.equal(state.navigation.at(-1).delta, 1)
    state.pages.push(coursePage)
  }
}
{
  const { state, page } = harness()
  state.load({ bookingId: '52' })
  await state.show()
  page.goOrders()
  assert.equal(state.navigation.at(-1).method, 'redirectTo', 'a direct entry without orders in its stack replaces the current page')
  assert.equal(state.navigation.at(-1).url, '/pages/orders/orders')
}
{
  const { state, page } = harness()
  state.pages = [
    { route: 'pages/payment-result/payment-result', options: { bookingId: '52' } },
    { route: 'pages/my-course/my-course', options: { bookingId: '52' } },
  ]
  state.load({ bookingId: '52' })
  await state.show()
  page.goPaymentResult()
  assert.equal(state.navigation.at(-1).method, 'navigateBack', 'an existing payment record for the same booking is reused')
  assert.equal(state.pages.length, 1)
}
console.log('My course ownership, historical receipt, navigation stack, preview and session tests passed')
