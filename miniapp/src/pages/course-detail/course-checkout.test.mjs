import assert from 'node:assert/strict'
import vm from 'node:vm'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./course-detail.vue', import.meta.url), 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import[^\n]+\n/gm, '')
const controllerSource = readFileSync(new URL('../../utils/coursePayment.js', import.meta.url), 'utf8')
const payment = await import(`data:text/javascript;base64,${Buffer.from(controllerSource).toString('base64')}`)
const enrollment = (owned = false, extra = {}) => ({
  owned, courseId: 'growth', bookingId: owned ? '91' : '', syncStatus: 'confirmed', catalogAvailable: true,
  order: owned ? { bookingId: '91', courseId: 'growth', title: '已报名的成长课', status: 'paid' } : null,
  ...extra,
})
function harness(priceCents, serverStatus = 'paid') {
  const calls = { intents: [], nav: [], create: [], status: [], toasts: [], login: 0, token: 'owner', pay: 0, enrollmentQueries: [], enrollment: async () => enrollment() }
  const item = { id: 'growth', title: '成长课', priceCents, paymentMode: priceCents > 0 ? 'paid' : 'consult' }
  calls.items = [item]
  const context = vm.createContext({
    ref: value => ({ value }), computed: getter => ({ get value() { return getter() } }),
    onHide: fn => { calls.hide = fn }, onLoad: fn => { calls.load = fn }, onUnload: fn => { calls.unload = fn }, onShow: fn => { calls.show = fn },
    onShareAppMessage: fn => { calls.share = fn }, onShareTimeline: fn => { calls.timeline = fn },
    showPublicShareMenu: () => {}, isTimelinePreview: () => false, requireFullMiniapp: () => true, buildShareCard: input => ({ appMessage: input, timeline: input }),
    getStoredSiteConfig: () => ({}), getCachedSiteConfig: async () => ({}),
    normalizeMiniappCourses: () => calls.items, normalizeTeachers: () => [],
    setBookingIntent: intent => { calls.intents.push(intent); return true },
    ensureLogin: async () => { calls.login++; await calls.loginGate; if (calls.loginToken) calls.token = calls.loginToken }, getToken: () => calls.token,
    getCourseEnrollmentApi: query => { calls.enrollmentQueries.push(query); return calls.enrollment(query) },
    createCourseOrderApi: async id => { calls.create.push(id); await calls.createGate; return calls.created || { bookingId: '52', outTradeNo: 'crs52', payParams: {} } },
    payWechatOrder: async () => { calls.pay++; if (calls.payError) throw calls.payError; await calls.payGate },
    getCourseBookingOrderStatusApi: async id => { calls.status.push(id); return { status: serverStatus } },
    createCoursePaymentController: options => payment.createCoursePaymentController({ ...options, wait: async () => {}, paymentTimeoutMs: 20 }),
    coursePaymentResultUrl: payment.coursePaymentResultUrl,
    userErrorMessage: (error, fallback) => error.message || fallback,
    UI_PREVIEW: false, STUDIO_COURSES: [], STUDIO_TEACHER: {},
    uni: {
      switchTab: p => calls.nav.push(p), navigateTo: p => calls.nav.push(p),
      showToast: p => calls.toasts.push(p), showModal: p => calls.toasts.push(p),
    },
  })
  vm.runInContext(`${script}\nglobalThis.page = { enroll, paying,
    get isEnrolled() { return isEnrolled }, get enrollment() { return enrollment },
    get enrollmentLoading() { return enrollmentLoading }, get enrollmentError() { return enrollmentError },
    get enrollmentPending() { return enrollmentPending }, get enrollmentActionText() { return enrollmentActionText },
    get enrollmentActionDisabled() { return enrollmentActionDisabled }, get confirmationPending() { return confirmationPending },
    get refreshEnrollment() { return refreshEnrollment }, get openMyCourse() { return openMyCourse },
    get course() { return course }
  }`, context)
  return { calls, page: context.page }
}
const flush = async () => { for (let i = 0; i < 15; i++) await Promise.resolve() }
const loadPage = async (calls, query = { id: 'growth' }) => { await calls.load(query); await calls.show() }
function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
{
  const { calls, page } = harness(19900)
  calls.enrollment = async () => enrollment(true)
  await loadPage(calls)
  await page.enroll()
  assert.equal(calls.create.length, 0, 'an owned course never creates another checkout')
  assert.equal(calls.pay, 0)
  assert.equal(page.isEnrolled.value, true)
  assert.equal(page.enrollmentActionText.value, '查看我的课程')
  assert.equal(calls.nav.at(-1).url, '/pages/my-course/my-course?bookingId=91')
}
{
  const { calls, page } = harness(0)
  await loadPage(calls)
  await page.enroll()
  assert.equal(calls.create.length, 0)
  assert.equal(calls.intents[0].courseId, 'growth')
  assert.equal(calls.nav[0].url, '/pages/booking/booking')
}
{
  const { calls, page } = harness(19900)
  await loadPage(calls)
  await Promise.all([page.enroll(), page.enroll()])
  assert.deepEqual(calls.create, ['growth'])
  assert.deepEqual(calls.status, ['52'])
  assert.equal(calls.intents.length, 0)
  assert.equal(calls.nav.at(-1).url, '/pages/payment-result/payment-result?bookingId=52')
  assert.equal(page.paying.value, false)
}
for (const nativeFailure of ['cancel', 'network']) {
  const { calls, page } = harness(19900, 'pending')
  calls.payError = { errMsg: `requestPayment:fail ${nativeFailure}` }
  await loadPage(calls)
  await page.enroll()
  assert.ok(calls.status.length, 'cancel or failure still confirms backend state')
  assert.equal(calls.nav.at(-1).url, '/pages/payment-result/payment-result?bookingId=52')
  assert.equal(calls.nav.length, 1)
  assert.equal(page.paying.value, false)
}
{
  const { calls, page } = harness(19900)
  calls.created = { bookingId: '52', status: 'paid' }
  await loadPage(calls)
  await page.enroll()
  assert.equal(calls.pay, 0, 'repeat purchase of paid course does not reopen cashier')
  assert.equal(calls.nav.length, 1)
}
{
  const { calls, page } = harness(19900)
  calls.payGate = new Promise(() => {})
  await loadPage(calls)
  const buying = page.enroll()
  await flush()
  calls.show()
  calls.show()
  await buying
  assert.equal(calls.status.length, 1, 'wallet return recovers missing callback')
  assert.equal(calls.nav.length, 1, 'multiple lifecycle signals navigate only once')
}
{
  const { calls, page } = harness(19900)
  await loadPage(calls)
  let release
  calls.loginGate = new Promise(resolve => { release = resolve })
  const buying = page.enroll()
  calls.unload()
  release()
  await buying
  assert.equal(calls.create.length, 0)
}
{
  const { calls, page } = harness(19900)
  await loadPage(calls)
  let release
  calls.createGate = new Promise(resolve => { release = resolve })
  const buying = page.enroll()
  await flush()
  calls.token = 'other-user'
  release()
  await buying
  assert.equal(calls.pay, 0)
  assert.equal(calls.nav.length, 0)
}
{
  const { calls, page } = harness(19900)
  calls.items = []
  calls.enrollment = async () => enrollment(true, { catalogAvailable: false, course: null })
  await loadPage(calls, { id: 'growth', owned: 'false', bookingId: 'forged' })
  assert.equal(page.course.value, undefined)
  assert.equal(page.isEnrolled.value, true, 'paid ownership survives catalog deletion')
  page.openMyCourse()
  assert.equal(calls.nav.at(-1).url, '/pages/my-course/my-course?bookingId=91')
  assert.equal(calls.create.length, 0)
}
{
  const { calls, page } = harness(19900)
  await loadPage(calls, { id: 'growth', paid: 'true', owned: 'true', bookingId: '91' })
  assert.equal(page.isEnrolled.value, false, 'URL flags cannot establish ownership')
  assert.equal(page.enrollmentActionText.value, '立即支付')
  assert.equal(calls.enrollmentQueries[0].courseId, 'growth')
  assert.equal(calls.nav.length, 0)
}
{
  const { calls, page } = harness(19900)
  const check = deferred()
  calls.enrollment = () => check.promise
  await calls.load({ id: 'growth' })
  const showing = calls.show()
  assert.equal(page.enrollmentLoading.value, true)
  assert.equal(page.enrollmentActionDisabled.value, true)
  assert.equal(page.enrollmentActionText.value, '确认报名状态')
  await page.enroll()
  assert.equal(calls.create.length, 0, 'checkout waits for backend ownership')
  check.reject(new Error('offline'))
  await showing
  assert.ok(page.enrollmentError.value)
  assert.equal(page.enrollmentActionText.value, '重试报名状态')
  await page.enroll()
  assert.equal(calls.create.length, 0, 'a failed check never falls through to checkout')
  calls.enrollment = async () => enrollment(true)
  await page.refreshEnrollment()
  assert.equal(page.isEnrolled.value, true)
  assert.equal(page.enrollmentError.value, '')
}
{
  const { calls, page } = harness(19900)
  calls.enrollment = async () => enrollment(false, { bookingId: '91', syncStatus: 'retrying', order: { bookingId: '91', courseId: 'growth', status: 'closed' } })
  await loadPage(calls)
  assert.equal(page.isEnrolled.value, false)
  assert.equal(page.confirmationPending.value, true)
  assert.equal(page.enrollmentActionText.value, '查看支付结果')
  await page.enroll()
  assert.equal(calls.create.length, 0)
  assert.equal(calls.nav.at(-1).url, '/pages/payment-result/payment-result?bookingId=91')
}
{
  const { calls, page } = harness(19900)
  calls.enrollment = async () => enrollment(false, { syncStatus: 'retrying' })
  await loadPage(calls)
  assert.equal(page.enrollmentActionText.value, '刷新支付结果')
  await page.enroll()
  assert.equal(calls.create.length, 0)
  assert.equal(calls.nav.length, 0, 'a missing booking identity is never invented')
}
for (const invalid of [
  enrollment(true, { order: { status: 'pending', bookingId: '91', courseId: 'growth' } }),
  enrollment(true, { bookingId: '' }),
  enrollment(true, { courseId: 'another-course' }),
  enrollment(true, { order: { status: 'paid', bookingId: '999', courseId: 'growth' } }),
]) {
  const { calls, page } = harness(19900)
  calls.enrollment = async () => invalid
  await loadPage(calls)
  assert.equal(page.isEnrolled.value, false, 'incomplete or mismatched ownership is not trusted')
  await page.enroll()
  assert.equal(calls.create.length, 0)
  assert.equal(calls.nav.length, 0)
}
{
  const { calls, page } = harness(19900)
  calls.token = ''
  await loadPage(calls)
  assert.equal(calls.enrollmentQueries.length, 0, 'visitors browse without an authenticated query')
  assert.equal(calls.login, 0)
  assert.equal(page.enrollmentActionText.value, '立即支付')
  calls.loginToken = 'new-session'
  calls.enrollment = async () => enrollment(true)
  await page.enroll()
  assert.equal(calls.create.length, 0, 'login must check ownership before creating an order')
  assert.equal(calls.enrollmentQueries.length, 1)
  assert.equal(calls.nav.at(-1).url, '/pages/my-course/my-course?bookingId=91')
}
{
  const { calls, page } = harness(19900)
  calls.token = ''
  await loadPage(calls)
  const login = deferred()
  calls.loginGate = login.promise
  calls.enrollment = async () => enrollment(true)
  const buying = page.enroll()
  await flush()
  calls.token = 'logged-in-on-return'
  calls.show()
  login.resolve()
  await buying
  assert.equal(calls.create.length, 0, 'a login onShow must not skip the new account ownership query')
  assert.equal(calls.enrollmentQueries.length, 1)
  assert.equal(calls.nav.at(-1).url, '/pages/my-course/my-course?bookingId=91')
}
{
  const { calls, page } = harness(19900)
  calls.enrollment = async () => enrollment(true)
  await loadPage(calls)
  calls.token = 'new-owner'
  const newCheck = deferred()
  calls.enrollment = () => newCheck.promise
  const showing = calls.show()
  assert.equal(page.enrollment.value, null, 'account changes clear the previous private result immediately')
  assert.equal(page.isEnrolled.value, false)
  assert.equal(page.enrollmentActionDisabled.value, true)
  newCheck.resolve(enrollment())
  await showing
  assert.equal(page.isEnrolled.value, false)
}
{
  const { calls, page } = harness(19900)
  const old = deferred()
  calls.enrollment = () => old.promise
  await calls.load({ id: 'growth' })
  const showing = calls.show()
  calls.token = 'new-owner'
  calls.enrollment = async () => enrollment()
  await calls.show()
  old.resolve(enrollment(true))
  await showing
  assert.equal(page.isEnrolled.value, false, 'stale account responses cannot restore ownership')
  assert.equal(page.enrollment.value.owned, false)
}
{
  const { calls, page } = harness(19900)
  const late = deferred()
  calls.enrollment = () => late.promise
  await calls.load({ id: 'growth' })
  const showing = calls.show()
  calls.token = 'new-owner'
  late.resolve(enrollment(true))
  await showing
  assert.equal(page.enrollment.value, null)
  assert.equal(page.isEnrolled.value, false)
  assert.ok(page.enrollmentError.value, 'mid-request account switches require a fresh query')
  assert.equal(calls.token, 'new-owner')
}
{
  const { calls, page } = harness(19900)
  const late = deferred()
  calls.enrollment = () => late.promise
  await calls.load({ id: 'growth' })
  const showing = calls.show()
  calls.unload()
  late.resolve(enrollment(true))
  await showing
  assert.equal(page.enrollment.value, null)
  assert.equal(page.enrollmentLoading.value, false)
  assert.equal(page.isEnrolled.value, false)
  assert.equal(calls.nav.length, 0)
}
{
  const { calls, page } = harness(19900)
  await loadPage(calls)
  calls.payGate = new Promise(() => {})
  const buying = page.enroll()
  await flush()
  const queries = calls.enrollmentQueries.length
  calls.show()
  calls.show()
  await buying
  assert.equal(calls.enrollmentQueries.length, queries, 'wallet return resumes payment without starting ownership requests')
  assert.equal(calls.nav.at(-1).url, '/pages/payment-result/payment-result?bookingId=52')
}
console.log('Course checkout, owned enrollment, query isolation and recovery tests passed')
