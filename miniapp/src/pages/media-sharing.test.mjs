import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { ref, computed, watch } from 'vue'
import { buildShareCard } from '../utils/share.js'
import { normalizeClassroomContent, normalizeClassroomSeries, classroomPurchaseAction } from '../utils/classroomDisplay.js'

const lesson = { id: '21', title: '关于关系的真实日常', coverUrl: '/static/lesson.jpg', contentType: 'video', effectiveAccess: 'paid', accessLevel: 'paid', canPlay: false, purchaseState: 'purchase_required' }
const series = { id: '7', title: '老师的关系系列', coverUrl: '/static/series.jpg', effectiveAccess: 'paid', canPlay: false }
const flush = async () => { for (let n = 0; n < 20; n++) await Promise.resolve() }
function deferred() { let resolve; const promise = new Promise(done => { resolve = done }); return { promise, resolve } }
function harness(name, options = {}) {
  const source = readFileSync(new URL(`./${name}/${name}.vue`, import.meta.url), 'utf8')
  const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1]
    .replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, '').replaceAll('import.meta.env', '({})')
  const calls = { hints: 0, progress: 0, privateReads: 0, navigations: 0, menus: [], load: null, show: null, hide: null, detailRequests: [], seriesRequests: [], playback: 0,
    list: async () => ({ items: [lesson] }), series: async () => ({ series, contents: [lesson] }),
    detail: async () => lesson }
  const context = vm.createContext({
    ref, computed, watch, buildShareCard,
    onLoad: fn => { calls.load = fn }, onShow: fn => { calls.show = fn }, onHide: fn => { calls.hide = fn }, onUnload: fn => { calls.unload = fn },
    onShareAppMessage: fn => { calls.friend = fn }, onShareTimeline: fn => { calls.timeline = fn },
    isTimelinePreview: () => options.timeline === true, requireFullMiniapp: () => { if (options.timeline) { calls.hints++; return false }; return true },
    showPublicShareMenu: enabled => calls.menus.push(enabled === undefined ? true : enabled),
    normalizeMiniappPayment: () => ({ enabled: true }), getStoredSiteConfig: () => ({}), refreshSiteConfig: async () => ({}), getToken: () => options.token || '',
    getClassroomContinueLearningApi: async () => { calls.privateReads++; return { items: [] } },
    normalizeClassroomContent, normalizeClassroomSeries, classroomPurchaseAction,
    listClassroomStandaloneApi: () => calls.list(), listClassroomSeriesApi: () => calls.list(),
    getClassroomSeriesApi: id => { calls.seriesRequests.push(id); return calls.series() },
    getClassroomContentApi: id => { calls.detailRequests.push(id); return calls.detail() },
    withClassroomPlaybackRetry: async (_id, consume) => { calls.playback++; return consume({ url: 'https://media.example/private.mp4?token=SECRET' }) },
    readAnonymousClassroomProgress: () => null, createClassroomProgressTracker: () => { calls.progress++; return { flush: async () => {} } },
    normalizeTeachers: () => [], STUDIO_TEACHER: {},
    userErrorMessage: (error, fallback) => error.message || fallback,
    uni: { navigateTo: () => { calls.navigations++ }, switchTab: () => { calls.navigations++ }, createVideoContext: () => ({ pause: () => {} }) },
  })
  const expose = name === 'classroom' ? 'activeTab, selectedSeries, expandedSeries, loadActiveList, openSeries, selectTab, seriesError, openContent, openContinueLearning, startSeriesPurchase' : 'contentId, content, loadDetail, loading, loadError, playbackUrl, startPurchase, consultTeacher, openTeacherDetail, recordProgress'
  vm.runInContext(`${script}\nglobalThis.page = { ${expose} }`, context)
  assert.equal(typeof calls.friend, 'function', `${name} registers page friend sharing`)
  assert.equal(typeof calls.timeline, 'function', `${name} registers page Timeline sharing`)
  assert.match(source, /<NxShareActions\b/, `${name} exposes both share options`)
  return { calls, page: context.page }
}
{
  const { calls, page } = harness('classroom-detail')
  const loading = deferred()
  calls.detail = () => loading.promise
  calls.load({ id: '21', paid: 'true', canPlay: 'true', position: '123', token: 'SECRET' })
  calls.show()
  assert.equal(calls.menus.at(-1), false)
  loading.resolve(lesson)
  await flush()
  assert.equal(calls.menus.at(-1), true, 'successful public metadata enables sharing even when lesson requires payment')
  assert.equal(page.content.value.canPlay, false, 'a share link never grants payment access')
  assert.equal(calls.playback, 0, 'locked metadata does not request private playback')
  const card = calls.friend()
  assert.equal(card.path, '/pages/classroom-detail/classroom-detail?id=21')
  assert.equal(calls.timeline().query, 'id=21')
  assert.equal(card.title, lesson.title)
  assert.equal(card.imageUrl, lesson.coverUrl)
  assert.doesNotMatch(JSON.stringify(card), /SECRET|position|canPlay|paid|playback/)
  page.content.value = { ...page.content.value, title: '更新后的公开标题' }
  assert.equal(calls.timeline().title, '更新后的公开标题')
  const retry = deferred()
  calls.detail = () => retry.promise
  const request = page.loadDetail()
  calls.hide()
  const menuCount = calls.menus.length
  retry.resolve({ ...lesson, contentType: 'audio' })
  await request
  assert.equal(calls.menus.length, menuCount, 'hidden page completion never enables menus on the current page')
  calls.show()
  assert.equal(calls.menus.at(-1), true)
  calls.detail = async () => { throw new Error('已下架') }
  await page.loadDetail()
  assert.equal(calls.menus.at(-1), false, 'deleted metadata never shares the stale successful record')
}
for (const invalid of ['', '21&token=SECRET', '0']) {
  const { calls } = harness('classroom-detail')
  calls.load({ id: invalid })
  await flush()
  assert.equal(calls.detailRequests.length, 0, 'malformed shared identities are rejected before requesting metadata')
  assert.equal(calls.menus.at(-1), false)
}
{
  const { calls, page } = harness('classroom')
  calls.list = async () => ({ items: [series] })
  const detailGate = deferred()
  calls.series = () => detailGate.promise
  const loading = calls.load({ tab: 'series', seriesId: '7', paid: 'true', token: 'SECRET' })
  calls.show()
  await flush()
  assert.equal(calls.menus.at(-1), false, 'series menus wait for actual series details')
  detailGate.resolve({ series, contents: [lesson] })
  await loading
  assert.equal(calls.menus.at(-1), true)
  assert.equal(calls.friend().path, '/pages/classroom/classroom?tab=series&seriesId=7')
  assert.equal(calls.timeline().query, 'tab=series&seriesId=7')
  assert.equal(calls.friend().title, series.title)
  assert.doesNotMatch(JSON.stringify(calls.friend()), /SECRET|purchaseState|canPlay/)
  assert.equal(page.expandedSeries.value.series.canPlay, false)
  const recipient = harness('classroom')
  recipient.calls.list = async () => ({ items: [series] })
  await recipient.calls.load(Object.fromEntries(new URLSearchParams(calls.timeline().query)))
  assert.equal(recipient.page.selectedSeries.value.id, '7', 'Timeline query roundtrips into the selected series')
  assert.deepEqual(recipient.calls.seriesRequests, ['7'])
  const hidden = deferred()
  calls.series = () => hidden.promise
  const refresh = page.openSeries(series, { force: true })
  calls.hide()
  const menuCount = calls.menus.length
  hidden.resolve({ series, contents: [] })
  await refresh
  assert.equal(calls.menus.length, menuCount)
  calls.show()
  assert.equal(calls.menus.at(-1), true)
  calls.series = async () => { throw new Error('已下架') }
  await page.openSeries(series, { force: true })
  assert.equal(calls.menus.at(-1), false)
  page.selectTab('standalone')
  await flush()
  assert.equal(calls.friend().path, '/pages/classroom/classroom', 'leaving a series clears the old series share query')
}
{
  const { calls, page } = harness('classroom-detail')
  calls.detail = async () => ({ ...lesson, effectiveAccess: 'public', accessLevel: 'public', canPlay: true })
  calls.load({ id: '21' })
  await flush()
  assert.equal(calls.menus.at(-1), true)
  assert.equal(calls.playback, 1, 'public video playback retains its original signed playback request')
  assert.match(page.playbackUrl.value, /token=SECRET/)
  assert.equal(calls.timeline().query, 'id=21')
  assert.doesNotMatch(JSON.stringify(calls.friend()), /SECRET|playback/)
}
{
  const { calls } = harness('classroom-detail')
  calls.detail = async () => ({ ...lesson, contentType: 'audio' })
  calls.load({ id: '21', type: 'video' })
  await flush()
  assert.equal(calls.timeline().query, 'id=21', 'audio/video identity is resolved from server metadata, never forged type flags')
}
{
  const { calls, page } = harness('classroom')
  await calls.load({ seriesId: '1&paid=true' })
  assert.equal(calls.seriesRequests.length, 0)
  assert.equal(calls.friend().path, '/pages/classroom/classroom')
  calls.list = async () => { throw new Error('网络不可用') }
  await page.loadActiveList({ force: true })
  assert.equal(calls.menus.at(-1), false, 'list refresh failure disables sharing stale public metadata')
}
for (const name of ['classroom', 'classroom-detail']) {
  const { calls, page } = harness(name, { timeline: true, token: 'previous-session' })
  await calls.load({ id: '21', position: '99', paid: 'true' })
  calls.show()
  await flush()
  if (name === 'classroom') { page.openContent(lesson); page.openContinueLearning(lesson); page.startSeriesPurchase(series) }
  else { page.startPurchase(); page.consultTeacher(); page.openTeacherDetail(); await page.recordProgress(99) }
  assert.equal(calls.progress, 0, 'Timeline preview never creates or uploads learning progress')
  assert.equal(calls.privateReads, 0, 'Timeline preview never loads personal continue-learning history')
  assert.equal(calls.navigations, 0, 'Timeline preview never calls forbidden navigation APIs')
  assert.equal(calls.hints, 3)
}
console.log('classroom share routes, access boundaries and lifecycle tests passed')
