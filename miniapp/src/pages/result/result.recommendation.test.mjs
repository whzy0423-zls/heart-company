import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const source = await readFile(new URL('./result.vue', import.meta.url), 'utf8')
const templateStart = source.indexOf('<template>')
const templateEnd = source.lastIndexOf('</template>')
const template = templateStart >= 0 && templateEnd > templateStart
  ? source.slice(templateStart + '<template>'.length, templateEnd)
  : ''
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1] || ''
const style = source.match(/<style scoped>([\s\S]*?)<\/style>/)?.[1] || ''

assert.ok(template && script && style, 'result page should expose template, script, and scoped style')
assert.match(script, /listClassroomStandaloneApi/, 'result page should request standalone classroom recommendations')
assert.match(script, /normalizeClassroomContent/, 'result page should normalize recommended classroom items')
assert.match(script, /classroomContentRoute/, 'result page should share classroom detail routing')
assert.match(script, /setBookingIntent\(\{\s*kind:\s*['"]enterprise['"],\s*intentText:\s*['"]企业九型工作坊['"]\s*\}\)/, 'result enterprise CTA should store an enterprise booking intent')
assert.match(template, /class="result-recommendations/, 'result page should render a classroom recommendation panel')
assert.match(template, /继续浏览老师课堂/, 'result page should keep a classroom entrance')
assert.match(template, /企业九型工作坊/, 'result page should expose enterprise workshop CTA copy')
for (const text of ['分享好友', '生成海报', 'AI 深度性格报告', '和 TA 合盘', '重新测试']) {
  assert.match(template, new RegExp(text), `result page should preserve ${text}`)
}

const executableScript = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, '')
const dir = await mkdtemp(join(tmpdir(), 'nx-result-recommendations-'))
const modulePath = join(dir, 'result-state.mjs')
const harnessPrelude = `
const ref = (value) => ({ value })
const computed = (getter) => ({ get value() { return getter() } })
const onMounted = (handler) => { globalThis.__resultHarness.onMounted = handler }
const getCurrentInstance = () => ({ proxy: {} })
const onShareAppMessage = () => {}
const onShareTimeline = () => {}
const TYPES_INFO = {
  1: { name: '改革者', center: 'gut', growth: 7, stress: 4, en: 'Reformer', keywords: '原则' },
  4: { name: '浪漫者', center: 'heart', growth: 1, stress: 2, en: 'Individualist', keywords: '感受' },
  7: { name: '享乐者', center: 'head', growth: 5, stress: 1, en: 'Enthusiast', keywords: '可能' },
}
const CENTERS = { gut: { name: '腹中心' }, heart: { name: '心中心' }, head: { name: '脑中心' } }
const RESULTS = { 1: { title: '一号改革者', summary: '重视原则', growth: '放松一点' } }
const isWing = () => false
const resultPersonaText = () => '个人画像'
const getLastResult = () => globalThis.__resultHarness.lastResult
const normalizeLastResult = (value) => value && value.type ? value : null
const ensureLogin = () => Promise.resolve()
const saveTestRecordApi = async () => ({ id: 'record-1' })
const reportStatusApi = async () => ({ unlocked: false, priceCents: 990 })
const reportContentApi = async () => ({ answer: '报告内容' })
const payForReport = async () => true
const userErrorMessage = (error, fallback) => error?.message || fallback
const reportDisplayState = (state) => state.recordId ? { key: 'ready' } : { key: 'needs-save' }
const createResultPoster = async () => '/poster.png'
const listClassroomStandaloneApi = (query) => {
  globalThis.__resultHarness.classroomCalls.push(query)
  return globalThis.__resultHarness.listStandalone(query)
}
const normalizeClassroomContent = (item = {}) => ({ ...item, id: String(item.id || ''), contentType: item.contentType === 'audio' ? 'audio' : 'video' })
const classroomContentRoute = (item) => item?.id ? '/classroom-detail/' + item.id + '/' + item.contentType : ''
const classroomAccessLabel = (value) => value === 'paid' ? '付费课件' : '免费'
const setBookingIntent = (intent) => { globalThis.__resultHarness.intents.push(intent); return true }
`
await writeFile(
  modulePath,
  `${harnessPrelude}\n${executableScript}\nexport { result, r, info, classroomRecommendations, classroomRecommendationLoading, classroomRecommendationError, loadClassroomRecommendations, openClassroomRecommendation, goClassroom, goBooking, restart, goRelation }\n`,
)

let caseId = 0
async function createHarness(overrides = {}) {
  const state = {
    lastResult: { result: { type: 1, second: 4, score: {}, centers: [{ key: 'gut', name: '腹中心', pct: 70 }] }, gender: 'female' },
    classroomCalls: [],
    navigations: [],
    switches: [],
    redirects: [],
    toasts: [],
    intents: [],
    listStandalone: async () => ({ items: [] }),
    ...overrides,
  }
  globalThis.__resultHarness = state
  globalThis.uni = {
    showToast(options) { state.toasts.push(options) },
    navigateTo(options) { state.navigations.push(options) },
    switchTab(options) { state.switches.push(options) },
    redirectTo(options) { state.redirects.push(options) },
    saveImageToPhotosAlbum() {},
  }
  caseId += 1
  const page = await import(`${pathToFileURL(modulePath).href}?case=${caseId}`)
  return { page, state }
}

try {
  {
    const { page, state } = await createHarness({
      listStandalone: async () => ({ items: [
        { id: 2, title: '第一节', contentType: 'video', effectiveAccess: 'public' },
        { id: 3, title: '第二节', contentType: 'audio', effectiveAccess: 'paid' },
        { id: 4, title: '第三节', contentType: 'video', effectiveAccess: 'public' },
      ] }),
    })
    state.onMounted()
    await page.loadClassroomRecommendations()
    assert.equal(state.classroomCalls.length, 1, 'result page should single-flight recommendation loading')
    assert.deepEqual(state.classroomCalls[0], { limit: 2, offset: 0 }, 'result page should request the first two standalone classroom items')
    assert.deepEqual(page.classroomRecommendations.value.map((item) => item.id), ['2', '3'], 'recommendations should preserve API order and cap at two')
    page.openClassroomRecommendation(page.classroomRecommendations.value[1])
    assert.deepEqual(state.navigations.at(-1), { url: '/classroom-detail/3/audio' }, 'recommendation card should open shared classroom detail route')
    page.goClassroom()
    assert.deepEqual(state.switches.at(-1), { url: '/pages/learn/learn' }, 'classroom entrance should remain available')
    page.goBooking()
    assert.deepEqual(state.intents.at(-1), { kind: 'enterprise', intentText: '企业九型工作坊' })
    assert.deepEqual(state.switches.at(-1), { url: '/pages/booking/booking' }, 'enterprise CTA should switch to booking tab')
  }

  {
    const { page, state } = await createHarness({
      listStandalone: async () => { throw new Error('课堂推荐失败') },
    })
    state.onMounted()
    await page.loadClassroomRecommendations()
    assert.equal(page.r.value.title, '一号改革者', 'recommendation failure should not block the main result')
    assert.deepEqual(page.classroomRecommendations.value, [], 'recommendation failure should hide the recommendation list')
    assert.equal(page.classroomRecommendationError.value, '课堂推荐失败')
    page.goClassroom()
    assert.deepEqual(state.switches.at(-1), { url: '/pages/learn/learn' }, 'classroom entrance should survive recommendation failures')
  }

  console.log('result recommendation tests passed')
} finally {
  delete globalThis.__resultHarness
  delete globalThis.uni
  await rm(dir, { force: true, recursive: true })
}
