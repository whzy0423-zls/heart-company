import assert from 'node:assert/strict'
import vm from 'node:vm'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./course-detail.vue', import.meta.url), 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import[^\n]+\n/gm, '')
const controllerSource = readFileSync(new URL('../../utils/coursePayment.js', import.meta.url), 'utf8')
const payment = await import(`data:text/javascript;base64,${Buffer.from(controllerSource).toString('base64')}`)
function harness(priceCents, serverStatus = 'paid') {
  const calls = { intents: [], nav: [], create: [], status: [], toasts: [], login: 0, token: 'owner', pay: 0 }
  const item = { id: 'growth', title: '成长课', priceCents, paymentMode: priceCents > 0 ? 'paid' : 'consult' }
  const context = vm.createContext({
    ref: value => ({ value }), computed: getter => ({ get value() { return getter() } }),
    onLoad: fn => { calls.load = fn }, onUnload: fn => { calls.unload = fn }, onShow: fn => { calls.show = fn },
    getStoredSiteConfig: () => ({}), getCachedSiteConfig: async () => ({}),
    normalizeMiniappCourses: () => [item], normalizeTeachers: () => [],
    setBookingIntent: intent => { calls.intents.push(intent); return true },
    ensureLogin: async () => { calls.login++; await calls.loginGate }, getToken: () => calls.token,
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
  vm.runInContext(`${script}\nglobalThis.page = { enroll, paying }`, context)
  return { calls, page: context.page }
}
const flush = async () => { for (let i = 0; i < 15; i++) await Promise.resolve() }
{
  const { calls, page } = harness(0)
  await calls.load({ id: 'growth' })
  await page.enroll()
  assert.equal(calls.create.length, 0)
  assert.equal(calls.intents[0].courseId, 'growth')
  assert.equal(calls.nav[0].url, '/pages/booking/booking')
}
{
  const { calls, page } = harness(19900)
  await calls.load({ id: 'growth' })
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
  await calls.load({ id: 'growth' })
  await page.enroll()
  assert.ok(calls.status.length, 'cancel or failure still confirms backend state')
  assert.equal(calls.nav.at(-1).url, '/pages/payment-result/payment-result?bookingId=52')
  assert.equal(calls.nav.length, 1)
  assert.equal(page.paying.value, false)
}
{
  const { calls, page } = harness(19900)
  calls.created = { bookingId: '52', status: 'paid' }
  await calls.load({ id: 'growth' })
  await page.enroll()
  assert.equal(calls.pay, 0, 'repeat purchase of paid course does not reopen cashier')
  assert.equal(calls.nav.length, 1)
}
{
  const { calls, page } = harness(19900)
  calls.payGate = new Promise(() => {})
  await calls.load({ id: 'growth' })
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
  await calls.load({ id: 'growth' })
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
  await calls.load({ id: 'growth' })
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
console.log('Course checkout result navigation and recovery tests passed')
