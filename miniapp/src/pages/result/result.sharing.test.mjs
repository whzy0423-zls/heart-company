import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { buildShareCard } from '../../utils/share.js'

const source = readFileSync(new URL('./result.vue', import.meta.url), 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1]
  .replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, '')
const metadata = readFileSync(new URL('../../data/enneagramGame.js', import.meta.url), 'utf8').replace(/^export /gm, '')
const cachedResult = { type: 9, second: 1, score: { 9: 17 }, centers: [{ key: 'gut', name: '本能中心', pct: 100 }] }

function harness(lastResult = null, timelinePreview = false) {
  const state = { cacheReads: 0, apiCalls: [], redirects: [], navigations: [], shares: [], menus: [], lastResult, timelinePreview }
  const recordCall = name => async () => { state.apiCalls.push(name); return {} }
  const context = vm.createContext({
    ref: value => ({ value }), computed: fn => ({ get value() { return fn() } }), getCurrentInstance: () => ({ proxy: {} }),
    onLoad: fn => { state.load = fn }, onShow: fn => { state.show = fn }, onMounted: fn => { state.mount = fn },
    onShareAppMessage: fn => { state.friend = fn }, onShareTimeline: fn => { state.timeline = fn },
    getLastResult: () => { state.cacheReads += 1; return state.lastResult || {} }, normalizeLastResult: value => value?.type ? value : null,
    resultPersonaText: () => '私人成长画像',
    getStoredSiteConfig: () => ({}), refreshSiteConfig: recordCall('config'),
    normalizeMiniappLearn: () => ({ classroom: { enabled: true } }), normalizeMiniappPayment: () => ({ enabled: true }),
    listClassroomStandaloneApi: recordCall('classroom'), saveTestRecordApi: recordCall('save'),
    reportStatusApi: recordCall('report-status'), reportContentApi: recordCall('report-content'),
    payForReport: recordCall('payment'), ensureLogin: recordCall('login'), createResultPoster: recordCall('poster'),
    reportDisplayState: () => ({ key: 'needs-save' }), userErrorMessage: (_, fallback) => fallback,
    normalizeClassroomContent: value => value, classroomContentRoute: () => '', classroomAccessLabel: () => '',
    isTimelinePreview: () => state.timelinePreview,
    previewImage() {}, setBookingIntent() {}, showTimelineShareHint() {}, showPublicShareMenu: enabled => state.menus.push(enabled),
    buildShareCard: options => {
      state.shares.push(options)
      return buildShareCard(options)
    },
    uni: { showToast() {}, redirectTo: item => state.redirects.push(item), navigateTo: item => state.navigations.push(item), switchTab() {} },
  })
  vm.runInContext(`${metadata}\n${script}\nglobalThis.page = {
    result, recordId, reportContent, saved, posterShow,
    sharedMode: typeof sharedMode === 'undefined' ? undefined : sharedMode,
    sharedType: typeof sharedType === 'undefined' ? undefined : sharedType,
    sharedInfo: typeof sharedInfo === 'undefined' ? undefined : sharedInfo,
    saveRecord, refreshReportStatus, loadReportContent, unlockReport, makePoster, restart,
  }`, context)
  return { state, page: context.page }
}
function enter(state, query) { state.load?.(query); state.show(); state.mount() }

{
  const { state, page } = harness()
  enter(state, { shareType: '3' })
  assert.equal(state.redirects.length, 0, 'cold shared result stays readable rather than redirecting to a new test')
  assert.equal(page.sharedMode.value, true)
  assert.equal(page.sharedType.value, 3)
  assert.equal(page.sharedInfo.value.name, '成就型')
  assert.equal(page.result.value, null, 'public introduction is not represented as a personal test result')
  assert.equal(state.cacheReads, 0)
  assert.deepEqual(state.apiCalls, [], 'single-page Timeline view needs no login, reports, recommendations or payment calls')
  assert.ok(state.menus.length, 'public result enables native sharing menus')
}
{
  const { state, page } = harness({ result: cachedResult, gender: 'female' })
  enter(state, { shareType: '2', recordId: 'private-record', score: '999', gender: 'male' })
  assert.equal(state.cacheReads, 0, 'recipient cache is never read in shared mode')
  assert.equal(page.sharedType.value, 2)
  assert.equal(page.result.value, null)
  assert.equal(page.recordId.value, '')
  assert.equal(page.reportContent.value, '')
  assert.equal(page.saved.value, false)
  await page.saveRecord(); await page.refreshReportStatus(); await page.loadReportContent(); await page.unlockReport(); await page.makePoster()
  assert.deepEqual(state.apiCalls, [], 'personal entry points remain inert in shared mode')
  assert.equal(page.posterShow.value, false)
  assert.equal(state.friend().path, '/pages/result/result?shareType=2')
  assert.equal(state.timeline().query, 'shareType=2')
  assert.match(state.friend().title, /2.*助人型/)
  assert.doesNotMatch(state.friend().title, /我是|私人成长/)
  for (const share of state.shares) {
    assert.equal(share.type, 2, 're-sharing preserves public type, never recipient personal type')
    assert.equal(share.imageUrl, '/static/share/result-2.jpg')
    assert.equal(Object.keys(share).some(key => /record|score|gender|token/i.test(key)), false)
  }
  page.restart()
  assert.equal(state.redirects.at(-1).url, '/pages/test/test')
}
for (const shareType of ['', '0', '10', '-1', '1.0', '01', 'invalid', '3&recordId=secret', null]) {
  const { state, page } = harness({ result: cachedResult, gender: 'female' })
  enter(state, { shareType })
  assert.equal(page.sharedMode.value, true, `${shareType}: invalid public query stays public`)
  assert.equal(page.sharedType.value, null)
  assert.equal(page.sharedInfo.value, null)
  assert.equal(state.cacheReads, 0)
  assert.equal(state.redirects.length, 0)
  assert.equal(state.timeline().query, 'shareType=0')
}
{
  const { state, page } = harness({ result: cachedResult, gender: 'female' })
  enter(state, {})
  assert.equal(page.sharedMode.value, false)
  assert.equal(page.result.value.type, 9, 'normal completed tests retain personal result')
  assert.equal(state.cacheReads, 1)
  assert.equal(state.friend().path, '/pages/result/result?shareType=9')
  assert.equal(state.timeline().query, 'shareType=9')
  assert.equal(state.apiCalls.includes('classroom'), true)
}
{
  const { state } = harness()
  enter(state, {})
  assert.equal(state.redirects.at(-1).url, '/pages/test/test', 'normal entry still recovers an absent test')
}
{
  const { state, page } = harness({ result: cachedResult }, true)
  enter(state, {})
  assert.equal(page.sharedMode.value, true, 'legacy empty Timeline query also opens a public invitation')
  assert.equal(page.result.value, null)
  assert.equal(state.cacheReads, 0)
  page.restart()
  assert.equal(state.redirects.length, 0, 'Timeline preview never triggers an unsupported test navigation')
  assert.deepEqual(state.apiCalls, [])
}
assert.match(source, /v-if="sharedMode"[\s\S]*?公开类型介绍/)
assert.match(source, /v-else-if="result"/)
assert.match(source, /测测我的类型/)
assert.match(source, /showTimelineShareHint/)
assert.match(source, /@click="showTimelineShareHint\(\)"/, 'Timeline hint uses the default uni runtime, not the click event argument')
for (const [name, kind] of [['enneagram-detail', 'type'], ['enneagram', 'explore']]) {
  const pageSource = readFileSync(new URL(`../${name}/${name}.vue`, import.meta.url), 'utf8')
  const pageScript = pageSource.match(/<script setup>([\s\S]*?)<\/script>/)[1]
    .replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, '')
  const state = { menuCalls: 0, shares: [], timelinePreview: false, navigations: [] }
  const context = vm.createContext({
    ref: value => ({ value }), computed: fn => ({ get value() { return fn() } }),
    onLoad: fn => { state.load = fn }, onShow: fn => { state.show = fn },
    onShareAppMessage: fn => { state.friend = fn }, onShareTimeline: fn => { state.timeline = fn },
    isTimelinePreview: () => state.timelinePreview, uni: { navigateTo: options => state.navigations.push(options) },
    getCurrentPages: () => [], showPublicShareMenu: () => { state.menuCalls += 1 },
    buildShareCard: options => { state.shares.push(options); return { appMessage: options, timeline: options } },
  })
  vm.runInContext(`${metadata}\n${pageScript}\nglobalThis.actions = ${kind === 'type' ? '{ openTest, openRelation, openOverview }' : '{ openType }'}`, context)
  assert.equal(typeof state.friend, 'function', `${name}: friend hook must be directly registered`)
  assert.equal(typeof state.timeline, 'function', `${name}: Timeline hook must be directly registered`)
  state.load?.({ type: '3' })
  state.show()
  assert.equal(state.menuCalls, 1)
  assert.equal(state.friend().kind, kind)
  assert.equal(state.timeline().kind, kind)
  if (kind === 'type') {
    assert.equal(state.friend().type, 3, 'cold entry query determines shared type content')
    assert.match(state.timeline().title, /3.*成就型/)
    state.load({ type: 'not-a-type' })
    assert.equal(state.friend().type, 1, 'invalid query consistently falls back to first public type')
  }
  state.timelinePreview = true
  state.show()
  for (const action of Object.values(context.actions)) action(3)
  assert.equal(state.navigations.length, 0, `${name}: no unsupported navigation in Timeline preview`)
  assert.match(pageSource, /<NxShareActions/)
}
console.log('result and public type sharing cold-entry/privacy tests passed')
