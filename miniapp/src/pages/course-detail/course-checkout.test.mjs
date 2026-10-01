import assert from 'node:assert/strict'
import vm from 'node:vm'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./course-detail.vue', import.meta.url), 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import[^\n]+\n/gm, '')
function harness(priceCents, resultState = 'success') {
  const calls = { intents: [], nav: [], create: [], status: [], toasts: [], login: 0, token: 'owner', pay: 0 }
  const item = { id: 'growth', title: '成长课', priceCents, paymentMode: priceCents > 0 ? 'paid' : 'consult' }
  const context = vm.createContext({
    ref: value => ({ value }), computed: getter => ({ get value() { return getter() } }),
    onLoad: fn => { calls.load = fn }, onUnload: fn => { calls.unload = fn },
    getStoredSiteConfig: () => ({}), getCachedSiteConfig: async () => ({}),
    normalizeMiniappCourses: () => [item], normalizeTeachers: () => [],
    setBookingIntent: intent => { calls.intents.push(intent); return true },
    ensureLogin: async () => { calls.login++; await calls.loginGate }, getToken: () => calls.token,
    createCourseOrderApi: async id => { calls.create.push(id); await calls.createGate; return { bookingId: '52', outTradeNo: 'crs52' } },
    payWechatOrder: async () => { calls.pay++ },
    getCourseBookingOrderStatusApi: async id => { calls.status.push(id); return { status: 'paid' } },
    createWechatPaymentController: options => ({
      stop() {}, async purchase() {
        const order = await options.create()
        await options.pay(order)
        if (resultState === 'success') await options.status(order)
        return { state: resultState, order, message: resultState === 'cancelled' ? '已取消支付' : '等待确认' }
      },
    }),
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
  assert.deepEqual(calls.create, ['growth'], 'double clicks create one checkout')
  assert.deepEqual(calls.status, ['52'], 'success comes from backend booking status')
  assert.equal(calls.intents.length, 0, 'paid checkout skips the consultation form')
  assert.ok(calls.nav.at(-1).url.startsWith('/pages/orders/orders'))
  assert.equal(page.paying.value, false)
}
for (const outcome of ['cancelled', 'failure']) {
  const { calls, page } = harness(19900, outcome)
  await calls.load({ id: 'growth' })
  await page.enroll()
  assert.equal(calls.nav.length, 0, 'unconfirmed payment stays on the course')
  assert.ok(calls.toasts.length)
  assert.equal(page.paying.value, false)
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
  assert.equal(calls.create.length, 0, 'leaving while logging in must not create an order')
}
{
  const { calls, page } = harness(19900)
  await calls.load({ id: 'growth' })
  let release
  calls.createGate = new Promise(resolve => { release = resolve })
  const buying = page.enroll()
  await new Promise(resolve => setImmediate(resolve))
  calls.token = 'different-user'
  release()
  await buying
  assert.equal(calls.pay, 0, 'account switch cannot pay the previous account order')
  assert.equal(calls.nav.length, 0)
}
console.log('course direct checkout behavior passed')
