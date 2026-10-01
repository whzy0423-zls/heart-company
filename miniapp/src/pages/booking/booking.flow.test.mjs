import assert from 'node:assert/strict'
import vm from 'node:vm'
import { readFile } from 'node:fs/promises'

async function scriptFor(path) {
  const source = await readFile(new URL(path, import.meta.url), 'utf8')
  return source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import[^\n]+\n/gm, '')
}
const paymentSource = await readFile(new URL('../../utils/coursePayment.js', import.meta.url), 'utf8')
const payment = await import(`data:text/javascript;base64,${Buffer.from(paymentSource).toString('base64')}`)
const bookingScript = await scriptFor('./booking.vue')
const detailScript = await scriptFor('../course-detail/course-detail.vue')
const previewCourse = { id: 'intro', title: '演示课程', schedule: '10月17日', price: 1980 }

function createHarness({ preview = true, draft = null, h5 = false } = {}) {
  const state = { draft, intents: [], saved: [], requests: [], clears: 0, toasts: [], failing: false, logins: 0, scrolls: [], navigations: [], setIntents: [] }
  const context = vm.createContext({
    onShareAppMessage: () => {}, onShareTimeline: () => {},
    buildShareCard: input => ({ appMessage: input, timeline: input }),
    showPublicShareMenu: () => {}, isTimelinePreview: () => false, requireFullMiniapp: () => true,
    ref: (value) => ({ value }),
    computed: (getter) => ({ get value() { return getter() } }),
    watch: () => {},
    nextTick: async () => {},
    onShow: (handler) => { state.show = handler },
    onHide: (handler) => { state.hide = handler },
    onUnload: (handler) => { state.unload = handler },
    onLoad: (handler) => { state.load = handler },
    setTimeout, clearTimeout,
    ensureLogin: async () => { state.logins++ },
    getToken: () => 'owner',
    createBookingApi: async (payload) => {
      if (state.failing) throw new Error('网络中断')
      state.requests.push(payload)
      return state.booking
    },
    createCourseBookingOrderApi: async (id) => { state.orderCreates = (state.orderCreates || 0) + 1; return state.createdOrder || { bookingId: id, payParams: {} } },
    payWechatOrder: async () => { state.payCalls = (state.payCalls || 0) + 1; await state.payGate },
    getCourseBookingOrderStatusApi: async (id) => { (state.checkedBookings ||= []).push(id); return { status: state.paymentState === 'success' ? 'paid' : 'pending', syncStatus: state.syncStatus } },
    createCoursePaymentController: options => payment.createCoursePaymentController({ ...options, wait: async () => {}, paymentTimeoutMs: 20 }),
    coursePaymentResultUrl: payment.coursePaymentResultUrl,
    userErrorMessage: (error, fallback) => error.message || fallback,
    clearBookingDraft: () => { state.clears++ },
    loadBookingDraft: () => state.draft,
    saveBookingDraft: (payload) => state.saved.push(payload),
    consumeBookingIntent: () => state.intents.shift(),
    setBookingIntent: (intent) => { state.setIntents.push(intent); return true },
    getStoredSiteConfig: () => ({}),
    getCachedSiteConfig: async () => ({}),
    normalizePersonalExpertHome: () => ({ enterprise: { serviceModes: [], processSteps: [] } }),
    normalizeMiniappLearn: () => ({ classroom: { enabled: true } }),
    normalizeMiniappCourses: () => [{ title: '工作室已配置的课程', bullets: [], id: 'course-1', paymentMode: 'consult' }],
    normalizeTeachers: () => [],
    STUDIO_COURSES: [previewCourse], STUDIO_TEACHER: {}, UI_PREVIEW: preview,
    window: h5 ? { document: {} } : undefined,
    uni: {
      showToast: (payload) => state.toasts.push(payload),
      pageScrollTo: (payload) => state.scrolls.push(payload),
      navigateTo: (payload) => state.navigations.push(payload),
      switchTab: (payload) => state.navigations.push(payload),
      showModal: () => {},
    },
  })
  return { state, context }
}
function createBooking(options) {
  const harness = createHarness(options)
  vm.runInContext(`${bookingScript}\nglobalThis.page = { form, kindIndex, currentKind, consent, submit, submitted, submittedKind, successTitle, fieldErrors, submitAnother, applyBookingIntent, flushDraftSave, courses }`, harness.context)
  return { ...harness, page: harness.context.page }
}

{
  const { state, page } = createBooking({ preview: false })
  assert.equal(page.currentKind.value, 'course', 'booking starts with courses')
  await page.submit()
  assert.equal(state.requests.length, 0, 'invalid forms never reach the API')
  assert.ok(page.fieldErrors.value.contactName)
  page.form.value = { contactName: '示例学员', phone: '13800138000', intent: '入门课', preferredTime: '周六', message: '希望了解关系沟通' }
  await page.submit()
  assert.equal(state.requests.length, 0, 'consent is required independently of valid contact data')
  assert.ok(page.fieldErrors.value.consent)
  page.consent.value = true
  await page.submit()
  assert.equal(state.logins, 1)
  assert.equal(state.requests[0].kind, 'course')
  assert.equal(page.submittedKind.value, 'course')
  assert.equal(page.submitted.value, true)
  assert.equal(page.successTitle.value, '你的学习意向，已收到')
  assert.equal(page.form.value.phone, '', 'successful submission clears private form data')
  assert.equal(page.consent.value, false, 'consent is not carried into another request')
}
{
  const { state, page } = createBooking({ h5: true })
  page.form.value = { contactName: '预览用户', phone: '13800138000', intent: '课程咨询', preferredTime: '', message: '' }
  page.consent.value = true
  await page.submit()
  assert.equal(state.requests.length, 0, 'H5 preview must never submit local or backend booking data')
  assert.match(state.toasts.at(-1).title, /微信小程序内提交/)
}
{
  const draft = { kind: 'consult', contactName: '示例学员', phone: '13800138000', intent: '关系沟通', preferredTime: '周末', message: '保留这条留言' }
  const { state, page } = createBooking({ draft, preview: false })
  assert.equal(page.currentKind.value, 'consult')
  assert.equal(page.consent.value, false, 'restoring a draft does not imply consent')
  page.consent.value = true
  state.failing = true
  await page.submit()
  assert.equal(page.submitted.value, false)
  assert.equal(page.form.value.message, draft.message, 'a failed request retains the draft')
  state.hide()
  assert.equal(state.saved.at(-1).phone, draft.phone)
  state.intents.push({ kind: 'course', intentText: '新课程 · 10月' })
  await page.applyBookingIntent()
  assert.equal(page.currentKind.value, 'course')
  assert.equal(page.form.value.intent, '新课程 · 10月', 'an explicit new course selection replaces the old direction')
  assert.equal(page.form.value.phone, draft.phone, 'course handoff retains entered contact data')
  assert.equal(state.scrolls.at(-1).selector, '#booking-form', 'course handoff lands on the populated form')
  state.failing = false
  page.kindIndex.value = 2
  await page.submit()
  assert.equal(state.requests.at(-1).kind, 'enterprise')
  assert.equal(page.successTitle.value, '你的企业需求，已收到')
}
{
  const { state, page } = createBooking({ preview: false })
  assert.equal(page.courses.value[0].title, '工作室已配置的课程')
  assert.equal(page.courses.value[0].price, undefined, 'real catalog never inherits demonstration prices')
  page.flushDraftSave()
  assert.equal(state.saved.length, 0, 'visiting the default tab does not save a blank draft')
}
{
  const { state, context } = createHarness()
  vm.runInContext(`${detailScript}\nglobalThis.page = { course, enroll }`, context)
  await state.load({ id: 'intro' })
  context.page.enroll()
  assert.equal(state.setIntents[0].kind, 'course')
  assert.equal(state.setIntents[0].courseId, 'intro')
  assert.equal(state.setIntents[0].intentText, '演示课程 · 10月17日')
  assert.equal(state.navigations[0].url, '/pages/booking/booking')
}
{
  const { state, context } = createHarness({ preview: false })
  vm.runInContext(`${detailScript}\nglobalThis.page = { course, enroll }`, context)
  await state.load({ id: 'intro' })
  assert.equal(context.page.course.value, undefined, 'production does not resolve fixture course IDs')
  context.page.enroll()
  assert.equal(state.setIntents.length, 0, 'unavailable courses cannot create a booking intent')
  await state.load({ id: 'course-1' })
  assert.equal(context.page.course.value.title, '工作室已配置的课程')
}
{
  const { state, page } = createBooking({ preview: false })
  page.form.value = { contactName: '示例学员', phone: '13800138000', intent: '成长课', preferredTime: '', message: '' }
  page.consent.value = true
  state.booking = { id: '51', paymentMode: 'paid', paymentStatus: 'pending' }
  state.paymentState = 'cancelled'
  await page.submit()
  assert.equal(page.submitted.value, false)
  assert.equal(state.navigations.at(-1).url, '/pages/payment-result/payment-result?bookingId=51', 'pending booking payment has an explicit result page')
  state.paymentState = 'success'
  await page.submit()
  assert.equal(state.requests.length, 1, 'retry payment reuses the saved booking')
  assert.equal(page.successTitle.value, '课程已支付，期待与你相遇')
  assert.equal(state.navigations.at(-1).url, '/pages/payment-result/payment-result?bookingId=51')
  assert.equal(state.payCalls, 1, 'server-paid retry does not reopen cashier')
}
{
  const { state, page } = createBooking({ preview: false })
  page.form.value = { contactName: '示例学员', phone: '13800138000', intent: '成长课', preferredTime: '', message: '' }
  page.consent.value = true
  state.booking = { id: '51', paymentMode: 'paid', paymentStatus: 'pending' }
  state.syncStatus = 'retrying'
  await page.submit()
  assert.equal(state.orderCreates || 0, 0)
  assert.equal(state.payCalls || 0, 0)
  assert.equal(state.navigations.at(-1).url, '/pages/payment-result/payment-result?bookingId=51')
}
for (const status of ['paid', 'pending']) {
  const { state, page } = createBooking({ preview: false })
  page.form.value = { contactName: '示例学员', phone: '13800138000', intent: '成长课', preferredTime: '', message: '' }
  page.consent.value = true
  state.booking = { id: '51', paymentMode: 'paid', paymentStatus: 'pending' }
  state.createdOrder = { bookingId: '48', status, ...(status === 'pending' ? { syncStatus: 'retrying' } : {}) }
  await page.submit()
  assert.equal(state.navigations.at(-1).url, '/pages/payment-result/payment-result?bookingId=48', 'result page uses the authoritative booking returned by checkout')
  assert.equal(state.payCalls || 0, 0, 'paid or reconciling orders never open the cashier')
  assert.equal(page.submitted.value, status === 'paid')
  if (status === 'pending') assert.equal(state.checkedBookings.at(-1), '48', 'confirmation follows the returned order identity')
}
console.log('Booking and course-detail flow tests passed')
