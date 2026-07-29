import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const source = await readFile(new URL('./booking.vue', import.meta.url), 'utf8')
const template = source.match(/<template>([\s\S]*?)<\/template>/)?.[1] || ''
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1] || ''
const style = source.match(/<style scoped>([\s\S]*?)<\/style>/)?.[1] || ''

assert.ok(template && script && style, 'booking page should expose template, executable page state, and scoped styles')

const requiredOrder = [
  'enterprise-hero',
  'enterprise-scenarios',
  'enterprise-modes',
  'enterprise-process',
  'booking-form',
]
let previousIndex = -1
for (const className of requiredOrder) {
  const index = template.indexOf(`class="${className}`)
  assert.ok(index > previousIndex, `${className} should appear in the enterprise booking page order`)
  previousIndex = index
}

for (const text of ['查看预约记录', '继续浏览老师课堂', '再提交一个需求']) {
  assert.match(template, new RegExp(text), `submitted state should include ${text}`)
}
assert.match(template, /v-if="submitted"[\s\S]{0,180}class="booking-success/, 'submitted should render an in-page success state')
assert.doesNotMatch(source, /80\+|96\s*%|满意度|客户案例|成功案例/, 'booking defaults must not invent numeric proof or customer case claims')
assert.doesNotMatch(script, /const serviceModes = Object\.freeze/, 'booking page should not retain local service mode defaults')
assert.doesNotMatch(script, /const processSteps = computed\(\(\) => \[/, 'booking page should not retain local process step defaults')

const executableScript = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, '')
const dir = await mkdtemp(join(tmpdir(), 'nx-booking-enterprise-page-'))
const modulePath = join(dir, 'booking-enterprise-state.mjs')

const harnessPrelude = `
const ref = (value) => ({ value })
const computed = (getter) => ({ get value() { return getter() } })
const watch = () => {}
const onShow = (handler) => { globalThis.__bookingEnterpriseHarness.onShow = handler }
const onHide = (handler) => { globalThis.__bookingEnterpriseHarness.onHide = handler }
const onUnload = (handler) => { globalThis.__bookingEnterpriseHarness.onUnload = handler }
const ensureLogin = () => globalThis.__bookingEnterpriseHarness.ensureLogin()
const createBookingApi = (payload) => globalThis.__bookingEnterpriseHarness.createBookingApi(payload)
const userErrorMessage = (error, fallback) => error?.message || fallback
const clearBookingDraft = () => { globalThis.__bookingEnterpriseHarness.draftClears += 1 }
const loadBookingDraft = () => {
  const draft = globalThis.__bookingEnterpriseHarness.draft
  return draft ? { ...draft } : null
}
const saveBookingDraft = (payload) => { globalThis.__bookingEnterpriseHarness.draftSaves.push({ ...payload }) }
const consumeBookingIntent = () => {
  globalThis.__bookingEnterpriseHarness.intentConsumes += 1
  return globalThis.__bookingEnterpriseHarness.intents.shift() || null
}
const getStoredSiteConfig = () => globalThis.__bookingEnterpriseHarness.siteConfig
const getCachedSiteConfig = () => {
  const state = globalThis.__bookingEnterpriseHarness
  state.cachedConfigCalls += 1
  const nextConfig = state.cachedConfigs.shift()
  return Promise.resolve(nextConfig === undefined ? state.siteConfig : nextConfig)
}
const normalizePersonalExpertHome = (config = {}) => {
  globalThis.__bookingEnterpriseHarness.normalizedConfigs.push(config)
  const enterprise = config?.home?.enterprise || {}
  const configuredServices = Array.isArray(enterprise.items) ? enterprise.items : []
  const courseServices = Array.isArray(config?.home?.courses?.items) ? config.home.courses.items : []
  const services = [...configuredServices, ...courseServices]
    .filter((item) => item && (item.title || item.description))
    .map((item) => ({
      title: String(item.title || '团队共学服务').trim(),
      description: String(item.description || item.summary || '围绕团队协作、沟通与领导力的九型共学。').trim(),
    }))
  const normalizeBookingItems = (items, defaults) => {
    const normalized = (Array.isArray(items) ? items : [])
      .filter((item) => item && (item.title || item.description))
      .map((item) => ({
        title: String(item.title || '团队共学服务').trim(),
        description: String(item.description || item.summary || '围绕团队协作、沟通与领导力的九型共学。').trim(),
      }))
      .slice(0, 4)
    return normalized.length ? normalized : defaults.map((item) => ({ ...item }))
  }
  return {
    enterprise: {
      eyebrow: enterprise.eyebrow || '企业共学',
      title: enterprise.title || '让团队更懂彼此',
      lead: enterprise.lead || '以九型人格为共同语言，支持团队协作与领导力成长。',
      buttonText: enterprise.buttonText || '预约沟通',
      modules: Array.isArray(enterprise.modules) ? enterprise.modules : [],
      services: services.length ? services.slice(0, 3) : [
        { title: '企业团队共学', description: '用九型语言帮助团队看见协作中的动机与沟通方式。' },
      ],
      serviceModes: normalizeBookingItems(enterprise.items, [
        { title: '企业内训', description: '围绕企业当下议题设计半天或全天共学。' },
        { title: '团队工作坊', description: '用互动练习帮助团队建立沟通和协作共识。' },
        { title: '管理者培训', description: '支持管理者识别不同类型成员的动机与压力反应。' },
      ]),
      processSteps: normalizeBookingItems(enterprise.processSteps, [
        { title: '需求沟通', description: '先了解团队背景、参与对象和希望解决的问题。' },
        { title: '方案共创', description: '结合九型主题、课件内容和企业节奏设计服务方式。' },
        { title: '落地交付', description: '完成课程或工作坊后，沉淀可复盘的团队语言。' },
      ]),
    },
  }
}
`

await writeFile(
  modulePath,
  `${harnessPrelude}\n${executableScript}\nexport { enterpriseView, scenarioItems, serviceModes, processSteps, kinds, kindIndex, selectedServiceModeIndex, form, submitted, restoredDraftNotice, fieldErrors, submitting, currentDraft, applyBookingIntent, selectServiceMode, submit, viewBookingRecords, continueClassroom, submitAnother }\n`,
)

let moduleCounter = 0
async function createHarness(overrides = {}) {
  const state = {
    draft: null,
    intents: [],
    intentConsumes: 0,
    draftClears: 0,
    draftSaves: [],
    bookings: [],
    toasts: [],
    navigations: [],
    switches: [],
    loginCalls: 0,
    normalizedConfigs: [],
    cachedConfigCalls: 0,
    cachedConfigs: [],
    siteConfig: {
      home: {
        enterprise: {
          eyebrow: '企业共创',
          title: '把理解带进团队',
          lead: '面向组织的九型工作坊',
          buttonText: '预约企业需求',
          modules: ['团队协作', '管理者觉察'],
          items: [{ title: '企业沟通工作坊', description: '从冲突中建立理解' }],
        },
        courses: { items: [{ title: '领导力与团队协作', description: '团队课程' }] },
      },
    },
    ensureLogin() { this.loginCalls += 1; return Promise.resolve() },
    createBookingApi(payload) { this.bookings.push({ ...payload }); return Promise.resolve({ id: this.bookings.length }) },
    ...overrides,
  }
  globalThis.__bookingEnterpriseHarness = state
  globalThis.uni = {
    showToast(options) { state.toasts.push(options) },
    navigateTo(options) { state.navigations.push(options) },
    switchTab(options) { state.switches.push(options) },
  }
  moduleCounter += 1
  const page = await import(`${pathToFileURL(modulePath).href}?case=${moduleCounter}`)
  return { page, state }
}

try {
  {
    const { page, state } = await createHarness()
    assert.deepEqual(state.normalizedConfigs[0], state.siteConfig, 'booking page should read stored site config')
    assert.equal(page.enterpriseView.value.title, '把理解带进团队', 'enterprise hero should use home.enterprise copy')
    assert.deepEqual(
      page.enterpriseView.value.services.map((item) => item.title),
      ['企业沟通工作坊', '领导力与团队协作'],
      'service cards should combine home.enterprise.items and home.courses.items',
    )
    assert.ok(page.scenarioItems.value.length >= 2, 'configured enterprise modules should feed applicable scenarios')
    assert.deepEqual(page.serviceModes.value.map((item) => item.title), ['企业沟通工作坊'], 'service modes should read from enterpriseView')
    assert.deepEqual(page.processSteps.value.map((item) => item.title), ['需求沟通', '方案共创', '落地交付'], 'process steps should read from enterpriseView defaults')
  }

  {
    const { page, state } = await createHarness({
      draft: { kind: 'course', contactName: '张经理', phone: '13800138000', intent: '', preferredTime: '周五上午', message: '保留备注' },
      intents: [{ kind: 'enterprise', intentText: '团队工作坊' }],
    })
    await state.onShow()
    assert.equal(state.intentConsumes, 1, 'onShow should consume the one-time booking intent')
    assert.equal(state.intents.length, 0, 'the one-time intent should be empty after first onShow')
    assert.equal(page.kinds[page.kindIndex.value].value, 'enterprise', 'enterprise intent should select the enterprise booking kind')
    assert.equal(page.form.value.intent, '团队工作坊', 'intentText should prefill an empty intent field')
    assert.equal(page.form.value.contactName, '张经理', 'restored contact name must not be cleared by intent consumption')
    assert.equal(page.form.value.phone, '13800138000', 'restored phone must not be cleared by intent consumption')
    assert.equal(page.form.value.preferredTime, '周五上午', 'restored preferred time must survive intent consumption')

    state.intents.push({ kind: 'enterprise', intentText: '企业内训' })
    await state.onShow()
    assert.equal(page.form.value.intent, '团队工作坊', 'subsequent intentText should not overwrite a filled intent field')
  }

  {
    const { page, state } = await createHarness({
      draft: { kind: 'enterprise', contactName: '李总', phone: '13900139000', intent: '已有企业需求', preferredTime: '', message: '' },
      intents: [{ kind: 'enterprise', intentText: '管理者培训' }],
    })
    await state.onShow()
    assert.equal(page.form.value.intent, '已有企业需求', 'intentText should only prefill when form.intent is empty')
    assert.equal(page.form.value.contactName, '李总', 'restored contact info should be preserved when intent text is ignored')
    assert.equal(page.form.value.phone, '13900139000', 'restored phone should be preserved when intent text is ignored')
  }

  {
    const { page, state } = await createHarness({
      intents: [{ kind: 'enterprise', intentText: '企业内训' }],
    })
    page.submitted.value = true
    await state.onShow()
    assert.equal(page.submitted.value, false, 'a new one-time intent should reopen the form from the submitted state')
    assert.equal(page.form.value.intent, '企业内训', 'a new one-time intent should still prefill the reopened form')
  }

  {
    const { page, state } = await createHarness({
      siteConfig: { home: { enterprise: {
        items: [
          { title: ' 企业内训 ', description: ' 组织议题共学 ' },
          { title: ' 团队工作坊 ', description: ' 协作共识 ' },
          { title: ' 管理者培训 ', description: ' 带队觉察 ' },
        ],
        processSteps: [
          { title: ' 需求澄清 ', description: ' 了解背景 ' },
          { title: ' 方案共创 ', description: ' 匹配节奏 ' },
          { title: ' 现场交付 ', description: ' 沉淀语言 ' },
        ],
      } } },
    })
    await state.onShow()
    assert.deepEqual(
      page.serviceModes.value.map((item) => item.title),
      ['企业内训', '团队工作坊', '管理者培训'],
      'enterprise service modes should read the configured enterprise view in backend order',
    )
    assert.deepEqual(page.processSteps.value.map((item) => item.title), ['需求澄清', '方案共创', '现场交付'])

    for (const [index, mode] of page.serviceModes.value.entries()) {
      page.selectServiceMode(index)
      page.form.value = {
        contactName: `联系人${index}`,
        phone: `1380013800${index}`,
        intent: mode.title,
        preferredTime: '工作日',
        message: '希望进一步沟通',
      }
      await page.submit()
      assert.equal(state.bookings[index].kind, 'enterprise', `${mode.title} should submit with kind=enterprise`)
      assert.equal(state.bookings[index].intent, mode.title)
      assert.equal(page.submitted.value, true, 'successful submit should enter submitted in-page state')
      page.submitAnother()
    }

    assert.equal(state.loginCalls, 3, 'each valid enterprise submit should keep the existing login gate')
    assert.equal(state.draftClears, 3, 'each successful submit should clear the local booking draft')

    page.viewBookingRecords()
    page.continueClassroom()
    assert.deepEqual(state.navigations.at(-1), { url: '/pages/booking-records/booking-records' }, '查看预约记录 should navigate to booking records')
    assert.deepEqual(state.switches.at(-1), { url: '/pages/learn/learn' }, '继续浏览老师课堂 should switch to the classroom tab')

    page.form.value = { contactName: '重新提交前', phone: '13800138000', intent: '旧需求', preferredTime: '明天', message: '旧备注' }
    page.submitted.value = true
    page.submitAnother()
    assert.equal(page.submitted.value, false, '再提交一个需求 should return to the form state')
    assert.deepEqual(page.form.value, { contactName: '', phone: '', intent: '', preferredTime: '', message: '' }, '再提交一个需求 should reset the form')
  }

  {
    const { page, state } = await createHarness({
      draft: { kind: 'enterprise', contactName: '赵总', phone: '13600136000', intent: '旧服务方向', preferredTime: '本周', message: '保留草稿' },
      siteConfig: { home: { enterprise: {
        items: [
          { title: '企业内训', description: '组织议题共学' },
          { title: '团队工作坊', description: '协作共识' },
        ],
      } } },
    })
    await state.onShow()
    page.selectServiceMode(1)
    assert.equal(page.form.value.intent, '团队工作坊', 'explicitly selecting a configured mode should replace an existing draft intent')
    await page.submit()
    assert.equal(state.bookings.at(-1).kind, 'enterprise', 'selected configured mode should submit as enterprise')
    assert.equal(state.bookings.at(-1).intent, '团队工作坊', 'selected configured mode should submit its title instead of the prior draft intent')
  }

  {
    const { page, state } = await createHarness({
      draft: { kind: 'enterprise', contactName: '王总', phone: '13700137000', intent: '团队工作坊', preferredTime: '下周', message: '保留草稿' },
      intents: [{ kind: 'enterprise', intentText: '企业内训' }],
      siteConfig: { home: { enterprise: {
        items: [
          { title: '企业内训', description: '组织议题共学' },
          { title: '团队工作坊', description: '协作共识' },
        ],
      } } },
      cachedConfigs: [{ home: { enterprise: {
        items: [
          { title: '团队工作坊', description: '刷新后排第一' },
          { title: '企业内训', description: '刷新后排第二' },
        ],
        processSteps: [{ title: '先沟通', description: '刷新流程' }],
      } } }],
    })
    await state.onShow()
    assert.equal(state.cachedConfigCalls, 1, 'onShow should load cached site config without forcing a refresh')
    assert.equal(state.intentConsumes, 1, 'onShow should preserve one-time booking intent handling')
    assert.equal(page.form.value.intent, '团队工作坊', 'one-time booking intent must not overwrite restored draft intent')
    assert.equal(page.form.value.contactName, '王总', 'site config refresh must not overwrite draft contact fields')
    assert.equal(page.form.value.message, '保留草稿', 'site config refresh must not overwrite draft message')
    assert.equal(page.selectedServiceModeIndex.value, 0, 'selected mode should rematch form.intent after configured modes reorder')
    assert.deepEqual(page.processSteps.value.map((item) => item.title), ['先沟通'], 'refreshed enterprise view should provide configured process steps')

    state.cachedConfigs.push({ home: { enterprise: {} } })
    await state.onShow()
    assert.deepEqual(page.serviceModes.value.map((item) => item.title), ['企业内训', '团队工作坊', '管理者培训'], 'legacy config should still render the three service mode defaults')
    assert.deepEqual(page.processSteps.value.map((item) => item.title), ['需求沟通', '方案共创', '落地交付'], 'legacy config should still render the three process step defaults')
    assert.equal(page.selectedServiceModeIndex.value, 1, 'selected mode should rematch by intent after legacy defaults replace configured modes')
  }

  {
    let resolveFirstConfig
    let resolveSecondConfig
    const firstConfig = new Promise((resolve) => { resolveFirstConfig = resolve })
    const secondConfig = new Promise((resolve) => { resolveSecondConfig = resolve })
    const { page, state } = await createHarness({
      cachedConfigs: [firstConfig, secondConfig],
    })
    const firstShow = state.onShow()
    const secondShow = state.onShow()
    resolveSecondConfig({ home: { enterprise: {
      items: [{ title: '新配置', description: '应保留' }],
    } } })
    await secondShow
    resolveFirstConfig({ home: { enterprise: {
      items: [{ title: '旧配置', description: '不得覆盖' }],
    } } })
    await firstShow
    assert.deepEqual(page.serviceModes.value.map((item) => item.title), ['新配置'], 'an older async config result must not overwrite the latest enterprise view')
    assert.equal(page.selectedServiceModeIndex.value, -1, 'stale config completion must not rematch selection after the latest view')
  }

  console.log('booking enterprise page tests passed')
} finally {
  delete globalThis.__bookingEnterpriseHarness
  delete globalThis.uni
  await rm(dir, { force: true, recursive: true })
}
